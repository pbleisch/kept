// Package run identifies one execution of kept: a process from start to exit.
//
// Every durable record names the run that wrote it, so a verdict can be traced
// back to the process that reached it. Several runs execute at once — see
// decision 0011 — and none of them coordinate, so the identifier is minted
// rather than allocated. It carries no meaning; see internal/ids for why, and
// see Record for where the diagnostics live instead.
package run

import (
	"time"

	"github.com/pbleisch/kept/internal/ids"
)

// ID names one execution of kept. Opaque: nothing may parse it, and two ids may
// only be compared for equality or sorted for display. Abbreviated on the
// command line by any unambiguous prefix, the way git abbreviates a sha.
type ID string

// Mint returns a fresh ID.
func Mint() ID { return ID(ids.Mint()) }

// Short returns the prefix used in logs and tables.
func (id ID) Short() string { return ids.Short(string(id)) }

// Record is what binds an ID to the machine that ran it. One file per run under
// the ledger side ref's runs/{id}.json, written at start and rewritten at exit
// — one writer per path, so it needs no merge strategy.
//
// Diagnostic, not durable: a Record may be pruned by retention while the
// records naming its ID remain. An unresolvable writer means the run's
// diagnostics aged out, not that the record naming it is suspect.
type Record struct {
	ID      ID        `json:"id"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitempty"`

	// What the run was. Version is the build the process was compiled from;
	// Argv is the invocation, less any value config marks as secret.
	Version string   `json:"version"`
	Argv    []string `json:"argv"`

	// Where it ran. Absent when config sets diagnostics.host: false, which a
	// shared repository may want. The ID and the attribution survive that.
	Host string `json:"host,omitempty"`
	PID  int    `json:"pid,omitempty"`

	// What it did. Counters, for reading a run back without walking the
	// ledger for every record that names it.
	Ticks        int `json:"ticks,omitempty"`
	Acts         int `json:"acts,omitempty"`
	Observations int `json:"observations,omitempty"`
	Transitions  int `json:"transitions,omitempty"`
	Threads      int `json:"threads,omitempty"`

	// Exit is "clean", "signalled", "failed", or empty for a run that did not
	// reach its own shutdown — a kill -9, a crash, a machine that went away.
	// Empty with Ended zero is the ordinary evidence of an unclean stop.
	Exit string `json:"exit,omitempty"`
}

// Exit values.
const (
	Clean  = "clean"
	Failed = "failed"
)

// Start returns the record for this execution: a fresh id, the moment it began,
// and what it was invoked as.
//
// Host and PID are left unset. Whether they may be written is `diagnostics.host`
// in .kept/config.yaml, and nothing reads that file yet — so writing them would
// put a hostname into a tracked repository in exactly the deployment that asked
// for it not to be. The id and the attribution survive their absence, which is
// what docs/decision/0012 says they are for; filling them in is what reading the
// config buys.
func Start(version string, argv []string) *Record {
	return &Record{
		ID:      Mint(),
		Started: time.Now().UTC(),
		Version: version,
		Argv:    append([]string(nil), argv...),
	}
}
