---
id: 0015-landing-is-a-merge-queue
type: decision
title: 0015 — Landing is a merge queue over a target, and a land records how it arrived
fields:
  statement: >-
    A land names a target — origin/main is a default and not a definition — gates
    outside a lock and checks inside one under a regime that follows whether the
    target has writers the lock cannot exclude; the change arrives either on the
    target's ref or in the working tree that holds it, and the act records the
    target, the regime and the arrival, because the two arrivals do not carry the
    same guarantee.
links:
  - { target: decision:0011-tracked-formats-must-merge, relation: extends }
  - { target: decision:0014-an-observation-names-its-world, relation: depends-on }
  - { target: decision:0033-due-is-a-digest-over-the-scope, relation: depends-on }
  - { target: decision:0063-an-act-names-the-world-it-was-performed-in, relation: extends }
---

# 0015 — Landing is a merge queue over a target, and a land records how it arrived

## Context

Landing has a destination, and it was never written down: **a repaired change
belongs where the operator's own work goes.** Anything short of that is a change
sitting in a place they have to go and collect, which is the failure mode the
design exists to avoid.

The first sketch — 0011's consequences table, and open question 9 — said several
concurrent lands "need an order", implying git needs help to push safely. It
does not. Ref updates are atomic and locked: two pushes to one ref, one wins and
the other is rejected non-fast-forward; two pushes to different refs both
succeed. Nothing is lost or corrupted, ever. That framing was wrong and this
decision replaces it.

What is missing is not atomicity but **isolation**. Landing is a
read-modify-write over the world:

```
gather again, on the target     ← read
land                            ← write
```

Git makes the write atomic. Nothing makes the pair serializable, and two
distinct failures follow.

**The clean merge is the dangerous one.** Two threads touch disjoint files. Git
merges them without complaint, both land, nothing is rejected and nothing
retries. Thread A gated against the target at X, thread B gated against the
target at X, and the tree that now exists at Z was gated by nobody. The
combination can break a promise neither gate evaluated, and because the merge
was clean, nothing tells either thread to re-check. The invariant that the gate
re-derives from the world holds — against a world that stopped existing between
the gather and the write.

**The rejected merge is a cost problem, not a safety one.** On rejection the
retry is reset, **re-gather**, write — and re-gathering is the expensive part,
often a test run. Under contention the loser can re-gather and lose repeatedly.
Git guarantees that someone progresses, not that any particular thread does.

### The first wording decided one deployment out of four axes

That much still stands. What did not is everything around it. The original
decided exactly one case — a **shared** target, checked out in **no worktree**,
reached by a **locked push**, from a box **cut from that target** — and wrote
three of those four as *deployment constraints* rather than as one case among
several. Outside them kept does not land at all.

*Outside them* is where a person works. On their own machine, with no remote at
all. On the branch they are standing on, which is the branch they want the
repair on. With a tree that is not clean, because a tree in the middle of a
day's work never is.

**A tool that can only land on a shared remote is a CI mechanism**, and that is
not what this is. The loop is meant to close for one person on one laptop before
it closes for a fleet, and the fleet's arrangement is the special case rather
than the general one.

**"Commit your work before kept can help you" is a bar nothing else in the model
asks for.** [0033](0033-due-is-a-digest-over-the-scope.md) made `World.Tree` a
content digest over the workspace root — *the same digest in a repository,
outside one, and inside a box* — computed from bytes rather than from git, and
deliberately not a tree oid. Everything downstream of a measurement already
works on a dirty tree: the digest differs, so the observation is a different
observation, addressed differently, and the verdict names the world it was
reached in. `git rev-parse HEAD:<path>` was rejected in 0033 in as many words
because it *cannot see an unstaged edit*, and the working tree was named there
as the case that had to be answered. **The box is the only thing in the loop
that needs a commit**, because a box is a worktree and a worktree is checked out
at one. That is a property of git's tooling, not of this model, and it is
[0068](0068-an-anchor-is-a-base-and-a-change.md)'s to solve rather than a reason
to hold a person to a commit.

### The invariant claimed more than it delivered, twice

