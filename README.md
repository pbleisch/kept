# kept

A promise is kept, or it is not. `kept` records what you have committed to,
observes the world to find out whether it still holds, and drives an agent to
close the gap when it does not.

It works the same way over code and over prose. The only thing that differs
between the two is what closes a gate.

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
| the ledger — append-only, on its own ref | standing, truth, observations, dispositions, threads, transcripts, cost | no. Nothing re-derives a decision, and re-running an agent does not reproduce a session |
| the index — SQLite | search, the link graph, drift queries | yes, from the other two |

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
hand-written corpus, validate it against `_schema/*.yaml`, resolve links from
frontmatter, prose and `_links/` manifests, and report every broken one. An
artifact's identity is the slug in its frontmatter, so renaming a file changes
nothing and two artifacts claiming one id fail the load naming both.

Standing and truth are not built, so nothing yet says whether a promise is
kept. The verbs that would — `ratify`, `status`, `drift`, `why` — exit 2.
