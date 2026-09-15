---
id: 0086-the-pass-rules-and-a-rejected-repair-is-reworked
type: decision
title: 0086 — The adversarial pass rules, and a rejected repair is reworked in its thread
fields:
  statement: >-
    The adversarial pass ends with one closed ruling, accept or reject, and that
    line is the only part of what it says kept reads. A reject, a derived check
    about the act, or a promise still not kept sends the repair back for another
    attempt in the same thread. The next box is cut at the last anchor, and the
    rework packet carries every earlier attempt's findings and argument verbatim.
    One run of `kept repair` drives the episode until it is ready to land or has
    handed itself to a person, bounded by an attempts budget and a usage-window
    ceiling. The ruling decides only whether to try again. It is never an input
    to the verdict, it refuses no land, and it creates no standing.
links:
  - { target: decision:0079-a-repair-is-an-episode-that-reworks-itself, relation: implements }
  - { target: decision:0081-the-adversarial-pass-argues-in-a-box-it-cannot-write, relation: amends }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: amends }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: amends }
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: extends }
  - { target: decision:0058-the-ask-is-declared-and-travels-beside-the-packet, relation: extends }
  - { target: decision:0019-a-refusal-is-a-disposition, relation: depends-on }
  - { target: decision:0080-a-box-folds-its-own-lineage, relation: depends-on }
  - { target: decision:0068-an-anchor-is-a-base-and-a-change, relation: depends-on }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0086 — The adversarial pass rules, and a rejected repair is reworked in its thread

## Context

[0079](0079-a-repair-is-an-episode-that-reworks-itself.md) set the northstar: *a
person prepares artifacts and lands work, and does not sit in the rework cycle*.
It designed the loop and did not build it. `internal/repair/perform.go` has no
rework path. [0081](0081-the-adversarial-pass-argues-in-a-box-it-cannot-write.md)
built the adversarial pass and kept it out of the loop on purpose: *nothing grades
it, nothing gates on it*.

Two real threads show what that costs. Both repaired
`promise:each-clause-of-a-predicate-answers-on-its-own`.

- **Thread `159qnynw8m5fhtkxt6kch6s8fh`**, landed as `78a34650f`. It met its
  `hold_when`. Its derived checklist was clear on every check about the act. An
  Opus pass argued, in seven numbered findings, that the tests asserted less than
  their paragraphs said. The repair landed anyway.
- **Thread `19y86jbnzqjtye6xv1vnxj614s`**, anchored at `85cc990` and not landed.
  It met the strengthened `hold_when`, and its checklist was again clear on every
  check about the act. The pass found weak tests and a real design bug: each
  `Clauses()` call gets a fresh step budget, the counter is shared and never
  reset, `Reads` is not counted, and the counter is mutated without
  synchronization.

Both times the fix had to come from a session outside kept. 0079's table would
not have caught either one, because it selects actions from the derived
checklist alone, and the argument that found the faults selects nothing.

Reading both entries with `kept review` settles a second question that 0079's
table did not face. **`shared-evidence` fired on both threads, and it will fire
on every repair under `internal/` in this repository.** The reason is that nine
other promises measure `internal/**`. That finding is a question about the
repository, not about the act. No attempt can clear it without leaving the scope
the promise bound, and 0037's gate is what answers it when the change lands.
Read literally, 0079's table reworks on that finding and then escalates when it
fires a second time. Every episode under `internal/` would reach a person after
two attempts, whatever the repair did.

The third fact is what a cold start costs. Thread `19y86…` ran for 47 minutes.
About 25 of them went on rediscovering what thread `159q…` and its pass had
already established.

## Decision

### The pass ends with a ruling, and only that line is machine-read

The assessor's ask gains one paragraph. Once the pass has said what it found, it
ends with a single line that is exactly `ruling: reject` or exactly
`ruling: accept`. **Reject** means the case it made is one that another attempt
at this repair should answer. **Accept** means it could not make such a case.
The ask also says what the line does not do: it does not decide whether the
promise is kept, and it does not decide whether the work lands.

The vocabulary is **ruling**, not verdict. Verdict is the promise's word, and a
token spelled like one would put the sentence
`policy:the-agents-report-is-never-an-input-to-the-verdict` forbids one careless
refactor away.

