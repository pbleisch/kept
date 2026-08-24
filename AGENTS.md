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
  `list`, `doctor` and `why` all call it clean. **Nothing reports the extra
  grant**, and a release empties the artifact's whole set rather than one
  wording, so a declined branch leaves a grant for content that exists nowhere
  and no way to withdraw just that one. Sequence a revision's ratification with
  the merge, not with the edit.
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
  transition rule compares outcome and condition and never the detail. A promise
  naming two instruments cannot be repaired: a packet carries one measurement.
  See 0060.
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

- **Re-deriving a verdict over this whole corpus inside a box.** It runs
  `core-packages-are-tested`, whose instrument shells out to `go test
  ./internal/...` *in the box* — which re-enters this suite, cuts more boxes,
  runs more canaries, and would dispatch another live session on the operator's
  credential. `internal/repair/claudeadmission_test.go` leaves that promise and
  `the-tree-vets-clean` out of the corpus it copies, which also takes the
  re-derivation from minutes to about 90 ms. **`internal/confine/piinbox_test.go`
  still copies the whole corpus** and has the same hazard; it is unfixed because
  changing it needs Pi's live proof re-run, which needs the self-hosted endpoint
  0052's Pi evidence was measured against.

  **A box is not the only place this bites: any `kept tick` does it**, on a
  scratch clone as readily as in a worktree, because the gatherer inherits the
  environment and `config.ReadUser()` finds the operator's real credential
  wherever it runs. So a tick taken to check corpus verdicts silently dispatches
  the live admission sessions again. Set
  `GOFLAGS=-skip=<the three live test names>` on the tick: `go test` honours it
  through the inherited environment and `go vet` ignores it, so
  `the-tree-vets-clean` is unaffected — measured. Do not reach for a fake `HOME`
  instead; it also hides nono's own configuration, which the confine tests need.

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

- **Forcing the ledger's fetch refspec, or anything else that lets
  `refs/kept/ledger` move backwards.** `FetchRefspec` is
  `refs/kept/ledger:refs/kept/ledger` and the leading `+` is not coming back.
  `RefStore.mutate` only ever advances the ref from a value it just read, so a
  ledger never rewinds and every legitimate fetch is already a fast-forward —
  the force added exactly one case, and on 2026-08-22 that case erased two
  ratifications from this repository during an ordinary `git fetch` a worktree
  pool ran on its own. Exit 0, no output, and — then — **no reflog**: git logs
  `refs/heads/`, `refs/remotes/`, `refs/notes/` and `HEAD` and nothing else
  unless `core.logAllRefUpdates` is `always`, which nothing set, so the durable
  tier sat in the one namespace with no safety log and the rewind was
  unreconstructable afterwards. `kept init` now sets it — see the entry below.
  A ledger ahead of its remote is the resting state of this system,
  not an incident, so a `git fetch` that fails with `(non-fast-forward)` is the
  design working. Adding the unforced spelling *beside* a forced one is not a
  softer repair and was measured not to work: with the forced line first — the
  order `config --add` produces — the fetch is still forced. `ConfigureFetch`
  removes it, reads the config back, and `init` says it did; `doctor` reports one
  nobody has corrected. See 0016.

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
  branches cut from the same commit both pick the next free number and only
  one of them is right, so rebase before assuming yours survived: renumber the
  file, the `id`, the title and every citation of it. That is cheap **only
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
