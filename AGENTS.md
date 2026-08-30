# Working on kept

## Read first

- [`docs/model.md`](docs/model.md) — the model. Three properties, the loop, the
  gate. Everything in the code should be traceable to something here.
- [`docs/decision/`](docs/decision/) — why it is this way, including the
  alternatives that were rejected and the costs that were accepted.
- [`docs/lineage.md`](docs/lineage.md) — which of the nine predecessor systems
  each part came from, the measurements that shaped it, and what was
  deliberately **not** adopted. Read this before proposing something that
  sounds like a return to one of them.
- [`docs/increments.md`](docs/increments.md) — what to build next, and what
  "done" means for it.
- [`docs/open-questions.md`](docs/open-questions.md) — what is genuinely
  undecided. Do not close one of these by guessing; close it when something
  forces the answer.
- [`docs/layout.md`](docs/layout.md) — the corpus, the ledger, what is derived,
  and how volume is kept down.
- [`docs/self-hosting.md`](docs/self-hosting.md) — the path to kept holding
  promises about kept. Thirteen of the invariants below have moved: they are
  ratified policies in [`docs/policy/`](docs/policy/), and five structural
  promises hold this repository to them.

`README.md` is the short version, and [`docs/model-deck.html`](docs/model-deck.html)
is the system end to end, with one promise followed the whole way. If the code and
`model.md` disagree, one of them is a bug; say which.

## Build

```
make build      # go build -o kept ./cmd/kept
make test       # go test ./...
make vet        # go vet ./...
```

Go 1.26, and dependencies are few on purpose. A module is admitted by the test
in [decision 0029](docs/decision/0029-dependencies-pass-a-test.md) and recorded
in [`docs/dependencies.md`](docs/dependencies.md); one in the trust path — it
sees secrets, decides authority, confines a process, or writes the ledger —
gets an ADR of its own. A module in `go.mod` with no line in that file is a
finding.

**`nono` is a dependency with no `go.mod` line**, so nothing catches it moving.
Its whole surface — the profile JSON kept renders, the vocabulary kept borrows,
and the behaviours kept relies on that nono never promised — is inventoried in
[`docs/nono-surface.md`](docs/nono-surface.md), classified as format / concept
/ behaviour and marked measured / documented / **assumed**. Read it before
changing anything under `internal/confine`, and before believing a claim about
what the provider does. Two things from it are worth carrying: **kept pins no
nono version and has no compatibility check**, and **the eighteen-group deny
baseline every box rests on is contradicted by nono's own authoring guide** —
so `nono profile show` on a rendered profile is the authority, not the guide.

## Invariants that do not change

These are the ones worth stopping over. Breaking any of them is a design
change, not a refactor.

**Thirteen of them are now policy artifacts in [`docs/policy/`](docs/policy/),
ratified.** They are the ones that are checked before an act — a policy is *a
ratified rule evaluated at the moment of acting*, and it declares which act it
is checked before. Each is one line here: the artifact's **title**, and a link
to it. The binding `statement` and the why live in the artifact and are not
restated here, because a rule written in two places is a rule that can disagree
with itself. A title is a name, and repeating a name is not a second copy of the
rule. Read the artifact.

The rest stay, in full, because they are not checkable before an act. They say
how a derivation is defined or what a record means, and nothing evaluates them
at the moment of acting — which is the test that decides where a rule lives.

- **Standing is created only by ratification, and only by a person** —
  [`policy:standing-is-created-only-by-ratification`](docs/policy/standing-is-created-only-by-ratification.md)
- **Standing and truth are never written to an artifact file** —
  [`policy:standing-and-truth-are-never-in-an-artifact-file`](docs/policy/standing-and-truth-are-never-in-an-artifact-file.md)
- **The box is not the jail** —
  [`policy:the-box-is-not-the-jail`](docs/policy/the-box-is-not-the-jail.md)
- **Record only what has already happened** —
  [`policy:record-only-what-has-already-happened`](docs/policy/record-only-what-has-already-happened.md)
- **A tick that does nothing still records why** —
  [`policy:a-tick-that-does-nothing-records-why`](docs/policy/a-tick-that-does-nothing-records-why.md)
- **Durability follows reproducibility, not size or type** —
  [`policy:durability-follows-reproducibility`](docs/policy/durability-follows-reproducibility.md)
- **Truth is derived on read.** Storing it is asserting it, and an asserted
  truth goes stale without saying so.
- **Standing is derived too, and nothing invalidates it.** An artifact is
  ratified when the ledger holds a ratification naming the hash its content
  produces *now*, and a release empties that set for the artifact rather than
  for one wording. There is no flag to clear, no cache to drop and no hook to
  fire on an edit — `internal/register` writes nothing at all, and a mark would
  be a second place the answer lived. See 0032.
- **Ratifying from a task branch writes the shared ledger, before review.**
  `refs/kept/ledger` is repository-wide, not branch-scoped, and a grant binds a
  content hash rather than a location — so `kept ratify` on a branch lands the
  act at once, for every worktree. Both wordings then hold standing, each against
  its own grant: `main` still reads `ratified` at the old hash, and `status`,
  `list` and `why` all call it clean. A release empties the artifact's whole set
  rather than one wording, so a declined branch leaves a grant for content that
  exists nowhere and no way to withdraw just that one. Sequence a revision's
  ratification with the merge, not with the edit.

  **`doctor` is the one reader.** An act records the world it was performed in
  and that world is **advisory** — nothing consults it to decide standing, which
  stays keyed to a content version and to nothing else — so `doctor` can say
  when a grant in force names a commit *this tree cannot reach*, and stay silent
  when it names one that landed and was revised. Reachability, never equality:
  every act is against a tree this is not the moment anything lands. A dirty
  tree is a second finding, because a commit that is reachable vouches for
  nothing that was hashed beside it. **Every act written before this names no
  world, and a missing world is silence** — *nobody recorded one* is not *it
  never landed*. Detection only: nothing refuses, and the exit code is
  unchanged. See 0063.
- **Nothing kept writes rewrites what a person wrote** —
  [`policy:nothing-kept-writes-rewrites-what-a-person-wrote`](docs/policy/nothing-kept-writes-rewrites-what-a-person-wrote.md)
- **A gatherer never repairs** —
  [`policy:a-gatherer-never-repairs`](docs/policy/a-gatherer-never-repairs.md)
- **A failed gatherer yields `unknown`, never `not-kept`** —
  [`policy:a-failed-gather-is-unknown`](docs/policy/a-failed-gather-is-unknown.md)
- **An observation's data holds nothing that moves on its own.** No duration,
  no timestamp, no pid, and no order that came from map iteration. It is
  addressed by its data, so a field like that mints a fresh record on every
  tick of an unchanging world and the dedup the whole content-addressed layout
  exists for quietly stops working. Sort what a fold produces. See 0034.
- **A gatherer that declares nothing digests exactly the tree.** `Reads()` is
  what a gatherer says its measurement depends on beyond the bytes in scope,
  and only acts a person performs may be named — truth and dispositions are
  written every tick, so declaring one makes the gatherer due again forever.
  The projection is folded into the **scope digest**, never into the
  observation preimage: the scope is already in the address and already in the
  cache key, so a declaration moves both together and moves nothing for anyone
  else. A composition applied unconditionally re-gathers every promise in every
  corpus for an answer that did not change, and the test over `Builtin()` is
  what catches that. `gather.Requests` builds the request for both `Tick.due`
  and `Tick.observe`, because two computations of one digest fail silently in
  the direction that leaves a promise never due. See 0046.
- **The agent's report is never an input to the verdict** —
  [`policy:the-agents-report-is-never-an-input-to-the-verdict`](docs/policy/the-agents-report-is-never-an-input-to-the-verdict.md)
- **A repair is one promise a person named, and a tick never dispatches.**
  `kept repair <promise>` cuts a box at `HEAD`, measures the promise *in the
  box* — that measurement is both the decision to dispatch and the evidence the
  packet carries — dispatches, re-derives, and prints the diff with 0059's
  review beside it. Nothing lands. A tick keeps its `refused`/`provider`
  disposition unchanged, because it verifies nothing and starts nothing;
  choosing among several drifts is the ranking and the budget of increment 7.
  The refusals `kept repair` records are `provider` (the jail) and `runtime`
  (the adapter, its version, its credential), and they are separate because the
  transition rule compares outcome and condition and never the detail. See 0060.
- **A land is one thread a person named, and it is the only verb that can reach
  the network.** `kept land <thread>` gates *outside* a lock, takes the lock,
  asks whether the target moved, then merges and writes — 0015's queue, with the
  expensive half outside the section. **The re-gate is unconditional and the
  disjoint-scope skip is withdrawn, not merely unbuilt**: 0037 widened the gate
  to every standing artifact the diff's paths touch, so a rule reading one
  promise's `RepairScope` answers *skip* for artifacts it never looked at — a
  land in `cmd/**` is disjoint from `core-packages-are-tested` and can still
  fail `the-tree-vets-clean`'s `go vet` over `**/*.go`. Its premise is missing
  too: a `landed` act records target, regime and arrival and **no paths**. 0015
  says all of this; do not reinvent it from the shape of the retry loop.
  **The target is a parameter and
  `origin/main` is its default rather than its definition**, and the check
  regime follows it: a target that names a remote is *shared*, so the in-lock
  check fetches and **reads the tracking ref, never `refs/heads/main`** — a
  local lock excludes local runs and nothing else, so a check against the local
  branch would miss the operator's other machine, and `internal/land`'s two
  `inlock_test.go` proofs fail loudly if it ever does. A target that names none
  is *local*: nothing outside this machine can write that ref, so the check is a
  ref read and the land reaches no network at all. **The backstop is a
  compare-and-swap in every shape and the lock is never load-bearing** (0013):
  an unforced push for a remote ref, an update against the expected value for a
  local one, and 0033's digest re-read for a working tree. A land records a
  `landed` act — the existing `Act` shape, no new record type — whose world is
  the tree it was gated against, and which names its **target, its check regime
  and its arrival**, because a ref advance and an arrival into a working tree do
  not carry the same guarantee and `landed` would otherwise mean both. Those
  three are a `ledger.Landing` and **none of them has a default**:
  `ledger.Land` refuses an act missing one *before it is appended*, `actWhy`
  quarantines the same line on the way back in, and both ask `landingWhy`, so
  the two ends cannot drift. It also refuses `regime: shared` with a worktree
  arrival, an `Uncommitted` that disagrees with the arrival, a ref arrival
  naming no commit, and a landing on any other act kind. The write is the only
  moment any of this can be prevented, because the ledger appends and never
  revises. **A land act creates no standing**: it carries an artifact, a hash and a norm exactly
  as a ratification does, and only `register`'s fold switching on kind keeps it
  from granting one, so that has its own test. **0018's crash guarantee is
  killed rather than argued for the ref arrival**
  (`internal/land/crash_test.go`); the worktree arrival's own third state is a
  partial write and is proven by the write failing part way and a second run
  finishing it, which the entry on staging a crash, below, says more about.
  See 0015, 0037, 0067 and 0068.
