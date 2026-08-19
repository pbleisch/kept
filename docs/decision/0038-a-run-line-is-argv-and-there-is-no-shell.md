---
id: 0038-a-run-line-is-argv-and-there-is-no-shell
type: decision
title: 0038 — A `run:` line is argv, and there is no shell
---

# 0038 — A `run:` line is argv, and there is no shell

**Status** accepted · settles the one thing
[0023](0023-what-may-be-captured.md) and
[0036](0036-the-predicate-language.md) leave unsaid about `command` — what
happens to the string before anything runs · reaches the same permanence
[0036](0036-the-predicate-language.md) claims for `hold_when`, by the same route

## Context

0023 gave `command` four layers against what it may capture, and 0036 gave it an
extractor written in the predicate language. Both are about the program's
output. Neither says what the program *is*.

Every ADR that mentions `command` writes the same shape — `run: "go test -json
./..."` — and every one of them is a string that a shell would accept and that
does not need one. That is why the question never got asked: the examples are
all in the intersection.

It has to be asked before the first `command` gathers, and the answer is
permanent for exactly 0036's reason. `run:` lives under `gather:`, and
`_schema/promise.yaml` names `gather` in its `hash` list, so the line binds. A
promise ratified while `run:` meant one thing and read later as meaning another
is not a promise anyone can rely on — and unlike an edit, nothing detects it,
because the bytes did not change. The reading is as permanent as the syntax of
`hold_when`, and it is being fixed here deliberately rather than by whatever the
first implementation happened to do.

## Decision

**`run:` is split into argv by kept, and executed directly. There is no shell,
ever, and an unquoted shell metacharacter is refused by name.**

The grammar is the small POSIX subset and nothing more: whitespace separates
words, single quotes are literal, double quotes allow `\"` and `\\`, and a
backslash outside quotes escapes the next byte. Everything else is a literal
byte of an argument.

`| & ; < > ( ) $` and a backtick, **unquoted**, are refused with a message
naming the character and saying what to do instead. Quoted, they are ordinary
data: `grep 'a|b' file` is a legal line and means what it says.

### Why not a shell

**It would make `run:` an injection surface that nothing else in kept is.** A
promise is reviewed as text, and a reviewer reading `run: "go test ./..."`
reasons about one program. Under a shell the same field is an arbitrary program
in a language with word splitting, globbing and expansion, and `$HOME` in a
`run:` line means something different on two machines — which is a `World` that
is not the tree, arriving through a door 0033 closed everywhere else.

**It would make the gather depend on what is installed.** `/bin/sh` is dash on
one machine and bash on another, and the two disagree about enough to change
what a ratified promise measures. That is the same class of split 0033 rejected
a git tree oid for: one tree producing two identities depending on what happens
to be present.

**It would hide the process group.** [0026](0026-nothing-external-is-unbounded.md)
requires a killed gatherer to be killed with its children. A shell between kept
and the program is a parent that dies while the pipeline it started keeps
running — a build that will corrupt the next tick's output, reported as a clean
timeout.

**And what it buys is a script.** Pipes, redirection and `&&` are a file, and a
file is a thing a person reads in review, a thing git tracks, and a thing a
promise can name in its scope so that changing it makes the promise due. The
escape hatch does not need an escape hatch.

### An absent scope is the whole workspace

`scope:` is one of `command`'s args, and a promise that runs `go build ./...`
legitimately names none. Read literally that is an empty glob list, and an empty
list matches nothing — which 0033 makes a *distinct outcome* that reads as
always due and is surfaced as a mis-stated scope. So an absent scope resolves to
`**`, and its digest is the tree's. A promise that named no scope did not
mis-state one; it depends on everything.

## Alternatives

**Run it through `sh -c`.** The obvious one, and the reason to write this down.
Rejected on all four grounds above. It is also the choice that cannot be taken
back: every promise authored under it would carry a line that only a shell can
read.

**Refuse a metacharacter always, quoted or not.** Rejected. `grep 'a|b'` is a
legitimate argument, and refusing it would push authors toward a script for a
reason that is not about shells at all.

**Pass the metacharacter through as a literal argument.** Rejected, and it is
the quiet failure this prevents: `go build ./... && go vet ./...` would run
`go build` with four arguments, exit zero, and report a promise kept that had
never run `go vet`. A refusal is loud; a wrong measurement is not.

**Take `run:` as a list of strings.** Unambiguous, no grammar to freeze, and
rejected because every ADR that mentions `command` writes it as a string. A list
is additive later if the quoting proves painful — a list and a string can both
be accepted, and only the string's reading is frozen here.

## Consequences

- **The reading of `run:` is frozen.** A promise that parsed one way parses that
  way forever, because it binds. Widening the grammar — adding a metacharacter's
  meaning, changing what a backslash does — is not an edit; it changes what
  ratified promises measure without changing their bytes.
- **A promise that wants a pipeline names a script**, and the script is in the
  world, so a scope covering it makes the promise due when it changes. That is
  strictly more of what kept is for than a shell line would be.
- **The refusal is a load error, not an `unknown`.** It is decidable without the
  world, so `Catalogue.Check` reports it when the corpus loads, alongside a
  missing extractor and an arg nothing accepts.
- **Windows is not addressed**, because nothing else here is yet. Direct
  execution needs no shell there either; what differs is the process group, and
  that is stated in the code rather than pretended away.
