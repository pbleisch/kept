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
  promises about kept. The invariants below are the first policies to move into
  the corpus.

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

- **Standing is created only by ratification, and only by a person.** No rule,
  no agent, no ingested document, no inference. If a code path can mint
  standing, that path is wrong.
- **Standing and truth are never written to an artifact file.** Both are ledger
  records keyed by a content hash. The repair agent can write the corpus; it
  must not be able to write either. Two mechanisms enforce that and both are
  load-bearing: the ledger is on a ref no box checks out, and `.kept/**` is out
  of scope for every repair whatever a promise declares. See 0016.
- **The box is not the jail.** A worktree bounds what may land. It says nothing
  about what a process may read, where it may connect, or what credentials it
  holds — and a box with push access to the origin makes the gate decorative.
  Confinement is configured separately, `unset` is not `none`, and a provider
  is verified by a canary rather than trusted by name. See 0017.
- **Record only what has already happened.** Observe, act, then record. A crash
  may lose the record of an act; it may never produce a record of an act that
  did not occur. Push before recording a land, never after. See 0018.
- **A tick that does nothing still records why.** Every refusal — budget, lock,
  staleness, policy, rank — is a disposition against the artifact. None of them
  can be reconstructed afterwards, and a queue whose silence cannot be explained
  is not a queue anyone should trust. See 0019.
- **Durability follows reproducibility, not size or type.** An observation is
  re-gatherable and therefore prunable. An agent session is not: re-running it
  produces a different session, at a different cost. Capture incrementally on
  every exit path, failures included, or a killed dispatch costs money nothing
  can attribute. See 0020.
- **Truth is derived on read.** Storing it is asserting it, and an asserted
  truth goes stale without saying so.
- **Standing is derived too, and nothing invalidates it.** An artifact is
  ratified when the ledger holds a ratification naming the hash its content
  produces *now*, and a release empties that set for the artifact rather than
  for one wording. There is no flag to clear, no cache to drop and no hook to
  fire on an edit — `internal/register` writes nothing at all, and a mark would
  be a second place the answer lived. See 0032.
- **Nothing kept writes rewrites what a person wrote.** Unknown frontmatter
  survives a read and is never re-emitted, and `kept init` writes only the
  files that are missing — it prints the `.gitattributes` rule rather than
  appending it to one somebody else authored. A config decides what is
  enforced and `* text=auto` decides how git stores every file in the tree, so
  neither is kept's to edit once it exists.
- **A gatherer never repairs.** It observes. One that fixes what it finds
  cannot be re-run to check the fix, and its output stops being evidence.
- **A failed gatherer yields `unknown`, never `not-kept`.** A machine that
  could not measure does not record what it thinks it saw. The rule lives in
  `gather.Record`, which files a measurement and files nothing for a failure,
  rather than at every call site: a site that forgets reports `not-kept` for a
  machine that was never installed.
- **An observation's data holds nothing that moves on its own.** No duration,
  no timestamp, no pid, and no order that came from map iteration. It is
  addressed by its data, so a field like that mints a fresh record on every
  tick of an unchanging world and the dedup the whole content-addressed layout
  exists for quietly stops working. Sort what a fold produces. See 0034.
- **The agent's report is never an input to the verdict.** The gate re-derives
  from the world. What the agent said is recorded, and the assessor reads it.
- **Out-of-scope paths are reported, never landed.** Work outside a promise's
  scope is not covered by the promise, so there is nothing to assess.
- **The pipeline is never in a packet.** There is no code path from a desire to
  an agent. Enforce it in the assembly function, not by convention.
- **Everything tracked in git must merge without a human.** There is one
  operator but many concurrent runs of `kept`, so no tracked file is ever
  read-modify-written in place, and every path is keyed by something already
  unique — the artifact, the thread, the observation's content hash, the run.
  Two runs never choose the same file, so nothing arbitrates between them.
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