- **The arrival is resolved from what the target admits, printed before
  anything is acted on, and recorded.** 0015's table is
  `internal/land/arrival.go` and its eight cells are
  `arrival_test.go`; `--arrival ref|worktree` pins one and **an impossible one
  is refused by name rather than satisfied by the other**, which is the whole
  reason the table is a function instead of a fallback chain. The rows turn on
  one question — which working tree, if any, has the target's branch checked
  out, and is that tree this one and clean — and a *shared* target is answered
  without asking git, because a remote branch is checked out nowhere. That is
  also the mechanism keeping **`regime: shared` from ever meeting
  `arrival: worktree`**, which `ledger.landingWhy` refuses on the way in.

  **A worktree arrival merges into the operator's tree and moves nothing** — no
  ref, no commit, no index entry — so its guarantee is narrower than a ref
  advance's and dated: *the change was gated against exactly the tree it
  produced, and the tree it was written into had not moved since the gate*. The
  second half is 0033's digest, taken before the gate and re-read immediately
  before the write; a tree that moved is `land.ErrTorn`, reported as
  contention, and **nothing is written**. Everything that can refuse — the
  swap, the merge, an unreadable blob, a submodule, a change too large to hold
  — happens before the first file is touched, because a half-applied merge into
  a tree the operator is standing in is worse than not landing at all. The
  merge itself is `git merge-tree` against a snapshot of the working tree taken
  through a **scratch index** (the operator's is never opened for writing), and
  only the paths where that snapshot and the merged tree differ are written —
  which is a subset of what the scope review saw, and has a test saying so.

  **`.kept/` is excluded from every git-side question a land asks**
  (`land.notKept`). A land cuts its gate box under `.kept/boxes/`, so by the
  time anything inside the lock asks *is this tree clean* the box is sitting in
  it: without the exclusion every local ref arrival refuses itself and every
  worktree snapshot swallows a second checkout. `internal/workspace` excludes
  the same directory from the digest, so the two instruments agree about what
  counts as the operator's tree; a repository that gitignores `.kept/` gets the
  same answer either way, and one that does not is not kept's to assume.

  **`kept why` prints the arrival beside every land and never the word `landed`
  alone**, on both surfaces — 0015's sixth mechanism, and the one a reader a
  year later actually has. `tick.Subject.Landings` carries the acts and
  `--json` carries the same three fields under `landings`, on the `why`
  document alone, so the `status` and `drift` goldens are untouched.
- **A repair anchors what its session left, and that is what a land is handed.**
  A repair commits the box's tree onto the commit the box was cut from and
  points `refs/kept/threads/<thread>` at it, before the box is discarded — so
  the commit's own diff against its parent is exactly the change a person
  reviewed. **An anchor is therefore a pair, and 0068 is what makes everything
  read it as one**: a land merges the change with the anchor's *parent* as the
  merge base rather than merging the anchor, so private ancestry — an operator's
  unlanded commits, or the snapshot a dirty repair takes of their workspace —
  never reaches the target, and what lands has the target's tip as its only
  parent. **Built for the land; 0068's replay of a dirty workspace into the box
  is not**, so a base is still the operator's `HEAD` and a break under test must
  still be committed. It anchors whatever the session produced, repaired or not and
  bounded or not, including when the diff was too large to print; what may land
  is the land's question and answering it twice is how a report and a gate come
  to disagree. The ref is **not** a branch and is deliberately outside 0062's
  `refs/kept/ledger*` fetch pattern, because an unlanded repair is local work
  nobody reviewed. **The refs are the queue**: `ledger.Store` has three
  operations and no listing one, so `kept land` with no argument enumerates what
  git already indexes. The anchor commit is **authored by the operator, never by
  kept or the agent** — kept's operator, else the identity git itself would use;
  failing both the repair still runs and simply is not landable. See 0067.
- **A packet carries every instrument's measurement, keyed by the promise's own
  name.** `gather:` declares `graph` and `names`, `hold_when` reads `obs.graph`
  and `obs.names`, and the packet carries `observations.graph` and
  `observations.names` — the key is already in the packet twice, so a
  measurement is matched to the clause that weighs it by name rather than by a
  position in a list. **All of them, always**: which half of a coupled predicate
  is failing is often undefined and always a *reading* of the evidence, and the
  half that holds is what says what the repair must not break. kept therefore
  names no failing instrument either — the agent holds `hold_when` and every
  measurement and can evaluate it, and a field with the answer in it is 0009's
  enumerated repair arriving as a hint. `PacketVersion` is 2 and the singular
  field is **gone** rather than kept beside the plural one; the one-instrument
  case is the plural case with one key. `AssemblePacket` refuses both a gathered
  name with no measurement and a measurement under a name the promise does not
  gather, and `tick.Measure` returns them keyed because it is the only caller
  that needs the name. A repair that mends one half and breaks the other reads
  `not-kept` from the re-derivation and is recorded `Unrepaired`. See 0061.
- **Out-of-scope paths are reported, never landed** —
  [`policy:out-of-scope-paths-are-never-landed`](docs/policy/out-of-scope-paths-are-never-landed.md)
- **The pipeline is never in a packet** —
  [`policy:the-pipeline-is-never-in-a-packet`](docs/policy/the-pipeline-is-never-in-a-packet.md)
- **Everything tracked in git must merge without a human** —
  [`policy:everything-tracked-in-git-merges-without-a-human`](docs/policy/everything-tracked-in-git-merges-without-a-human.md)
- **Ids are minted, never composed and never counted.** This covers runs,
  threads, boxes — anything concurrent runs each name. `ids.Mint()`: 128 random
  bits and nothing else. Not composed, because anything encoded in an identifier
  becomes parseable, then depended on, then unfixable, and a hostname in a
  tracked record cannot be taken back. Not counted, because two runs allocating
  from a counter collide and their records merge into one apparent thing with
  nothing reporting it. Diagnostics belong in the record the id names, where
  they can be pruned or omitted.
- **Derived reads fold by event time, then id — never by file order.** Union
  merge interleaves arbitrarily. A fold that depends on file order is silently
  wrong and will look correct in every single-run test.
- **An observation names the world it measured.** The tree, not just the paths.
  A cache or a truth series keyed without it lets a gather in a box answer for
  the tree that ships, which is how a repair that did nothing passes the check
  meant to catch it. See 0014.
- **The world is a digest kept computes, and the accelerations are not the
  definition.** `World.Tree` is a content digest over the workspace root
  honouring ignore rules — the same number in a repository, outside one, and in
  a box, and it asks git nothing. The stat cache, the parallel path and git's
  index are accelerators that must agree with the plain walk, and the test that
  they agree is what stops one becoming a second definition. Stat is the cache
  key and content is the definition, because two identical checkouts have
  different mtimes and their observations must still dedup. `.kept/` is
  excluded or the tick feeds itself, and a scope matching nothing is a distinct
  outcome rather than a stable digest over an empty set. See 0033.
- **A measurement is hashed as it was measured.** The observation preimage
  reuses 0031's *encoding* under `obs:1` and never 0022's *text rule*:
  normalizing a gatherer's output collides four classes of genuinely different
  measurement, and under content addressing the second is then discarded as a
  duplicate of the first. It is addressed by gatherer@version, args, world,
  scope digest and data; `at` is recorded outside the address and means
  first-seen, so freshness comes from the truth stream's heartbeat and never
  from that timestamp. No writer, no seq, no error. See 0034.
- **A verdict in force is the last one that is not due.** A world is the whole
  tree, so any edit anywhere produces a world no verdict was ever reached in.
  Reporting only verdicts reached in *this* world would make every promise read
  `unknown` after any keystroke and force a gather of everything on every tick,
  which is the cost 0033 exists to avoid. So the tick carries an answer forward
  exactly when nothing in the artifact's scope has moved — the same test
  due-ness makes, and a stronger one than tree equality — and says which world
  it was reached in. Never carry one forward on the tree alone, and never on a
  verdict reached in a box: `ledger.Reached` refuses those outright.
- **The heartbeat is `max_age` and there is no second timer.** A truth record is
  appended when the verdict changes, or when a gather ran and the last record is
  older than `max_age`. An observation's `at` means first-seen and never
  advances for an unchanging world, so freshness has nowhere else to come from,
  and `max_age` is already the second half of *due*. A configured interval would
  be a third place staleness lived. See 0025 and 0034.
- **Nothing has answered is not an answer of `unknown`.** Never observed, an
  answer about a tree this is not, and a gatherer that could not run are three
  states, and only the third is a verdict. The first two say the question was
  not put; the third says it was put and could not be settled. Collapsing them
  is the same mistake as collapsing `unknown` into `not-kept`, one level up.
- **Truth folds then filters by world; a disposition never does.** Both are one
  file per artifact with the world as a field, because a path keyed by world
  mints tracked files without bound. A truth fold that forgets the filter is
  silently wrong and looks correct in every single-run test; `kept why` folds
  the whole disposition stream on purpose, because the question is about the
  queue rather than about a tree. A verdict reached in a box belongs to the
  thread. See 0035.
- **No lock is load-bearing.** Two runs may repair the same drift; the second
  one's gate finds the promise already kept and lands nothing. A lock saves an
  agent dispatch, and code that would be *wrong* without one is code that has
  moved correctness out of the gate. The exception is the section between the
  gate and the push, which is isolation, not exclusion. See 0013 and 0015.

  **The ledger's ref is where concurrency is actually arbitrated, and it is a
  retry rather than a lock.** Every ledger write is a read-modify-write against
  one ref, so a losing writer rebuilds on what it read and appends its own bytes
  — nothing is merged and nothing is excluded. `docs/layout.md`'s *no
  compare-and-swap* was written about the **file** tier, three days before 0016
  moved the ledger onto a ref, and it now says so. **The schedule is not a
  detail**: eight attempts with no wait between them abandoned 26 of 80 appends
  under eight concurrent tick-sized writers, because every loser of one
  compare-and-swap is a cohort with a common clock and retrying them together
  re-runs the whole cohort into the next collision. **Size a backoff window in
  attempts, never in milliseconds** — an uncontended attempt is seven Git
  invocations, 53 ms idle here and ~220 ms with the suite running beside it, and
  a constant 2 s cap tuned on the idle number lost three of eighty the first
  time the machine was busy. And **one line per writer reproduces nothing**: a
  tick appends acts, truth, observations, dispositions and diagnostics, so a
  contention test appends a run's worth. See 0064.

  **Losing that race exits 3, not 2.** 2 means the operator asked for something
  wrong; a run that gathered, evaluated and then could not record is *no
  evidence either way*, which is 0027's own definition of 3. `ledger.ErrContended`
  is the discrimination and `cmd/kept`'s `unrecorded` is its one reader; a
  genuine write failure — a broken ref, a permission problem, a torn line — is
  never retried at all and still exits 2.

## Things that will be tempting and are wrong

- **Giving `command` a shell.** There is none, and a `run:` line containing an
  unquoted metacharacter is refused by name. A shell would make `run:` an
  injection surface, make the gather depend on which shell the machine has, and
  hide the process group 0026's deadline has to kill. What it would buy is a
  script, and a script is a file a person reads in review and a promise names.
  The reading of a `run:` line is frozen, because it binds. See 0038.
- **Giving the tick a schedule.** There is no scheduler, no daemon and no
  interval: `kept` ticks once and exits, the way `make` does, and a cadence is
  due when the bytes in its scope move. Driving it on a timer is cron's job and
  lives outside kept. See 0025.
- **Adding a repair library.** The packet states the gap; the agent decides.
  See 0009. If a repair recurs, that is a predicate nobody has written.
- **Adding a probe field to promises.** See 0006. Verification attaches to
  acts, not instruments. If an instrument needs testing, that is a gatherer a
  promise can name.
- **Making work a corpus entity.** See 0008. Nobody authors a work item.
- **Letting a predicate read facts.** A fact is a record of an observation
  *then*, not evidence of the world *now*. If you want to gate on a fact,
  ratify it and it becomes canon.
- **Growing the predicate language.** A builtin is added when a promise in this
  repository or in a manuscript needs one, never in advance, and the refusal
  message for an unknown function says so. `hold_when` is authored in the corpus
  and bound by `promise.yaml`'s hash list, so the syntax is effectively
  permanent: an operator or a type added now cannot be taken away. And nothing
  may tell *nobody measured* from *this expression is wrong* by reading a
  message — `predicate.Unknown` is a value and `predicate.Error` is an error,
  and that split is the whole reason the language is hand-written. See 0036.
- **Widening `doctor` past what 0050 admits.** `doctor` reports **what is
  well-formed and unwise** and `kept list` reports **what is wrong** — a schema
  violation, a broken link or a predicate that will not parse is a defect
  somebody must fix, and an unchecked instrument is a promise nothing refuses
  and nothing else reads. A finding belongs in `doctor` when all three hold:
  nothing rejects it, it has no other reader, and kept does nothing about it.
  `--fix` writes identity and only identity, whatever `doctor` grows. See 0042
  and 0050.
- **Caching standing in the file for display**, or compacting the decision
  stream into a current-state file. A cache that can disagree with its source is
  worse than none, and the compacted one would have to be written by a reader —
  which makes `kept status` something that cannot be run against a read-only
  checkout. `kept status` and `kept why` recompute. See 0001 and 0032.

- **Refusing a gather entry in `Declares` and nowhere else.** An authoring
  error in a gather spec has to refuse the *gather*, not only the load, and
  `gather.refuseUnrunnableSpec` is the one place that decides — `Catalogue.Check`
  asks it when the corpus loads and `Runner.Gather` asks it before it measures,
  so `kept list` and the tick can never disagree about whether a spec is
  readable. Asking only at load is not a smaller version of the same thing: it
  is the failure kept had already written a guard against and still shipped.
  Four of the five built-ins validated their args in `Declares` alone, so a
  promise carrying an arg nothing accepts was reported by `kept list` as a
  `bad-gather-spec` **and measured with anyway**, and the observation then
  faithfully recorded args nothing had honoured — a `core-packages-are-tested`
  truth record says `kept` from a gather whose `exclude:` never happened. Only
  `command` escaped, and only because `readCommand` happens to be shared
  between its two entry points. A new gatherer therefore validates in
  `Declares` and does not re-validate in `Gather`;
  `TestAnUnknownArgIsRefusedOnTheGatherPathByEveryGatherer` runs over
  `Builtin()` and fails outright for a catalogue entry it has no args table
  for.

- **Resolving a gatherer on `PATH`.** The built-in catalogue is closed and it
  wins: a bare name always resolves inside it, nothing on `PATH` can shadow a
  built-in, and a third party is written `x/<name>`. `gather` is in
  `promise.yaml`'s hash list, so moving a promise from `go/tests` to `x/tests`
  drops its standing — which is the mechanism, not a side effect. The wire in
  0043 is a **sketch marked not yet binding**; the first real plugin revises it
  rather than being held to it. And a gatherer declares its fields where the
  shape is decided: in code where kept decides, in the promise where the promise
  does — never at gather time, where a declaration arrives too late to be an
  authoring error. See 0043.

  The catalogue is `command` and `corpus`, which work on any tree, and
  `go/tests`, `go/imports` and `go/symbols`, which need the Go toolchain and
  say so in the name a promise binds. A group is one segment and it answers
  *what must this tree have*; `x/…` is not kept's and kept makes no claim about
  what follows it. See 0051.
  **`go/imports` and `go/symbols` parse with `go/parser` and never shell out**, and
  the reason is not the dependency: a toolchain cannot answer on a tree that
  does not compile, which is the tree a repair agent stands in. A cycle, a type
  error or a missing symbol parses, so the graph is reported; a *syntax* error
  yields a partial AST and is `unknown`, because reading imports out of one
  under-approximates the edge set silently. `corpus` reads a corpus root and
  never the ledger. The three costs — build tags unevaluated, no scope analysis
  on a selector, `_test.go` excluded until `tests: true` — are in 0049 rather
  than only in the code, because `Declares` binds.

- **Loosening a schema's `hash` list without counting what it de-ratifies.**
  Changing the list is a bulk re-ratification (0003), so the time to bind a
  field is before the first artifact of that kind exists — and **the count is
  the whole decision, so say it**: "every promise in the corpus" and "two
  promises" are the same sentence and opposite instructions. Canon binds its
  instrument as of 0045, and that was free at **zero** canon entries; `promise`
  binds `repair` and `policy` binds `on_fail` as of 0047, which cost two
  re-ratifications and nothing else because the counts were **two** and
  **zero**. Every kind that declares a field deciding what is owed now binds it,
  so a list that grows from here is adding a field, not closing a gap.

  **`max_age` is unbound on purpose, in both.** The test is whether a field says
  *what is owed* or *how kept goes about finding out*: `repair.scope` is the
  bound on what may be changed in a promise's name, and `max_age` is how often
  kept re-asks. Binding it would charge a re-ratification for tuning a schedule.
  A hash list is not yours to change unasked: stop and report, and report the
  count with it — a change now re-ratifies every artifact of that kind,
  including ones that do not declare the field you added.

- **Padding a table to a guessed width, truncating a value to fit one, or
  adding a second machine format.** `cmd/kept/table.go` is the one renderer and
  it measures: a column is as wide as its widest cell, the last column is never
  padded, and nothing is ever cut. Three format strings guessed `%-38s` for an
  artifact id and this corpus's longest is **73**, so the column after it
  vanished on exactly the rows worth reading. kept has no terminal width to
  truncate to (0065 declined the dependency, and `internal/progress` assumes 80
  for the same reason) and an abbreviated id cannot be pasted into `kept why`.
  Measured, `git branch -v` and `git for-each-ref --format='%(align:30)'` both
  let a long value push instead — `%(align:)` is a **minimum**.

  Three rules follow and each is load-bearing. **Bounded fields left, the
  unbounded name right** (`ls -l`, `git status -s`), so an overlong row spends
  its overflow on the part already identified. **A value the same in every row
  is not a column** — the type is in the id, a title whose slug is a prefix of
  it is dropped, and the principal is a footer, with `kept show`, `kept why` and
  `--json` holding what a table drops. And **nothing on stdout asks whether
  stdout is a terminal**: `internal/progress` is the only `isatty` in the tree,
  it needs *both* streams and writes to stderr, so an answer is the same bytes
  piped and not. Piping is promised stability, never a parse.

  **The machine surface is `--json` and nothing else.** Not `--csv`, not
  `--format`, and not a `list --json`: measured, `kept status --json` already
  emits every artifact `kept list` prints — identical id sets, plus `title` and
  `statement` — so there is no gap. A second format is a second thing to
  version (0027 chose one envelope for exactly that reason) and a flat row
  cannot carry the reason beside a verdict, which is the collapse the
  three-valued model exists to prevent. `cmd/kept/testdata/*.json` are goldens
  that fail if a presentation change moves the payload. See 0066.

  **Some sentences are printed in both surfaces, and those are frozen.** A
  corpus problem's `detail`, a due `because` and a truth `detail` are written
  under `internal/` and appear in the tables *and* in the document, so
  rewording one moves the machine surface — `cmd/kept/testdata/drift.json`
  carries a dozen of them verbatim. A pass over kept's wording is a pass over
  the sentences `cmd/kept` composes itself, and nothing else. What kept prints
  otherwise carries **no decision numbers, no internal vocabulary and no
  personification**: a person at a terminal cannot follow a citation, and the
  reasoning belongs in the corpus.

  **A `doctor` finding says what is wrong, what to run, and at most one line of
  why**, and the same bar holds for `kept init`'s notes and the refusals in
  `cmd/kept/main.go` and `cmd/kept/ledger.go`. The design rationale — why the
  push is unforced, what a fork means, what a reflog does *not* protect — is
  already in the decisions, and printing it to everyone serves neither reader.
  The line is what a caveat does: keep one that changes what the reader should
  do (*as of the last fetch*, *the value `true` does not do it*), cut one that
  justifies a choice already made or describes what does not happen. Go finding
  by finding; a single transformation over all of them gets it wrong in both
  directions. And **a phrase a test asserts on must not be split by a hard wrap**
  in a concatenated format string — twice here a reflow broke a passing
  assertion, so the wrap is load-bearing where one is.

- **Making a listing verb's filter mean one thing on one verb and another on
  its neighbour, or reading an empty result as one outcome.** `kept status`,
  `kept drift` and `kept list` take `--kind`, `--id` and `--state`; repeat one
  and it is `or`, give two and it is `and`. The tick takes none of them,
  because a filter there narrows what is **recorded**.

  **`--state` is the vocabulary of the verb it is typed on and refuses the
  other by name** — `status` reports standing, `drift` reports truth, they share
  only the word `absent`, and `list` reports neither because it never touches
  the ledger. A flag that quietly accepted both sets would match nothing on the
  wrong verb and print an empty table. `--kind` reads `docs/_schema` (union the
  kinds present, since a type directory with no schema still loads and still
  prints), and `--id` globs the **whole printed id**, `type:slug` — a pattern
  matching the slug alone answers differently from what the reader sees, so a
  slug-shaped pattern is answered with `*:<pattern>`.

  **An empty result is two outcomes and the split is the selector/predicate
  one.** `--kind` and `--id` name a population, so one that names nothing means
  *the question was never put* — exit 3, the same rule as *nothing has answered
  is not an answer of `unknown`* one level up. `--state` is a question about
  that population, so a state nothing is in is an **answer** and exits 0 — else
  `kept status --state unratified` would exit 3 on the good news. Both say so
  out loud; neither renders as an empty table, which is
  `internal/gather/tests.go`'s rule that a suite of no packages is not `ok`.

  **The exit code and `--json` describe the narrowed set**, quarantined ledger
  lines excepted (one might be the act that settles the question asked), and
  **problems are never narrowed** — a filter must not be able to make a broken
  corpus look clean. Unfiltered output is byte-identical to what it was and
  `cmd/kept/testdata/{status,list,drift}.txt` are goldens cut from the commit
  before the flags existed. A narrowed run says so in one footer line. See 0071.

- **Recording what a progress display reports, giving it levels, or reporting
  from inside `internal/gather`.** A tick and a repair say what they are doing
  on **stderr** while they do it; `--json` on stdout is byte-identical either
  way and the exit codes are untouched, which is what keeps 0027's contract
  intact. `internal/progress` is presentation and holds one `io.Writer` and
  nothing else — a timing that exists only inside a gatherer is passed out for
  display and never stored, or it becomes a field that moves on its own in a
  record addressed by its data. What earns a line is **an invocation kept
  started and does not control**, and nothing else: measured over this
  repository, two `go test` calls are 94% of a cold tick and everything that is
  not an external call is 55 ms together, so a display that also announced the
  walk and the fold would spread itself over phases that are not evenly
  expensive. For the same reason there is no `n of m` counter — those
  instruments differ by three orders of magnitude, so a fraction of artifacts is
  a fraction of the wall clock that can be wrong by that factor. The live form
  needs **both** streams to be a terminal and everything else gets append-only
  lines whose first four fields are fixed. `--quiet` is the only flag and it is
  on the two slow verbs alone: it earns itself because problems and progress
  share stderr, so redirecting the stream to lose progress loses the findings
  with it. See 0065.

  Two things about exercising it. **A repair box is cut at `HEAD` with no dirty
  overlay**, so a break has to be *committed* before `kept repair` gets past
  `Holds` — 0068's replay of the workspace into the box replaces that and is
  **unbuilt** (its land half is built), so a committed break is still the only
  way in. And
  `promise:the-tree-vets-clean` is the wrong promise to try it on either way:
  its repair scope `**/*.go` renders as a grant for the box root, which
  `NewNono` refuses outright, so that promise authorizes a repair that always
  refuses. `core-packages-are-tested`, whose scope is `internal/**`, is the one
  that reaches the canary.

- **Templating what `kept new` writes, or putting scaffold text in a field that
  binds.** `kept new <type> <slug>` generates an artifact from
  `docs/_schema/<type>.yaml` — the field list in its authored order under
  `fields:`, the H2 headings `body:` names, the relations `links:` admits — so a
  field added to a schema reaches authors with no code change. A per-kind
  template would be a second copy of seven schemas, which is the defect the verb
  exists to prevent one level up.

  **The hash list is the discriminator and nothing else is.** A bindable field is
  one a ratification is granted against, so `statement: TODO` is ratifiable
  exactly as it stands and nothing downstream would ever say so — kept therefore
  writes *nothing* into one, and a field that is required **and** bindable
  (`promise.statement`, `policy.statement`, `policy.applies_to`,
  `canon.statement`) arrives on `--set field=value` or the verb refuses by name.
  An enum is refused for the neighbouring reason: every value it admits is an
  assertion, and its declared default says what absence means rather than being a
  value to write — the same rule `TestCheckDoesNotFillInADefault` enforces on the
  read path. Guidance goes in **YAML comments**, which nothing parses and no hash
  covers.

  Three shapes are load-bearing and each has a test. An optional field is a
  comment rather than an empty key, because `max_age:` with nothing after it is a
  duration nobody could parse and a `fields:` key holding only comments parses as
  null; a kind whose **whole** body binds (`canon`) gets **no body at all**,
  H1 included; and a sectioned kind gets one empty heading per declared section,
  because an absent declared section is reported by every read and
  `body.decision` must not cover words kept invented. It refuses to overwrite
  (`O_EXCL`), refuses a slug the corpus already claims, and writes **nothing**
  else — no ledger, no standing.

  **The id shape is in the schema too, and `decision` is the only kind that
  declares one**: `id: {shape: numbered, width: 4}`, absent meaning a plain slug.
  The number is in the **identity** rather than the filename — sixty-eight of
  sixty-eight decisions carry one and 0 of 21 policies and promises do — so
  `kept new decision what-was-decided` mints `0069-what-was-decided`, one past
  the highest the kind has. Only the number is minted: a slug that already
  carries one is refused, because renumbering is an edit to the file. The set of
  shapes is **closed rather than a pattern**, because one declaration serves two
  readers — a regex validates and cannot mint, so a pattern needs a second
  declaration beside it. Adding the key **de-ratifies nothing** (0021 keeps the
  id out of every hash list), pinned by
  `TestDeclaringAnIDShapeMovesNoArtifactsHash` rather than argued.

- **Treating a number two artifacts claim as something to diagnose rather than
  to prevent.** It is a four-rung ladder and each rung exists because the one
  above it can be got round. **Minting holds `internal/lock`'s `mint` lock** and
  re-reads the corpus inside it, so two concurrent `kept new` runs cannot pick
  one number; the wait is two seconds because the section is a walk and an
  `O_EXCL` create, and losing it costs the word *again*. **`kept list` and
  `doctor` report `duplicate-number`** — a diagnostic, not the load error a
  duplicate *id* is, because the slugs differ and the map from id to artifact is
  still right. **`kept ratify` refuses either artifact of a contested number**
  and names `kept doctor --fix`: ratification is the trap door, since an id is
  the ledger key and moving one afterwards strands its standing in an
  append-only record. Refusing *both* is deliberate — one sentence a person can
  hold beats a race. `release` is never refused; it is the way out. **`kept
  doctor --fix` renumbers**, two cases and one rule: the artifact that cannot
  move keeps the number — the one the ledger *names* where there is one (a
  release names an id too), else the one cited most.

  **A renumber carries every citation or it is not one.** Measured: a decision's
  id is written as 194 `target:` entries, 14 `[[…]]` links and 285 relative
  `](NNNN-….md)` links each labelled `[NNNN]`. The first two are what
  `promise:every-citation-in-the-corpus-resolves` is in force over; the third
  **kept does not parse at all**, so leaving those behind breaks references
  nothing reports. All four forms move. The rewrite replaces the old **slug**
  bounded by non-slug characters, plus the ordinal opening the artifact's own
  `title:` and H1 — never a bare number in prose, which is not a name. That
  bound is what keeps `--fix` inside
  `policy:nothing-kept-writes-rewrites-what-a-person-wrote`'s *identity and only
  identity*, and it is the first time `--fix` writes into a file other than the
  one at fault: widen it and the policy is broken rather than read. An
  `id-shape` problem is never renumbered — it is one file's authoring, and 0021
  makes an id a person's. See 0069.

- **Building the derived index.** There is none, and `internal/index` was
  deleted rather than left as a stub. Every verb walks the corpus and folds the
  ledger — `kept drift` is 11 ms over this corpus and 108 ms over two thousand
  artifacts — and the index's other two jobs are a link graph already resolved
  at load and a search verb that does not exist. 0044 names the two triggers
  that re-open it.

- **Asserting a confinement boundary in a temporary directory.** nono's
  baseline grants the operating system's temporary trees write access
  recursively, so a child confined to a box under `t.TempDir()` writes wherever
  it likes and every assertion still passes. It is measured, not theoretical: a
  path kept declared read-only was written from inside the sandbox and the
  suite stayed green. The rendered profile now denies those trees by name, and
  because denial beats a grant, `NewNono` refuses a box inside one outright —
  otherwise the confined child can reach nothing at all, every escape probe is
  refused, and the canary passes having measured nothing. So a box goes
  somewhere the baseline grants nothing, which is under the operator's home;
  a checkout is not that place, because a checkout may itself be sitting in the
  temporary tree. `nono why --path <p> --op write --profile <f>` says which,
  and it is how a probe location is chosen rather than assumed.

- **Reading confinement as the enforcement of a repair scope, or the provider's
  audit trail as the report.** A scope is a glob and a filesystem grant is a
  path, so a rendered `Profile.Write` is always at least as wide as the bound it
  stands for — `internal/gather` is the coarsest expression of
  `internal/gather/**/*.go` and there is no finer one. Measured in a real box: a
  write the grant admits and the glob does not **lands**, and so does a
  *delete*. Only a write outside the grant is refused. So the report is derived
  from the box's changed paths: `box.Change` reads the paths and the diff from
  one snapshot, `Change.Against(scope)` is the one classification, and
  `Review.Bounded` is what a land will refuse on. **Paths come from `git diff
  --name-only -z`, never from `+++ b/`** — a delete's header names `/dev/null`,
  which is exactly the change a scope review most needs to see.

  And the provider does not report the refusal. Measured with `--no-audit`
  removed: nono's audit trail holds three session-level events and **no
  filesystem event of any kind**, granted or denied; the denial is not on stderr
  and not in a `RUST_LOG=trace` log either, because macOS enforces in the kernel
  and nono is not in that process. `--audit-integrity` hashes only the
  **writable** set before and after, which is the set that is not the question.
  So `--no-audit` stays, and now for a measured reason: it would buy nothing and
  would durably record the entrypoint's full argv, unredacted, in a store
  outside kept's ledger that kept cannot bound or prune (0023). What the sandbox
  refused is therefore **not reported and cannot be**; the policy's subject is
  work that stays in the box, and a refused write is work that never happened.
  See 0059 and `docs/nono-surface.md` N77–N80.

- **Leaving a directory with no write bit standing longer than the thing it
  proves.** A directory at `0o500` will not let its contents be unlinked, so
  neither `t.TempDir`'s own removal nor an ordinary `rm -rf` can take the tree
  away: measured, `rm -rf` reports `Permission denied` on each file and
  `Directory not empty` all the way up, and only `chmod -R u+w` first clears it.
  Two places seal one deliberately and both are irreducible — `sealedCanary`
  must hold the seal for a whole confined session because tool-sandbox refuses
  an entrypoint whose parent the launching process can write, and
  `TestRealPiAcceptsTheProjectionAndWritesOnlyInsideIt` must hold it for the
  confined run whose `EACCES` is the assertion. So the rule is the window, not
  the seal: restore the bit the moment the evidence exists, and register the
  restore as a `t.Cleanup` or a `defer` as well, which covers a passing, failing
  or panicking test and nothing else. A test binary that is killed or times out
  runs neither, which is why the window is the part that matters.

- **Pointing a temp-directory demonstration at `TMPDIR`.** Go 1.26 roots
  `t.TempDir()` at **`GOTMPDIR`** when it is set, whatever `TMPDIR` says — the
  test process sees the machine's `TMPDIR` and writes somewhere else entirely.
  A run with `TMPDIR` pointed at a scratch directory therefore leaves it empty
  and proves nothing, which is the same shape as a canary that measured
  nothing. `os.MkdirTemp("")` still follows `TMPDIR`, so the two temporary roots
  in a suite are not the same root.

- **Aiming an escape probe at a path kept itself denied.** The canary's target
  has to be somewhere the profile never named: a probe against a directory kept
  created and then added to `filesystem.deny` demonstrates that a denied path is
  denied, which is a tautology dressed as evidence. Every canary sentinel now
  lives under the operator's home, the canary adds no deny entry of its own, and
  the parent proves it can perform each write itself before the child is refused
  it — a refusal from a merely unwritable target proves nothing either. A probe
  reports **reached**, **refused** or **absent**, and absent — the thing it
  probes is not there — fails the canary, because 0017 requires a probe that
  could not be attempted to count as a failure. Every probe has a test that
  breaks its property and watches it fail.

  **`origin_push` is the same rule for an instrument rather than a target.**
  `git push` exits non-zero for reaching origin and for having nothing to push
  from, and reading the second as the first is what that probe did: in a box
  that is not a git repository it exited 128 saying so, and the canary reported
  that a confined agent cannot reach origin. So the box is handed a real
  repository in its own temporary directory with a real origin under the
  operator's home, the parent pushes there itself first, and the probe checks
  its own premises — a repository, an origin, a commit — before it reads the
  push. A missing premise is **absent**, never refused. What no probe can
  separate is an origin that is refused from one that is not there: measured,
  git reports `does not appear to be a git repository` and exit 128 for both,
  which is why that question belongs to the parent. The **network** half of
  reaching origin stays with `host_outside_allowlist`; this probe's origin is a
  path, because 0017 puts the local repository there precisely so no credential
  reaching a remote exists in the box.

  **The instrument is a premise too, and its two streams are not one.** Apple's
  `/usr/bin/git` is an xcrun shim, it is the only git on a Mac without homebrew,
  and `credential_test.go` already names that path. Inside a box it writes
  `couldn't create cache file … (errno=Operation not permitted)` to stderr on
  every invocation, because — as the temporary-directory entry below records —
  Apple's `/usr/bin` wrappers find the per-user scratch tree through `confstr`
  and never read `TMPDIR`. Measured: it is **non-fatal**. `git --version` exits
  0 in a box, the premises exit 0, and the push that follows reaches origin and
  is refused there with git's own `does not appear to be a git repository`. So
  `refused` is the correct outcome under that git, and a probe rewritten to
  avoid it would report a non-refusal for a boundary that held. What the shim
  does break is a probe that reads a *value* out of combined output: `remote
  get-url origin` came back as two cache-file errors followed by the URL, and
  the record then named the shim's scratch directory as the origin. Read values
  from stdout, keep stderr for the detail, and prove `git --version` runs in the
  box before reading a push — a git that cannot start has measured nothing about
  origin, so it is **absent**.

- **Scanning an absolute path for a component named `.kept`.** A box is at
  `<repository>/.kept/boxes/<id>`, so every path in one carries that component
  and a scan refuses the whole design. The tier is located relative to the tree
  being checked. What must stay out of reach — the ledger, the artifacts, the
  `kept` executable — lies outside the box, and containment is what keeps a
  scope path away from it.

- **Inheriting the operator's environment into the provider process, or naming
  a toolchain policy group.** nono synthesizes the confined child's `PATH` from
  the `PATH` of *its own* process and ignores the profile's variable allowlist
  for it — there is no field and no flag — so `os.Environ()` in
  `childEnvironment` handed a repair every directory on the operator's search
  path while the rendered policy named none. The provider's environment is now
  constructed: the declared search path, plus `HOME`, which nono resolves its
  configuration and every `~`-rooted policy path against and refuses to start
  without. `HOME` and `PATH` are refused on the *child's* allowlist, because
  they are the provider's.

  A search path is resolution and not containment — a name on it grants nothing
  and a name off it forbids nothing — so kept renders no grant for those
  directories and no record may claim one contained anything. A toolchain is
  located by asking it (`go env GOROOT GOPATH GOMODCACHE GOCACHE`), and
  `groups.include` is rendered **empty**: with the resolved paths present a
  group only grants directories nobody resolved and the pre-flight never
  checked, and the eighteen-group deny baseline applies either way. What makes
  that safe is `nono why --json`, asked before a process exists about every path
  kept declared; a denial refuses the dispatch by name. See 0053, which is
  macOS and Go only and says so.

- **Leaving a box with no writable temporary directory.** The profile denies the
  host's scratch trees, so `TMPDIR` has to be granted or a toolchain fails
  wherever its own scratch file happened to be used. `internal/box` cuts one
  *beside* the worktree and discards it with the box — beside, because a
  temporary directory inside the tree puts every scratch file a build wrote into
  the repair's diff. On macOS this does not redirect Apple's `/usr/bin`
  wrappers, which find the per-user temporary directory through `confstr`: a
  cgo link fails, and names the wrong thing when it does.

- **Writing `network.allow_domain: []` and calling it no network.** That is the
  provider's spelling of *unrestricted*. `network.block: true` is none, and it
  wins over an allowlist standing beside it — so a profile blocks or names a
  host, never both. A build needs no network at all against a warm cache; that
  is an install-time need and no install capability exists.

- **Naming a host to reach an endpoint on loopback.** The allowlist runs the
  provider's proxy and sets `NO_PROXY=localhost,127.0.0.1` in the child, so a
  loopback request bypasses the proxy and is then refused at the socket:
  naming `localhost` grants nothing. `Profile.LoopbackPorts` is the lever, it
  stands beside `network.block: true`, and it grants **connect and bind** —
  macOS has no connect-only capability. **The pre-flight must not ask about
  it**: `nono why --host 127.0.0.1 --port <p>` answers `network_blocked`
  against the very profile that reaches the port, so asking would refuse every
  dispatch that works. What proves the boundary is the canary's existing
  `host_outside_allowlist` probe, which dials a port kept never named. See
  0055.

- **Declaring only the interpreter's directory for a Node-hosted runtime, or
  allowing `HOME` anywhere but `Profile.Home`.** `pi` is a Node script reached
  through `#!/usr/bin/env node`, and it has two locations: `ResolveNode` asks
  `node -p process.execPath` and follows the runtime's own executable up to the
  nearest `package.json`, so the interpreter's root *and* the package Node
  reads are both declared and both pre-flighted. Undeclared, a box reports
  `env: node: No such file or directory` or `MODULE_NOT_FOUND` — measured, and
  neither is a refusal. It works on this machine only because `/opt/homebrew`
  is baseline-granted; `npm install -g` under `/usr/local/lib/node_modules` is
  granted by nothing. `HOME` stays refused on the child's allowlist (the
  provider fills an allowed name from kept's own process) and refused in
  `ToolEnv`; `Profile.Home` grants the directory and sets the variable in one
  place, and an invocation may name `HOME` only with that exact value. See 0055.

- **Closing a runtime's input after writing the packet.** Pi's RPC session ends
  at input EOF: measured, that produced a clean exit 0 after the user message
  with no assistant turn, no tool call, no cost and no turn — a completed
  session that did nothing. `Invocation.BindInput` hands the adapter an
  `io.WriteCloser` and the adapter decides when the session ends; Pi closes on
  its own `agent_settled`. The launch closes the channel only when no adapter
  took it, and the supervisor's deadline still bounds one that never does.

- **Resolving a native runtime the way a Node-hosted one is resolved.** Claude
  Code 2.1.241 is a self-contained Mach-O image, not a script: the npm
  package's `bin` entry *is* the platform binary. `ResolveNode` does not fail on
  it, which is the trap — it succeeds, because the image happens to sit under
  the wrapper package's `package.json`, and it returns the **Node installation
  root**, a grant for a tree this runtime never opens, while requiring an
  interpreter a native install need not have. `ResolveNative` inspects the file
  instead and **declares nothing at all**: measured at a path the provider
  answers `path_not_granted` for, a full confined session is identical with the
  image's directory granted and with nothing granted, because the provider's
  command policy gives the launched entrypoint a direct exec bypass. So `nono
  why` is not a complete account of what a box can run. What resolution is *for*
  is the refusal: the same npm package ships a `#!/usr/bin/env node` fallback
  launcher for `--ignore-scripts` installs, and that shape is refused against its
  own interpreter line rather than launched into 0055's measured
  `env: node: No such file or directory`. See 0056.

- **Assuming a runtime reads `TMPDIR`.** Claude Code reads `CLAUDE_CODE_TMPDIR`
  and otherwise uses a hardcoded `/tmp` — `TMPDIR` is never consulted. The
  profile denies the host's scratch roots by name and denial beats a grant, so a
  confined session without it dies at `/tmp/claude-<uid>` with `EPERM`, exit 1
  and **not one stream event**. `Profile.TempEnv` names the variables `Temp` is
  published under, rendered from the one field that also grants the directory;
  an invocation may name one only with `Temp`'s value, and a name on the child
  allowlist or in `ToolEnv` as well is refused, because a variable with two
  sources has one value and two policies. See 0056.

- **Pinning an argv you never started.** The Claude argv was read off `--help`,
  fixture-tested and reviewed without a process ever accepting it, and it could
  not start one: `--print` with `--output-format stream-json` exits 1 without
  `--verbose`. `--replay-user-messages` is there for evidence rather than
  output — the runtime re-emits the message it read, so a receipt carries the
  runtime's own account of the packet even when the session is killed before an
  assistant turn exists. And `--safe-mode` is load-bearing rather than tidy:
  measured both ways in a box, a `SessionStart` hook planted in the tree under
  repair runs without it and does not run with it. Its built-in skills survive
  the flag either way, whatever its help says.

- **Reading a domain allowlist as a socket grant.** It starts a proxy and sets
  `HTTP_PROXY`/`HTTPS_PROXY` in the child. Measured from inside a box on the
  allowlisted host: a proxy-aware client gets the upstream's own reply, and a
  raw connection to that host's port 443 is refused. It is the same fact 0055
  records from the loopback side. The adapter still refuses those names in the
  *parent* environment — an operator's proxy is an unreviewed redirect of the
  credential — and the two do not collide, because kept never sets them and the
  provider always does.

- **Putting a credential where kept can read it, now that a phantom is
  proven.** The brokered route works and is measured end to end: nono resolves a
  reference kept never reads, publishes a session phantom in the variable the
  profile names, and rewrites the header on egress. Read at a loopback listener
  standing in for the upstream, the child sent `Bearer <phantom>` and the
  listener received `Bearer <the real value>`; a real Claude Code 2.1.241 session
  reached `result: success` through it. So the direct `CLAUDE_CODE_OAUTH_TOKEN`
  channel is no longer the simplest thing that works — it is the one that puts a
  live subscription token where a tool-capable agent reads it. Four facts are
  load-bearing and each was measured. **`network.block: true` and a credential
  route are refused by the provider as contradictory**, so a box with one names
  its host and cannot block. **A route with no domain allowlist reaches every
  host on the internet** — the empty-allowlist trap arriving by a second door.
  **The phantom is a capability**: a wrong bearer value or none is refused at the
  proxy with 401 and never reaches the upstream. And **`endpoint_rules` are
  default-deny**, measured with an unnamed path and an unnamed method each
  answered 403 without reaching the upstream, and a rule matching with a query
  string present. `internal/repair/confined_test.go` is all of it, and it needs
  no valid credential — which is why it can run. See 0057.

  **A 401 from `api.anthropic.com` is not evidence of a rewrite.** The upstream
  answers `OAuth access token is invalid` for *any* bearer value, measured both
  ways, so a phantom passed straight through is indistinguishable from a
  credential injected. Only reading the request at the upstream separates them.

  **kept cannot redact the phantom**, because the provider mints it and never
  says what it is — so a session that echoes its own environment into a tool
  result puts it in the receipt. It dies with the session and is useless outside
  its box, which is a smaller exposure than the direct channel's and is not
  nothing. Every test here uses a `file://` reference outside every grant, and
  the reason is not the approval dialog an earlier note recorded: resolving the
  real reference would pull a live secret into the test process.

- **Guessing which reference form the provider resolves, or which keystore it
  reads.** Both were settled by measurement and both are counter-intuitive.
  **`keyring://` does not parse on a credential route** — nono's own README
  shows it inside a worked `custom_credentials` entry and its parser refuses
  every spelling of it, so `credentialReferenceRefused` names it beside `env`
  and `cmd`. What resolves on macOS is `file://<absolute path>` and
  **`apple-password://<server>/<account>`, which reads the login keychain's
  *internet* password class and no other**: the provider runs `security
  find-internet-password`, so a generic password stored under the same two
  names resolves to nothing. That failure is silent — the launch exits 0, the
  phantom is set but empty, and the child gets a 503 — because nothing
  pre-flights resolution the way `nono why` pre-flights a path. The guide's
  `apple-password://account/name` ordering is wrong and the binary's error
  string is right. Re-store with `security add-internet-password -U -s <host>
  -a <account> -w`, where a valueless `-w` prompts twice rather than putting the
  token in `argv`. Contrary to the note this replaces, **no approval dialog is
  raised** for an item the `security` CLI wrote: it carries partition id
  `apple-tool:`, which admits the Apple-signed binary nono invokes. See
  `docs/nono-surface.md` N11, N57 and N74.

- **Reading a runtime version constraint as a pin, or bumping one by editing a
  number.** An adapter states a **review record**: the versions somebody
  examined, plus the `major.minor` line they sit on where its evidence supports
  a neighbour. Claude declares the `2.1` line; Codex and Pi declare none, which
  means their reviewed set is the whole of what they accept. `repair.runtime_versions`
  in the operator's own config is `reviewed` (the default, and what an unset
  value means) or `line`, and there is deliberately no member that accepts
  anything — off the line, older than the oldest reviewed, or unreadable as
  three numbers refuses under every policy.

  The reason it is a review and not a measurement is measured. The constraint
  stands for two properties with **opposite failure modes**: the direct stream
  shape fails *loudly* (a flag that no longer parses exits before a session
  exists), and the documented credential precedence fails *silently* (an
  unrefused parent variable aims the credential elsewhere and the session looks
  fine). An exact pin never delivered the silent half — every credential-bearing
  name `claudeRefusedEnvironment` omits is read by **2.1.241 too**, the build the
  pin was taken against — so widening it loses an illusion rather than a
  guarantee. A probe cannot replace it either: the loud half already announces
  itself, and establishing where a credential goes means spending a live request,
  which 0052 forbids. Open question 12 is the gap; 0072 is the account.

  Two things follow. **Adding a reviewed entry means reading the binary**, and
  what was checked goes in the comment beside it — argv against `--help`, the
  stream tokens `claudeStream` projects, and a `process.env` diff against the
  previous entry, which is only possible because there is now more than one.
  And **the adapter version stamped on every native event is composed from the
  version that ran**, carrying `+unreviewed` when nobody read it; it is computed
  by the same call that admits, because a second computation would disagree in
  the direction that writes a receipt claiming a session that did not happen.
  `ClaudeVersion` is the newest reviewed version, not the only accepted one —
  a guard comparing against it retires evidence the day a newer entry lands, so
  ask `ClaudeSupports`.

