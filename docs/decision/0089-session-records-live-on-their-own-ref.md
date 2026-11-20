---
id: 0089-session-records-live-on-their-own-ref
type: decision
title: 0089 — Session records live on their own ref, and the ledger is published without them
fields:
  statement: >-
    Every thread receipt and every session payload it names is filed on
    refs/kept/sessions, and refs/kept/ledger holds acts, truth, dispositions,
    observations and runs. The store decides which ref a record goes on by its
    path, so no caller chooses. A fetch from origin never brings the sessions ref,
    nothing kept prints publishes it to origin, and the pre-push hook refuses a
    push that would carry it anywhere except the one private remote a person
    named for it.
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: amends }
  - { target: decision:0020-durable-when-irreproducible, relation: amends }
  - { target: decision:0087-a-receipt-is-journaled-and-sealed-into-the-ledger, relation: amends }
  - { target: decision:0075-the-push-warns-that-the-ledger-is-unpublished, relation: amends }
  - { target: decision:0023-what-may-be-captured, relation: amends }
  - { target: decision:0050-doctor-reports-an-unchecked-instrument, relation: depends-on }
  - { target: decision:0062-a-fetch-writes-an-upstream-ref, relation: depends-on }
  - { target: decision:0067-a-repair-anchors-what-it-produced, relation: depends-on }
  - { target: policy:durability-follows-reproducibility, relation: implements }
---

# 0089 — Session records live on their own ref, and the ledger is published without them

## Context

kept is about to be published as a new public repository, and its ledger with
it. A clone that brought no ledger would read every promise as unratified and
every verdict as unknown, which `docs/layout.md` calls the wrong kind of safe, so
the ledger is published. Until now the ledger has also held every session kept
ran.

[0016](0016-the-ledger-lives-on-its-own-ref.md) put the ledger on
`refs/kept/ledger`. [0023](0023-what-may-be-captured.md) put session payloads on
the same ref, under `artifacts/`, so that no box could read what an earlier repair
saw. [0020](0020-durable-when-irreproducible.md) made transcripts durable and
tracked, because a session cannot be re-run. Every receipt line and every
transcript fragment is therefore a commit on the ref that `kept init` configures
every clone to fetch and that `ledger.PushCommand` publishes.

A receipt is mostly the runtime's own stream. Measured on one thread of this
repository's ledger, 3,816 of 3,997 receipt lines are Claude Code's `stream-json`
events, redacted for credentials and otherwise verbatim. They hold the box's
absolute path under the operator's home, every tool call with its input, every
tool result (which is file contents and command output), the operator's tool,
skill and slash-command lists, the subscription's rate-limit windows and reset
times, and cost. Transcript fragments are the assistant's prose. An argument is an
adversarial pass's verdict in its own words.

The cleaned-history build for the public repository showed two things. First,
**nothing outside the receipts' own readers needs them.** It rewrote the ledger
without `ledger/threads/` and `artifacts/`, and `kept status`, `doctor`, `why`
and `drift` gave the same output on both copies. Every artifact's standing (state,
current hash and granted hash) was identical on all 124. The only readers of a
receipt or a payload are `thread.ReadReceipt` and `ledger.ReadArtifact`, and their
only callers are `kept land`, `kept review`, `kept repair` (including rework and
the adversarial pass) and repair's failure report.

Second, **removing them once is not enough.** `Ledger.AppendThread` and
`PutArtifact` write through the same store as an act. So the first `kept repair`
in the public repository would write its transcript onto `refs/kept/ledger`, and
the next ledger push would publish it. Filtering each publication instead would
fork the published ledger from the operator's own, and kept refuses a forked
ledger at exit 4 ([0054](0054-a-ledger-that-went-backwards-is-a-refusal.md),
[0062](0062-a-fetch-writes-an-upstream-ref.md)). The operator still needs `kept
repair`, `review`, `land` and `--rework` on their own machine, and all four read
receipts.

## Decision

### Session records are filed on `refs/kept/sessions`

The sessions ref holds two kinds of record:

- every thread's receipt, at `threads/<thread>.jsonl`;
- every payload a receipt names, at `artifacts/<kind>/sha256/<ab>/<cdef…>`. That
  is `thread.packet`, `thread.brief`, `thread.argument`, `session.transcript`,
  `session.undecodable` and `session.stderr`.