As first written the invariant was *every commit on `origin/main` was gated
against the tree it landed on*, which reads as a statement about the tree. It
was not: the gate re-derived only the promise its thread was about, so a repair
could satisfy A, break B, and land clean.
[0037](0037-a-conflict-is-discovered-by-exhausted-repair.md) closed that by
widening the gate to every standing artifact whose scope the diff's paths touch.
The wording below is the one 0037 supplies, generalized past `origin/main`, and
it is stated once here rather than in two places that can drift apart.

The second over-claim is the one this revision is about, and it is worse because
it is not a wording problem. **Two different things are about to be called
`landed`.** A change that reaches a shared ref is gated, locked, compare-and-swapped
and recorded against the tree it landed on. A change written into the operator's
working tree is none of those in the same sense: it touches files they were
already editing, nothing becomes a commit, and the gate is a statement about
their tree at one moment rather than an invariant about a place. Under one verb
the weaker case borrows the stronger case's name unless something stops it, and
in git that something is the reflog and the commit graph. Here it has to be the
record.

## Decision

**A land has a target, a check regime and an arrival. The target is a
parameter, the regime follows from the target, the arrival follows from what
the target admits — and all three are on the act, because `landed` otherwise
means two things.**

```
outside the lock   gate the change against the target at X       minutes
inside the lock    is the target still X?                        seconds
     still X       write, release                                seconds
     moved to Y    release; re-gate against Y; retry
```

The lock is `flock` on a path under the shared git common directory, which every
worktree of a repository can see and none of them track. It is held for seconds,
not for the duration of a gate.

### The target is a parameter and `origin/main` is one instance

The shape is *gate against the target, land onto the target*. A remote branch is
one instance of a target and a local branch is another, and nothing above the
target's own resolution knows which it has.

The obvious third clause — *cut from the target* — is deliberately **not** part
of it, and the reason is worth stating here rather than only in
[0068](0068-an-anchor-is-a-base-and-a-change.md), because it looks like the
symmetry this decision ought to want. A box cut from the target is a box
measuring a world nobody reported drift in, and it cannot express a workspace
that is not clean at all. What *cut from the target* was for — that the change
applies to the target by construction — 0068 gets from the anchor's own base
instead, without either cost.

A target is spelled `<remote>/<branch>` or `<branch>`. The first segment is read
as a remote when this repository configures one by that name, and the whole
string is a local branch otherwise; a branch whose own name would be read as a
remote is spelled in full, as `refs/heads/<name>`. **A target is a branch**, and
a detached HEAD is not one: a land is a change reaching a place that outlives
the run, and a detached HEAD is not that place.

### The default is where the operator's own work goes

`kept land <thread>` with no `--onto` resolves the target from the branch this
working tree has checked out: **its upstream if it has one, and the branch
itself otherwise.** That is git's own answer to *where does my work go*, it is
one rule rather than a table of deployments, and it degrades in the right
direction at every step.

| standing on | resolves to | regime |
|---|---|---|
| `main`, tracking `origin/main` | `origin/main` | shared |
| a task branch tracking `origin/task` | `origin/task` | shared |
| `main`, no upstream, no remote | `main` | local |
| a detached HEAD | nothing; `--onto` is required | — |

**Every land prints the target it resolved before it acts on it**, so the
derivation is never silent, and `--onto` overrides it.

The common case is worth naming, because the resolution reads oddly the first
time. Standing on a dirty `main` that tracks `origin/main`, the default target
is `origin/main` — a remote branch is checked out in no working tree, so the
change goes there as a ref advance and the operator's dirty tree is not touched
at all. That is `kept land` doing what `git push` would have done. Wanting the
repair *here* instead is `--onto main`, which resolves to a worktree arrival
because that branch is the one this tree holds and this tree is not clean.

### The check regime follows the writers the lock cannot exclude

The original's reason for reading `origin/main` rather than local `main` was
about other writers: *a local lock excludes local runs and nothing else — the
operator pushing from a laptop, CI, a second machine.* That reason is exactly
right and it is a rule about the target rather than a constant. Applied
generally:

| | shared | local |
|---|---|---|
| the target | names a remote | names none |
| writers the lock cannot see | yes — another machine, CI, a second checkout of the same remote | no — a local ref is written only by a process on this machine |
| the in-lock check | fetch the target's refspec, re-read the tracking ref | read `refs/heads/<branch>` |
| what it costs | a network round trip inside the critical section | nothing |
| does a land reach the network | yes | **no**, and it says so |

