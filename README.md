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

`ratify`, `land` and `release` are yours alone. Landing may be delegated to a
policy with a deadline; ratifying and releasing may not.

`repair` writes, inside a box, confined. `assess` reads, and cannot write.
Gathering, evaluating, ranking and refusing are deterministic code.

A repaired change lands on `main`, and `main` lands on `origin`. `kept` ticks
once and exits — it has no scheduler and no daemon, and it is run the way
`make` is run.

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
| [`docs/model.md`](docs/model.md) | the model as reference: properties, loop, gate, limits |
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
id fail the load naming both.

Standing works. `kept ratify` grants standing to what an artifact says now,
`kept release` retires it, and `kept status` shows the whole corpus at once:
what is in force, what was approved and has been edited since, what has been
retired, and what was ratified and whose content has gone missing.

`kept status` exits 1 when something that was ratified no longer stands —
edited since, or its content gone — 3 when it could not tell, and 0 when
nothing is owed, so it is a hook or a step in CI rather than something to
parse. `kept init` writes what a workspace needs: the config that says where
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
writes.

Truth is built. `kept` with no verb is the tick: it gathers what is due, applies
each predicate, and says which promises are *kept*, which are not, and which
nobody could measure. `kept drift` and `kept why` answer from what it found and
observe nothing themselves, and `kept doctor` collects the checks about what
names an artifact — with `--fix` writing the frontmatter an adopted file is
missing, and nothing else.

Repair is reachable. `kept repair <promise>` cuts a disposable worktree at the
commit you are on, measures the promise there, hands an agent a packet stating
the gap under a confinement provider whose canary passed first, re-derives the
promise from the tree the session left, and prints the diff with the paths its
repair scope does not cover named beside it. The verdict is that re-derivation
and never the agent's account of itself. `--dry-run` does everything up to the
dispatch, including the canary, and prints the packet instead of spending a
session on it. **Nothing lands**: the box is discarded, your tree is untouched,
and the merge is your hands on git.

It repairs one promise, named. Choosing which of several to spend a session on
is a ranking and a budget, and neither is built, so with no name it lists what
it could be pointed at and leaves the choice to you.

What is left is `agent`, `land` and `promote`, which are unwritten. Each says
so.