- **Admitting a runtime because another one is admitted.** `piAdmitted` and
  `claudeAdmitted` are true and `codexAdmitted` is false: 0052's gate is per
  adapter, and each one needs its own confined interrupted live proof. Pi's is
  `internal/confine/piinbox_test.go`; Claude's is
  `internal/repair/claudeadmission_test.go`, whose header is the account of every
  session that was run and what each one settled.

  **What admitted Claude was not the credential and not confinement.** Eight
  confined sessions with the operator's own brokered credential met the whole
  launch half of 0052's table — packet replayed, projections naming native
  events, a killed session keeping its partial receipt, timeout cause and reaped
  signal — and repaired nothing. Every clean session located the fault from the
  packet alone, file, line and enclosing function, wrote a correct account of
  why the promise does not hold, and stopped without editing. A shell in the same
  box appends to the file under repair, so it was never the write boundary.
  **A packet states authority and evidence and never an ask (0009), and this
  runtime launched with `--print` reads one as a question.** [0058](docs/decision/0058-the-ask-is-declared-and-travels-beside-the-packet.md)
  is what closed that: the adapter receives a declared **ask** and renders it on
  a channel that is not the packet's — for Claude, `--append-system-prompt`
  appended to 0052's pinned argv. Three sessions later, with nothing else
  changed, a confined Claude removed the offending line and the promise
  re-derived from the box read `kept`.

  **The ask is a value from a closed set, never a string a caller composes.** It
  names no promise, no file and no evidence, it is byte-identical for every
  packet, and `claudeInput` still puts exactly the packet's bytes in the
  user-message channel with nothing prepended. A second kind of work — the
  assessor, which judges whether a repair held and needs the opposite framing —
  is a second value and a second constant beside the first, never a template, a
  registry or an operator-supplied prompt. And the ask is a property of the pair,
  this runtime and this kind of work: Pi needs none today and Codex is
  unadmitted, so neither has one.

  Three measured details are worth carrying. **A killed session may retain no
  transcript at all**: this adapter's transcript is assistant *text*, and a turn
  spent on tools emits `input_json_delta` and `tool_use` and no text — so the
  incremental evidence a kill keeps can be turn updates alone. **A framed session
  is faster than an unframed one**, 30 and 90 seconds against 37–67, which is why
  the interruption proof's deadline moved to fifteen seconds: a proof whose
  deadline the session beats is not one. And **the argv now carries prose**, so
  the no-shell test's metacharacter rule exempts the ask by name and holds it to
  a stronger rule instead — it is a reviewed constant, one argv element, and it
  does not vary with the packet.