**kept reads the last line that is not blank, and nothing else.** It trims
surrounding whitespace and any emphasis or code marks a markdown renderer would
draw around the line, because a runtime that bolded its last line wrote the same
token. Then it compares the line against the two exact strings.
Anything else is **no ruling**, and a no-ruling reading says why: either the words
ended without one, or the last line was not one of the two. Nothing scans for the
words anywhere else in the argument. A ruling in the middle of a paragraph is not
a ruling, and nothing is ever inferred from the tone. The argument stays verbatim
and unparsed, just as 0081 filed it. `thread.ReadRuling` is the one function that
reads the line.

**The ruling is derived on read, and no record carries it.** It is a pure
function of words that are already durable, which is the same test 0078 applies
to a derived finding. A stored copy would be a cache that could disagree with its
source. `thread.ArgumentRecord` therefore keeps its five fields, and the test that
fails on a sixth still means what it meant.

This amends 0081 for exactly one purpose. The pass still grades nothing: a reject
is not a severity, and nothing counts or categorizes what was argued. It still
gates no land, creates no standing and moves no verdict. What changes is that one
line of it may send a repair back for another attempt.

### What an attempt's reading selects

After an attempt has dispatched, re-derived, anchored and been argued over, its
reading selects exactly one action. The rows are checked in this order, and the
first row that applies wins:

| reading | action |
|---|---|
| a check about the act fired, and it also fired in an earlier attempt of this thread | **escalate** |
| a check about the act could not be worked, for a cause outside the episode | **escalate** |
| the pass argued and no ruling can be read from what it said | **escalate** |
| an assessor is configured and the pass did not reach an argument: refused, killed, or it wrote nothing | **escalate** |
| the promise reads not-kept or unknown in the tree the attempt left | **rework** |
| a check about the act fired for the first time | **rework** |
| the pass ruled reject | **rework** |
| none of the above: the promise holds, every check about the act is clear, and the pass accepted or no assessor is configured | **ready to land** |

**Escalation comes first.** Every escalating row describes something no rework can
fix. A second attempt dispatched past one of them spends a session and meets the
same condition.

**Four checks are about the act, and one is about the repository.** These four
select actions: `instrument-changed`, `already-satisfied`, `property-exercised` and
`paths-in-scope`. Each asks whether this change is a legitimate way to reach the
promise, and a different attempt could answer differently. `shared-evidence` asks
what else in the repository reads these paths. It is still worked, reported,
printed and put in front of the pass, and it selects nothing. The Context above
gives the measured reason. `assess.Check.OfTheAct` is the classification, and a
test breaks it in both directions.

**`unread` is split by cause, as 0079 split it.** A check about the act is unread
*from inside the episode* in two cases: the attempt left the promise unmeasurable
(`Undecided`), or the attempt changed nothing and the thread holds no anchor. In
both cases the repair itself is why the evidence is missing, and the attempt's own
row reworks it. Every other unread check is outside the episode and escalates. The
usual causes are a diff that could not be read, a packet the receipt does not
name, and an anchor that could not be written because nobody is named to author
it.

**An unreadable ruling escalates, and it is never read as an accept.** By 0079's
split this is outside the episode. The half of the assessor that failed is not the
repair, and no amount of rework makes a pass end with a line it did not write. The
same holds for a pass that was refused, killed at its deadline, or wrote nothing.
A machine that configured no assessor is a different state. Nobody set one up,
and an unset `assess:` block has never been an error. An episode there is judged
by its verdict and its derived checks alone.

**"The same finding twice" means two different things for the two halves.** For a
check about the act, it is 0079's rule: the check fired in an earlier attempt of
this thread and fired again, so the episode escalates. A finding that repeats is a
predicate nobody has written, and writing one is a person's work. For the pass,
kept cannot tell whether two rejects make the same case without reading the words,
and reading them is what this decision refuses. 0081 measured two passes over one
thread arguing two unrelated things. **So a reject reworks until the attempts
budget is spent**, and three attempts means three chances to answer an argument.
The pass is shown every earlier attempt's argument (below). A case that keeps
coming back therefore says so in words, and a person reads those words when the
budget escalates.

### The run is the episode, and a person's word opens one

`kept repair <promise>` opens a new episode and runs it. It dispatches, re-derives,
anchors, works the derived checks and argues. Then it reworks or stops. It returns
control only once the episode is **ready to land**, has **escalated**, or has
**stopped** because an attempt did not dispatch: the promise holds where the box
was cut, nothing could measure it, a dry run, or a refusal.

