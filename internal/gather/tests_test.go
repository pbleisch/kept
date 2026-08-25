package gather

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pbleisch/kept/internal/model"
)

const stream = "testdata/go-test.jsonl"

// gatherStream runs the tests gatherer over a canned `go test -json` stream, by
// pointing it at a program that prints one. The fold is what is under test, and
// a real toolchain would make the answer depend on what this repository's tests
// happen to be doing today.
func gatherStream(t *testing.T, path string, exit string) Fields {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	req := request(t, nil)
	fields, err := (tests{binary: writeStub(t, abs, exit)}).Gather(t.Context(), req)
	if err != nil {
		t.Fatalf("the fold failed: %v", err)
	}
	return fields
}

// `go test -json` is a heterogeneous stream, not a table: a package-level event
// carries no Test, a build failure carries an ImportPath and no Package, most
// lines are output and carry nothing. Assuming a uniform shape is the mistake
// that made has() necessary in the predicate language, and it is the same
// mistake here.
func TestTheTestsGathererFoldsAHeterogeneousStream(t *testing.T) {
	fields := gatherStream(t, stream, "1")

	if fields["ok"] != false {
		t.Errorf("ok is %#v, and a package failed to build", fields["ok"])
	}
	if fields["passed"] != 2 || fields["failed"] != 2 || fields["skipped"] != 1 {
		t.Errorf("passed/failed/skipped are %v/%v/%v", fields["passed"], fields["failed"], fields["skipped"])
	}

	cases := textList(fields["cases"])
	want := []string{"TestAGuardThatFires", "TestAGuardThatFires/stdout", "TestRevisionDropsStanding", "TestSkippedOnThisPlatform", "TestWorldNamesItsTree"}
	if !same(cases, want) {
		t.Errorf("cases are %q, want %q", cases, want)
	}
	if failures := textList(fields["failures"]); !same(failures, []string{"TestAGuardThatFires", "TestAGuardThatFires/stdout"}) {
		t.Errorf("failures are %q", failures)
	}

	packages := records(t, fields["packages"])
	if len(packages) != 4 {
		t.Fatalf("the fold found %d packages in a stream naming four", len(packages))
	}
	for _, want := range []map[string]any{
		{"package": "example/broken", "ok": false, "tests_run": 0},
		{"package": "example/empty", "ok": true, "tests_run": 0},
		{"package": "example/good", "ok": true, "tests_run": 2},
		{"package": "example/mixed", "ok": false, "tests_run": 3},
	} {
		found := false
		for _, got := range packages {
			if got["package"] != want["package"] {
				continue
			}
			found = true
			for k, v := range want {
				if got[k] != v {
					t.Errorf("%s: %s is %#v, want %#v", want["package"], k, got[k], v)
				}
			}
		}
		if !found {
			t.Errorf("%s is not among the packages", want["package"])
		}
	}
}

// A build failure has no test events at all, so without a record of its own the
// package would be absent and a promise about every package would read as kept.
func TestAPackageThatDidNotBuildIsOnePackageAndItFailed(t *testing.T) {
	packages := records(t, gatherStream(t, stream, "1")["packages"])
	for _, p := range packages {
		if p["package"] == "example/broken" {
			if p["ok"] != false {
				t.Fatalf("the package that would not compile reads as %#v", p)
			}
			return
		}
	}
	t.Fatal("the package that would not compile is not in the observation at all")
}

// The declared fields carry what a real promise about this repository is
// written against, and the promise in the corpus is the check on that.
func TestTheDeclaredFieldsCarryARealPromise(t *testing.T) {
	fields := gatherStream(t, stream, "1")
	obs := model.Observation{Data: map[string]any(fields)}

	for _, c := range []struct {
		src  string
		want model.Value
	}{
		{`all(obs.tests.packages, p => p.tests_run > 0 and p.failed == 0)`, model.NotKept},
		{`obs.tests.failed == 0`, model.NotKept},
		{`"TestRevisionDropsStanding" in obs.tests.cases`, model.Kept},
		{`count(obs.tests.failures, f => true) == 2`, model.Kept},
		{`obs.tests.coverage > 80`, model.Unknown},
	} {
		if got := verdict(t, "tests", obs, c.src); got != c.want {
			t.Errorf("%s is %s, want %s", c.src, got, c.want)
		}
	}
}