- **Copying one adapter's parent-environment refusal onto another.** Each
  adapter's preflight reads `os.Environ()` for names only, to *refuse* — never
  to obtain a credential (0052) — because `Invocation.Env` is a complete child
  environment and a confinement provider that merged instead of replacing would
  hand the child a credential, a configuration or an upstream kept never chose.
  The value of such a list is entirely that it was read off **that** runtime, so
  a name goes in only with its source recorded beside it, and the exclusions are
  tested as hard as the refusals. Adapting Claude's list to Pi would have been
  wrong in both directions, measured: `ALL_PROXY`/`all_proxy` are refused for
  Claude and guard **nothing** for Pi, whose global dispatcher is undici's
  `EnvHttpProxyAgent` and reads only the http/https pairs — **lowercase first**,
  which Pi's own documentation never mentions; and Claude's `ANTHROPIC_`
  namespace rule has no `PI_` counterpart, because a live Pi session exports
  `PI_CODING_AGENT` to every child and its bash tool five more, so a namespace
  refusal would refuse every dispatch made from inside one. `HOME` is set by
  kept for the child and is never refusable in the parent: it is on every
  machine. Two Pi facts worth carrying past the comment: **`--no-extensions`
  does not remove the built-ins** — `dist/main.js` composes
  `[...builtInExtensions, ...]` unconditionally, so the llama.cpp extension and
  its `LLAMA_BASE_URL`/`LLAMA_API_KEY` reads are always loaded — and **a literal
  grep over Pi's bundle under-reports**, because names like
  `PI_CODING_AGENT_SESSION_DIR` are composed at runtime from `APP_NAME`. The
  membership, each name's source and every exclusion's reason are in the
  `piRefusedEnvironment` and `claudeRefusedEnvironment` maps -- not in the
  `…RefusedParentEnvironment` functions beside them, which only scan and sort.