`kept repair --rework <thread>` runs the next attempt of an episode that already
exists. From there it goes on exactly as above. It is refused, and nothing is
recorded, in four cases: the thread holds no repair packet, it anchored nothing,
its promise no longer reads ratified, or its promise was re-ratified at a wording
the thread's packets do not carry. A revised promise is a new standard, and a
rework must not measure the old one in a box and call that the gap. Those
episodes are opened fresh with `kept repair <promise>`.

`--no-auto-rework` turns off rework without a person's word. A reading that would
rework escalates instead, and the reason names the `--rework` command that would
continue it. `--attempts N` bounds one run, as below.

### Two budgets, both checked before an attempt opens

**Attempts bound one run.** The repository sets the bound in `.kept/config.yaml`
as `repair.attempts`, and it is 3 when unset. An operator's own `repair.attempts`
and `--attempts N` may make it stricter and never looser. A flag above the bound
is refused with exit 2, naming the bound and the file it came from: silently
granting fewer attempts than a person asked for would satisfy an impossible
request with a different one. An operator value above the bound does not apply,
and the run prints the bound in force and where it came from.

The rule of "stricter only" follows who owns each number. How many unattended
sessions may change this repository in one go is the repository's policy,
reviewed where that policy is reviewed. A machine or a command line can spend less
of it and cannot spend more. This is the ceiling's rule below with the owners
reversed.

**The count is per run, not per thread.** The budget exists to bound what happens
between two words from a person, and `--rework` is such a word. 0079 scoped the
budget to "what a person named", and a `--rework` names again. A thread holding
three attempts can therefore be reworked once more by someone who asks. The
repeated-check rule still reads the whole thread, so a finding that keeps
repeating escalates after that single extra attempt.

**The usage-window ceiling is the operator's.** It lives in the operator's config
as `repair.window_ceiling`, a fraction. Before an attempt opens, kept reads the
latest window reading reported by this episode's repair sessions, meaning the
account the next attempt would spend. If any window reads at or above the
ceiling, the attempt is refused. **A runtime that reported no window is not a
window at zero.** `thread.LatestWindow`'s second value is the whole of that
distinction: the ceiling refuses nothing and the run says that nobody reported
one. A new episode has no reading of its own yet, so its first attempt is never
refused by the ceiling. A reading from another thread is not a reading of the
session about to start. A ratified dispatch policy may only be stricter than the
operator's ceiling. Nothing evaluates such a policy at dispatch yet, since that
evaluation is 0079's and unbuilt, so today the operator's value is the only
ceiling.

**At either limit, the episode escalates.** If the run had not opened any attempt,
the record is `refused` on condition `budget` instead, which is 0079's rule
unchanged.

### The next box continues the work, and the anchor still names the whole change

**A rework's box is cut at the last anchor.** That is 0079's choice, and the
measurement above is the reason to keep it. Cutting at the base and carrying the
diff as evidence spends the start of every attempt rebuilding a tree kept already
holds. Cutting at the anchor lets the session begin inside the work.

**The anchor is committed onto the episode's base, with the previous anchor as its
second parent.** 0067 committed the box's tree onto the commit the box was cut
from. Under rework that commit is the previous anchor, so the pair would name only
the rework's own edits. 0068's land merges the anchor's change against its
parent, and it would drop everything the earlier attempts did. So an attempt cut
at an anchor commits its tree with the base as the **first** parent and the
previous anchor as the second. `Anchor.Parent` stays the base, so the pair is the
whole episode's change, which is what a land takes and what a person reviews. The
second parent keeps every earlier anchor reachable from the one ref, so an
attempt's own diff can still be read after gc. An anchor has never been landed as
a commit: the landed commit's only parent is the target's tip, so none of this
ancestry reaches a branch. This amends 0067 for an attempt cut at an anchor.

**The derived checks judge the episode's whole act.** They use the episode's first
`before` measurement, which is the gap a person asked about, together with the
latest `after`, the latest anchor's diff against the base, and the authority in
the latest packet. The alternative is to judge each rework on its own edits. Then
the second attempt's `already-satisfied` fires because the first attempt already
made the promise hold, and a path the first attempt put out of scope is forgotten
by the second.

