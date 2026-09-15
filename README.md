# kept

A promise is kept, or it is not. `kept` records what you have committed to,
observes the world to find out whether it still holds, and drives an agent to
close the gap when it does not.

It works the same way over code and over prose, and over any language or stack.
The only thing that differs between them is what closes a gate.

The instruments say so in their own names. `corpus` and `command` work on any
tree; `go/tests`, `go/imports` and `go/symbols` need the Go toolchain and say
that in the name a promise binds, so a promise about a repository kept cannot
measure is a mistake you find when you write it rather than one you find later.
Groups for other stacks are how the catalogue grows.

```
$ kept
  core-packages-are-tested        not-kept
      2 of 24 packages have no test that runs
      internal/audit · internal/effect

  kael-eyes-are-green             kept        by text@1 at 09:14
  the-epub-builds                 kept        by build@1 at 09:14
  chapter-12-reads-well           unknown     last read 3 revisions ago
```

## The model

Three properties, three sources.

- **Content** is authored. Everything the artifact says.
- **Standing** descends from you. Created only by ratification, passed to work
  by delegation. No rule, no agent, and no ingested document acquires it.
- **Truth** ascends from the world. Acquired only by observing, derived when
  read, never stored on the artifact.

None of the three converts into another. Evidence does not grant permission,
and permission does not make anything so.

A **promise with standing and without truth is drift**. Work is what you do
about it.

## The nouns

One artifact type. The familiar names are combinations of which properties it
has and how it got them.

| Name | Standing | Trigger | Truth |
|---|---|---|---|
| Promise | ratified | a cadence | predicate over an observation |
| Policy | ratified | the moment of acting | predicate over an observation |
| Canon | ratified | a cadence, or none | predicate where one is possible |
| Work | delegated | once, then spent | predicate over an observation |
| Fact | — | — | inherited from provenance |
| Document | — | — | — |
| Desire | — | — | — |

Canon is ratified, not observed: if the text disagrees with canon, the text is
wrong. A fact is observed, not ratified: if the text disagrees with a fact, the
fact is stale.

## The acts

`ratify`, `land` and `retire` are yours alone. Landing may be delegated to a
policy with a deadline; ratifying and retiring may not.

`repair` writes, inside a box, confined. `assess` reads, and cannot write.
Gathering, evaluating, ranking and refusing are deterministic code.

A repaired change lands where your own work goes: `origin/main` where there is
an origin, the branch you are standing on where there is not, and either way a
target you can name. It arrives on that branch's ref, or — when the branch is
the one you are standing on and your tree is not clean — in the working tree
beside the edits you were already making; the act records which, because the two
are not worth the same. `kept` ticks once and exits — it has no scheduler and no
daemon, and it is run the way `make` is run.

## Where things live

| Store | Holds | Rebuildable |
|---|---|---|
| the corpus — git, on `main` | content: entities, links, attachments, schemas | no. This is the source |
| the ledger — append-only, on its own ref | standing, truth, observations, dispositions, threads, transcripts, cost | no. Nothing re-derives an act, and re-running an agent does not reproduce a session |

There is no third, derived store. Caches and boxes are thrown away, and the
index that was planned is not built: every verb walks the corpus and folds the
ledger, which is milliseconds at any size one operator writes.

The ledger is on a ref nothing checks out, so no repair worktree contains it.
The agent that may write the corpus never holds the bytes of the record that
says what it is allowed to do.

## Reading order

| | |
|---|---|
| [`docs/document/model.md`](docs/document/model.md) | the model as reference: properties, loop, gate, limits |
| [`docs/model-deck.html`](docs/model-deck.html) | the system end to end: what it is for, how the loop works, and one promise followed the whole way — including the repair that was wrong |
| [`docs/decision/`](docs/decision/) | why it is this way, and what was rejected |
| [`docs/lineage.md`](docs/lineage.md) | which of nine predecessor systems each part came from, the measurements behind it, and what was not adopted |
| [`docs/increments.md`](docs/increments.md) | build order, with what "done" means at each step |
| [`docs/open-questions.md`](docs/open-questions.md) | what is genuinely undecided |
| [`docs/layout.md`](docs/layout.md) | the corpus, the ledger, and what is derived |
| [`docs/self-hosting.md`](docs/self-hosting.md) | the path to kept holding promises about kept |
| [`AGENTS.md`](AGENTS.md) | invariants, the changes that will be tempting and are wrong, and how to name things and write commits |

