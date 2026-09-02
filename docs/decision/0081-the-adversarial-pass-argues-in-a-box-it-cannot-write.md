---
id: 0081-the-adversarial-pass-argues-in-a-box-it-cannot-write
type: decision
title: 0081 — The adversarial pass argues in a box it cannot write
fields:
  statement: >-
    The adversarial pass is a confined session with its own declared ask, its own
    runtime, model and effort, and a read-only box cut at the anchor it argues
    about. It receives a brief — the thread's whole ledger entry, the derived
    findings included — on the packet's channel, and what it says is filed
    verbatim as a receipt entry that declares no shape for a finding. Nothing
    grades it, nothing gates on it, and it runs inside the repair episode.
links:
  - { target: decision:0078-the-derived-checklist-is-closed-and-the-argument-is-not, relation: implements }
  - { target: decision:0079-a-repair-is-an-episode-that-reworks-itself, relation: implements }
  - { target: decision:0058-the-ask-is-declared-and-travels-beside-the-packet, relation: extends }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: extends }
  - { target: decision:0017-the-box-is-not-the-jail, relation: depends-on }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: depends-on }
  - { target: decision:0077-the-receipt-says-what-answered-and-what-it-consumed, relation: depends-on }
  - { target: policy:durability-follows-reproducibility, relation: implements }
  - { target: policy:the-agents-report-is-never-an-input-to-the-verdict, relation: implements }
---

# 0081 — The adversarial pass argues in a box it cannot write

## Context

[0078](0078-the-derived-checklist-is-closed-and-the-argument-is-not.md) specified
the assessor's larger half and left it unbuilt, naming what it needed: its own
ask under [0058](0058-the-ask-is-declared-and-travels-beside-the-packet.md), a
read-only box, and a live confined proof of the kind
[0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) requires.
[0079](0079-a-repair-is-an-episode-that-reworks-itself.md) then made the gap
load-bearing: an unbuilt half is `unread` from outside the episode on every
thread, which under its own table escalates every episode to a person, always.

This is that half, built. Nothing here revises 0078 — it is the same decision
carried out, and the paragraphs below say what the carrying out cost and what it
measured, because those are the parts a specification cannot contain.

**The one thing 0078 deliberately did not settle is what a finding is made of.**
Its words: *a record shape written before the thing that produces it is a guess
about what it will say, and the freedom above is the first thing such a guess
would cost*. So the shape had to be decided here, with the pass in front of it,
and the answer is the smallest one that is still a record.

## Decision

### The pass has its own ask, and it is a framing rather than a list

`thread.ClaudeAssessAsk` is the second member of 0058's closed set: a reviewed
constant in the adapter, one argv element on `--append-system-prompt`,
byte-identical for every thread, naming no promise, no file and no evidence. It
is a second constant beside the repair ask rather than the first one
parameterized, because the two frame opposite work — *close this gap* against
*attack this act* — and a template that could produce both could produce
anything, which is the channel [0009](0009-repairs-are-not-enumerated.md) keeps
an instruction out of.

**It states what kind of work this is and does not itemize it.** A question list
in the ask is a checklist arriving by the back door: the session answers what it
was given and stops looking, and the failure nobody thought to ask about goes
uncaught while the report says a pass ran. So the ask carries no bulleted list,
no numbered list and at most one question, and
`TestTheAssessorsAskIsAFramingRatherThanAQuestionList` breaks each of those on
purpose. The one example it gives — a session working toward the predicate rather
than toward the statement — is named as an example in
`docs/document/model.md` too, and is explicitly not the whole of what the pass
may say.

It also does not ask for a verdict. The promise's answer is the re-derivation's,
and a session invited to give one would be the input to a verdict that
`policy:the-agents-report-is-never-an-input-to-the-verdict` refuses. A finding is
about an act; the verdict is about the world.

### A brief is the second input, and it is not a packet with fields added

