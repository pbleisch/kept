---
id: 0077-the-receipt-says-what-answered-and-what-it-consumed
type: decision
title: 0077 — The receipt says what answered and what it consumed
fields:
  statement: >-
    A receipt projects which model the runtime says answered and what the
    runtime reads of the usage windows it is spending, each from the runtime's
    own event and each missing rather than wrong when it cannot be read; and a
    disposition names the session an act opened, as a field rather than as prose.
links:
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0019-a-refusal-is-a-disposition, relation: extends }
---

# 0077 — The receipt says what answered and what it consumed

## Context

One runtime is reachable, one model answers, and one repair has run: thread
`6ca1kqeyfz205a2s4qf4cb991a`, 2026-08-29, `$0.1472082` in 18.4 seconds. Its
receipt already holds both of the facts a reader most needs, and projects
neither.

**Which model answered** is in the `system/init` event's `model` field, and
again in `result.modelUsage`. **What the session consumed of the scarce thing**
is in two `rate_limit_event` entries whose `unifiedWindows.five_hour.utilization`
moved 0.24 to 0.25, with `seven_day` at 0.11. Both are inside `native.payload`,
which [0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) defines as evidence rather than
as a queryable projection.

Today that costs nothing, because there is one runtime and one model. It stops
costing nothing at the moment there are two: the corpus then mixes evidence of
different strengths and the only way to unmix it is a regex over a vendor's
JSON.

The window matters for a second reason, which is that the scarce resource is not
money. A five-hour window is shared across every machine and every session on an
account, including a person's interactive ones, and it **refuses** rather than
bills. It is the counter [open question 4](../open-questions.md) says a budget
needs, and it is a better one than that question imagines: it is the runtime's
own reading of a shared counter, so it needs no lock, no decrement, and cannot
drift out of step with what is actually being spent.

And the record that says a session happened at all — the `act` disposition —
names it only inside a human-facing sentence. `thread 6ca1…: a confined claude
session ran in a box under nono` is the right sentence for a person and the
wrong one for a reader, which must take the id back out of it with a regex. The
`landed` act already does this correctly, in `world.thread`.

## Decision

**A `model` projection, beside `cost` and `turn`, carrying its `native_ref`.**
It holds what the runtime said answered and never what kept asked for. Those are
different claims and for Claude they disagree by construction — kept names no
model on the way in — so a field named for what answered that sometimes held
what was requested would collapse the two. What kept selected is a separate
record and is not built here.

**A `window` projection, carrying every window the runtime reported, its
utilization as the runtime's own decimal, and its reset.** Utilization is
retained as text for the reason `cost` retains an amount as text: kept converts
nothing it did not measure. It is not bounded above, because a window in overage
reads above one and refusing that would turn a true reading into a missing one.

**Both are missing rather than wrong.** `rate_limit_event` is one runtime's
native vocabulary and not a contract. A build that renames it, restructures it,
or reports a window whose utilization is no longer a number projects **nothing**,
and the native event is still retained, so a reader who knows the new shape has
the bytes. A window is read in full or not at all: a partial reading is a
receipt that answers *what is left* with one window silently dropped, which is
worse than one that answers nothing.

**The projection is optional per adapter, and *no window reported* is a state
the receipt can be in.** It is said by holding no window entry, and never by an
entry holding no window — which is why an empty reading is refused at the write.
`thread.LatestWindow` returns that distinction as its second value, and a caller
that drops it has made the mistake this shape exists to prevent: a self-hosted
runtime reports no window at all, and a budget reading that as a window at zero
would refuse nothing on the day it should refuse everything. Nothing has
answered is not an answer.

**A disposition carries the thread an act opened, and only an act carries one.**
It is a join and not a copy: everything about the session is in the receipt, and
a runtime or a model repeated on the disposition is a cache that can disagree
with its source, which is what [0001](0001-standing-and-truth-live-in-the-ledger.md)
and [0032](0032-standing-is-a-fold-over-acts.md) refuse for standing. The
other outcomes name none because none of them has one to name: a refusal that
reaches the runtime is refused before a packet is stored, so its id would send a
reader to a stream holding nothing.

**That rule is checked at the write and never at the read.** Requiring a thread
on the way back in would quarantine every act disposition written before the
field existed — this repository's own among them — and an act taken out of the
fold by a refactor is an append-only record revised after the fact. A missing
thread is silence: nobody recorded one, which is not the same as no session
having run. The transition rule is untouched, so the thread is not compared any
more than the detail is.

## Consequences

**A budget has its counter and is still not built.** What it counts, what it is
scoped to, and what happens at the limit stay open question 4 and increment 7's.
This decision delivers the reading and names the dependency.

**A second runtime that reports no window is not a runtime with an empty one**,
and every reader from here has to say which it got. That is a cost: one more
three-valued answer in a system that already has several. It is the same cost
`unknown` pays one level up, and for the same reason.

**Two projections are one runtime's vocabulary.** `system/init` and
`rate_limit_event` are Claude's names, and an adapter that grows a window of its
own will report it under its own names. The projection carries the runtime's
name for each window rather than a normalized one, because a normalization
across runtimes is a conversion nobody has evidence for yet — the same position
`cost` takes on usage.

**The model is corroborated and projected once.** `result.modelUsage` names it a
second time; projecting from both would put two answers to one question in the
receipt, and the init is the one that exists whether or not the session reached
a result. A session that reported two would be a finding, and the receipt is
append-only, so it can hold both and say so.

## Alternatives

**A `selection` entry recording what kept chose, written in the same pass.**
Deferred rather than rejected. It is a different claim from what answered, it is
composed by the call that admits a runtime — [0072](0072-a-runtime-version-constraint-is-a-review-record.md)'s
rule — and it has nothing to say while kept selects nothing: today it would
record *no model requested* on every session. It belongs with the second runtime
that makes selection real.

**Copying the runtime and model onto the disposition.** Rejected. The join
already exists and a copy is a cache that can disagree with its source. `kept
why` should derive the session line by following the id, the way every other
kept read derives.

**Utilization as a number rather than as text.** Rejected for consistency with
`cost` and because the literal is what the runtime wrote. A reader that needs
arithmetic parses once, at the point it needs it, rather than every writer
converting and the receipt holding a value nobody measured.

**Reading the window from `rate_limit_info.rateLimitType` and the top-level
`resetsAt`.** Rejected: those name one window, the one that happened to be
reported on, and `unifiedWindows` names all of them. A projection that carried
whichever window the runtime chose to foreground would answer a different
question on different days.
