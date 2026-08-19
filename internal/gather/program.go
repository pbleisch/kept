package gather

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/pbleisch/kept/internal/predicate"
)

// waitDelay is how long a killed process group has to die before its pipes are
// closed out from under it.
//
// Without one, a grandchild holding the pipe keeps the read alive after the
// process kept started is gone, and the gather that "timed out" never returns.
// See docs/decision/0026: a killed process is killed, including its children.
const waitDelay = 5 * time.Second

// runProgram runs one program under the request's deadline and returns what it
// produced.
//
// A non-zero exit is a **measurement**, not a failure: `go test` exits 1 when a
// test fails, and reporting that as a gather failure would turn every broken
// promise into an `unknown`. What is a failure is the program never starting,
// the deadline expiring, another hand killing it, or either stream running past
// the raw bound — and each of those is its own error type, because
// docs/decision/0026 requires a timeout to be tellable from a failure in the
// record even though both resolve to `unknown`.
//
// The environment is inherited. A gatherer running `go test` needs PATH, HOME
// and a build cache, and a program the operator authored in their own corpus
// runs on their own machine with their own credentials. That is the same trust
// boundary `run:` already is, and it is not the box: what a repair agent may
// read, connect to and hold is confinement, configured separately (0017).
func runProgram(ctx context.Context, req Request, argv []string) (predicate.Output, error) {
	if len(argv) == 0 {
		return predicate.Output{}, errors.New("no program to run")
	}
	name := argv[0]
	limit := req.deadline()

	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, argv[1:]...)
	cmd.Dir = req.Root
	// exec.CommandContext kills the process and not the group it leads. A
	// gatherer that shells out leaves one behind, and abandoning the parent
	// while calling it a timeout leaves a build running that will corrupt the
	// next one's output.
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = waitDelay
	setGroup(cmd)

	// The raw bound fires before an extractor reads anything, so a runaway
	// program cannot exhaust memory before any other layer runs. Past it the
	// bytes are dropped and the group is killed rather than read to the
	// deadline, because a program emitting gigabytes wedges the tick whether or
	// not it is being stored.
	stdout := &capped{limit: MaxRawOutput, stop: cancel}
	stderr := &capped{limit: MaxRawOutput, stop: cancel}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	// Order matters. Overflow cancels the context, so asking the context first
	// would report every runaway program as a timeout.
	if stdout.over {
		return predicate.Output{}, Bound("stdout", stdout.n, MaxRawOutput)
	}
	if stderr.over {
		return predicate.Output{}, Bound("stderr", stderr.n, MaxRawOutput)
	}

	out := predicate.Output{Stdout: stdout.b.String(), Stderr: stderr.b.String()}
	if err == nil {
		return out, nil
	}

	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		// It never started: no such binary, not executable, a toolchain that is
		// not installed. This is the case the three-valued verdict was built
		// for.
		return predicate.Output{}, &NotRunnable{Program: name, Err: err}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return predicate.Output{}, &TimedOut{What: name, After: limit}
	}
	if code := exit.ExitCode(); code >= 0 {
		out.Exit = code
		return out, nil
	}
	// Killed by something that is not kept's own deadline. A program that did
	// not choose its own exit did not finish saying what it saw.
	return predicate.Output{}, &Signalled{Program: name, Status: exit.ProcessState.String()}
}

// capped is a bounded sink. It keeps counting after the bound so a refusal can
// say by how much, and it drops the bytes so counting costs nothing.
//
// It is written by exactly one of os/exec's copying goroutines, and read after
// Wait has joined them, so it needs no lock.
type capped struct {
	limit int
	n     int
	over  bool
	b     bytes.Buffer
	// stop is called once, the moment the bound is passed, to kill the group.
	stop func()
}

func (c *capped) Write(p []byte) (int, error) {
	c.n += len(p)
	if c.n > c.limit {
		if !c.over {
			c.over = true
			if c.stop != nil {
				c.stop()
			}
		}
		return len(p), nil
	}
	c.b.Write(p)
	return len(p), nil
}

// argv splits a `run:` line into a program and its arguments.
//
// There is no shell. A shell would make `run:` an injection surface, make the
// gather depend on which shell the machine has, and hide the process group the
// deadline has to kill. What it would buy — pipes, redirection, `&&` — is a
// script, and a script is a file a person can read in review and a promise can
// name. So an unquoted metacharacter is refused by name rather than passed
// through as a literal argument nobody meant.
//
// Quoting is the small POSIX subset: single quotes are literal, double quotes
// allow \" and \\, and a backslash outside quotes escapes the next byte.
//
// The reading is frozen. `run:` lives under `gather:`, which _schema/promise.yaml
// names in its hash list, so a promise that parsed one way must parse that way
// forever — widening the grammar changes what ratified promises measure without
// changing their bytes. See docs/decision/0038.
func argv(line string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var started bool

	flush := func() {
		if started {
			out = append(out, cur.String())
			cur.Reset()
			started = false
		}
	}

	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		case c == '\'':
			started = true
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, errors.New("a single quote is never closed")
			}
			cur.WriteString(line[i+1 : i+1+end])
			i += end + 1
		case c == '"':
			started = true
			j := i + 1
			for ; j < len(line) && line[j] != '"'; j++ {
				if line[j] == '\\' && j+1 < len(line) {
					j++
				}
				cur.WriteByte(line[j])
			}
			if j >= len(line) {
				return nil, errors.New("a double quote is never closed")
			}
			i = j
		case c == '\\':
			started = true
			if i+1 < len(line) {
				i++
				cur.WriteByte(line[i])
			}
		case strings.IndexByte(shellMeta, c) >= 0:
			return nil, &Shell{Char: c}
		default:
			started = true
			cur.WriteByte(c)
		}
	}
	flush()

	if len(out) == 0 {
		return nil, errors.New("names no program")
	}
	return out, nil
}

// shellMeta is what an unquoted `run:` line may not contain. Each of these
// means something to a shell and nothing here, so passing one through as a
// literal argument would run something other than what was authored:
// `go build ./... && go vet ./...` would run go build with four arguments, exit
// zero, and report a promise kept that had never run go vet. A refusal is loud
// and a wrong measurement is not. See docs/decision/0038.
const shellMeta = "|&;<>()$`"

// Shell is a `run:` line asking for something only a shell does.
type Shell struct{ Char byte }

func (e *Shell) Error() string {
	return "run: contains " + string(e.Char) +
		", and there is no shell here — put it in a script the promise names, so a person can read it in review"
}