**A promise that already holds can still be reworked.** The first attempt of an
episode still refuses to dispatch on a promise that holds, because its packet
states a gap and there would be none. A rework packet states 0079's gap of a
second kind: the promise may hold, and the way it got there was not accepted.

### A rework packet carries the earlier attempts, and has an ask of its own

`PacketVersion` becomes 3, with a `rework` field. The field carries:

- the episode's base and the tip this box was cut at;
- the change so far, as touched paths and a diff bounded the way a brief's is,
  with any elision said out loud;
- **every earlier attempt**, each with its anchor, its re-derived value, every
  derived finding, the argument verbatim, and the ruling read from it;
- and, when the promise could not be measured in this box, what that measurement
  said.

In that last case the packet carries no observations. A box that measured nothing
has no measurement to carry, and the packet says why instead of borrowing one from
a tree the agent is not standing in. `thread.packetWhy` holds that to both
directions.

Everything in the field is evidence about acts that already happened. 0079
already argued that such a finding is in the same category as an observation.
`policy:the-pipeline-is-never-in-a-packet` is untouched, because the field names
no budget, no attempts left, no disposition and no reason an action was
selected. The session gets the findings and the argument, and deciding what they
mean is its work.

**The rework packet travels with a third ask**, `thread.ClaudeReworkAsk`. The
repair ask says to stop when the promise holds. A session reading that inside a
box where the promise already holds stops at once, which looks exactly like a
session that did its work. 0058 makes a second kind of work a second constant, and
this is the third. The ask names no promise, no file and no evidence. It says the
earlier work was not accepted for the reasons in the packet, and that an account
agreeing with the criticism while leaving the tree unchanged is a failed rework.
Pi and Codex have no rework ask, so both refuse a rework packet by name.

**The brief becomes version 2** and carries the earlier attempts in the same shape.
0079 says the pass reads the whole series. Whether an agent is converging on an
argument or circling it can only be seen from the arguments before this one.

### Dispositions, and where an escalation is read

**An attempt that opens a session writes `act`, every time.** The transition rule
in 0019 exists for a standing condition that a tick observes over and over. A
dispatched session is an event: it spent a credential, and the attempts budget
bounds how many there are. 0077's wiring treated a second `act` as the same
disposition as the first. That was harmless when a thread had one session, and
wrong once a thread holds several. `ledger.Disposition.Moved` now always counts an
`act` as moved.

**An escalation is the disposition of the attempt the episode declined to open.**
Each attempt therefore writes exactly one disposition, 0019's rule unchanged. An
`escalate` names the thread it hands over, as a field. It amends the rule that
only an `act` names a thread: an escalation always follows a receipt that exists,
and a person must know which thread to read, rework or land. A `refused` on
condition `budget` still names none.

**An escalation stays visible until it is answered.** `kept why` and `kept status`
read one from the disposition stream. It is answered by any of three later events:
an `act` on the same promise, which means somebody reworked or reopened it; a
`landed` act naming its thread; or a grant of the promise at a different wording,
which means somebody wrote the predicate. A tick's later `refused` on condition
`provider` does not answer an escalation, because a tick does not act on one. So
the escalation is derived on read, and no record of it being answered is written.

### The run says what each phase established, and what to do next

`internal/repair` emits a structured event as each phase finishes, and `kept repair`
prints one line for each on stdout while the run is still going. The events are:

- an attempt starting, and where its box was cut;
- the session ending;
- the re-derived value;
- the anchor;
- which checks about the act fired;
- the pass starting;
- the pass's ruling;
- the action selected, and why.

The lines are the same bytes whether stdout is a terminal or a pipe. The live
progress row on stderr is cleared before each line is written, so the two never
share a row.

The run ends with the next action. When the episode is ready, that is
`kept review <thread>` followed by `kept land <thread> --onto <target>`. When it
escalated, it is what the escalation asks for. `kept review <thread>` gains an
attempts section: every attempt, its box, its value before and after, its anchor,
the checks about the act that fired, its ruling, and what followed it.

## Consequences

