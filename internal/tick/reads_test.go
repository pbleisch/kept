package tick

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/gather"
	"github.com/pbleisch/kept/internal/ledger"
	"github.com/pbleisch/kept/internal/run"
)

// reader is a gatherer that declares a read of the acts a person performs and
// measures nothing at all.
//
// It measures nothing on purpose. What is under test is when a promise is
// asked again, not what the answer is, and a gatherer whose data moved would
// make the two indistinguishable.
type reader struct{}

func (reader) Name() string                              { return "x/reader@1" }
func (reader) Reads() []ledger.Kind                      { return []ledger.Kind{ledger.Ratified} }
func (reader) Declares(map[string]any) ([]string, error) { return []string{"ok"}, nil }
func (reader) Gather(context.Context, gather.Request) (gather.Fields, error) {
	return gather.Fields{"ok": true}, nil
}

// docs/decision/0046's claim, end to end and with no clause added to [Tick.due]
// to make it true: a person records an act about an artifact in a promise's
// scope, and the promise is asked again.
//
// Nothing kept ships declares a ledger read, so without this test the
// mechanism has never fired.
func TestARecordedActMakesADeclaringPromiseDue(t *testing.T) {
	b := newBox(t)
	b.write("docs/promise/reads-the-acts.md", `---
id: reads-the-acts
type: promise
title: reads-the-acts
fields:
  statement: something in docs/promise has been ratified
  gather:
    acts:
      gatherer: x/reader
      scope: "docs/promise/**"
  hold_when: obs.acts.ok
  trigger: cadence
---

Why: because a reading moves nothing the tree can see.
`)

	catalogue := gather.Catalogue{"x/reader": reader{}}

	// First pass answers, second finds nothing has moved.
	if s := subjectOf(t, b, catalogue, "promise:reads-the-acts"); !s.Due {
		t.Fatal("nothing has ever answered and the promise is not due")
	}
	settled := subjectOf(t, b, catalogue, "promise:reads-the-acts")
	if settled.Due {
		t.Fatalf("nothing moved and the promise is due: %s", settled.Because)
	}

	// A person ratifies something in the promise's scope. The tree is
	// untouched — this is the whole case 0046 was written about.
	before := treeDigest(t, b, catalogue)
	l := ledger.Open(b.store(), run.Start("test", nil))
	if _, err := l.Ratify("promise:reads-the-acts", "sha256:"+strings.Repeat("a", 64), 1, "A Person", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	if after := treeDigest(t, b, catalogue); after != before {
		t.Fatal("recording an act moved the world digest, so this proves nothing")
	}

	woken := subjectOf(t, b, catalogue, "promise:reads-the-acts")
	if !woken.Due {
		t.Fatal("a person recorded an act about an artifact in scope and the promise was not asked again")
	}
	if !strings.Contains(woken.Because, "an act it reads was recorded") {
		t.Errorf("the reason a person reads is %q", woken.Because)
	}
}

// An act about an artifact the promise's scope does not cover leaves it alone,
// which is what keeps a promise about chapters 1–5 from being asked again when
// chapter 40 is read.
func TestAnActOutOfScopeDoesNotWakeADeclaringPromise(t *testing.T) {
	b := newBox(t)
	b.write("docs/promise/reads-the-acts.md", `---
id: reads-the-acts
type: promise
title: reads-the-acts
fields:
  statement: something in docs/promise has been ratified
  gather:
    acts:
      gatherer: x/reader
      scope: "docs/promise/**"
  hold_when: obs.acts.ok
  trigger: cadence
---

Why: because a reading moves nothing the tree can see.
`)
	b.write("docs/decision/0001-a-decision.md", "---\nid: 0001-a-decision\ntype: decision\ntitle: one\n---\n\nBody.\n")

	catalogue := gather.Catalogue{"x/reader": reader{}}
	subjectOf(t, b, catalogue, "promise:reads-the-acts")
	if s := subjectOf(t, b, catalogue, "promise:reads-the-acts"); s.Due {
		t.Fatalf("nothing moved and the promise is due: %s", s.Because)
	}

	l := ledger.Open(b.store(), run.Start("test", nil))
	if _, err := l.Ratify("decision:0001-a-decision", "sha256:"+strings.Repeat("b", 64), 1, "A Person", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}

	if s := subjectOf(t, b, catalogue, "promise:reads-the-acts"); s.Due {
		t.Errorf("an act about an artifact out of scope woke the promise: %s", s.Because)
	}
}

func subjectOf(t *testing.T, b *box, catalogue gather.Catalogue, id string) Subject {
	t.Helper()
	c, err := corpus.Load(filepath.Join(b.root, "docs"), b.root, corpus.Options{Gatherers: catalogue})
	if err != nil {
		t.Fatal(err)
	}
	store := b.store()
	l := ledger.Open(store, run.Start("test", nil))
	tick := &Tick{Corpus: c, Store: store, Catalogue: catalogue, Ledger: l}
	tick.Options.Deadline = 30 * time.Second

	res, err := tick.Run(t.Context())
	if err != nil {
		t.Fatalf("the tick failed: %v", err)
	}
	if err := l.Close(run.Clean); err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Subjects {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no subject called %s", id)
	return Subject{}
}

func treeDigest(t *testing.T, b *box, catalogue gather.Catalogue) string {
	t.Helper()
	c, err := corpus.Load(filepath.Join(b.root, "docs"), b.root, corpus.Options{Gatherers: catalogue})
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&Tick{Corpus: c, Store: b.store(), Catalogue: catalogue}).Survey()
	if err != nil {
		t.Fatal(err)
	}
	return res.World.Tree
}
