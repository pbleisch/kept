package gather

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/pbleisch/kept/internal/predicate"
)

// command runs a program the promise named and records the fields the promise
// extracted from what it printed. It is the escape hatch, and every one of
// docs/decision/0023's layers exists because of it.
//
// It never stores stdout. An extractor is required, not defaulted, because
// wanting the whole log means asking for it and asking is the moment to think
// about what is in it — a build log carries tokens, environment dumps and
// connection strings, and a ledger's whole design is that nothing rewrites it.
//
// Three of 0023's four layers are here or beneath here: the declared fields are
// what `fields:` names, the explicit extractor is that there is no default, and
// the two size bounds are [MaxRawOutput] and [ledger.MaxObservation]. The
// fourth, redaction, is deliberately absent — see docs/decision/0034, which
// accepts it as a known cost on the ground that an extractor is *authored*, so
// a person wrote the capture and can read it in review, and that it *binds*, so
// it cannot be changed without a person granting standing again.
type command struct{}

func (command) Name() string { return "command@1" }

// The args a command entry takes.
const (
	argRun    = "run"
	argFields = "fields"
)

// spec is a command entry read and parsed: the program to run, and the
// extractor for each field it declares.
type spec struct {
	argv   []string
	names  []string
	fields map[string]*predicate.Expr
}

func (command) Declares(args map[string]any) ([]string, error) {
	s, err := readCommand(args)
	if err != nil {
		return nil, err
	}
	return s.names, nil
}

// readCommand reads a command entry, and refuses one that cannot be run as
// written.
//
// Everything it refuses is decidable without the world, which is why it is what
// [Catalogue.Check] calls at load: an extractor that will not parse, an arg
// nothing accepts, or a missing `fields:` is a thing a person fixes rather than
// an `unknown` reported against a promise nobody could have kept.
func readCommand(args map[string]any) (spec, error) {
	if err := refuseUnknownArgs(args, argScope, argRun, argFields); err != nil {
		return spec{}, err
	}

	line, _ := args[argRun].(string)
	if line == "" {
		return spec{}, errors.New("command runs a program, and run: names none")
	}
	parts, err := argv(line)
	if err != nil {
		return spec{}, fmt.Errorf("run: %w", err)
	}

	raw, ok := args[argFields].(map[string]any)
	if !ok || len(raw) == 0 {
		return spec{}, errors.New(
			"command never stores what a program printed, so fields: must name what to pull out of stdout, stderr and exit")
	}

	s := spec{argv: parts, fields: make(map[string]*predicate.Expr, len(raw))}
	for _, name := range sortedKeys(raw) {
		src, ok := raw[name].(string)
		if !ok || src == "" {
			return spec{}, fmt.Errorf("fields: %s is not an expression", name)
		}
		expr, err := predicate.Parse(src, predicate.Extractor)
		if err != nil {
			return spec{}, fmt.Errorf("fields: %s: %w", name, err)
		}
		s.fields[name] = expr
		s.names = append(s.names, name)
	}
	sort.Strings(s.names)
	return s, nil
}

// Gather runs the program and evaluates each declared field over what it
// printed.
//
// A non-zero exit is data: `exit` is one of the three roots an extractor reads,
// and a promise that means "it built" writes `ok: exit == 0` rather than
// relying on kept to guess. What is a failure is the program not starting, the
// deadline expiring, or a stream running past the raw bound — and every one of
// those returns an error, which means no observation, which means `unknown`.
func (c command) Gather(ctx context.Context, req Request) (Fields, error) {
	s, err := readCommand(req.Args)
	if err != nil {
		return nil, err
	}

	out, err := runProgram(ctx, req, s.argv)
	if err != nil {
		return nil, err
	}

	scope := out.Scope()
	fields := Fields{}
	for _, name := range s.names {
		v, measured, err := s.fields[name].Extract(scope)
		if err != nil {
			// The expression is wrong, which is not a verdict. It is a gather
			// failure naming the field, so the answer is unknown and a receipt
			// can say which extractor to fix rather than that the gather
			// failed. Most of these are caught at load; what reaches here is
			// the kind only a real program's output produces.
			return nil, &BadField{Field: name, Err: err}
		}
		if measured {
			// An extractor that measured nothing yields no field rather than a
			// field holding an unknown, so the observation carries what was
			// measured and nothing else. A predicate reading the absent field
			// gets unknown for its own reason. See docs/decision/0036.
			fields[name] = v
		}
	}
	return fields, nil
}