// An observation is addressed by its data, so an ordering that came from map
// iteration would give one measurement a different address every time it was
// made, and two runs measuring one tree would write two files where
// docs/layout.md says they write one.
func TestOneStreamFoldsToOneAddress(t *testing.T) {
	first := address(t, gatherStream(t, stream, "1"))
	for range 8 {
		if again := address(t, gatherStream(t, stream, "1")); again != first {
			t.Fatalf("one stream produced two addresses: %s and %s", first, again)
		}
	}
}

// Nothing timing-dependent is recorded. Every event in the stream carries an
// Elapsed and none of it is here: a duration would mint a fresh record on every
// tick of an unchanging world, and dedup would quietly stop working.
func TestNothingTimingDependentIsRecorded(t *testing.T) {
	line := string(mustMarshal(t, gatherStream(t, stream, "1")))
	for _, forbidden := range []string{"elapsed", "Elapsed", "time", "Time", "0.03"} {
		if strings.Contains(line, forbidden) {
			t.Errorf("the observation carries %s: %s", forbidden, line)
		}
	}
}

// A missing toolchain is unknown, never not-kept. This is the invariant in
// AGENTS.md and it is what the three-valued verdict is for.
func TestTheTestsGathererWithNoToolchainIsUnknown(t *testing.T) {
	_, err := (tests{binary: "kept-there-is-no-go-here"}).Gather(t.Context(), request(t, nil))
	if err == nil {
		t.Fatal("a gatherer with no toolchain produced an observation")
	}
	if _, ok := err.(*NotRunnable); !ok {
		t.Fatalf("the failure is %#v, and nothing can tell it from a suite that ran", err)
	}
}

// A suite that ran and failed still emits events. An empty stream with a
// non-zero exit is `go` refusing the invocation, which is a failure to measure
// rather than a measurement — and reporting it as one would say the tests
// failed when nothing ran.
func TestAStreamNobodyCouldReadIsAFailureRatherThanAFailingSuite(t *testing.T) {
	req := request(t, nil)
	_, err := (tests{binary: writeStub(t, os.DevNull, "2")}).Gather(t.Context(), req)
	if err == nil {
		t.Fatal("a suite that printed nothing and exited 2 was read as a measurement")
	}
	if !strings.Contains(err.Error(), "nothing kept could read") {
		t.Errorf("the failure reads as %q", err)
	}
}

// The args bind, so a gatherer that quietly ignored one would bake the mistake
// into a ratified promise.
func TestTheTestsGathererRefusesArgsItDoesNotTake(t *testing.T) {
	if _, err := (tests{}).Declares(map[string]any{"package": "./internal/..."}); err == nil {
		t.Fatal("tests accepted an arg it does not have")
	}
	if _, err := (tests{}).Declares(map[string]any{argPackages: "./internal/..."}); err != nil {
		t.Fatalf("tests refused the arg it does take: %v", err)
	}
	if _, err := (tests{}).Declares(map[string]any{argPackages: 7}); err == nil {
		t.Fatal("tests accepted a number as a package pattern")
	}
	if _, err := (tests{}).Declares(map[string]any{argExclude: "./internal/confine"}); err != nil {
		t.Fatalf("tests refused the exclusion it does take: %v", err)
	}
	if _, err := (tests{}).Declares(map[string]any{argExclude: ""}); err == nil {
		t.Fatal("tests accepted an exclusion that names no package")
	}
	if _, err := (tests{}).Declares(map[string]any{argExclude: 7}); err == nil {
		t.Fatal("tests accepted a number as an exclusion")
	}
}

// excluded is a module of two packages whose tests both leave a mark, so what a
// run touched is read from the filesystem rather than from the stream it chose
// to print. A package that was excluded and ran anyway would fold away
// invisibly; a mark it left cannot.
func excluded(t *testing.T) (root string, marks map[string]string) {
	t.Helper()
	root = t.TempDir()
	marks = map[string]string{}
	write(t, filepath.Join(root, "go.mod"), "module example\n\ngo 1.26\n")
	for _, name := range []string{"kept", "confined"} {
		mark := filepath.Join(root, name+".ran")
		marks[name] = mark
		write(t, filepath.Join(root, name, name+"_test.go"), "package "+name+`

import (
	"os"
	"testing"
)

func TestItRan(t *testing.T) {
	if err := os.WriteFile(`+strconv.Quote(mark)+`, []byte("ran"), 0o600); err != nil {
		t.Fatal(err)
	}
}
`)
	}
	return root, marks
}

