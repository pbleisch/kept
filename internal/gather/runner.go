package gather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/model"
)

// Runner gathers, and gathers each distinct measurement once.
//
// It is docs/decision/0014's observation cache: keyed by gatherer, args and
// world, which is exactly what an observation is addressed by less the data it
// has not produced yet. Two promises naming the same gatherer over the same
// scope in one tick share the work — and they must, because a gather runs a
// program and the second run of it would cost the same again to produce a
// record content addressing would immediately discard as a duplicate.
//
// It is safe for concurrent use. A second caller asking for a measurement
// already in flight waits for it rather than starting a second one, because
// otherwise the sharing would hold only for callers that happened not to
// overlap — which is every single-threaded test and no real tick.
type Runner struct {
	catalogue Catalogue

	mu   sync.Mutex
	work map[string]*measurement
}

// measurement is one gather, in flight or finished.
type measurement struct {
	done chan struct{}
	obs  model.Observation
}

// NewRunner returns a runner over a catalogue. Its cache lives as long as it
// does, which is one tick: an observation is durable and content-addressed, and
// nothing here is a substitute for reading one back.
func NewRunner(c Catalogue) *Runner {
	return &Runner{catalogue: c, work: map[string]*measurement{}}
}

// Gather returns the observation for one gather entry.
//
// It never returns an error, because a failed gather is not the caller's error
// to handle — it is a value the caller records nothing for. The failure travels
// on model.Observation.Err, which never reaches the ledger: a failed gather
// leaves no observation at all, which is what makes the verdict `unknown`
// rather than `not-kept`. See docs/decision/0004 and 0034, and use [Record] to
// file the result so that rule lives in one place rather than at every call
// site.
func (r *Runner) Gather(ctx context.Context, spec model.GatherSpec, req Request) model.Observation {
	// The gatherer defaults to the gather map's key, and that defaulting is
	// the corpus's (docs/decision/0005). A spec that arrives here without one
	// was built by hand and names nothing, which is a mistake rather than an
	// invitation to guess.
	refuse := func(err error) model.Observation {
		return model.Observation{
			Gatherer:    spec.Gatherer,
			Args:        req.Args,
			ScopeDigest: req.Scope,
			World:       req.World,
			Err:         err,
		}
	}
	if spec.Gatherer == "" {
		return refuse(errors.New("the gather entry names no gatherer"))
	}
	g, ok := r.catalogue[spec.Gatherer]
	if !ok {
		return refuse(fmt.Errorf("there is no %s gatherer", spec.Gatherer))
	}

	key := cacheKey(g.Name(), req)

	r.mu.Lock()
	if m, running := r.work[key]; running {
		r.mu.Unlock()
		<-m.done
		return m.obs
	}
	m := &measurement{done: make(chan struct{})}
	r.work[key] = m
	r.mu.Unlock()

	m.obs = measure(ctx, g, req)
	close(m.done)
	return m.obs
}

// measure runs one gatherer and shapes what came back into an observation.
//
// Observe, then record — and this half is the observing. The address is
// computed here rather than at write time so that the in-process observation is
// already the thing the ledger will file, which is what lets prior() dedup by
// id and what makes two runs measuring one tree agree without talking.
func measure(ctx context.Context, g Gatherer, req Request) model.Observation {
	obs := model.Observation{
		Gatherer:    g.Name(),
		Args:        req.Args,
		ScopeDigest: req.Scope,
		World:       req.World,
		At:          time.Now().UTC(),
	}

	fields, err := g.Gather(ctx, req)
	if err != nil {
		// No observation. Not a false field, not an empty observation, and
		// never a not-kept: a machine that could not measure does not record
		// what it thinks it saw.
		obs.Err = err
		return obs
	}
	obs.Data = fields

	// The second of docs/decision/0023's two bounds, checked here rather than
	// left to the write. Over it the gather **failed**, so the verdict is
	// unknown; discovering it at write time instead would mean the predicate
	// had already run on data too large to keep, and a promise would be
	// reported kept against a record that does not exist.
	if err := boundRecord(obs); err != nil {
		obs.Data = nil
		obs.Err = err
		return obs
	}

	obs.ID = ledger.Measured(obs).Address()
	return obs
}

// boundRecord refuses a measurement that will not fit the record.
func boundRecord(obs model.Observation) error {
	line, err := json.Marshal(ledger.Measured(obs))
	if err != nil {
		return fmt.Errorf("the measurement cannot be recorded: %w", err)
	}
	return Bound("the encoded observation", len(line), ledger.MaxObservation)
}

// cacheKey is the observation's own address with the data left out, which is
// (gatherer@version, args, world, scope) and is precisely what
// docs/decision/0014 keys the cache by.
//
// Deriving it from the same preimage rather than composing a string is what
// stops the cache and the address becoming two answers to one question: a
// change to what an observation is addressed by moves this with it.
func cacheKey(gatherer string, req Request) string {
	return ledger.Observation{
		Gatherer: gatherer,
		Args:     req.Args,
		World:    req.World,
		Scope:    req.Scope,
	}.Address()
}

// Record files a measurement, and files nothing for a failure.
//
// The rule is one line and it is the single most important one in this package,
// so it lives in one function rather than at every call site. A caller that
// forgot it would write a record for a gather that did not happen, and a
// predicate reading that record would report not-kept for a machine that could
// not measure — which is the collapse the three-valued verdict exists to
// prevent. See docs/decision/0004 and 0034.
//
// The second result is whether anything was recorded.
func Record(l *ledger.Ledger, obs model.Observation) (string, bool, error) {
	if obs.Err != nil {
		return "", false, nil
	}
	address, err := l.Observe(ledger.Measured(obs))
	if err != nil {
		return "", false, err
	}
	return address, true, nil
}
