# Build order

The ordering lesson from the nine: **get something that survives contact with
real work first, then make its record richer.** The systems with the best
models produced the least. So the corpus and the loop come before threads, and
threads come before anything clever.

Each increment declares a **usable end state** — what a person can actually do
when it lands, on a real repository or a real manuscript. An increment that
cannot name one is too big, or it is bookkeeping wearing a milestone's clothes.

## 1 — The corpus reads

`corpus`, `schema`. Parse `{type}/{slug}.md` with frontmatter, load
`_schema/*.yaml`, validate leniently, resolve the three link sources, detect
broken links. `kept show <artifact>`, `kept list`.

Identity is settled here or not at all: the slug is read from frontmatter,
duplicates are a load error, and the filename is not identity (0021). The YAML
parser is admitted and recorded per 0029 — and what it has to do is narrower
than it looks: unknown fields must survive a *read*, not a re-emission. kept
does not rewrite human-authored files.

**Usable end state.** You can keep a directory of promises and canon entries by
hand and have something read them back faithfully — every field, every link,
and every broken one. On a manuscript that is worth having on its own: it is a
linter for a corpus nothing else understands.

**Done when** it reads a hand-written corpus and prints it faithfully, unknown
fields intact and available, renaming a file changes nothing about what it
read, and two artifacts claiming one slug fail the load with both paths named.

## 2 — Standing

`ledger`, `register`, `ids`, `run`. Append-only ledger, content hashing over
the schema's `hash` list, `kept init`, `kept ratify`, `kept release`, and `kept
status` — the corpus-wide view, without which one artifact at a time is not a
workflow. Every record names the run that wrote it, carries its `seq` and its
envelope version, and the run writes its own diagnostics to
`ledger/runs/{id}.json`.

`kept init` lands here too, because 0022 asks it to write `* text=auto eol=lf`
and that is worth having as soon as anything is hashed. It is hygiene and not
the mechanism — the guarantee is inside the hash function, which is why the
CRLF test below runs deliberately without a `.gitattributes`. It writes only
what is missing: a config decides what is enforced and a text attribute decides
how git stores every file in the tree, so neither is rewritten once a person
has written it.

Hashing normalizes before it hashes and records the rules' version (0022), so a
CRLF checkout does not de-ratify the corpus. The ledger stays as plain files
under `.kept/ledger/` for now: it has to leave `main` before a box can contain
it (0016), and boxes arrive in increment 5. The record **shape** is one-way and
must be right here; where it is stored is mechanism and moves cheaply.

**Usable end state.** A ratification register. Write down what you have
committed to, ratify it, and find out — for the whole corpus at once — which of
those things have been edited since you approved them. On a manuscript that
answers *which canon have I changed without meaning to*; on a repository, the
same for documented invariants. No gatherer runs, so nothing is checked against
the world yet.

**Done when** editing a ratified file makes it report `unratified` on the next
read with no invalidation step anywhere in the code, `kept status` shows that
across the corpus, and the same corpus checked out with `core.autocrlf=true`
reports every promise still ratified.

## 3 — Truth

`gather`, `predicate`, `tick`. Two gatherers to start — `command` and one
structural one — plus the predicate evaluator and the tick. `kept`, `kept
drift`, `kept why`. Dispositions land here: the tick records why it did or did
not act on every drift it saw, because none of those reasons can be
reconstructed later (0019).

`--json` lands here, and deliberately not before. 0027 makes the structured
payload a compatibility surface from the first release, and what it has to
carry is the three-valued verdict — `kept`, `not-kept`, `unknown` — with the
reason nobody could measure. Freezing an envelope for `status` alone while
those values do not exist means versioning a shape that has to grow the moment
they arrive. The exit codes are the half of 0027 that could go early and did,
in increment 2; this is the half that could not.

**Blocked on open question 1.** The predicate language is the one component
with no precedent in the nine, and this cannot start until `prior()`, how far it
reaches, and the grammar are decided. Decide it against real promises about this
repository rather than in the abstract.

**Usable end state.** The thing on the front of the README. `kept` prints what
is kept, what is not, and what nobody could measure — and it is right, because
it just looked. Nothing repairs anything yet, and this is still where the tool
starts being worth running every day.

**Done when** a real promise about this repository goes from `unknown` to
`not-kept` to `kept` as the world changes, a missing toolchain yields `unknown`
rather than `not-kept`, `kept why` can say why a drift was left alone, and
`--json` carries the same three-valued answer with a schema version on it.

## 4 — The gatherer catalogue

Enough gatherers to hold a real set of promises about one repository and one
manuscript. Answers open question 8 by building it. Each declares the fields it
returns (0023), which is also what a third-party gatherer over a stdio protocol
would declare.

**Usable end state.** You can write the promises you actually have rather than
the ones two starter gatherers happen to support — without dropping to a
`command` escape hatch, and without teaching each promise to parse output.

**Done when** the promises in `lineage.md`'s prose example can all be written
without `command`.

## 5 — The box, and the agent in it

`box`, `thread`, `confine`. Cut a worktree, assemble the packet, dispatch an
agent into it under a named confinement provider with the canary passing first,
re-gather in the box, present the diff. **Nothing lands automatically** — you
read the diff and merge it yourself.

The ledger and the artifacts move to `refs/kept/ledger` here (0016, 0023),
because this is the increment where a worktree would otherwise contain them.
Capture is incremental from the first session (0020): a dispatch killed halfway
still leaves a cost, a partial transcript and a reason.

**Usable end state.** Drift you can hand to an agent with one command, and get
back a bounded diff you can read — produced in a jail that was verified before
the agent started, in a tree that cannot reach your working copy. Landing is
still your hands on `git merge`.

**Done when** a promise that is not kept is repaired by an agent, the re-gather
in the box is a real re-derivation rather than the agent's account, out-of-scope
paths are reported and not landed, the canary fails closed on a misconfigured
provider, and a dispatch killed halfway is still attributable.

## 6 — Landing

`land`. The merge queue over `origin/main` (0015): the gate outside the lock,
the cheap check inside it, the disjoint-scope skip, and the record of what each
land was gated against.

**Usable end state.** The loop closes without you. A repair that passes its gate
reaches `origin/main` on a human act — one keystroke rather than a merge — and
every commit that arrives there was gated against the tree it landed on, with
several threads in flight.

**Done when** two threads finishing at once both land correctly, the second
re-gates when `main` moved and skips the re-gate when the moved paths are
outside its scope, and a crash between the push and the record leaves the next
tick observing the truth rather than the ledger asserting a land that did not
happen.

## 7 — The assessor and the ranking

The thread assessor with its checklist recorded per check, the never-fired
signal, the act/wait/suppress/escalate policy (open question 3), and a budget
that refuses (open question 4).

**Usable end state.** Unattended operation you can defend. The queue drains
overnight inside a budget it will not exceed, a repair that gamed its predicate
is caught and says so, and every promise it left alone has a recorded reason.

**Done when** the throughline in `model-deck.html` runs for real: the assessor
catches an instrument substitution the predicate missed, and strengthening the
predicate makes the same failure mechanical.

## 8 — Presence

Whatever the operator surface turns out to need. Five of the nine grew one
independently and none was reusable, so build it last and build it for the
batch review, which is the only interaction that happens often.

**Usable end state.** The morning pass over a night's work takes minutes
instead of being avoided.
