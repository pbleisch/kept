package gather

import "github.com/pbleisch/kept/internal/model"

// UncheckedInstrument reports a `command` gather entry whose declared fields
// never reference `exit`: you ran a program and never looked at whether it
// worked.
//
// **The false-green is a class bug in `command`, and a native gatherer fixes
// instances of it.** A gatherer kept ships cannot make this mistake, because
// its failure is a gather failure and [Record] files nothing for one — the
// verdict is `unknown` and there is nothing for an author to remember. But a
// manuscript author will write `command` promises for years before a native
// gatherer exists for their domain, so this protects the case the gatherers do
// not reach.
//
// It is a **finding and never an error**: `wc -l` legitimately ignores its exit
// status, and so does any program whose output is the measurement and whose
// failure mode is empty output the predicate already handles. `doctor` reports
// it and the corpus still loads. See docs/decision/0050.
//
// The question is asked of the parsed expression and never of its text. `exit`
// is a root of the extractor namespace; `exit_code` is a different name,
// `"exit"` is a string, and `r.exit` is a field selected off a record, and a
// substring match reports all four.
func UncheckedInstrument(spec model.GatherSpec) bool {
	if spec.Gatherer != Command {
		return false
	}
	s, err := readCommand(spec.Args)
	if err != nil {
		// An entry that cannot be read as written is already reported, by the
		// load, as the thing it is. Saying it is also unwise would be a second
		// finding about one mistake.
		return false
	}
	for _, name := range s.names {
		if s.fields[name].Reads(exitRoot) {
			return false
		}
	}
	return true
}

// exitRoot is what an extractor calls a program's exit status. It is one of the
// three roots predicate.Extractor declares, and it is named here rather than
// spelled inline so the two cannot drift — a rename there would make the
// finding above stop firing silently, which is what the test that parses it
// exists to notice.
const exitRoot = "exit"