- **Putting a proof's own answer key in the box it measures.** A repair box is a
  copy of a repository, and this repository contains the tests that author the
  breaks those repairs are measured on. The first confined Claude repair grepped
  for the marker, read `internal/repair/claudeadmission_test.go` out of its own
  box, wrote "this is the injected violation" and removed the line — a repair
  that measured reading rather than repairing, and afterwards nobody can tell
  which happened. `keptRepairRepository` now leaves that file out and scans what
  remains, and `TestTheScanThatKeepsTheAnswerKeyOutOfTheBoxCanFindOne` points the
  scan at this checkout, where the file really is, so it is a scan that has found
  something. **The same rule one step out**: it also leaves out decision 0058,
  which names no break but is kept's own account of what a session carrying a
  packet is expected to do — a subject that reads it learns it is the subject of
  a proof and what the proof hopes for. Commit the break rather than overlaying it, too: the one added line
  would otherwise be the only uncommitted change in the tree and `git diff`
  inside the box points straight at it.

- **Folding `the-confinement-suite-passes` back into `core-packages-are-tested`,
  or giving it a `repair` block.** `internal/confine` is measured by its own
  promise, on the same scope and the same cadence as every other package under
  `internal/`, and repaired by nobody. The mechanism is that it writes **no
  `repair` block** — a promise that bounds nothing authorizes no repair, and
  `internal/repair` and `internal/thread` each refuse to open a thread for one.
  There is no flag and no second mechanism, so the guard is
  `TestTheConfinementPromiseAuthorizesNoRepair`, which reads the artifact out of
  this repository's own corpus. The reason is not that a box cannot nest a
  provider: those tests cut their boxes and plant their canary sentinels **under
  the operator's home**, because the provider's baseline grants nothing there,
  and a repair box that granted a confined agent that place would dismantle what
  the suite measures. Open question 11 is the account.

  **`go/tests` takes an `exclude`, and it is not cosmetic.** Go package patterns
  cannot negate, so `./internal/...` minus one package is expressed by
  `exclude: "./internal/confine"`; the gatherer resolves both sides through `go
  list` and **refuses an exclusion that removes nothing**, so a rename reads
  `unknown` rather than quietly running the suite it was written to keep out. Do
  not enumerate the remaining packages instead — a list in a binding field goes
  stale silently, which is the failure `count(obs.tests.packages, p => p.ok) > 0`
  already exists to refuse. The repair bound stays `internal/**`: a glob cannot
  negate either, and what keeps `internal/confine` out of a repair is the box's
  re-derivation measuring the narrowed promise.

