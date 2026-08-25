package main

import (
	"fmt"
	"io"

	"github.com/pbleisch/kept/internal/corpus"
	"github.com/pbleisch/kept/internal/tick"
)

// driftCmd prints what has standing and lacks truth.
//
// It measures nothing. Drift is a query result — nothing creates it and nothing
// resolves it — and a query that ran a test suite would make reading the queue
// cost what deriving it costs. `kept` is what re-derives; this is what reads
// back what `kept` found, and it says plainly when an answer is about a tree
// this is not.
func driftCmd(args []string, stdout, stderr io.Writer) int {
	fs, root := flags("drift", stderr)
	asJSON := fs.Bool("json", false, "the structured document, per decision 0027")
	rest, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "kept: drift takes no arguments; did you mean why %s?\n", rest[0])
		return exitUsage
	}

	c, code := open(*root, stderr)
	if code != exitOK {
		return code
	}
	if code := sound(c, stderr); code != exitOK {
		return code
	}
	res, code := survey(c, stderr)
	if code != exitOK {
		return code
	}

	code = answer(res)
	if *asJSON {
		return writeDocument(stdout, stderr, newDocument("drift", c, res, code, false))
	}
	writeDrift(stdout, res)
	return code
}

func writeDrift(w io.Writer, res *tick.Result) {
	t := table{indent: "  "}
	for _, s := range res.Subjects {
		if !s.Drifting {
			continue
		}
		t.add(string(s.Drift.Kind), ago(s.Drift.Since), s.ID)
		t.beneath(detailLines(s)...)
	}
	t.write(w)
	drifting := len(t.rows)

	if drifting == 0 {
		fmt.Fprint(w, "nothing with standing is without truth\n")
		return
	}
	fmt.Fprintf(w, "\n%s with standing and without truth\n", count(drifting, "artifact"))
	fmt.Fprint(w, "kept why <artifact> says what the tick did about one, and why\n")
}

// lookup resolves an artifact the way every verb that takes one does: by
// "type:slug", or by a slug alone when only one type answers to it.
func lookup(c *corpus.Corpus, name string, stderr io.Writer) (string, int) {
	entry, err := c.Lookup(name)
	if err != nil {
		fmt.Fprintf(stderr, "kept: %s\n", err)
		return "", exitUsage
	}
	return entry.Artifact.ID, exitOK
}
