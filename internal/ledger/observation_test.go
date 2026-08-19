package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/model"
)

func measured() Observation {
	return Observation{
		Gatherer: "tests@2",
		Args:     map[string]any{"run": "go test ./...", "timeout": "2m"},
		World:    model.World{Tree: "sha256:aaaa"},
		Scope:    "sha256:bbbb",
		Data:     map[string]any{"passed": int64(41), "failed": int64(0)},
	}
}

// Two runs that observe the same thing write identical bytes to one path, which
// is what docs/layout.md claims content addressing is for and what a writer
// field would have taken away. The first writer wins, so the timestamp keeps
// meaning first-seen.
func TestTwoRunsMeasuringOneThingWriteOneFile(t *testing.T) {
	s := store(t)
	first, _ := open(t, s)
	second, _ := open(t, s)

	early := measured()
	early.At = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	a, err := first.Observe(early)
	if err != nil {
		t.Fatal(err)
	}

	late := measured()
	late.At = early.At.Add(72 * time.Hour)
	b, err := second.Observe(late)
	if err != nil {
		t.Fatal(err)
	}

	if a != b {
		t.Fatalf("one measurement got two addresses: %s and %s", a, b)
	}
	held, err := ReadObservation(s, a)
	if err != nil {
		t.Fatal(err)
	}
	if !held.At.Equal(early.At) {
		t.Errorf("At = %s, want the first writer's %s: a measurement of an unchanging world would look fresh forever", held.At, early.At)
	}
}

// What is addressed and what is merely recorded. Each of the five addressed
// fields must move the address, and the timestamp must not: a timestamp in the
// address would mean no two runs ever produce one file.
func TestWhatIsAddressedAndWhatIsNot(t *testing.T) {
	base := measured().Address()

	for _, tc := range []struct {
		name   string
		change func(*Observation)
	}{
		{"a gatherer's version, so a bug fix does not reuse pre-fix observations",
			func(o *Observation) { o.Gatherer = "tests@3" }},
		{"the args, so two command gathers do not collide",
			func(o *Observation) { o.Args["run"] = "go test ./internal/..." }},
		{"the world, which is 0014's whole rule",
			func(o *Observation) { o.World.Tree = "sha256:cccc" }},
		{"the scope digest, which is part of what the gatherer saw",
			func(o *Observation) { o.Scope = "sha256:dddd" }},
		{"the data, which is the measurement",
			func(o *Observation) { o.Data["failed"] = int64(1) }},
	} {
		o := measured()
		tc.change(&o)
		if o.Address() == base {
			t.Errorf("changing %s did not change the address", tc.name)
		}
	}

	dated := measured()
	dated.At = time.Now()
	if dated.Address() != base {
		t.Error("the timestamp is in the address, so nothing will ever dedup")
	}
}

// The sharp edge of 0034. These four pairs are genuinely different
// measurements, and 0022's rules make them one — so an address computed under
// them would discard the second as a duplicate of the first, which is the stale
// answer 0014 exists to prevent. This is the test that fires if someone reaches
// for the binding hash's text rule here.
func TestAMeasurementIsAddressedAsItWasMeasured(t *testing.T) {
	seen := map[string]string{}
	for _, output := range []string{
		"FAIL\ttrailing   \n", "FAIL\ttrailing\n",
		"line\r\nline2\r\n", "line\nline2\n",
		"out", "out\n\n\n",
		"café", "café",
	} {
		o := measured()
		o.Data = map[string]any{"stdout": output}
		address := o.Address()
		if other, taken := seen[address]; taken {
			t.Errorf("%q and %q address identically, so one of them is discarded as a duplicate", other, output)
		}
		seen[address] = output
	}
}

// The thread loop gathers three times against two worlds. A box mid-repair and
// the tree that ships are different content and land in different files, or the
// post-repair gather reads a result from before the repair ran.
func TestTwoWorldsAreTwoObservations(t *testing.T) {
	origin := measured()
	box := measured()
	box.World.Thread = "3k9wq2m1x8b4nptv0zrh7cdfy6"

	if origin.Address() == box.Address() {
		t.Fatal("a gather in a box and one on the tree that ships are one observation")
	}
}

// An observation names no run. Not an exception to docs/decision/0013's rule
// but the rule itself: the same tree measured by the same gatherer is the same
// observation, whoever ran it — and a writer would make two runs write
// different bytes for one measurement.
func TestAnObservationNamesNoRunAndCarriesNoError(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	address, err := l.Observe(measured())
	if err != nil {
		t.Fatal(err)
	}
	where, err := ObservationPath(address)
	if err != nil {
		t.Fatal(err)
	}
	src, err := s.Read(where)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(src, &raw); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"writer", "seq", "err", "error"} {
		if _, held := raw[name]; held {
			t.Errorf("the record carries %q", name)
		}
	}
	if raw["v"] == nil {
		t.Error("no envelope version, and it cannot be added to records already written")
	}
}