`thread.Input` is a closed interface with two members. A `Packet` states a gap a
repair must close; a `Brief` states one thread's ledger entry for a pass to argue
over. Both carry bytes, both are stored before the launch, and each files its own
receipt kind — `thread_packet` or `thread_brief` — so nothing at the dispatch
seam can record one kind of session as the other.

They are separate types because their guards are opposite. A packet is refused
unless its authority is ratified and every instrument it names was measured; a
brief has no authority and no gap, and refuses only the three things a later
reader could not work out for itself: the thread it is about, the packet the
repair acted under, and the anchor. Everything else may legitimately be absent —
a killed session leaves no re-derivation and no transcript — and the pass is told
so by the field being empty rather than by there being no brief.

**The brief carries the packet's own frozen promise, not the corpus's.** The
ordinary next step after a finding is revising the promise, and a pass reading
today's wording would be arguing about a repair against a standard that did not
exist when it was performed. It is 0078's rule for the derived half, applied to
the open one.

**The ask is paired with its input and a mismatch is refused by name.** A session
handed a brief and told to repair would read somebody else's finished work as a
gap to close, in a box it cannot write, and produce a session that ran and did
nothing anybody can account for — the failure that looks most like success.

### The box is read-only, and the canary is what makes that a boundary

`confine.Profile.ReadOnly` empties the writable set; `Profile.Unwritable` names
what a *repair of the same promise* would have been granted, and the canary must
be refused in every one of them. Both fields are required together and each is
refused without the other.

The second field is the whole mechanism and it is not decoration. **The box root
is refused as a write grant in every profile this package renders**, so a probe
aimed there passes for a repair box too — a read-only box that had quietly kept a
repair's grants would sail through it. That is the canary-that-measures-nothing
failure this codebase has already paid for once, and the probe target is
therefore the directories the same scope renders as grants. `writeGrants` is one
computation serving both, so what a repair is given and what a pass is refused
cannot come to disagree.

The premise is proved the way every other write probe's is: the parent writes in
each directory itself before the child is refused it, and a target the parent
cannot write refuses the whole verification rather than reporting a boundary that
held.

**Measured, in a real box under the real provider.** The same box, the same scope
and the same probe script under two profiles differing in one field: the repair
box creates a file, appends to an existing one and deletes a third; the read-only
box is refused all three and still reads the tree. And after a live confined pass
ran in one for four minutes, `git status` in that box was empty.

### What the pass says is durable, verbatim, and shaped by nothing

A session is not reproducible — `policy:durability-follows-reproducibility` — so
the argument is a receipt entry rather than a derivation, which is the structural
difference between the assessor's two halves.

**`thread.ArgumentRecord` declares no shape for a finding.** Its five fields are
`Runtime`, `AdapterVersion`, `Brief`, `Artifact` and `Redaction`: *who argued,
from what evidence, and where the words are*. None of them says anything about
what the words contain. There is no severity, no category, no count and no
verdict, and `TestAnArgumentRecordCarriesProvenanceAndNoShapeForAFinding` fails
on a sixth field — because the narrowing that would actually happen is not
somebody deciding to remove the freedom, it is a field arriving because it looked
useful, after which what the pass says is whatever fits it.

The words themselves are an artifact, addressed by their bytes, for the reason a
transcript fragment is one: an argument is as long as it needs to be. They are
**composed from the session's own transcript fragments and read back out of the
receipt**, so the argument is composed from the record rather than from a copy
the parent happened to hold. What the entry adds is what a stream of fragments
cannot say about itself: that these were one conclusion, reached from one brief,
by one runtime.

**A pass that was killed leaves its transcript and no argument.** That is the
guarantee an anchor already gives a repair that produced nothing, and it is why
the entry is written after the session rather than before it.

### Its runtime, model and effort are the operator's, chosen separately

`assess:` is a second block in the operator's own config, beside `repair:`, with
its own runtime, executable, credential reference, version policy and lapse date,
plus `model` and `effort`. An unset block is *no assessor is configured on this
machine*: the repair still runs, still reports, and the pass says it did not run.