**The in-lock check never reads the local branch of a shared target.** Fetching
inside the lock is what makes the invariant true rather than merely locally
consistent, and a check against `refs/heads/main` would be answering about this
machine when the question is about the remote.

**Landing onto a local branch publishes nothing.** If that branch has an
upstream, it is now ahead of it and the operator pushes when they choose. kept
does not push on their behalf: a push is the one act that reaches other people,
and reaching the network is a property of the target a person named rather than
something a land decides for itself.

### The backstop is a compare-and-swap, and the lock only saves a wasted gate

No lock is load-bearing, which is [0013](0013-the-gate-makes-concurrency-safe.md)'s
thesis and is why a lost lock is an ordinary outcome here. What actually refuses
a write against a target that moved is a compare-and-swap, and there is one in
every shape:

| arrival · regime | what is compared and swapped |
|---|---|
| ref · shared | the remote ref, by an **unforced** push — git rejects a non-fast-forward however the check was reasoned |
| ref · local | the local ref, by an update against the value the gate saw |
| worktree · local | the working tree, by its 0033 digest, re-read immediately before the write |

The lock excludes kept's runs and not the operator's own `git` in another
terminal, so it could never have been the guarantee. Stating the three together
is what keeps a future arrival from arriving without one.

### One verb, two arrivals

A land has one high-level meaning — *bring that repair back* — and two ways the
change gets there, the way git has a fast-forward and a merge under one `git
merge`. They are **user-distinguishable, user-requestable, and refused when the
requested one is impossible.**

**A ref advance.** The target's ref moves to a commit kept made. When the target
is checked out in this working tree and that tree is clean, git updates the
files along with the ref, and the operator's files change under them — which is
the point rather than a side effect.

**A worktree arrival.** The change is merged into the working tree and nothing
moves: no ref, no commit. It lands beside the operator's own edits, and what
happens to it next is theirs. **A worktree arrival's target is always the branch
this working tree has checked out**, so it is always the local regime — the
strongest check and the weakest arrival never combine.

kept resolves the arrival from what the target admits, names it in the report
before it acts, and `--arrival ref|worktree` pins it. Pinning an impossible one
is refused by name rather than quietly satisfied by the other.

| the target is | the tree is | ref | worktree |
|---|---|---|---|
| checked out in no worktree | — | **resolved** | refused: there is no working tree at the target |
| checked out here | clean | **resolved** — git updates the files with the ref | on request |
| checked out here | dirty | refused: git will not move a branch out from under a dirty tree | **resolved** |
| checked out in another worktree | — | refused, naming the worktree | refused: it is not this working tree |

### The two arrivals do not carry the same guarantee, and the act says which

A ref advance carries the invariant:

> **Every commit that reaches a landing target was gated against the tree it
> landed on** — and, per 0037, for every promise that tree's change could
> affect.

A worktree arrival is **not an exception to it**. It puts no commit on a target,
so it is outside the invariant by construction rather than by dispensation. What
it carries instead is narrower and dated, and it is worth having:

> **The change was gated against exactly the tree it produced, and the tree it
> was written into had not moved since the gate.**

Those are different claims and a reader must be able to tell which one a given
`landed` is worth. Six things make that so, and each is a mechanism rather than
a caution:

1. **The act names its target, its regime and its arrival.** Three fields
   beside 0063's `Where`, on a `landed` act and on no other act kind. They are
   recorded rather than derived because none can be: the world names a tree and
   a commit and says nothing about where the change was going, and reading the
   arrival off an empty field is how a reader who guesses wrong reads a worktree
   arrival as a land onto a ref.
2. **There is no default for any of the three.** The ledger refuses a `landed`
   act that names no target, no regime or no arrival, on the way in and beside
   the requirement `actWhy` already makes of a land. A defaulted arrival is
   precisely how the weaker case borrows the stronger case's name.
3. **`regime: shared` implies `arrival: ref`**, so the strongest word in the
   record already tells a reader a commit reached a ref.
4. **`Uncommitted` is the discriminator a reader already knows.** It is false on
   every ref arrival, because the gate box is clean by construction, and true on
   every worktree arrival, because the change it wrote is in no commit. Both are
   refusals. 0063 already gives that field its meaning — *the commit's
   reachability says nothing about those bytes* — so a worktree land reads
   correctly with no new vocabulary.