// An excluded package is not filtered out of the stream afterwards — it is
// never started. Filtering would be enough for the fold and useless for the
// reason the exclusion exists: `internal/confine`'s tests start real confined
// children, and a box must not be where that happens.
func TestAnExcludedPackageIsNeverRun(t *testing.T) {
	realToolchain(t)
	root, marks := excluded(t)

	req := request(t, map[string]any{argPackages: "./...", argExclude: "./confined"})
	req.Root = root
	fields, err := (tests{}).Gather(t.Context(), req)
	if err != nil {
		t.Fatalf("the suite could not be measured: %v", err)
	}

	if _, err := os.Stat(marks["kept"]); err != nil {
		t.Fatalf("the package that was not excluded did not run: %v", err)
	}
	if _, err := os.Stat(marks["confined"]); !os.IsNotExist(err) {
		t.Fatalf("an excluded package ran anyway (%v)", err)
	}

	packages := records(t, fields["packages"])
	if len(packages) != 1 {
		t.Fatalf("the fold found %d packages, and one of two was excluded", len(packages))
	}
	if name, _ := packages[0]["package"].(string); name != "example/kept" {
		t.Fatalf("the package that ran is %q", name)
	}
	if fields["ok"] != true || fields["passed"] != 1 {
		t.Fatalf("the observation is %#v", fields)
	}
}

// The guard, fired. An exclusion that no longer removes anything — the package
// renamed, moved, or never in the suite to begin with — is the failure mode of
// naming a package in a promise, and it is silent unless something refuses it.
// It is `unknown` and not `not-kept`: what could not be established is which
// packages the promise meant.
func TestAnExclusionThatRemovesNothingIsRefused(t *testing.T) {
	realToolchain(t)
	root, marks := excluded(t)

	for _, c := range []struct {
		name    string
		args    map[string]any
		want    string
		wantRan bool
	}{
		{"the package it names is gone", map[string]any{argPackages: "./...", argExclude: "./renamed"},
			"removes no package", false},
		{"it names a package this suite never ran", map[string]any{argPackages: "./kept", argExclude: "./confined"},
			"removes no package", false},
		{"it removes the whole suite", map[string]any{argPackages: "./confined", argExclude: "./..."},
			"removes every package", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, mark := range marks {
				os.Remove(mark)
			}
			req := request(t, c.args)
			req.Root = root
			fields, err := (tests{}).Gather(t.Context(), req)
			if err == nil {
				t.Fatalf("an exclusion that removes nothing was measured as %#v", fields)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal reads as %q, want it to name %q", err, c.want)
			}
			for name, mark := range marks {
				if _, err := os.Stat(mark); err == nil {
					t.Fatalf("%s ran under an exclusion nothing accepted", name)
				}
			}
		})
	}
}

// The end-to-end path, against the toolchain that is actually here. It is
// skipped rather than failed where there is none, because a machine without a
// Go toolchain is exactly the case every other test in this file covers
// deliberately.
func TestTheTestsGathererAgainstTheRealToolchain(t *testing.T) {
	realToolchain(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example\n\ngo 1.26\n")
	write(t, filepath.Join(dir, "a_test.go"), "package example\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {}\n")

	req := request(t, nil)
	req.Root = dir
	fields, err := (tests{}).Gather(t.Context(), req)
	if err != nil {
		t.Fatalf("the suite could not be measured: %v", err)
	}
	if fields["ok"] != true || fields["passed"] != 1 {
		t.Fatalf("the observation is %#v", fields)
	}
	if cases := textList(fields["cases"]); !same(cases, []string{"TestOne"}) {
		t.Errorf("cases are %q", cases)
	}
}

// realToolchain skips where there is none, because a machine without a Go
// toolchain is exactly the case TestTheTestsGathererWithNoToolchainIsUnknown
// covers deliberately.
func realToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain here, which is the case TestTheTestsGathererWithNoToolchainIsUnknown covers")
	}
}
