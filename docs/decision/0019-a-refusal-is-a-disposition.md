---
id: 0019-a-refusal-is-a-disposition
type: decision
title: 0019 — A refusal is a disposition, and it is recorded
---

# 0019 — A refusal is a disposition, and it is recorded

**Status** accepted · completes [0009](0009-repairs-are-not-enumerated.md),
which made the act/wait/suppress/escalate policy deterministic and did not say
that its output is kept

## Context

The ledger records what happened. The tick spends most of its time deciding
what should *not* happen: drift exists, and no thread opens. That outcome has a
reason every time, and none of them are written down.

```
for each drift, ranked:
    disposition = policy(drift)
    if disposition == act and budget.allows():
        open a thread
```

Everything that falls out of that `if` vanishes. The reasons are enumerable and
each comes from a different part of the system:

| | |
|---|---|
| the policy said `wait`, `suppress` or `escalate` | 0009 |
| the budget refused | open question 4 |
| another run holds the lock | [0013](0013-the-gate-makes-concurrency-safe.md) |
| the view is stale, so acting on standing is refused | 0013 |
| it ranked below the cut for this tick | 0009 |
| no provider is configured, or the canary failed | [0017](0017-the-box-is-not-the-jail.md) |

The design's premise is a queue that drains without you. A queue is only
trustworthy if the negative case is answerable — *why did you not fix this* is
the question an operator actually has at nine in the morning, and today the
answer is a shrug. 0009 already made this explainable by construction: the
policy is compiled, not a model call, precisely because *a queue you cannot
explain is worse than a queue that is occasionally wrong*. The explanation
exists at decision time and is thrown away.

**It cannot be reconstructed later.** Truth can — observe again. A refusal
cannot: the budget's balance at that moment, the lock another run held then,
and the rank among that tick's drift are all gone. This is a one-way door of
the kind [0013](0013-the-gate-makes-concurrency-safe.md) enumerates.

## Decision

**Every tick records its disposition of every artifact that has standing and
lacks truth.**

```
disposition(promise:core-packages-are-tested, sha256:ab12…,
            refused, budget, "day limit reached: 4 of 4 dispatches", run, at)
```

A disposition is one of `act`, `wait`, `suppress`, `escalate`, `refused`, and a
refusal names the condition that produced it — one of the table above — plus a
human-facing detail drawn from that condition.

**It is not written to the act stream.** That stream is the spine: human
acts, low volume, and meaningless out of order, per 0011. Dispositions are
machine output at a rate three orders of magnitude higher, and mixing them
would drown the record a person actually reads. They follow the shape truth
already uses:

```
ledger/disposition/promise/{slug}.jsonl
```

One file per artifact, which is 0011's keying rule and gets the same property:
two runs disposing of different artifacts never touch the same file.

**Transitions, not ticks.** A drift refused for the same reason on four hundred
consecutive ticks is one record and a periodic heartbeat, exactly as truth
works. A *change* of disposition or of reason is a new record. Without this the
volume argument defeats the decision, and with it the stream is smaller than
the truth stream beside it.

**The word is `disposition`, not `decision`.** `decision` is the corpus's — an
artifact kind, authored by a person and read by one — and this is machine
output at three orders of magnitude the volume. A word that named both would
make `kept why` ambiguous about which it was answering, and the ambiguity would
fall on the verb that exists for the negative case.

The ledger's own act stream was called `decisions` when this was written, which
made the same word mean three things at once. That collision is gone —
[0039](0039-the-ledger-records-acts.md) moved the ledger to `acts` — and this
choice is unaffected: a disposition is not an act, it is what the tick decided
about one, and it is written by a machine rather than performed by a person.

### What it buys

`kept why <artifact>` answers the negative case, which is the question that verb
exists for. "Not repaired for six days, because the budget refused on four of
them and the policy suppressed on two" is a different conversation from silence.

It also makes the never-fired signal work in the other direction. A promise
whose disposition has been `suppress` since it was ratified is a promise nobody
ever intended to hold.

## Alternatives

**Log it.** Rejected: logs are derived, untracked, per-machine and rotated. The
question arrives after the log is gone, which is the same structural loss grft
recorded in its observability document.

**Derive it later from the inputs.** Rejected as impossible. The budget balance,
the lock holder and the rank are all momentary.

**Record only the refusals that turned out to matter.** Rejected: that judgment
can only be made afterwards, which is when the record no longer exists.

**Put them in the act stream with a `by: policy` field.** Rejected on volume.
The spine's value is that a person can read it.

## Consequences

- **A third per-artifact stream**, beside truth. Same shape, same merge
  behaviour, same fold rule, and it prunes on the same terms.
- **The tick writes even when it does nothing**, which is a change in character:
  a quiet system is no longer a silent one. This is the point.
- **Suppression becomes visible.** A suppressed promise currently looks
  identical to a healthy one in every view. It should not.
- **The disposition names the run that reached it**, so a fleet that refuses
  everything because one run holds a lock it never released is diagnosable
  rather than merely quiet.