5. **`World.Commit` keeps 0063's meaning and gains no second one.** It is the
   commit the act was performed at. On a ref arrival the act is performed in the
   tree that lands, so it is also what reached the ref; on a worktree arrival it
   is where the operator was standing and it landed nowhere. The arrival field is
   what says which reading applies, and the existing refusal — *a land with no
   commit does not say what landed* — is narrowed to the ref arrival, which is
   the one it was written for.
6. **`kept why` prints the arrival beside every land and never the word
   `landed` alone.** It gains a landing section: the date, the target, the
   regime and the arrival, in 0066's measured table; a ref arrival names the
   commit that reached the target, and a worktree arrival says in its place that
   nothing was committed. `--json` carries the same three fields, because a
   distinction a person can see and a machine cannot is a second surface.

### Landing onto a branch this working tree has checked out

The original's first deployment constraint — *`main` is checked out in no
worktree* — is false by construction locally, because the branch you want the
repair on is the one you are standing on. It stops being a constraint and
becomes the cheapest row of the arrival table above.

A clean tree fast-forwards and the operator's files update under them. A dirty
tree cannot, and git will not; that is where the worktree arrival is the
resolved answer rather than a fallback. A target checked out in *another*
worktree of this repository admits neither — git refuses to move it, and it is
not this working tree to write into — and the refusal names the worktree, which
is a thing a person can go and look at.

### Uncommitted work, and what it can and cannot do

A repair may be started from a dirty workspace. The box is cut from the world
the promise was measured in and the workspace is replayed into it, so the box's
tree digests identically to the operator's and the measurement is about the tree
in front of them; the anchor is then a base and a change, and a land takes the
change. That mechanism is [0068](0068-an-anchor-is-a-base-and-a-change.md)'s and
is stated there rather than here.

What it means for a land, plainly, as limits rather than surprises:

- **It can land.** A dirty repair reaches a shared target like any other, and
  the operator's uncommitted work does not go with it, because a land takes the
  change and not the ancestry.
- **It can arrive in the tree it came from.** That is the ordinary local shape
  and it is what the worktree arrival is for.
- **It cannot tell the operator's half-finished work from the code under
  repair.** The measurement was over the dirty tree, so the drift being mended
  may be theirs and in progress. The repair scope bounds *where* the agent may
  write and says nothing about *whose* work is there. The land's re-gate catches
  a break; it does not catch a change that is merely not what the person meant.
- **It cannot leave nothing behind.** A dirty repair snapshots the workspace
  into a commit on no branch, which is what makes its evidence reproducible at
  all, and which is also work in the object store that the person did not commit.
  0068 names that cost.
- **It cannot advance the ref of the branch it is standing on.** Not while the
  tree is dirty. Git's refusal is the same one kept reports.

### The re-gate is unconditional, and the skip is withdrawn

When the target moved to Y, the answer is to re-gate against Y. There is no
exception, and the exception this decision used to state is **withdrawn rather
than merely unbuilt**, because a reader who found it would implement it in good
faith and get a worse tree for the trouble.

What it said was: the ledger records what paths the intervening land touched,
the promise declares its `RepairScope`, and where the two are disjoint the
intervening change cannot have affected this gate, so the re-gate is skipped on
a stated rule rather than on optimism. Three things are wrong with it.

**It is unsound after [0037](0037-a-conflict-is-discovered-by-exhausted-repair.md),
and unsound for this decision's own reason.** 0037 widened the gate past the
thread's own promise to every standing artifact whose scope the diff's paths
touch, so what a gate covers stopped being a property of the promise being
repaired. A rule that consults only that promise's `RepairScope` is asking about
a subset of what the gate evaluates, and it answers *skip* on behalf of the
artifacts it never looked at. Measured on this repository:
`promise:the-tree-vets-clean` gathers over `**/*.go` and
`promise:core-packages-are-tested` over `internal/**`, so an intervening land in
`cmd/**` is disjoint from the second and the rule says skip — while `go vet`
reads the whole tree, and the combination can fail it. That is **the clean merge
is the dangerous one**, above, re-entering through the optimization written to
make it cheaper.

**Its premise does not exist.** A `landed` act carries a target, a check regime
and an arrival, and no paths at all; nothing records what an intervening land
touched, and the ledger holds no `landed` act to record it in. The rule was
never merely unimplemented — it read a field this decision had not decided to
keep.

