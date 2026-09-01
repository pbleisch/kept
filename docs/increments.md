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
the schema's `hash` list, `kept init`, `kept ratify`, `kept retire`, and `kept
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
CRLF checkout does not de-ratify the corpus. The ledger and its artifacts now
live on `refs/kept/ledger`, before a box can exist (0016). The record **shape**
is one-way and must be right here; where it is stored is mechanism and moves
cheaply.

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

`gather`, `predicate`, `tick`. Two gatherers to start — `command` and `tests`
— plus the predicate evaluator and the tick. `kept`, `kept drift`, `kept why`.
Dispositions land here: the tick records why it did or did not act on every
drift it saw, because none of those reasons can be reconstructed later (0019).

`tests` is the structural one, and it is `tests` rather than `build` because
the promises in `self-hosting.md` are about this repository's own tests, and
because `go test -json` is a heterogeneous stream rather than a table — which
is what forced `has()` into the predicate language (0036).

`--json` lands here, and deliberately not before. 0027 makes the structured
payload a compatibility surface from the first release, and what it has to
carry is the three-valued verdict — `kept`, `not-kept`, `unknown` — with the
reason nobody could measure. Freezing an envelope for `status` alone while
those values do not exist means versioning a shape that has to grow the moment
they arrive. The exit codes are the half of 0027 that could go early and did,
in increment 2; this is the half that could not.

**Was blocked on open question 1, which [0036](decision/0036-the-predicate-language.md)
closes.** The predicate language is the one component with no precedent in the
nine, and it was decided the way this said to decide it — against real promises
about this repository, by building it. It is hand-written, three-valued, and it
is also `command`'s extractor. Build it first, and per that decision the tests
are the deliverable rather than the parser.

Three more of increment 3's shapes are settled and are one-way, so they are
worth reading before writing any of them: the scope digest and `World.Tree`
([0033](decision/0033-due-is-a-digest-over-the-scope.md)), what an observation
is addressed by ([0034](decision/0034-the-observation-record.md)), and where a
verdict names its world ([0035](decision/0035-a-verdict-names-its-world-in-the-record.md)).

Six more decisions were written alongside this increment and are part of what
it delivered: [0037](decision/0037-a-conflict-is-discovered-by-exhausted-repair.md)
on how `conflicted` is reached,
[0038](decision/0038-a-run-line-is-argv-and-there-is-no-shell.md) freezing what
a `run:` line means before the first one bound,
[0039](decision/0039-the-ledger-records-acts.md) renaming the ledger's stream
to `acts`, [0040](decision/0040-a-sectioned-body-and-its-names.md) on sectioned
bodies — which closes open question 5 —
[0041](decision/0041-a-decision-is-a-kind.md) making `decision` a kind, and
[0042](decision/0042-doctor-adopts-a-corpus.md) collecting the identity checks
under `kept doctor`. The last two are what let this repository's own ADRs
become artifacts: fifty decisions, thirteen policies and seven promises, all
ratified, which is the corpus `kept` now reads about itself — the policies are
`AGENTS.md`'s invariants that are checked before an act, each citing what it
rests on. What has **not** moved in is `model.md` and `lineage.md` as documents
— see [`self-hosting.md`](self-hosting.md).

**Usable end state.** The thing on the front of the README. `kept` prints what
is kept, what is not, and what nobody could measure — and it is right, because
it just looked. Nothing repairs anything yet, and this is still where the tool
starts being worth running every day.

**Done when** a real promise about this repository goes from `unknown` to
`not-kept` to `kept` as the world changes, a missing toolchain yields `unknown`
rather than `not-kept`, `kept why` can say why a drift was left alone, `--json`
carries the same three-valued answer with a schema version on it, and `kept
doctor` can adopt a directory of hand-written files into a corpus without a
person editing forty-two frontmatter blocks.

## 4 — The gatherer catalogue, and its boundary