## Status

The model is settled. The corpus reads: `kept list` and `kept show` parse a
hand-written corpus, validate it against `_schema/*.yaml`, and resolve links
from frontmatter, prose and `_links/` manifests. Either end of a link is an
artifact, a file in the workspace, or a URI, and every one with nothing at the
other end is reported — except a URI, whose reachability is a question for the
world and so belongs to a gatherer. An artifact's identity is the slug in its
frontmatter, so renaming a file changes nothing and two artifacts claiming one
id fail the load naming both. `kept new <type> <slug>` writes one from the
schema its type declares — the fields in their authored order, the headings its
body names — so a field added to a schema reaches an author with no code change.
It writes the file and nothing else: no record, no standing, and a required field
that binds arrives on `--set` rather than being scaffolded with a value nobody
meant.

Standing works. `kept ratify` grants standing to what an artifact says now,
`kept retire` ends it, and `kept status` reads the corpus at once. By default it
prints what needs a person — an artifact nobody ratified, one edited since it
was, one whose grant this build cannot compare against, and one that was
ratified and whose content has gone missing — and counts the rest, because a
corpus is mostly artifacts that are exactly as they were approved and a row for
each of them buries the few that are not. A ratified artifact and a retired one
are not findings; nor is one whose kind declares no hash list, since nothing
about it binds and nothing can ever ratify it, so it is counted in a line saying
so rather than listed forever. `kept status --all` is the whole register: what is
in force, what has been edited since, what has been retired, and what has gone
missing.

`kept status` exits 1 when something that was ratified no longer stands —
edited since, or its content gone — 3 when it could not tell, and 0 when
nothing is owed, so it is a hook or a step in CI rather than something to
parse. Nothing here is meant to be parsed: every table is sized from the data
in it and laid out for a person, and `--json` is the one shape a machine reads,
carrying the version it was written under. The tables may change between
releases; the document changes with a version. See decisions 0027 and 0066. `kept init` writes what a workspace needs: the config that says where
the corpus is, the directory it names, and the `.gitattributes` rule that keeps
the bytes from churning in diffs. It never rewrites a file you wrote.

Neither standing nor the answer is written to the artifact. A ratification names
a hash of the fields the schema declares binding, so an edit makes the record
stop matching and the standing stop applying — with nothing having to notice the
edit, and no invalidation step anywhere. Rewriting the prose of a promise costs
nothing; changing its predicate drops its standing. The hash normalizes before
it hashes, so a checkout with `core.autocrlf=true`, or on a machine that
disagrees about unicode form, does not de-ratify the corpus.

The ledger and its durable artifacts live on `refs/kept/ledger`, a ref no
working tree checks out, so a repair box cannot read or write standing, truth,
or evidence from a prior repair. `kept init` adds a fetch refspec that brings it
to `refs/kept/upstream/ledger` — the second ref git gives every branch — so an
ordinary `git fetch` never writes the durable record. `kept ledger sync`
advances the ledger onto what a fetch brought, and so does every verb that
writes. Nothing publishes it for you: `git push` does not carry a side ref, so
`kept init` also writes a `pre-push` hook that says at push time how far ahead
the ledger is and hands over the one command that publishes it. It warns, never
refuses, and never pushes anything itself.

Truth is built. `kept` with no verb is the tick: it gathers what is due, applies
each predicate, and says which promises are *kept*, which are not, and which
nobody could measure. `kept tick` is the same run under its own name, for a
script where a bare word reads as a mistake. `kept drift` and `kept why` answer from what it found and
observe nothing themselves, and `kept doctor` collects the checks about what
names an artifact — with `--fix` writing the frontmatter an adopted file is
missing, and nothing else.

Trialling is built. `kept shape <artifact> --trial` measures one artifact
against a tree, prints what each of its instruments said beside the verdict its
predicate reaches, and records nothing. It needs no standing, which is the point:
it answers before a ratification the question a ratification cannot be taken back
on — whether the check can fire at all. A predicate over a measurement of nothing
reads `kept` in exactly the same word as one over a measured world, and what
tells them apart is the observation printed beside it.