- **Re-deriving a verdict over this whole corpus inside a box.** Three promises
  shell the Go toolchain into the box: `core-packages-are-tested` runs `go test`
  over `./internal/...`, which re-enters this suite and would dispatch another
  live session on the operator's credential; `the-confinement-suite-passes` runs
  `./internal/confine`, which cuts more boxes and runs more canaries; and
  `the-tree-vets-clean` runs `go vet ./...`.
  `internal/repair/claudeadmission_test.go` leaves all three out of the corpus it
  copies, which also takes the re-derivation from minutes to about 90 ms.
  **`internal/confine/piinbox_test.go` still copies the whole corpus** and has
  the same hazard — its `verdict` is a `tick.Tick` over everything the box holds,
  not a `tick.Measure` of one artifact; it is unfixed because changing it needs
  Pi's live proof re-run, which needs the self-hosted endpoint 0052's Pi evidence
  was measured against.

  **A `kept land` in this repository is the same hazard, doubled.** 0037's gate
  re-derives every standing artifact whose *gather* scope the change touches,
  and it measures each one **on both sides of the merge** — so the cost is two
  passes, not one. Measured against this corpus: `the-tree-vets-clean` scopes
  `**/*.go`, so **any** Go change pulls in `go vet ./...` twice, and a change
  under `internal/**` adds `core-packages-are-tested` (`go test ./internal/...`,
  which re-enters the live admission suite) and `the-confinement-suite-passes`
  (`go test ./internal/confine`, which cuts more boxes and runs more canaries) —
  each twice. Use the `GOFLAGS` skip list below, and expect a land here to cost
  minutes rather than seconds.

  **Both sides is not optional.** *A collateral break* is a promise that held on
  the target and does not hold with this change in it. A gate that measured only
  the merged tree would refuse every land into a repository that already has a
  broken promise, which reports the state of the target rather than the effect
  of the change — and only `kept → not-kept` is a break, because reading
  `unknown` as one is the lie the three-valued model exists to prevent, one
  level up from the instrument.

  **A box is not the only place this bites: any `kept tick` does it**, on a
  scratch clone as readily as in a worktree, because the gatherer inherits the
  environment and `config.ReadUser()` finds the operator's real credential
  wherever it runs. So a tick taken to check corpus verdicts silently dispatches
  the live admission sessions again — **and so does a plain `go test ./...`**,
  which is the same hazard without a tick in front of it. The names, because
  "the live test names" is a sentence that costs a session to resolve:

  ```
  GOFLAGS=-skip='TestAConfinedClaudeSessionCarriesThePacketAndProjectsItsOwnEvents|TestConfinedClaudeRepairsABrokenPromiseAndTheVerdictIsReDerived|TestConfinedClaudeKilledMidSessionRetainsItsPartialReceipt|TestARealConfinedClaudeIsAnsweredByTheRealUpstreamThroughTheRoute'
  ```

  The first three each dispatch a real Claude session on the operator's
  credential — measured at 32 s, 63 s and 18 s. The fourth reaches
  `api.anthropic.com` through the provider's proxy and is answered 401, so it
  costs nothing, and it is skipped with them because a network call from a suite
  is a network call. Pi's live proofs skip on their own here: they need the
  self-hosted endpoint. `go test` honours `GOFLAGS` through the inherited
  environment and `go vet` ignores it, so `the-tree-vets-clean` is unaffected —
  measured. Do not reach for a fake `HOME` instead; it also hides nono's own
  configuration, which the confine tests need.

  **This list is the only mitigation again.** While the adapter pinned one exact
  patch and the installed build was newer, all four skipped on the mismatch and
  the drift was an accidental second guard. 0072 admits the installed build, so
  they are re-armed: set `GOFLAGS` before a tick, a `go test`, or a land that
  touches `internal/**`.

  When a tick after such a run still reports drift, read it before repairing it:
  a verdict that did not change and is younger than `max_age` appends no truth
  record, so `kept drift` keeps reporting the old world. That is the heartbeat,
  not a defect — compare against `main` before concluding a branch caused it.

  **`kept repair` re-derives with `tick.Measure`, which is one artifact and no
  ledger at all.** Not `Tick.Run` with a scratch store: that measures the whole
  corpus, which is this hazard, and opening a store is itself the shape
  `promise:standing-is-minted-in-one-place` refuses — see below.

- **Calling `ledger.Open` anywhere under `internal/`.** A package that acquires
  a writable ledger of its own has stopped being handed one by the command a
  person typed, and `promise:standing-is-minted-in-one-place` refuses the shape
  by name — the reference, not the write. It caught `internal/repair` on the
  first tick of a scratch clone, where the box re-derivation had opened a
  scratch store to satisfy `Tick.Run`. The answer was not a narrower scope: it
  was `tick.Measure`, which gathers one artifact, applies its predicate and
  writes nowhere, which is what a verdict reached in a box should do anyway
  (0035). `_test.go` is outside the scope, which is why the package's tests may
  still open one. Same rule for `ledger.Ratify`.

- **Breaking `gatherers-do-not-write` in kept's own tree to try something.** It
  takes `core-packages-are-tested` down with it, and the reason is not obvious
  from the failure: `internal/repair`'s `TestTheBreakIsRealAndItsRepairIsOneLine`
  asserts that removing *its* one injected writer call mends the promise, and a
  second writer call anywhere in `internal/gather` means it does not. Break it in
  a scratch clone, or expect two red promises rather than one.

- **Cutting boxes from the landing target, or reading the anchor as one commit.**
  **The fix was never the cut, it was the read**, and the read is built: 0015's
  third deployment constraint — *boxes are cut from `origin/main`* — is
  **retired and delivered**, and nothing should propose it again. It removes the
  symptom and costs more than it buys, because it measures a repair in a tree
  the operator is not standing in and forecloses a dirty repair permanently.

  `box.Merge` states the anchor's parent as the merge base
  (`git merge-tree --write-tree --merge-base=`) and commits the result with the
  target's tip as its **only** parent, so an operator's unlanded commit and the
  snapshot under a dirty repair both drop out while the target's own commits
  stay. Two things follow that a reader will otherwise re-derive. **The gate's
  path selection is only the wider set because the merge states this same
  base** — while git computed it, an unlanded commit touching a promise's repair
  bound landed without that promise ever being selected, which
  `TestNothingLandsThatTheGateDidNotSelect` breaks and watches fail. And **a
  merge that leaves the box's own tree commits nothing**: landing the same
  anchor twice is a no-op rather than an empty commit, and the test that catches
  a regression there is 0018's own
  `TestALandRunAgainAfterACrashRecordsWhatIsAlreadyOnTheTarget`, which needs the
  re-run to record the act the crash lost. See 0068.

