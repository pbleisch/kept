---
id: 0090-running-out-of-capacity-is-not-a-result-and-a-regression-is-not-anchored
type: decision
title: 0090 — A session the account cut off is not a result, and a rework never anchors over work that holds
fields:
  statement: >-
    A repair session or an adversarial pass that the runtime says ended because
    the account had no capacity left is not a result. Nothing it left is
    re-derived, anchored or argued over; its box is kept when it changed
    anything; and the episode waits rather than reworking or escalating, with a
    `wait` disposition naming the thread. `kept repair --rework <thread>` resumes
    it, refuses to dispatch before the reset the runtime named, and argues an
    attempt whose pass was cut off before opening another. A rework cut at an
    anchor that re-derives kept moves the thread's anchor only to work that also
    re-derives kept.
links:
  - { target: decision:0086-the-pass-rules-and-a-rejected-repair-is-reworked, relation: amends }
  - { target: decision:0079-a-repair-is-an-episode-that-reworks-itself, relation: amends }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: amends }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: amends }
  - { target: decision:0085-a-failed-repair-explains-itself-from-its-receipt, relation: extends }
  - { target: decision:0019-a-refusal-is-a-disposition, relation: depends-on }
  - { target: decision:0025-no-scheduler-cadence-is-change-driven, relation: depends-on }
  - { target: policy:record-only-what-has-already-happened, relation: implements }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0090 — A session the account cut off is not a result, and a rework never anchors over work that holds

## Context

On 2026-11-20, `kept repair promise:a-receipt-costs-the-ledger-about-what-it-holds`
ran from `main` at `5a29fcf` and escalated after three attempts, in thread
`7v2p52085pvwtdepzzhz34f3ze`. Its receipt is the evidence for everything here.

- **Attempt 2** was cut at attempt 1's anchor, ran for 50 minutes, and re-derived
  `not-kept → kept`. It anchored `0f983db`. Its pass ruled reject, so the episode
  reworked.
- **Attempt 3** was cut at `0f983db`. Its box measured the promise `kept`. After 41
  minutes the runtime closed the session with `is_error: true`,
  `api_error_status: 429` and the text *You've hit your session limit · resets
  12:40am*, exit 1. Its last window reading put `five_hour` at `1.01`. kept read
  that like any other exit: it re-derived the half-finished tree (`not-kept`),
  committed it as `cc1de97`, and moved `refs/kept/threads/7v2p…` to it. The land
  queue now names a tree that does not keep the promise, in place of one that did.
- **Attempt 3's pass** was refused at its first request with the same 429. That,
  and only that, escalated the episode, on 0086's row *the pass reached no
  argument*.

Two rules produced this, and neither was wrong when it was written.

**`thread.TerminalError` explains an outcome and never selects one.** 0086 and
AGENTS.md give the reason: a pass that argued for half an hour and then met a
limit on its last request still argued. That is true of the words. It is not true
of the tree a repairer leaves when the account refuses it partway, and nothing
separated the two. A half-done change was re-derived, anchored and offered to a
pass as though somebody had decided it was finished.

**An attempt's tree is always anchored.** 0067 anchors whatever a session
produced, repaired or not, and 0086 commits a rework's tree onto the episode's
base. Nothing compared the new tree with the anchor it replaced. With a single
attempt that comparison has nothing to compare against. In an episode it is the
difference between keeping the best work so far and keeping only the latest.

The captain's intent settles one more point: **running out of capacity is
accepted**. It is an expected way for a long episode to end, not a failure the
run should be built to avoid.

## Decision

### A session the account cut off is not a result

The runtime's closing event is read for one more thing. `thread.OutOfCapacity`
reports whether the runtime says the account refused the session, and what it
said. For Claude that is a closing `result` with `is_error` true and
`api_error_status` 429. The text is kept as evidence, and the status is the test.
The reset is the earliest `resets_at` of any window the session's last reading put
at or past its limit, and it is zero when none was. It is the runtime's reading,
never kept's guess. A runtime this build reads no such report for answers no.

**For a repair session:** the session ran and cost a credential, so its `act` is
recorded as before. Then the attempt stops. It is `Exhausted`. Its tree is not
re-derived, not anchored and not argued over. Its box is kept when the session
changed anything, because nothing else holds that work. This is the existing
rule for unanchored work, and the run names the box.

**For a pass:** whatever it wrote stays in the receipt as native events, and none
of it is filed as an argument. A filed argument is what every reader takes a
ruling from. A pass the account cut off never reached the last line it would
have ended on. The pass is `exhausted`, and the attempt it was over keeps its
anchor and its re-derivation, because those were results.

### The episode waits, and says what resumes it

Either kind of cut-off ends the run as **waiting**. It does not rework, because a
session started now meets the same refusal. It does not escalate, because nothing
about it is a person's to fix.