// A failed gather leaves no observation at all, which is what makes the verdict
// unknown. The narrowing from the in-process value is where that is enforced.
func TestAFailedGatherLeavesNoObservation(t *testing.T) {
	rec := Measured(model.Observation{
		Gatherer:    "tests@2",
		World:       model.World{Tree: "sha256:aaaa"},
		ScopeDigest: "sha256:bbbb",
		Err:         errors.New("no toolchain"),
	})

	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(line), "no toolchain") {
		t.Fatal("the gatherer's error reached the record, which makes a failure a kind of observation")
	}
}

// 0018's completeness guarantee, which observations get better than a stream
// does: a partial write is detectable by hashing what is there, so the file is
// discarded rather than read.
func TestATornObservationIsDiscardedRatherThanRead(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	address, err := l.Observe(measured())
	if err != nil {
		t.Fatal(err)
	}
	where, err := ObservationPath(address)
	if err != nil {
		t.Fatal(err)
	}
	src, err := s.Read(where)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(where, src[:len(src)/2]); err != nil {
		t.Fatal(err)
	}

	var corrupt *Corrupt
	if _, err := ReadObservation(s, address); !errors.As(err, &corrupt) {
		t.Fatalf("a torn observation read as %v, want a discard", err)
	}

	// And the next run's measurement replaces it, because what was there was
	// not somebody else's answer.
	if _, err := l.Observe(measured()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadObservation(s, address); err != nil {
		t.Fatalf("the torn file was not replaced: %v", err)
	}
}

// A record whose fields say one thing and whose address says another is not
// read, whichever of the two was tampered with.
func TestARecordThatDoesNotHashToItsPathIsRefused(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	address, err := l.Observe(measured())
	if err != nil {
		t.Fatal(err)
	}
	where, err := ObservationPath(address)
	if err != nil {
		t.Fatal(err)
	}

	edited := measured()
	edited.Data["passed"] = int64(0)
	line, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(where, line); err != nil {
		t.Fatal(err)
	}

	var corrupt *Corrupt
	if _, err := ReadObservation(s, address); !errors.As(err, &corrupt) {
		t.Fatalf("an edited observation read as %v", err)
	}
}

// The cap that bounds what the absence of redaction can cost. It fires, per
// AGENTS.md, and it says which bound and by how much — because that is what a
// gather receipt has to carry for the refusal to be diagnosable.
func TestTheEncodedObservationIsCapped(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	o := measured()
	o.Data = map[string]any{"stdout": strings.Repeat("x", MaxObservation+1024)}

	_, err := l.Observe(o)
	var large *TooLarge
	if !errors.As(err, &large) {
		t.Fatalf("Observe returned %v, want a refusal over the bound", err)
	}
	if large.Bound == "" || large.Size <= large.Limit {
		t.Errorf("the refusal does not say which bound and by how much: %+v", large)
	}
	if !strings.Contains(large.Error(), "over") {
		t.Errorf("the refusal reads as %q", large.Error())
	}
}

func TestAnObservationWithoutAWorldIsRefused(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	o := measured()
	o.World = model.World{}
	if _, err := l.Observe(o); err == nil {
		t.Fatal("recorded a measurement that names no tree, and nothing else records which one it was")
	}
}

func TestAnAddressIsAPathAndBack(t *testing.T) {
	address := measured().Address()
	where, err := ObservationPath(address)
	if err != nil {
		t.Fatal(err)
	}
	if want := "observations/sha256/"; !strings.HasPrefix(where, want) {
		t.Errorf("path %q does not start %q", where, want)
	}
	if strings.Count(where, "/") != 3 {
		t.Errorf("path %q is not observations/sha256/ab/cdef…", where)
	}
	if _, err := ObservationPath("nonsense"); err == nil {
		t.Error("a string that is not an address resolved to a path")
	}
}

// Verifiable by re-deriving the preimage from the record's own fields, which is
// the property a non-addressed timestamp would otherwise have taken away. It
// has to survive the decoder: a gatherer returns Go values and a reader gets
// whatever JSON carried, so the two must address alike for every shape a
// gatherer can declare.
func TestARecordVerifiesAgainstItsOwnFieldsAfterAReadBack(t *testing.T) {
	s := store(t)
	l, _ := open(t, s)

	o := measured()
	o.Data = map[string]any{
		"passed":   int64(41),
		"coverage": 87.5,
		"whole":    float64(9),
		"green":    true,
		"missing":  nil,
		"packages": []any{"corpus", "ledger"},
		"slowest":  map[string]any{"name": "predicate", "seconds": 3.25},
		"log":      "PASS\ttrailing   \r\n",
	}

	address, err := l.Observe(o)
	if err != nil {
		t.Fatal(err)
	}
	held, err := ReadObservation(s, address)
	if err != nil {
		t.Fatalf("a record written moments ago does not verify: %v", err)
	}
	if held.Address() != address {
		t.Fatalf("read back as %s, filed under %s", held.Address(), address)
	}
	if held.Data["log"] != "PASS\ttrailing   \r\n" {
		t.Errorf("the measurement came back as %q", held.Data["log"])
	}
}