Capturing a want is one command. `kept want "<text>"` writes a desire holding an
id and the want and nothing else, under a slug made from its words, and refuses
a want whose words make a slug already taken by naming the file that has it. A
desire carries no state of its own: `kept status` and `kept why` derive whether
it is raw, shaped, committed or retired, and it is committed only when an
artifact somebody ratified says in its own links that it realizes the want —
writing a candidate promise commits nothing. `kept retire` declines a want and
keeps the file, so the next person to propose it finds the decline.

Repair is reachable. `kept repair <promise>` cuts a disposable worktree at the
commit you are on, measures the promise there, hands an agent a packet stating
the gap under a confinement provider whose canary passed first, re-derives the
promise from the tree the session left, and prints the diff with the paths its
repair scope does not cover named beside it. The verdict is that re-derivation
and never the agent's account of itself. `--dry-run` does everything up to the
dispatch, including the canary, and prints the packet instead of spending a
session on it. **Nothing lands**: the box is discarded and your tree is
untouched. What the session produced is committed onto the commit its box was
cut from and pointed at by a ref, so the change is anchored and reviewable
without anything having moved.

It repairs one promise, named. Choosing which of several to spend a session on
is a ranking and a budget, and neither is built, so with no name it lists what
it could be pointed at and leaves the choice to you.

The assessor judges the thread rather than the artifact, and it has two halves.
`kept review <thread>` works the derived half: an explicit checklist over one
repair's ledger entry — the packet it was given, the two measurements it was
judged by, the anchor's own diff — reporting each question on its own, because
which one fired is what a reviewer actually has. It is framed as an attack:
every check looks for a way a thread can pass every gate and still be wrong, and
the one it is built for is an instrument substitution, where the promise goes
green because the repair made the measurement smaller in a dimension the
predicate never reads. It measures nothing, records nothing, and runs against a
checkout you cannot write to.

The other half is an agent arguing, and it is deliberately not a list: it reads
everything the entry holds, the checklist's own findings included, and is asked
to make the case that the repair does not meet the thread's intent. Its findings
are unbounded, because the failure worth catching is the one nobody thought to
ask about and a checklist cannot reach that. It runs inside the repair, in a
box it cannot write, on a runtime and model you choose separately from the
repairer's — and what it says is kept whole in the thread, because nothing here
grades an argument. `kept review` reads it back.

A repair does not stop at one session. The pass ends with a ruling,
`ruling: reject` or `ruling: accept`, and that last line is the only part of what
it says kept reads. A reject, a derived check about the act, or a promise that still does
not hold sends the repair back for another attempt in the same thread: the next
box is cut at the work the last one left, and its packet carries every earlier
attempt's findings and the pass's argument verbatim. `kept repair` keeps going
until the episode is ready to land or hands itself to you — when the same check
fires twice, when the pass rules nothing it can read, or when a budget is spent.
The repository bounds one run at three attempts, `repair.attempts` in
`.kept/config.yaml`; `--attempts N` and your own config may only lower it, and
`repair.window_ceiling` in your own config refuses the next attempt once the
runtime reports a usage window at or above it. `--no-auto-rework` hands an
attempt that would be reworked to you instead, and `kept repair --rework
<thread>` is how you send one back yourself: the next attempt of that thread,
carrying everything so far, with a budget of its own. The ruling decides only
whether to try again: the verdict is still the re-derivation, and landing is
still yours. A run prints a line as each phase finishes and ends by saying what
to do next; `kept review <thread>` shows every attempt, and `kept status` and
`kept why` show an episode that is waiting on you.

A finding that keeps coming back is a predicate nobody has written. The answer
to one is usually a clause in `hold_when`, after which the re-derivation refuses
the same repair and the checklist has nothing left to say about it. One step
further out, an argument that keeps making the same point is a derivation nobody
has written.

Landing is built, and it is the only verb that reaches the network. `kept land
<thread>` gates one anchored repair outside a lock, takes the lock, asks whether
the target moved, and merges only then — so a land that raced another one
re-derives against what actually won rather than against what it read first. The
change lands where your own work goes: `origin/main` where there is an origin,
the branch you are standing on where there is not, and either way a target
`--onto` can name. Every landing records its target, its check regime and its
arrival, because a commit reaching a ref and a change arriving in a working tree
are not worth the same and `landed` must not mean both.

What is left is `agent` and `promote`, which are unwritten. Each says so, and
each says why rather than offering a spelling correction.

`kept help` prints the verbs and the flags they share. `kept version` prints the
build.