**Stated soundly it would fire on almost nothing here.** The sound form asks for
disjointness from the gather scope of every artifact the gate *selects*, not
from the repair scope of the one it is for. Every Go change touches `**/*.go`,
which a standing promise gathers over, so on this corpus the sound rule skips a
re-gate approximately never. A mechanism that can be got wrong and that fires on
nothing is worse than the cost it removes.

So: **the re-gate is unconditional.** A land that loses its in-lock check
re-gates against the target it lost to, and pays a full gate for it — bounded by
a retry count rather than by a skip, because a target moving faster than this
repository can gate is a contention measurement rather than something to
optimize around in advance. If that cost ever becomes worth removing, what to
build is not this rule: it is a skip stated over the set 0037's gate actually
evaluates, and it cannot be stated at all until a `landed` act records what its
change touched.

**The path overlap itself stays, and is worth separating from the rule that
misused it.** *Which standing artifacts care about these paths* is the question
0037's gate asks on every land, and it is answered from the diff's paths and the
artifacts' declared scopes — inputs the change and the corpus already carry.
That is the direction that selects what to gate, and it is load-bearing. The
other direction — *can an intervening land have invalidated this gate* — is the
one withdrawn here, and it is the one that needed a record nothing writes.

### What is left of the deployment constraints

1. **`main` is checked out in no worktree** — **retired as a constraint.** It is
   the first row of the arrival table, and the rows below it are the cases the
   original could not express.
2. **Boxes are detached, never on a branch** — **kept**, and generalized off
   `origin/main`. Git forbids one branch in two worktrees and permits any number
   of detached worktrees at one commit, so every box cut for a gather at one
   commit observes the same world, and under 0014 those observations are
   identical content and dedup to one file. The one-branch-one-worktree rule
   stops being a constraint and concurrent gathers stop paying for each other's
   build output.
3. **Boxes are cut from the target** — **retired**, and replaced rather than
   dropped. What it was for is that a thread's diff applies to the target by
   construction; 0068 gets that from the anchor's own base without requiring a
   repair to be measured in a tree the operator is not standing in, and without
   foreclosing a dirty repair, which cutting from the target does.

## Alternatives

**Push, detect rejection, reset, retry — 0011's answer.** Rejected as
insufficient rather than wrong: it is exactly right for the rejected case and
does nothing at all for the clean-merge case, which is the one that silently
ships an ungated tree.

**Hold the lock across the gate.** Correct and simple, and it caps land
throughput at one per test run. Rejected on cost; the merge-queue split gets the
same invariant with a seconds-long critical section.

**A daemon owning a land queue.** Rejected for now. `flock` releases on process
death because the lock lives on an fd, so a crashed lander needs no TTL, no
reclaim, and no takeover — a daemon would have to reimplement that, and would
add a component with its own liveness and its own state that can disagree with
the ledger. The case that would justify one is speculative batching (test A+B
together, split on failure), which is a throughput optimization to build after
measuring contention, not before.

**Land onto a branch per thread and reconcile later.** Rejected: it removes
contention by removing convergence, and leaves the operator merging N branches
by hand. The destination is the point.

**Keep `origin/main` as the only target and refuse everywhere else.** Rejected.
A repository with no remote is a repository kept cannot land in, and that is
every repository on its first day and many of them forever. It also makes the
tool's reach a property of somebody's hosting rather than of the model.

**A second verb for the working-tree case.** The obvious way to keep the two
guarantees apart, and rejected. There is one high-level concept — bring that
repair back — and splitting it puts the distinction in the operator's memory,
in two help texts and in two code paths, while leaving the *record* saying
nothing. The distinction has to survive being read a year later by somebody who
did not type either verb, and only the act can carry it that far. Distinguishable
and requestable, like a fast-forward against a merge, is what was wanted; a
second verb is more than that and less.

**Resolve the arrival silently — patch when the tree is dirty and say nothing.**
Rejected: that is exactly how `landed` comes to mean two things. Resolving is
fine and printing the resolution is what makes it fine; what is not fine is a
record that cannot tell them apart afterwards.

**Refuse a worktree arrival unless it is asked for by flag.** Considered, and
rejected as friction that buys nothing the record does not already buy. A person
standing on a dirty tree asking for their repair means the only arrival there
is, and making them say so twice does not make the act more legible — the act's
own fields do that.