Enough gatherers to hold the promises `self-hosting.md`'s structural table
names, and a settled answer to what is inside the catalogue and what is
outside it. That second half is open question 8, and it is the one-way part:
a gatherer's declared fields and its name both bind — the fields through the
observation's address (0034), the name through `gather`, which
`_schema/promise.yaml` hashes — so what a promise may name is a compatibility
surface before there is anything third-party to be compatible with.

Three gatherers, and the list is short because increment 3's `tests` did more
than this increment was planned around: the prose example in `lineage.md`, and
every promise naming a particular test, are writable with what shipped there.
`imports` reads the in-module package graph from source, so a boundary promise
is answerable on a tree that does not compile — which is the tree a repair
agent stands in. `symbols` is the same parse with a different query: which
packages name a given identifier, read off the syntax tree rather than out of
the bytes. `corpus` is kept reading a corpus root and reporting what is wrong
with it, so a broken citation becomes a verdict rather than a line in `kept
list` — and it reads the corpus and never the ledger, because data that moved
while the world stood still would break the dedup the content-addressed layout
exists for (0034).

`symbols` rather than a regexp is what makes two of those promises worth
having. A scan over bytes is a *pattern* instrument in `model.md`'s autonomy
table, so it sets an assessor gate and cannot auto-land, which means it is not
trustworthy until increment 7 — and here it is also provably the wrong
instrument. *The core carries no domain vocabulary* is true of the code and
false of the bytes: `model`, `predicate`, `ledger` and `tick` mention `tests`
or `command` seventeen times, every one of them in a doc comment, and the ones
that say `"tests@2"` say it on purpose. Reading identifiers rather than lines
moves that
promise and *gatherers do not write* from pattern to structural, where they
auto-land inside their scope.

The ones that are not built are not built deliberately, and answering open
question 8 means saying why for each. Two are worth knowing here: `lint` is
`command` plus an extractor, and a native one would be a per-linter output
parser, which is precisely the accretion 0036 exists to stop; and `fold` may
never need to exist at all, because 0036 put the fold in the predicate
language, where `all`, `count`, `filter` and `map` already do it over any
gatherer's structure.

**Usable end state.** The invariants in `AGENTS.md` that erode silently — the
ones no test would catch, because nothing is broken until somebody deletes a
boundary — stop being prose an agent was asked to read and start being
promises with verdicts. That is the first time `kept` holds this repository to
something a reviewer would otherwise have to notice.

**Done when** deleting the `internal/corpus` → `internal/ledger` boundary
makes a promise go `not-kept` within one tick, *and* a tree that does not
compile makes the same promise go `unknown` rather than `not-kept`. The second
half is the one worth building for: a boundary check that reads a broken build
as a satisfied boundary is the hollow promise `lineage.md` records, one level
up.

## 5 — The box, and the agent in it

`box`, `thread`, `confine`. Cut a worktree, assemble the packet, dispatch an
agent into it under a named confinement provider with the canary passing first,
re-gather in the box, present the diff. **Nothing lands automatically** — you
read the diff and merge it yourself.

[0052](decision/0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)
proposes the runtime boundary for this increment: direct adapters for exactly
Claude, Codex, and Pi; an immutable packet; and incremental native receipts.
Each adapter remains contingent on evidence of its documented invocation,
native event mapping, and interrupted-session capture before implementation
begins.

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

`land`. The merge queue over a target (0015): the gate outside the lock, the
cheap check inside it under the regime the target's sharedness decides, an
unconditional re-gate when the check says the target moved, and the record of
what each land was gated against — its target, its check regime and its
arrival. 0015 used to state a skip for a re-gate whose intervening land touched
paths outside the promise's scope; it is withdrawn there as unsound after 0037,
and there is nothing here to build.