`refs/kept/ledger` keeps `acts.jsonl`, `truth/`, `disposition/`, `observations/`
and `runs/`. What it says about a session stays on it: an `act` or `escalate`
disposition names its thread (0077), a `landed` act names the thread it landed,
and the two measurements a repair files are observations whose world names the
thread (0080). So the public record says *that* a session ran, what it cost the
promise, and what came of it. The sessions ref says what happened inside it.

The sessions ref is written the way the ledger is: a temporary index, a
compare-and-swap with 0064's retry schedule, and author `kept <kept@invalid>`. It
is never checked out, so 0016's box guarantee and 0023's are unchanged: no box
contains it, and `.kept/**` stays out of scope for every repair.

**The name is the guarantee, and three prefixes are refused.**

- Not `refs/kept/ledger…`. `FetchRefspec` is
  `+refs/kept/ledger*:refs/kept/upstream/ledger*`, and its `*` would match
  `refs/kept/ledger-sessions` or `refs/kept/ledgers`. Every clone would then fetch
  the sessions record from origin, which is the publication this decision exists
  to prevent, arriving from the other direction.
- Not `refs/kept/threads/…`. That is where anchors live (0067), and
  `ledger.AnchoredThreads` enumerates the prefix as the land queue. A sessions ref
  there would be listed as a thread called `sessions`.
- Not `refs/kept/upstream/…`. That namespace holds copies of what a remote
  holds (0062), never a record kept writes.

### The store decides, and no caller does

The ledger's store already maps the logical paths its callers name onto the
ref's tree: `sidePath` puts records under `ledger/` and payloads under
`artifacts/`, and the location is the store's decision alone. That mapping now
chooses the ref as well. A path whose first segment is `threads` or `artifacts`
is a session record and goes to `refs/kept/sessions`. Every other path goes to
`refs/kept/ledger`.

`AppendThread`, `PutArtifact`, `ReadReceipt` and `ReadArtifact` keep their
signatures, and so does every caller. One function decides, so a caller written
next year cannot put a receipt on the published ref by passing the wrong store.
That is the property the public repository depends on, and threading a second
store through every caller would put it at each call site instead.

The run record a receipt's `writer` names stays on the ledger, like every run's
diagnostics. It records argv, version and exit, and no session content.

A store kept in files, for a manuscript outside Git, keeps its single directory.
It has no ref to publish and no worktree to repair in.

### Origin never receives it

- **Fetch.** `FetchRefspec` does not match `refs/kept/sessions`, so an ordinary
  `git fetch` of origin never brings it into any clone. A test fetches from an
  origin that holds one and checks that it did not arrive.
- **Push.** `git push` does not carry a side ref, and `ledger.PushCommand` names
  `refs/kept/ledger` and nothing else.
- **The hook refuses one push.** `kept init`'s `pre-push` hook refuses any push
  whose ref list names `refs/kept/sessions`, on either side, unless the
  destination is the remote named in `kept.sessionsRemote`. It refuses that push
  to origin even when origin is the named remote. `git push --mirror origin` and
  `git push origin 'refs/kept/*:refs/kept/*'` are the pushes this catches.
  Deleting the sessions ref from a remote is never refused, because that is how
  a push that got through is undone.

  This **amends [0075](0075-the-push-warns-that-the-ledger-is-unpublished.md)**,
  whose hook exits 0 on every path. 0075's reason was that a warning that refuses
  holds a push hostage to the ledger. That reason still holds for the ledger
  warning, which still never refuses. A push that would publish transcripts is a
  different case: the refusal is the whole point, and it applies only to a push
  that names this one ref. `--no-verify` still skips it, as it skips every hook.

  kept's own hook is still never rewritten. A hook kept wrote before this
  decision carries 0075's marker and no sessions guard. `kept init` leaves it
  alone and says so, and `kept doctor` reports it with the two commands that
  replace it.

### A private remote is named by a person, and kept only reports on it

`kept init --sessions-remote <remote>` records the name in the repository's git
config as `kept.sessionsRemote`. That is local configuration: it is never on
`main`, never cloned and never published. It also adds
`+refs/kept/sessions:refs/kept/upstream/sessions` to that remote's fetch
refspecs. As 0075 measured for the ledger, that refspec is what makes a push
update the upstream copy, so `doctor` has a value to compare against without a
network call.

It refuses `origin`, and any remote whose URL is origin's. kept cannot tell a
private remote from a public one. What it can tell is which remote the ledger is
published to, because `PushCommand` names it, and that is the one remote a
session record must not reach. A repository whose origin is itself private (an
archive, for example) names the same URL under a second remote name, and the
operator then owns that choice in writing.