- **Looking for a `repaired` act in the ledger.** There is no such kind. A
  repair writes a **disposition** whose outcome is `act`, and it is written when
  the session returns rather than after the re-derivation, because a session that
  ran and cost a credential must be in the record either way. `Repaired` and
  `Unrepaired` are `repair.Result.Status`, in memory and on stdout, and nothing
  persists them: what durably survives a repair is that disposition, the receipt
  at `ledger/threads/<id>.jsonl`, and the anchor ref. `landed` is the only act a
  repair's own path can produce, and only a land produces it — so *did that
  repair hold?* is answered by re-deriving from the anchor, never by reading a
  verdict back. And **`kept land` takes the commit from the receipt's anchor, not
  from `refs/kept/threads/<id>`**; the ref is the queue, which is enumeration.

- **Putting a lock anywhere but the shared Git common directory, or letting a
  land trust `remote.origin.fetch`.** Both were measured here and both fail
  silently in the direction that looks fine.

  A lock beside a worktree **excludes nobody**: `internal/lock` resolves
  `git rev-parse --git-common-dir` — absolute from a linked worktree, relative
  from the primary checkout, so it is joined against the repository — and pointed
  at `--git-dir` instead, two worktrees of one repository get two lock files and
  every exclusion test passes while proving nothing. `flock` lives on the open
  file description, so it releases on process death (no TTL, no reclaim) and two
  opens in one process exclude each other, which is why the tests need no second
  binary. The file is never unlinked on release: another run that already opened
  that path would hold a lock on an unlinked inode and exclude nobody.

  And **a `remote.origin.fetch` naming the ledger and no branch writes no
  `refs/remotes/origin/*` at all**, so a land reading that ref would be reading
  one nothing had maintained. It was this repository's own state until
  2026-08-25 — the ledger's refspec as the sole entry, `git fetch origin`
  exiting 0, and `refs/remotes/origin/main` eight commits stale — because 0062's
  line was added where there had been none. `kept init` now configures
  `+refs/heads/*:refs/remotes/origin/*` beside it and `kept doctor` reports a
  workspace missing one; **an existing branch refspec of any width is never
  removed, narrowed or widened**, and nothing is written where
  `remote.origin.url` is unset. A remote an operator narrowed is still theirs,
  so `internal/land` states its refspec in full, forced, from the `Target`
  value — which is what keeps 0015's in-lock check reading a tracking ref it
  refreshed itself. **The landing target is carried as a value
  and not spelled out at each site**: only `land.Origin()` names `origin/main`,
  and the ref, the tracking ref, the refspec and the push destination are all
  derived from it. **A target with no remote is a local target**, so `Tracking`
  and `Refspec` are empty for one and `Check` reads `refs/heads/<branch>`
  instead — the fetch, the in-lock read and the write all dispatch on
  `Target.Shared()` and nowhere else. `--onto` resolves through
  `land.ParseTarget`, which reads the first segment as a remote **only when this
  repository configures one by that name** and takes `refs/heads/<name>` as the
  escape hatch for a branch that would otherwise be read as one; with no
  `--onto`, `land.ResolveTarget` is git's own answer to *where does my work go*
  — this branch's upstream, else this branch, and a detached HEAD is not a
  branch and says to name one.

  Two smaller ones, both measured. **`git merge` cannot read a message from
  standard input** — `-F` takes a file and refuses `-`, exiting 129 with `could
  not read file '-'` — so a merge message is an argv element, which is safe here
  because kept composes every one of them and each begins with a fixed word.
  And **`git config --get user.name` outside a repository answers from the
  machine's global config**, so a test asserting that kept refuses to invent an
  identity has to take `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_SYSTEM` out of the
  way or it passes or fails depending on whose laptop runs it.

- **Simulating a land's crash by not calling the function that records it.**
  That proves the order of two lines in one file; 0018 claims *survivability*,
  which is a property of what a killed process leaves behind. The window is held
  open by the thing kept does not control: a `post-update` hook on the bare
  remote runs **after** the refs are updated and `git push` does not return
  until it exits — measured — so a hook that reports and then waits puts a land
  in exactly one state, the write done and the record impossible.
  `internal/land/crash_test.go` re-executes the test binary through `TestMain`
  as the process that lands, waits for that marker, kills the process **group**
  so the push and the remote's blocked hook die with it, and then reads the
  remote, the ledger and a real tick rather than the crashed run's own account.
  Two things it measured are worth carrying: **a crash leaves no run record at
  all** (`declare()` writes diagnostics on the first append and a land appends
  nothing before the push, so *no `runs/{id}.json`* is the ordinary evidence
  here rather than one with an empty `Ended`), and **a crashed land leaves its
  gate box registered as a git worktree** with nothing to prune or report it —
  0018's sweep section, unbuilt.

  **The worktree arrival's third state is a partial write, and it is produced
  rather than staged.** There is no external process to block inside — the write
  is a handful of renames — so the interruption comes from the write itself: a
  directory standing where the second file has to go is a rename the code cannot
  perform, and it fails with the first file already on disk. What makes that
  survivable is what makes the ref arrival's lost record survivable, and it is
  the same argument: nothing reads the ledger to answer the question. A second
  run snapshots the tree it *finds*, merges into that, and writes only what is
  still missing —
  `TestAWorktreeArrivalInterruptedPartWayThroughIsFinishedByRunningItAgain`.
  Note what that means for the swap: after an interruption the tree no longer
  matches the digest the interrupted run was gated against, so a run resuming
  from the old digest would read the tree as torn. It does not, because the
  digest is taken per attempt from the tree in front of it; a *concurrent*
  editor is what `ErrTorn` catches, and a crash is what convergence handles.
  Do not build a second detector for the first case out of the second one.

  **The window between the write and the record is held open at whichever end
  has something outside kept in it.** A ref arrival has the remote, above. A
  worktree arrival has neither remote nor external process — the write is a
  handful of renames — so the barrier goes on the far side instead: the act
  stream is replaced with a **named pipe nobody reads**, and the append that
  records the land blocks in `open` and can never complete. It is absolute
  rather than timed, which is what makes a kill land inside the window every
  time. Two details are load-bearing. The barrier makes the stream *unreadable*
  as well as unwritable, so it may only go in after the run has read the acts —
  hold the land lock and wait for the child's `take the land lock` phase, which
  is exactly that point. And **wait for `runs/{id}.json` rather than for the
  file the change carries**: `declare()` runs immediately before the blocked
  append, so a new run record is the moment the record was reached, whereas the
  arrived file is only the moment the write began, and a kill on that signal
  lands before the record often enough to flake — one run in five, here. That is also the entry above corrected:
  *no run record at all* is the ref arrival's evidence because a land appends
  nothing before the **push**; on this side the crash leaves one, with `Ended`
  zero, `Exit` empty and `Acts` zero.
  `TestACrashBetweenTheWorktreeWriteAndTheRecordLosesTheRecordAndNotTheChange`.

- **Testing a land's concurrency with sleeps, or expecting two worktree
  arrivals to both land.** The land lock is the only place in `Perform` where
  one land waits for another, so it is where an interleaving is *driven*: a test
  holds it, starts both lands, waits for each to print `take the land lock` —
  the plain `internal/progress` form, which is what anything that is not two
  terminals gets — and releases. Both gates then provably ran against one
  commit, because nothing could write the target while it was held, and the
  loser's first gate provably passed, because the gate refuses *before* the
  lock. Nothing sleeps for a race to happen.

  **Which pairs are real is three answers, not one.** Two ref arrivals onto one
  target is the pair the queue exists for, and both land — the loser re-gates
  unconditionally and its commit has the winner's as its only parent, disjoint
  scopes included (0015's withdrawn skip, pinned by the assertion on
  `Attempts`). Two worktree arrivals into one tree **cannot both land and are
  not meant to**: the target's ref never moves under a worktree arrival, so the
  in-lock check sees nothing and what catches the collision is 0033's digest —
  the second is `ErrTorn`, writes nothing, and is landed by running it again.
  A ref arrival racing a worktree arrival onto one target dies at the arrival
  table asked again inside the lock, which is
  `TestATreeThatGoesDirtyUnderAPlannedRefArrivalRefuses` and needs no second
  test. And **prove a re-gate by its answer changing, never by its having
  run**: two threads each adding one artifact to a promise that wants exactly
  two both pass alone, and the loser's re-gate reads `kept → not-kept` and
  refuses. A re-gate that re-merged without re-deriving passes every other
  assertion — including the push, which fast-forwards — and puts a broken
  promise on the target.

- **Pointing the ledger's fetch refspec at the ledger.** `FetchRefspec` is
  `+refs/kept/ledger*:refs/kept/upstream/ledger*`, and every character of it is
  load-bearing. **Git gives every branch two refs** — an upstream copy a fetch
  overwrites freely, and a local one only an explicit act advances — and the
  ledger had one, which made the destination of a network fetch the durable
  record itself. Forced, that erased two ratifications on 2026-08-22 during an
  ordinary `git fetch` a worktree pool ran on its own; unforced, git *refuses*
  that fetch, and a ledger ahead of its remote is the resting state of this
  system, so the pool then hands out no worktree at all and no task can spawn.
  With a single-ref source it is worse still on a fresh adoption: `git fetch`
  exits **128** with `couldn't find remote ref` before anything has ticked. All
  three are one defect and the upstream destination removes it — measured, an
  ordinary fetch exits 0 in every state and the ledger ref is untouched in every
  one. **The `+` is right here for the reason it was wrong there**: a tracking
  ref is a copy of what the remote holds and git forces every one of them. **The
  `*` is not decoration** — a single-ref refspec whose source is absent is
  `fatal` and a pattern that matches nothing is a no-op, and that is the whole
  of the fresh-adoption fix. `ConfigureFetch` removes *both* old spellings —
  forced and unforced — rather than adding beside one, because git applies the
  first refspec that matches and `config --add` puts the old line first;
  `init` says what it removed, and `doctor` reports one nobody has corrected.
  See 0062, which amends 0016.

- **Pushing the ledger for the operator, or dating `doctor`'s comparison against
  origin.** `git push` does not carry a side ref, so the symmetric failure to
  the fetch one is a ratification that never leaves the machine — measured here
  on 2026-08-25, three ledger commits unpublished for a day with `git status`
  clean. The answer is a **report**: `doctor` compares `refs/kept/ledger` against
  `refs/kept/upstream/ledger` in the outbound direction and hands over
  `ledger.PushCommand` verbatim, unforced, because a push would be the first
  network call kept ever makes and `kept ledger sync`'s help states that
  boundary. **The wording may claim only what a local ref supports** — *origin
  did not have these when this repository last heard from it*, never *origin
  does not have these* — and the test that keeps it honest makes the second
  false by publishing to a real origin and rewinding the local upstream ref.
  **A date is not knowable and none is printed**: measured, git writes a reflog
  entry for that ref only when it *moves*, so a fetch bringing nothing new
  leaves no trace, and `refs/kept/` is logged at all only under
  `core.logAllRefUpdates: always`. Three states are silent — no origin, no
  ledger, and a fork, which the divergence finding owns — and *no upstream value
  at all* is **not** *you are ahead*: nobody asked, so it says so and names the
  `git fetch` that separates *nobody has fetched* from *origin holds no ledger*.
  Nothing refuses any of it and the exit code is untouched. See 0062 and 0050.

