---
id: the-readme-names-every-verb
type: promise
title: The README names every verb
fields:
  statement: >-
    Every verb kept dispatches appears in README.md. The front door names the whole
    binary, or it describes a program that is not this one.
  gather:
    verbs:
      gatherer: command
      run: cat cmd/kept/main.go
      scope: ["cmd/kept/main.go"]
      fields:
        ok: exit == 0
        names: >-
          map(find_all(stdout, 'case "(?P<verb>[a-z]+)"'), m => m.verb)
    readme:
      gatherer: command
      run: cat README.md
      scope: ["README.md"]
      fields:
        ok: exit == 0
        mentions: >-
          map(find_all(stdout, 'kept (?P<verb>[a-z]+)'), m => m.verb)
  hold_when: >
    obs.verbs.ok and obs.readme.ok
    and count(obs.verbs.names, v => true) > 0
    and all(obs.verbs.names, v => v in obs.readme.mentions)
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["README.md"]
links:
  - { target: decision:0038-a-run-line-is-argv-and-there-is-no-shell, relation: cites }
  - { target: decision:0023-what-may-be-captured, relation: cites }
---

`docs/self-hosting.md` names *the deck's verb list matches `main.go`* in its own
"two things to watch", and gives the reason: a promise about the code is easier
to write than a promise about the documents, and the documents are where the
value is. This is that promise, aimed at the README rather than at the deck,
because the README is the file somebody reads first and the one that is wrong
longest before anybody notices.

**Standing over a document's truthfulness is real; standing over its existence
is not.** A promise that a file is present buys nothing — a document something
cites is already held by `every-citation-in-the-corpus-resolves`, and one
nothing cites has an id nobody would use. What is not otherwise checked anywhere
is whether the prose still describes the program. This is checkable today, with
no instrument that does not already exist, and it was `not-kept` the moment it
was written: `kept new`, `kept tick` and `kept land` — three real verbs, one of
them the subject of the week's work — appeared nowhere in `README.md`.

**The first two clauses are the promise, not a formality.** A `command` entry
reports a non-zero exit as data rather than as a failure, so `cat` on a file that
is not there prints nothing, exits 1, and hands the predicate an empty set — and
`all()` over an empty list is true. Read without `ok`, a missing `README.md` and
a missing `main.go` would both report this promise kept. `all()` over an empty
list is true for the second reason as well: a dispatch rewritten into a form the
pattern does not match measures no verbs and checks nothing. That is the exact shape of a check that cannot fire, and
this promise's own subject is a document going stale unnoticed, so it may not
have one. `obs.verbs.ok`, `obs.readme.ok` and
`count(obs.verbs.names, v => true) > 0` are the three guards, and together they
are the same job `obs.corpus.ok` does one promise over. `kept doctor` reports a
`command` entry whose fields never read `exit`, and it reported both of these
before they had one.

**It reads the dispatch rather than the usage text.** `usage()` is prose in the
same file and drifts the same way the README does; the `switch` is what decides
whether a verb exists, so it is what a reader of the README is owed a match with.
The cost is that any future `switch` over string literals in `cmd/kept/main.go`
widens the measured set and demands a README line for something that is not a
verb. That is a loud wrong answer rather than a quiet one, which is the direction
to be wrong in.

**It demands `help` and `version` too, deliberately.** Both are dispatched, so
both are verbs, and narrowing the pattern to exclude them would put an editorial
judgment about which verbs are interesting into a field that binds — where it
would go stale silently, and where changing it later costs a re-ratification.
Two lines in the README is the cheaper side of that trade.

**What a repair may change, and why one cannot run yet.** The bound is
`README.md` and nothing else: this promise is about prose, and a repair that
reached `cmd/kept/main.go` could satisfy the predicate by deleting a verb.
Measured on 2026-08-30, `kept repair` refuses to dispatch against that bound —
a repair scope naming a file at the workspace root renders as a write grant for
the box root, which the confinement provider refuses outright, so no session
starts. The bound is still the right one and is recorded here rather than
omitted: `repair` binds, so a promise that acquires one later pays a
re-ratification for it, and this one is correct the day the grant can be
expressed more finely than a directory.

See [[decision:0023-what-may-be-captured]] for why an
extractor is authored rather than defaulted, and
[[decision:0038-a-run-line-is-argv-and-there-is-no-shell]] for why `run:` is two words
and not a pipeline.