**Make the check regime a configuration key.** Rejected: it is derivable from
the target, and a configured regime is a second place the answer lives and can
disagree with where the change is actually going.

**Keep the disjoint-scope skip and state it soundly.** Rejected on what it
would cost against what it would buy. The sound form is disjointness from the
gather scope of every artifact 0037's gate selects, which needs a `landed` act
to record the paths its change touched — a new field on the one act kind whose
three existing fields this decision spent a section justifying. On this corpus
it would then skip approximately no re-gates, because every Go change touches
`**/*.go`. A rule that is subtle to state, needs a record to carry it, and
almost never fires is not a saving.

**Withdraw the skip in the consequences and leave the decided text standing.**
Rejected, and it is the tempting one because it is free: `body.decision` binds
and `## Consequences` does not, so a retraction written there drops no standing
and needs no re-ratification. That is exactly what is wrong with it. The rule
was decided, so unsaying it is a change to what was decided, and writing the
retraction where nothing binds would leave the ratified half of the document
still stating the unsound rule and an unratified half contradicting it — a rule
in two places that can disagree with itself, with nobody's grant on the half
that is true. An implementer reads the decision; they would find the skip and
build it. The retraction goes where the rule was made, standing drops, and a
person grants the new wording, which is what a ratification is for.

**Push a local land's target to its upstream afterwards.** Rejected. It turns
one act into two with different audiences, and the second one reaches other
people, which is the thing 0017's whole box arrangement is built to keep out of
anything but an act a person named.

## Consequences

- **`flock` has no TTL, so a wedged holder blocks landing indefinitely.** The
  gate inside the critical section is bounded by a timeout; this is the honest
  cost of preferring `flock` to a lease.
- **Every land that loses its in-lock check pays a full re-gate**, and the
  bound on that cost is the retry count rather than a rule that skips. That is
  the cost accepted for withdrawing the skip, and it is paid only under
  contention: a land whose target did not move re-gates never.
- **Land throughput is one merge per few seconds**, not one per gate. Whether
  that is enough is a measurement, and the answer if it is not is speculative
  batching.
- **A local land is materially faster than a shared one**, because its in-lock
  check is a ref read rather than a fetch. That is the deployment a person on a
  laptop is in, which is the one it matters for.
- **A land records what it was gated against** — the world from 0014 — and its
  target, regime and arrival. Without the first, an ungated combination is
  unattributable after the fact; without the other three, two different
  guarantees share a word. It records no paths, and that is the whole reason the
  withdrawn skip could not have been built as stated.
- **The `landed` act grows a landing block and nothing in a `hash` list moves,
  so the count of artifacts this de-ratifies through the record is zero.** It is
  a ledger record; no schema mentions it. What this decision does de-ratify is
  itself and the decisions it amends, which is counted where the change is made.
- **Three refusals that must be able to fire**, per `AGENTS.md`: a `landed` act
  with no target, regime or arrival; a `ref` arrival that names no commit or
  that is `uncommitted`; a `worktree` arrival that is not `uncommitted`.
- **Two more, on the arrival**: a requested `ref` arrival onto a checked-out
  dirty tree is refused rather than downgraded, and a requested `worktree`
  arrival onto a target this working tree does not hold is refused rather than
  redirected.
- **A crash part-way through a worktree arrival leaves a third state**, which no
  arrival onto a ref has. [0018](0018-what-survives-a-crash.md) says what that
  leaves and what kept may not do about it.
- **This is single-machine coordination**, per 0013. Machines that share no
  `flock` share no critical section, and this is the one place where that costs
  correctness rather than money — everything else 0013 leaves to the gate. The
  section is small and well defined, which is what makes the port tractable when
  it is wanted. In the local regime the question does not arise: a local ref has
  no writer on another machine.
- **Two threads repairing different promises that touch the same files** still
  collide, and now collide in a place that reports: the second one's re-gate
  sees an overlapping scope and re-runs. The lease-granularity question in open
  question 9 loses most of its force, because the correctness decision has moved
  to land time where the facts are known — which is the same move 0013 makes for
  repair.
- **`internal/land/land_test.go`'s two pinned tests are 0068's to replace**, not
  this decision's. They record what a land does today when the checkout is not
  the target, and the one asserting a refusal on scope should fail once the
  anchor is read as a pair.