- **Making a read verb advance the ledger, or letting a writing verb append
  without advancing.** A fetch writes `refs/kept/upstream/ledger` and kept moves
  the ledger onto it: `ledger.Advance` fast-forwards when the upstream is
  strictly ahead, refuses a fork, and is called by `tick`, `ratify`, `release`
  and `repair` — never by `status`, `why`, `drift`, `list` or `doctor`, because
  a reader that writes cannot run against a read-only checkout (0001, 0032). The
  other direction is the one that is easy to miss: **appending onto a ledger
  that is behind forks it**, so a clone that fetched and then ticked would
  manufacture a divergence out of nothing. Git makes you merge before you can
  push for the same reason. A fork is therefore what a fetch discovers, never
  what a write creates.

  `kept ledger sync` is the whole sync path and it is deliberately small: it
  **does not fetch** (kept makes no network call, and `git fetch` is now safe),
  **does not push** (had *every run pushes at exit* existed, the first tick after
  the 2026-08-22 rewind would have published the damaged ledger to origin as a
  clean fast-forward, overwriting the last good copy), and **does not merge** — a
  fork is refused at exit 4 naming both tips, because the answer for an
  append-only record is a union commit a person makes, and a merge kept composed
  could lose the one file in this system that is not reproducible. A fork
  refuses that verb and nothing else: every other verb keeps answering and a
  writing verb keeps writing, with a warning. `doctor` reports *behind* and
  *forked* offline and says nothing about *ahead*, which is the resting state.
  See 0062.

- **Rebuilding the ledger's witness, or trusting `core.logAllRefUpdates: true`
  to be one.** Git logs a ref update only for a hardcoded set of prefixes, and
  `refs/kept/` is not among them; **`always` lifts that and `true` does not**,
  which is measured on both the `files` and `reftable` backends and is the fact
  the whole mechanism rests on. `ConfigureFetch` sets `always`, `doctor` reports
  a repository where it is not, and every verb that reads or writes the ledger
  refuses at **exit 4** — naming the value to recover — when the reflog shows a
  value the ref can no longer reach. Do not build a high-water mark, a witness
  file, an act count or a hash chain instead: git writes this one during a `git
  fetch`, when kept is not running, with the reason attached, and its entries
  anchor the lost commit against `gc`. The rule is over git's DAG rather than
  over the ledger's contents, because the failure replaces the ref rather than
  editing the records — which is also why it self-clears under a union commit
  and only under one.

  Two things are easy to get wrong here. **Read both ends of the reflog**:
  `reflog show` gives each entry's *new* value, and the value at risk in a
  repository adopted while it already had a ledger survives only as the oldest
  entry's pre-image, `Ref@{n}` where n is the entry count — reading only the new
  values reports that rewind as clean. And **report the lost heads, not every
  lost value**: the ref moves several times per act, so one rewind strands a
  chain, and `merge-base --independent` reduces it to what a person must
  actually union back.

  It makes loss **survivable, not impossible**, and every verb that mentions it
  says so: deleting the ref takes its reflog with it, `.git/logs` is deletable,
  entries expire at `gc.reflogExpire` (90 days), and a clone is unprotected
  until `kept init` runs there. See 0054.

- **An artifact's identity is authored, and absence is not an act.** The `id`
  in frontmatter is the identity; the filename is not, so renaming a file
  changes nothing. A ratified artifact whose content has vanished is *absent*
  drift — it keeps standing and says so. Never infer a release from a missing
  file: it cannot be told apart from a bad merge or a misconfigured corpus
  root. See 0021.
- **The hash normalizes before it hashes.** NFC, LF, no BOM, no trailing
  whitespace, one final newline — so a CRLF checkout does not silently
  de-ratify the corpus. The rules carry a version in every ratification. See
  0022, and 0031 for the preimage they feed: it is frozen at `norm:1`, so
  changing the encoding, the sort order, or what a reserved `hash` name
  resolves to is a version bump and never an edit.
- **Only what an artifact's own author wrote binds.** A `_links` manifest edge
  never does, or a third party could change what a ratification was granted
  against by writing a file next to the artifact — and the repair agent may
  write a manifest, because a manifest is corpus. See 0030.
- **Nothing external runs without a deadline.** Gatherer, agent, git, canary,
  lock. A timeout resolves to the same outcome as the matching failure, and is
  recorded as a timeout so the two can be told apart. See 0026.
- **Pointing a runtime at the operator's own configuration**, or projecting all
  of it. A runtime reads a *projection*: `internal/runtimestate` copies the
  selected credential entry, the selected provider and model, and settings it
  authors, into a fresh owner-only directory discarded with the box. It refuses
  an OAuth entry, a symlink, an inline key, a `!command` value, an overlap with
  the box, and any endpoint that is not loopback or a private address. The
  handle's type is the check, because an adapter that took a path could be
  handed `~/.pi`. **Read-only is not the safer version** — a runtime that locks
  a file to read it fails on a read-only copy, or authenticates with nothing and
  does not say so. And the credential never enters the projection: the file
  holds a reference, the child environment holds the value, which is what keeps
  0052's two credential channels the only two. Under the *brokered* one there is
  no projection and no value at all: `internal/repair` assembles a reference and
  the credential never enters kept's process. See 0057.

## Conventions

- US spelling throughout, prose and identifiers alike. `artifact`, not
  `artefact`; `-ize` where both are current.
- Every guard gets a test that deliberately breaks the thing it is supposed to
  catch. A check that has never fired is not a check.
- An architectural change gets an ADR before the code. Copy the shape of an
  existing one: context, decision, real alternatives, consequences including
  the costs accepted. `## Context`, `## Decision` and `## Consequences` are
  what `_schema/decision.yaml` declares; `## Alternatives` and anything else
  are undeclared sections, which are legal and never bind (0040).
- **A decision's number is claimed by whoever reaches `main` first.** Two
  branches cut from the same commit both pick the next free number — `kept new`
  mints the highest plus one, which finds the same free number on both — and only
  one of them is right, so rebase before assuming yours survived: renumber the
  file, the `id`, the title and every citation of it. `kept list` reports a
  contested one as `duplicate-number`, a diagnostic rather than a load error,
  because the slugs differ and the map from id to artifact is still right. That
  is cheap **only
  while the decision is unratified** — the `id` is the identity a grant binds
  (0021), so renumbering a ratified one drops its standing and costs a
  re-ratification. Land the ADR, then ratify; never the reverse.
- **A decision is revised, superseded, or added to — and the hash says which.**
  The old rule was *superseded, not edited*, written when nothing could tell
  whether an edit changed what was decided. `body.decision` binds, so now
  something can, and there are three cases rather than two:

  | | |
  |---|---|
  | adding a consequence, context, an alternative | edit freely; standing survives, because none of it binds |
  | **changing what was decided** | edit, and standing **drops**. Re-ratify: a person grants the new wording, and the ledger holds both hashes with a date between them |
  | **replacing the answer** | supersede, and record the relation. The old reasoning stays readable as what was thought at the time |

  Revise when evidence corrects the operative detail of a decision whose answer
  still stands — 0017's provider rules under measurement is the worked case.
  Supersede when the answer itself changes, because leaving a ratified decision
  standing with instructions now known to be wrong is what that rule was written
  to prevent, and two documents where one is true is how it happens. **Never
  edit what was decided without re-ratifying**: the edit is not the act, the
  grant is.
- **A decision carries no status line and no `status:` field.** Accepted-ness
  is standing: `kept status` is what says whether a decision is in force, and a
  second place for that answer to live is the cache that can disagree with its
  source. See 0041.
- **A `statement:` binds only under `fields:`, and a top-level one is
  decoration.** `canonical.resolve` reads an ordinary hash-list name out of
  `Artifact.Fields`, and `Fields` is populated from the `fields:` block alone —
  so a decision whose `statement` sits at the top level hashes as though it had
  none, and rewording it changes nothing. Measured both ways on 0015: the hash
  does not move for a top-level statement and does move for a nested one, and
  `body.decision` binds while `## Alternatives` does not.

  **It is reported now, and the rule is narrow on purpose.** `corpus.Misplaced`
  is the one owner — a top-level key that is not reserved and that the kind's
  *own schema declares as a field* — and two callers ask it: the load, which
  raises a `misplaced-field` problem naming the key, the file and `fields:`, and
  `kept show`, which used to label such a key *nothing declares this* and now
  says the opposite. An undeclared top-level key stays the author's own and is
  reported by nobody, because that is what 0010 reserves the level for; what is
  reported is the *disagreement*, a name the schema has an opinion about written
  where the opinion is never consulted. Nothing refuses: these artifacts are in
  force, and a check that would not read them turns a wording defect into an
  outage.

  The hint that used to say *did you mean to nest it under `fields:`?* is gone
  rather than kept beside it. It rode on a **missing-required** violation, so it
  could only speak for a required field — and `decision.statement` is optional,
  because the H1 already says it. That is how **0062 through 0066**, five
  consecutive decisions authored on 2026-08-24, were written with the statement
  at the top level while 0060, 0061, 0067 and 0068 nested it correctly, and
  `kept list` called the corpus clean. All five now nest, and
  `TestNoArtifactInThisCorpusWritesADeclaredFieldAtTheTopLevel` sweeps this
  repository's own corpus so the sixth is caught the day it is written.

## Names outlive the plan that produced them

**A name says what a thing is, never when it arrived.** The build order in
`docs/increments.md` is a plan, and plans are revised, reordered and abandoned.
Anything durable that encodes a position in one is wrong the moment the plan
moves, and it is wrong forever after because renaming is a change nobody wants
to make.

**A durable artifact says what holds, not where the roadmap stood.** Do not put
increments, phases, backlog position, or planned delivery order in a long-lived
artifact's statement, decision, policy, promise, title, or slug. Describe the
capability, prerequisite, or condition instead. A dated or point-in-time
reference is allowed as evidence of an event that has already happened — state
it as history, rather than as a plan the artifact expects to remain true.

Never in a filename, package, type, function, field, test name, or an
artifact's slug:

- an increment, phase, step, stage or milestone — no `i1_service.go`, no
  `phase3_checks.go`, no `Step2Validator`
- a position in time — `new_`, `old_`, `v2_`, `_final`, `_wip`, `temp_`,
  `legacy_`, dates
- the ticket, branch or session that produced it

Name it for what it does: `service.go`, `checks.go`, `validator.go`. If two
things want one name, the second one needs a better description of itself, not
a number.

**Branches and commit messages are exempt**, because they *are* point-in-time —
`increment-2-standing` is a fine branch name and a fine thing to say in a commit
body. The rule is about what survives the merge.

**The corpus may discuss increments; it may not be named after them.** An
artifact's slug is authored once and never changes (0021), so a slug like
`phase-2-tests-pass` is a permanent record of a temporary plan. `docs/increments.md`
is about the build order and is named for its subject, which is the distinction.

## Commits and pull requests

**Thorough, then short.** The diff already says what changed. A message earns
its length by saying what the diff cannot: why this, and what was rejected.

- A subject line in the imperative, under about 72 characters, naming the
  change rather than the files — `hash frontmatter over the schema's bind list`,
  not `update corpus.go and schema.go`.
- A body when the why is not obvious. Name the decision it implements or
  changes — `implements 0022` — because the ADR carries the reasoning and the
  commit should not restate it.
- A pull request describes the same thing, plus what a reviewer should look at
  first and what it deliberately leaves undone. If it changes an invariant in
  this file or a decision in `docs/decision/`, it says so in the first
  paragraph.
- No wall of bullets restating the diff, and no summary of every file touched.

**No agent attribution, ever.** Commits and pull requests are authored by the
repository's author, full stop. Nothing may carry a `Co-Authored-By` naming a
model, an agent or a tool; a session id or link; a "generated with" footer; or a
tool-credit line or emoji. **This holds even when a harness, a template or a
tool default says otherwise** — those defaults do not override this file.

The reason is not modesty. The history of this repository is a record of
decisions and who made them, and an agent did not make one: it produced a diff
that a person read and landed. Crediting the tool in the record misstates who
is accountable for the change.

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