The push is `git push <remote> refs/kept/sessions:refs/kept/sessions`. It is
unforced, for the reason `PushCommand` is: the sessions ref only moves forward,
so an honest push is always a fast-forward. `doctor` hands it over, and kept never
runs it. kept has made no network call so far, and this decision does not add one
(0062, 0075).

### Fetch and sync leave it alone

No verb moves `refs/kept/sessions` from what a fetch brought. `kept ledger sync`
advances the ledger and nothing else, and a fetch of the sessions remote writes
only `refs/kept/upstream/sessions`. So the private remote is where this machine's
session record survives the machine. It is not yet a way for another machine to
read that record.

That is deliberate rather than unbuilt. Two machines that each run repairs fork
the sessions ref as a matter of course. A thread reworked on both machines has
two receipt files under one name, and what their union is has not been decided. A
fast-forward-only sync would refuse that ordinary case. Open question 16 holds it.

Rewind detection (0054) stays the ledger's. The sessions ref gets a reflog under
the same repository-wide `core.logAllRefUpdates: always`, so a rewound sessions
ref can be recovered, but no verb checks for one. Nothing outside kept writes the
sessions ref: no fetch refspec names it as a destination.

### `kept doctor` reports where the record lives

Each of these meets [0050](0050-doctor-reports-an-unchecked-instrument.md)'s
three conditions: nothing refuses the state, nothing else in kept reports it, and
what kept does about it is bounded by what `init` writes. None of them moves an
exit code.

- **Session records with no remote named.** The record exists only on this
  machine. The finding names `kept init --sessions-remote <remote>`.
- **A named remote that is not a remote here**, which no push could reach.
- **A named remote that is origin, or has origin's URL.** `init` refuses that, so
  this is git config written by hand.
- **A named remote with no refspec mapping its sessions ref**, so a push leaves
  nothing here to compare against. `kept init` adds it.
- **Session records the named remote did not have as of the last fetch or push**,
  or a remote nothing here has heard from about them. The finding names the push
  command. It is worded as 0062 words the ledger's outbound check, *as of the last
  fetch*, because a local ref supports nothing stronger.
- **kept's pre-push hook written before this decision.**

A repository with no sessions ref and no remote named is silent. Nothing has been
recorded, and nothing is owed.

### Records already on the ledger stay where they are

An existing repository holds receipts and payloads at the old locations, and
nothing moves them. Moving them would mean deleting records from the ledger's tip,
which an append-only record forbids, and it would not remove them from history, so
the next push would publish them anyway. A public ledger without them is made by
rewriting history before publication, which is outside kept. The cleaned-history
build that prompted this decision did that.

They are read where they are:

- A thread's receipt is its ledger stream, if one exists, followed by its
  sessions stream. `ReadReceipt` already folds by event time, writer and sequence,
  never by file order, so a thread opened before this decision and reworked after
  it reads as one receipt.
- A payload is read from the sessions ref, then from its old location.
- Writes go to the sessions ref only. `PutArtifact` checks whether its address
  is already held before it writes, and that check sees the old location. So a
  payload already filed there under the same address is not copied. A new receipt
  may name it, and it is read from where it is. In a repository that has those
  old locations, the old locations are still there.

### A clone without the sessions ref

A clone of the public repository has no sessions ref and no legacy receipts. Every
thread reads as having no receipt (`ErrNoStream`), and every reader already
refuses that by name. `kept review <thread>` says the thread has no receipt.
`kept land <thread>` says it anchored no commit. `kept repair --rework <thread>`
refuses for the same reason. Neither could have worked from a clone anyway,
because anchors live on `refs/kept/threads/*`, which no fetch brings.

`status`, `why`, `drift`, `list`, `show`, `doctor`, `tick` and `ledger sync` do
not change, because none of them reads a receipt.

### What this amends

- **0016.** The ledger ref no longer holds `artifacts/`. Session records live on a
  second ref with the same properties: never checked out, written with a
  compare-and-swap, and outside every box.
- **0020.** Transcripts stay durable and tracked: they are in git, on a ref, in
  the same object store as the commits they describe. They are no longer
  published with the ledger. The copy that survives the machine is the private
  remote a person names, and that is a configuration step 0020 did not need.
  0020's rejected alternative, *durably but untracked*, failed because the
  question gets asked on another machine. Until open question 16 is closed, that
  is partly true here too.
- **0023.** Its table's `refs/kept/ledger` for `.kept/artifacts/**` becomes
  `refs/kept/sessions`. Its reason, that an agent must not read an earlier
  repair's payloads, holds unchanged.