The target is a parameter and `origin/main` is its default rather than its
definition, so this increment is also the one that makes kept usable on a
machine with no remote: a local branch is a target, the branch a person is
standing on is a target, and a change can arrive in the working tree that holds
it rather than only on a ref. An anchor is read as a base and a change (0068),
which is what lets a repair start from a tree that is not clean.

**Usable end state.** The loop closes without you, on a laptop as well as on a
fleet. A repair that passes its gate arrives where your own work goes on a human
act — one keystroke rather than a merge — and every commit that reaches a target
was gated against the tree it landed on, with several threads in flight. A
repair started from a dirty tree arrives beside the edits it was measured
alongside, and the act says which of the two guarantees it carries.

**Done when** two threads finishing at once both land correctly, the second
re-gates when the target moved, a crash between the write and the record leaves
the next tick observing the truth rather than the ledger asserting a land that
did not happen, and a `landed` act with no target, regime or arrival is refused
rather than written.

Each of those four is a test, and each drives its interleaving rather than
timing it: `TestTwoThreadsFinishingAtOnceBothLand`,
`TestTheSecondThreadReGatesAgainstWhatTheFirstLanded`,
`TestACrashBetweenTheWorktreeWriteAndTheRecordLosesTheRecordAndNotTheChange`
beside the ref arrival's `TestALandRunAgainAfterACrashRecordsWhatIsAlreadyOnTheTarget`,
and `TestALandActWithNoTargetRegimeOrArrivalIsRefusedRatherThanWritten`. The
first clause is about ref arrivals, which is the pair the queue exists for; two
worktree arrivals into one tree cannot both land at once and are not meant to,
because the second collides with the tree rather than with the ref and 0033's
digest refuses it. `TestTwoWorktreeArrivalsIntoOneTreeLeaveTheSecondTornRatherThanWritten`
is that, including the re-run that then lands it.

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

**The assessor's derived half is built** — `kept review <thread>`,
`internal/assess`, and
[0078](decision/0078-the-derived-checklist-is-closed-and-the-argument-is-not.md).
It works five questions over one thread's ledger entry and records a finding per
question, three-valued, each derived from evidence a repair already made
durable. A repair now files both of its box measurements, which is what a
comparison needs. Both halves of the "done when" run in the suite over a real
receipt, and the second of them now runs where it has to: a box names the thread
it belongs to and folds that thread's own earlier measurements, so strengthening
the predicate is mechanical in the box as well as on the tick and the repair is
recorded `Unrepaired` rather than `Undecided`. See
[0080](decision/0080-a-box-folds-its-own-lineage.md).

**The adversarial argument is not built, and it is the larger half.** 0078
commits kept to a model reading the whole entry — the derived findings included
— asked to argue the repair does not meet the thread's intent, with findings
that are unbounded and unslotted rather than answers to a list. That is where
the failure nobody thought to ask about gets caught, and a checklist cannot
reach it. It needs its own ask under
[0058](decision/0058-the-ask-is-declared-and-travels-beside-the-packet.md), a
read-only box and a live confined proof.

What is left in this increment is that pass, the never-fired signal, open
question 3 and open question 4.

## 8 — Presence

Whatever the operator surface turns out to need. Five of the nine grew one
independently and none was reusable, so build it last and build it for the
batch review, which is the only interaction that happens often.

**Usable end state.** The morning pass over a night's work takes minutes
instead of being avoided.

### Ledger staging

After a tick, a person needs one inspectable answer to *what belongs in the
ledger commit*. The surface computes the closure of changed ledger records: a
truth record brings along each observation it names in `from`, while run records
are identified as optional diagnostics because a durable record's `writer` may
outlive them. It never infers that a cache, a box, or ordinary working-tree
change belongs in that commit.

**Usable end state.** A person can review and stage a complete shareable ledger
change without reading JSON or reconstructing references by hand.

**Done when** the surface deterministically reports the required ledger files,
flags optional run diagnostics separately, detects an untracked observation
named by a truth transition, and has a test for each of those distinctions.