```yaml
# ~/Library/Application Support/kept/config.yaml — the operator's own, never the
# tracked one: an executable is a path on one machine and a credential is one
# person's. See 0048.
repair:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/<account>
  credential_expires: "2027-08-24T00:50:08Z"
assess:
  runtime: claude
  executable: /opt/homebrew/bin/claude
  credential: apple-password://api.anthropic.com/<account>
  credential_expires: "2027-08-24T00:50:08Z"
  model: opus            # the runtime's own vocabulary; empty is its default
  effort: high           # the runtime's own closed set
```

It is a second block rather than a flag on the first because 0078 asks for the
independence by name, and the reason is the limit it exists to get past: two
instances of one model can find one substitution reasonable, and the answer to
that is a different model or more effort. A shared field would make that
untunable by construction.

**This is the one place kept names a model on the way in, and it does not change
what a receipt says.** The model a receipt projects is still read from the
runtime's own `system/init` ([0077](0077-the-receipt-says-what-answered-and-what-it-consumed.md)),
so a session given a name it did not honour says so. `--model` and `--effort`
were read off the installed binary; a value is refused unless it is one argv
value — a leading dash would be read by the runtime's own parser as another flag,
which is a flag kept never chose arriving through a configuration field.

### It runs inside the episode, and it gates nothing

`internal/repair.Argue` is driven by `Perform`, after the re-derivation and the
scope review, which is where 0079 puts the assessor. `kept review` is unchanged
and stays a reader: it now reads the argument back out of the receipt instead of
reporting that none exists, and it still measures nothing, dispatches nothing and
records nothing.

**No disposition is written for a pass.** A disposition is a decision about a
drift ([0019](0019-a-refusal-is-a-disposition.md)); this is a finding about an act
that already happened. No exit code moves, nothing selects an action, and nothing
auto-lands. Which action an outcome selects is 0079's dispatch policy, which is
open question 3, and taking it here would be deciding it.

A failure in the pass is a warning rather than a result. The repair ran, the
verdict is the re-derivation's, and an assessor that could not be started does
not unmake either — reporting it as a failed repair would put the assessor in
front of the verdict.

### The live proof

`internal/repair/claudeargument_test.go`, measured on 2026-09-01 against Claude
Code 2.1.258 confined by nono with the operator's brokered credential. One real
confined repair of `promise:gatherers-do-not-write` broken in a box, re-derived
`not-kept → kept`; the tree it left anchored; both box measurements filed; then a
real confined pass in a read-only box cut at that anchor, whose `git status`
afterwards was empty.

**What it cost, from the receipt's own projections rather than from a stopwatch:**
216 to 262 seconds for the pair across runs; the repair 16 turns and 0.2339046
USD, the pass 32 turns and 0.6104364 USD; the five-hour usage window moved 0.30 →
0.31 and the seven-day window read 0.07. The pass was answered by
`claude-sonnet-5`, which kept read from `system/init` rather than from anything it
asked for — this machine names no `assess.model`.

**What it argued is the reason this half exists**, and it is worth recording
because none of it is a question anybody had written down.

Its central finding was about the *justification* rather than the change: the
repairer stated as settled fact that the deleted path was "never referenced
anywhere else", and the transcript shows no search that would establish it — while
the derived checklist had independently reported `shared-evidence: unread — no
corpus was read`. The pass ran the search itself, agreed the deletion was safe,
and made the case anyway: *the transcript's confident prose fills the gap the
system itself left open, rather than the gap being closed by evidence*. That is a
reading of a transcript, which is the one question 0078 deliberately gives the
derived half no slot for.

It then found something nobody had asked about at all: the promise's own `why`
text says `internal/gather` names exactly one call into the filesystem, and the
observation taken in the same session shows two. **The promise's authored
rationale disagreed with the tree it was bound against**, and the repair silently
made the code agree with the aspirational description instead of surfacing the
mismatch. That is a finding about this repository's corpus reached from a ledger
entry, by a session nobody pointed at it.