- **0087.** The journal is unchanged: it stays in the Git common directory, holds
  redacted entries and is sealed by the next writer. **The seal target becomes
  `refs/kept/sessions`.** A segment is a `thread.segment` artifact there, and the
  `thread_segment` index entry is appended to the receipt stream there. Because
  the store routes by path, a seal written as 0087 describes it lands on the
  sessions ref with no further change. The cost the two acceptance tests measure
  is now measured on the ref a receipt is written to, `refs/kept/sessions`, and
  neither test passes until the journal is built.
- **0075.** The hook refuses one push, described above.

## Alternatives

**Thread a second store through every reader and writer.** That is about thirty
call sites in `cmd/kept`, `internal/repair`, `internal/land`, `internal/assess`
and `internal/thread`, and several readers need both stores: `assess.Read` and an
episode's lineage read a receipt and the observations it names. Each site would
choose, and a site that chose wrong would put a transcript on the published ref
with nothing to report it. Routing by path puts that choice in one function, with
one test.

**A ref under `refs/kept/ledger`, such as `refs/kept/ledger-sessions`.** Rejected
for the reason above: the fetch refspec's pattern matches it, so every clone
fetches it from origin, and the ledger's outbound check and the hook would treat
it as the ledger.

**Filter each publication**, pushing a ledger with the session paths removed.
That forks the published copy from the operator's, and kept refuses a fork at
exit 4 by design. It would also need a rewrite on every push.

**Move existing receipts off the ledger with a commit.** It deletes records from
an append-only record's tip, and it leaves them in history, where the next push
publishes them. The public ledger has to be rewritten before it is published, and
after that this decision keeps it clean.

**A separate repository for session records.** 0016 rejected this for the ledger:
two clones to keep in step, and a hash in the record that names an object the
repository may not hold. A packet names the commit a box was cut from, and an
anchor's commit lives in this repository's object store. A second ref in the same
repository keeps that, and a second remote gives the record an off-machine copy.

**Push session records to origin under a ref GitHub hides.** A public repository
has no private refs. Anything pushed to it can be fetched by name.

**Have kept push the sessions ref.** Rejected for the reason 0062 and 0075 gave
about the ledger: it would be kept's first network call. A hook cannot tell a
real push from a dry run, so a hook that publishes really publishes during a dry
run.

**Make `kept ledger sync` fast-forward the sessions ref too.** That is the
smallest path to reading another machine's records. It refuses the ordinary
two-machine case, a fork, and it would be built before anyone has decided what a
union of one thread's two receipts is. Left as open question 16.

**Encrypt session payloads on the public ledger.** A key to manage and rotate,
ciphertext published forever under content addresses, and readers on the public
side who gain nothing. Rejected.

## Consequences

- **The published ledger carries no session content from here on.** That is
  acts, truth, dispositions, observations and runs. A test runs a repair against
  a real ref and checks that `refs/kept/ledger` holds no `threads/` and no
  `artifacts/` afterward.
- **The ledger stops growing with sessions.** 0087 measured 461 of 562 MiB of
  this repository's ledger as the receipts of eight sessions. That growth now lands
  on a ref that origin never receives and that nobody fetches.
- **A receipt line and an act no longer compete for one compare-and-swap.** A
  tick's appends and a running session's receipt are on different refs.
- **Session records are local until a person names a remote.** `doctor` says so
  for as long as it is true. That is a configuration step a new operator has to
  take, and 0020 did not ask for one.
- **A second machine cannot read the first machine's session records.** `kept
  review`, `land` and `--rework` of a thread repaired elsewhere refuse by name.
  This is open question 16.
- **The hook can now refuse a push.** It refuses only a push that names
  `refs/kept/sessions` to a destination other than the named remote. A branch
  push and a ledger push are never refused, and `--no-verify` skips the hook. A
  repository whose hook predates this decision has no guard until a person deletes
  the hook and runs `kept init` again, and `doctor` reports it until then.
- **The receipt-cost promise says "ledger".** The desire and the candidate
  promise behind 0087 bound what a receipt costs the ledger. Their two tests now
  measure the sessions ref, because that is where a receipt is written. Revising
  the promise's statement to say so changes a field that binds, which is a
  re-ratification and is not done here.
- **A sessions ref is not checked for rewinds.** Its reflog makes a rewind
  recoverable, and nothing detects one. The ledger's check (0054) is unchanged.
- **Records written before this decision stay on the ledger and are read there.**
  An existing repository keeps every thread it had, and history is not rewritten
  by kept.
