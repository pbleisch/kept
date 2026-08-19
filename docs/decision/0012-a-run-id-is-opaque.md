---
id: 0012-a-run-id-is-opaque
type: decision
title: 0012 — A run is identified by minted bits, not by where it ran
links:
  - { target: decision:0011-tracked-formats-must-merge, relation: amends }
---

# 0012 — A run is identified by minted bits, not by where it ran

## Context

Decision 0011 removed the need for writers to coordinate: every stream is keyed
by something naturally unique, so no two processes ever write the same path.
That left attribution unanswered. A truth transition should say which process
reached it, and a thread record should say which process opened it, so a verdict
can be traced to the run that produced it.

The first sketch was to compose that identity from what the process already
knows — host, pid, version. It is convenient to read and it is wrong twice.

**It is not unique.** Pids are recycled within a boot, and two runs of the same
version on the same host that land on the same pid are indistinguishable. The
window is small and the failure is silent: two runs' records merge into one
apparent run, and nothing reports it.

**It fixes what should stay soft.** Everything encoded in an identifier becomes
parseable, and everything parseable acquires readers. A hostname would then be
in a tracked repository for as long as the record lives, unfixable and
unremovable, and adding a field would mean changing the identity format.

## Decision

**A run is one execution of `kept`, from process start to exit, identified by a
`run.ID`: 128 random bits rendered as 26 characters of Crockford base32.**

The value is opaque. It is not sortable by time, does not name a machine, and
nothing may parse it. Two ids may be compared for equality; a command line may
match an unambiguous prefix, the way git matches a sha.

Ordering does not need to come from the id. Records carry their own event time,
and 0011 already requires every derived read to fold by event time then id — so
a time-sortable id would buy a property nothing uses, at the cost of a
timestamp everyone would eventually parse.

**Anything a run names follows the same rule.** A thread id is minted, not
counted: two runs opening a thread in the same second would both call it `t_9b2`
and their records would merge into one apparent thread — silently, since nothing
compares them. `internal/ids` holds the minting; `run.ID` and any later id type
are distinct named types over it.

**Diagnostics are a record, not an id.** One file per run at
`.kept/ledger/runs/{id}.json`, holding host, pid, version, argv, start, exit and
counters. One writer per path, so it needs no merge strategy — the same keying
rule as 0011.

**The record is diagnostic; the reference is durable.** Run records are pruned
by retention while the transitions naming them remain. A `writer` that resolves
to nothing means the diagnostics aged out — it does not put the record naming it
in doubt. Host and pid are omitted entirely when `diagnostics.host: false`, which
a shared repository may want; the id and the attribution survive that.

## Not a session

A `kept` run is not called a session, and the word is reserved. In every
predecessor — grft's `agent.session`, pt's `session start`, chopworks'
`AgentSession`, backlog's tmux — a session is one run of a *model* against a
context window. `kept` invokes those inside threads and records them, so both
meanings would be live in one system at once.

## What is not resumable

A run is not resumable, and pausing one is not a feature. A tick is cheap and
idempotent; a run that dies mid-tick is replaced by running again, which
re-derives from the corpus and the ledger. There is no run state to recover.

**A thread is resumable**, and that is where pause, resume and inspect belong.
It has a box on disk, an agent that ran, artifacts recorded, and possibly a gate
awaiting a person — and it already has an identity independent of any process,
so a thread opened by one run is resumed by another with nothing handed over.
Run ids appear in its record as the history of which processes touched it.

That holds for another run reaching the same box. The box is derived and local,
so a thread does not follow the ledger to a workspace that cannot see it; see
[0013](0013-the-gate-makes-concurrency-safe.md).

## Consequences

- `ids.Mint()` panics if `crypto/rand` fails. Every durable write depends on
  distinctness and there is no weaker id worth continuing with.
- Ids are 26 characters, so console output and tables abbreviate to a prefix
  (`t_3k9wq2m1`). Records always hold the full value.
- Reading "what did run X do" means either its counters or a ledger scan for
  records naming it. Accepted: it is a debugging question, not a hot path.
- The verb that opened a thread was `kept run <artifact>`. Renamed to
  `kept repair <artifact>`, so *run* is the noun and nothing else. The verb
  reads better for it: what it starts is a repair, and whether that takes one
  process or several is not the caller's business.