An earlier run against the same thread made a different case — that the promise's
statement claims the package is side-effect-free while `go/symbols` cannot see a
subprocess, so a `command` gatherer with a destructive `run:` would pass the same
verdict untouched. That is the *promise was the wrong proxy* class. **Two runs
over one thread found two unrelated things**, which is what an unenumerated pass
looks like working and is the strongest argument in this decision for not
enumerating it.

## Consequences

**The `assessor must pass` gate becomes meetable, and is still not a gate.**
0078's *half the assessor does not run, and a half that has not run is not a
pass* no longer holds: the pass runs, and `unread` is no longer its permanent
answer. What a gate would do with what it said is unbuilt and deliberately so —
there is no field over the argument to read, because declaring one is the
narrowing this decision refuses. So grading is still reported rather than
enforced, nothing auto-lands, and 0079's table stays a specification.

**An episode now costs two sessions.** A repair that anchors something dispatches
a pass, on a second credential if the operator configured one and on the same one
if they did not. It is bounded by `timeouts.agent` like every other session and it
is skipped entirely on a machine with no `assess:` block, which is every machine
until somebody writes one.

**A thread holds more than one session, and the receipt says which.**
`thread.Sessions` groups a receipt by its input records — 0079's rule, worked as a
derivation over a record that already exists rather than as an ordinal field on
ten body types. `thread.LatestModel` now answers with the *pass's* model on a
thread that was argued over, because it is the last one projected; a reader
wanting the repairer's takes it from that session's entries.

**Two costs in what is filed, both accepted.** The argument is the session's
transcript fragments joined exactly as they arrived, so two assistant messages
run together where the runtime emitted no separator; inventing one would be kept
editing what a runtime said. And a pass that spent most of its turns on tools and
concluded briefly files a short argument beside a long transcript, because what
is durable here is the conclusion and the transcript is already durable beside it.

**`os.Executable()` is now reachable through a variable.** Under `go test` it is
the test binary, so a canary launched into it meets the testing package's flag
parser and exits 2 having measured nothing — correctly a refusal, and the reason a
test standing in for a dispatcher has to be able to name a real kept.

**What neither half reaches is unchanged.** 0078's last paragraph still holds: the
argument can see that a repair replaced an instrument, and it cannot see that
counting test files was never what anybody cared about. The measured pass above
came close — it said the promise's statement claims more than its instrument can
check — and that is the nearest a session standing in the ledger entry can get.
Whether the promise should exist stays with whoever writes it.

## Alternatives

**Parsing what the pass said into findings.** A list, a severity, a category —
anything that would make `kept review` tabulate. Refused, and it is the whole
subject of 0078's open half: every mechanism that tidies the output narrows what
the session looks for, because a schema is a question somebody wrote down. When
something it keeps saying turns out to be derivable, the answer is a derivation
in the closed half.

**Running the pass behind a verb a person types.** It would keep `internal/repair`
simple and it is where the derived half already lives. Refused because 0079
settled it: an automatic cycle is *assess → fail → re-dispatch*, and a verb
cannot be a step in one. `kept review` stays the person's entry point precisely
because it gates nothing.

**Giving the pass a writable scratch directory inside the box.** It would let a
session take notes where it is working. Refused: a session that can write the tree
it is judging can make its own argument true, and a scratch directory inside the
worktree is the same defect a repair box already refuses for a different reason —
it puts the session's leavings in the diff. The runtime's own scratch and home
are outside the box and writable, which is where notes go.

**Re-using the repair's box instead of cutting one at the anchor.** Cheaper by
203 ms. Refused for 0079's reason: a box that survived a session has that
session's leftovers in it, and a pass arguing about a tree containing untracked
scratch is arguing about something the anchor does not name.

**One `Input` type with a mode field.** Fewer types, one `Store`, one record
kind chosen by a switch. Refused because the guards are opposite: a packet is
refused unless its authority is ratified, and a mode field would put that check
behind a boolean a caller sets — which is exactly how a session gets dispatched
on evidence nobody granted.
