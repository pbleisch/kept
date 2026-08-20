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
  `promise.yaml`'s hash list, so moving a promise from `tests` to `x/tests`
  drops its standing — which is the mechanism, not a side effect. The wire in
  0043 is a **sketch marked not yet binding**; the first real plugin revises it
  rather than being held to it. And a gatherer declares its fields where the
  shape is decided: in code where kept decides, in the promise where the promise
  does — never at gather time, where a declaration arrives too late to be an
  authoring error. See 0043.

  The catalogue is `command`, `tests`, `imports`, `symbols` and `corpus`.
  **`imports` and `symbols` parse with `go/parser` and never shell out**, and
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
- **A decision is superseded, not edited.** Write a new one that says what
  moved, and record the relation in the earlier one's `links:`. The single
  exception is a terminology rename, which changes nothing a reader would do —
  0039 states the rule and is the licence for the edits it made. `body.decision`
  is what binds, so a consequence discovered later can be added to an accepted
  decision without dropping its standing; rewriting what was decided drops it.
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