The record is a `wait` disposition, the outcome 0019 declared for *not yet* and
nothing had written. It names the thread. That **amends 0077 and 0086's rule**
that only an `act` or an `escalate` names one: a wait always follows a receipt
that exists, and whoever resumes it has to know which thread to resume. The
transition rule is unchanged, so the same wait on the same grounds is one record.

kept runs once and exits, and has no timer to come back on (0025). So the run
ends by naming the word that resumes it:

- `kept repair --rework <thread>` when the episode anchored anything;
- `kept repair <promise>` when it anchored nothing, because there is no work to
  resume from and the next attempt is a new episode's first.

A run that waits exits 1. A refusal to act on drift is drift, which is the reading
a budget refusal already gets.

### Resuming

`kept repair --rework <thread>` gains two steps, both taken before anything is
cut.

1. **The account is asked first.** If the thread's last session was cut off and
   the runtime named a reset that has not come, the run waits again and
   dispatches nothing. A reading with no reset, or one whose reset has passed, says
   nothing about now, and the next session is the only way to ask.
2. **An attempt whose pass was cut off is argued before anything is opened.** It
   is the last attempt, it is anchored, and it has no argument. Its repair is
   already paid for, and reworking it unread would spend a session to replace
   work a pass might have accepted. The resumed pass is read by 0086's table like
   any other. The run reports that attempt as the one it resumed, and it does not
   count it against the run's attempts budget, because this run did not open it.

### A rework never anchors over work that holds

A rework's box is cut at the anchor in force, and its own before-measurement is
that anchor re-derived. **When that reads `kept` and the attempt's re-derivation
reads anything else, the attempt's work is not anchored.** The thread's anchor,
its ref and the receipt's latest anchor all stay where the promise held. The box
is kept. No pass is dispatched: the anchor it would cut a box at has already been
argued about, and the next packet carries that argument.

The attempt's reading reworks, and the next attempt is cut at the anchor that
holds. When the budget is spent, the episode escalates with that anchor as the one
a land would take.

The rule is that narrow on purpose. `kept` is the only value an anchor is held
to. `unknown` is not ranked below `not-kept`, because a tree nothing could measure
and a tree that measured short are not points on one scale. A rework that moves
`not-kept` to `unknown` is anchored as before, and its reading says so.

**The derived checks judge the anchor in force by its own re-derivation.** 0086
says the checks use the episode's first before-measurement and its latest
after-measurement. An attempt that was not anchored measured a tree a land will
never take. So the after-side is now the one filed by the attempt that left the
anchor in force. For an attempt that changed nothing, that is the same tree.

## Consequences

**0086's row for a pass that reached no argument no longer covers a 429.** A pass
that was refused at launch, killed at its deadline, or wrote nothing still
escalates. A pass the account cut off waits.
`TestANoticeTheRuntimeWroteItselfIsNotTheArgumentAndTheEpisodeSaysSo` keeps the
first half of its claim: nothing is filed as an argument and the notice is
retained. Its second half moved from *escalates* to *waits*.

**`TerminalError` still decides nothing.** `OutOfCapacity` is the one reading of
a closing event that does, and it is a separate function so that the rule stays
easy to state.

**Status 429 is the whole test.** A 429 that was a burst limit rather than an
exhausted window also waits, and the person resumes at once. The reset gate
blocks a resume only when the runtime put a window at or past its limit. A
runtime whose vocabulary moves produces no reading, and kept falls back to
treating the session as it did before this decision. That is the same cost 0077
accepted for a window.

**Each cut-off or regressed attempt that changed anything leaves a box.** It is a
registered worktree. `kept doctor` reports it, and removing it is a person's
choice. The other choice is to discard the only copy of an hour's work.

**A regressed attempt reaches the next packet as a reading, not as a diff.** The
next session sees that attempt 3 re-derived `not-kept` and was not anchored. It
does not see what attempt 3 changed. Its box holds that, for a person.

**No live runtime has been run through the 429 path.** The stand-in in
`internal/repair/episode_test.go` emits the closing event and the window reading
in the shape thread `7v2p…` recorded, and it runs under the real provider.
`internal/thread`'s fixture is the real payload. The first real cut-off after this
decision is the live proof, and the account running out is not something a test
should spend.

## Alternatives

**Wait inside the run until the reset.** Rejected. It would be a timer in a CLI
that is meant to tick once and exit (0025). It would also hold a terminal, a lock
and a box for hours with no way to report what it was doing.

**Anchor the cut-off work and mark it.** Rejected. The ref is the land queue, and
a mark is one more thing every reader of the queue would have to know about. A
half-done change that is not anchored cannot be landed by mistake.

**Escalate on a 429.** Rejected. The captain accepted exhaustion as an expected
outcome. An escalation stays open until a person answers it, and there is nothing
here for a person to answer.

**Anchor a regressed attempt on a second ref.** Rejected. That would be a second
queue, and 0068 already reads an anchor as a pair. The box holds the work, and the
receipt holds the reading.