**Both acceptance threads now read as rework.** Thread `159q…`'s pass argued a
case and ended without a ruling line, because it predates this decision. Read
today, that is **escalate**. The thread has landed, so the point is only that the
episode would not have read "ready". Thread `19y86…` has no ruling either. The
command a person types for it is `kept repair --rework 19y86jbnzqjtye6xv1vnxj614s`,
which opens attempt 2 in a box cut at `85cc990`. That attempt's packet carries the
pass's argument verbatim, and its own pass rules on the result.

**The rework ask and the ruling line have no confined live proof.** AGENTS.md says
admission is per adapter and per kind of work, and nothing here has been run
against a live runtime: the suite dispatches stand-ins under the real provider and
spends no credential. The first real `--rework` is that proof. Until someone has
read one, how reliably a live pass ends with an exact ruling line is unknown, and
so is whether a rework session acts rather than agreeing with its criticism. When
it does not, the loop escalates rather than quietly accepting, which is the failure
direction this design chose.

**An episode can spend several sessions before a person sees it.** Three attempts
is six sessions: three repairs and three passes. This is the trade 0079 accepted,
and the attempts budget is its bound. The phase lines exist so that a person
watching can tell attempt 2's session from attempt 1's pass.

**Earlier transcripts are not carried forward.** Each earlier attempt reaches the
next packet and the next brief as its findings and its argument, and the argument
already read that attempt's transcript. Every transcript stays in the thread for
whoever reads it. This is short of 0079's "the pass reads every session", and it
is short of it on purpose. A 47-minute session's transcript is larger than the
bound a brief carries, and a brief a runtime refuses at launch lets the pass read
nothing at all.

**The window ceiling only reads this episode.** A new episode's first attempt
starts whatever another thread reported an hour ago. The trade is that a reading is
only as fresh as the session that produced it.

**The anchor is now a merge commit once an episode reworks.** Nothing kept reads
takes the graph's word for the base: land and review both use `Anchor.Parent`
explicitly. A person running `git log` on the ref will see the attempts as a
chain, which is an accurate picture.

**The table is code, not a ratified dispatch policy.** 0079 put the choice of
action in a policy artifact evaluated at dispatch, and nothing evaluates one yet.
The captain decided what a reject does, and this decision records that rule. A
policy artifact standing between the rule and the code would add machinery 0079
left unbuilt and would change no answer. When that evaluation exists, a ratified
policy may only make the loop stricter: it may escalate earlier, or cap attempts
or windows lower. The table's order already puts every escalation first, so a
stricter rule has somewhere to go.

**The run escalates in more situations than 0079's table would.** An unreadable
ruling, a pass that could not run, and a check about the act that could not be
worked each hand the episode to a person before any rework is spent. Each of these
is a machine or an assessor problem, and a person is the only one who can fix it.

## Alternatives

**Parse the argument into findings, and rework on their count or severity.** 0078
and 0081 refused this, and the reason is unchanged: a schema is a question someone
wrote down. The ruling is one closed token that the pass chooses to write, not a
reading of what it wrote.

**Store the ruling in the argument record, or in an entry kind of its own.** This
was rejected because the ruling is derivable from durable bytes by one function. A
stored copy would be the cache 0001 refuses, and a sixth field on `ArgumentRecord`
is exactly the narrowing its test exists to catch.

**Treat a missing ruling as accept.** This is the silent accept the northstar
cannot survive. A pass that forgot its last line would land every repair it had
just argued against.

**Treat a missing ruling as reject.** This was rejected by 0079's split. The
failure is the assessor's, and reworking the repair does not fix it. The loop
would spend its whole budget and then escalate anyway.

**Let two consecutive rejects escalate.** This would make the budget of three
unreachable through rejects alone, and it assumes the two rejects made the same
case, which kept cannot know without reading them.

**Let `shared-evidence` select, as 0079's table reads.** Rejected on the
measurement in the Context: every episode under `internal/` would escalate after
two attempts, and the gate that answers the question already runs at land.

**Cut each rework's box at the base and carry the diff as evidence.** This would
give every attempt a clean tree whose own diff is the whole change. It was
rejected because of the cold-start measurement, and because the anchor's first
parent already gives the whole change without discarding the tree.

**Count attempts across the whole thread.** Then a person's `--rework` after an
escalation on budget would be refused, and the only re-entry the budget exists to
create would stay closed until someone edited a tracked file.

**Put rework behind its own verb.** `kept repair --rework` continues the same
act, a repair of one promise that a person named, and a second verb would be a
second name for it.
