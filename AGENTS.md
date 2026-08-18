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
- **A gatherer never repairs.** It observes. One that fixes what it finds
  cannot be re-run to check the fix, and its output stops being evidence.
- **A failed gatherer yields `unknown`, never `not-kept`.** A machine that
  could not measure does not record what it thinks it saw.
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
- **No lock is load-bearing.** Two runs may repair the same drift; the second
  one's gate finds the promise already kept and lands nothing. A lock saves an
  agent dispatch, and code that would be *wrong* without one is code that has
  moved correctness out of the gate. The exception is the section between the
  gate and the push, which is isolation, not exclusion. See 0013 and 0015.

## Things that will be tempting and are wrong

- **Adding a repair library.** The packet states the gap; the agent decides.
  See 0009. If a repair recurs, that is a predicate nobody has written.
- **Adding a probe field to promises.** See 0006. Verification attaches to
  acts, not instruments. If an instrument needs testing, that is a gatherer a
  promise can name.
- **Making work a corpus entity.** See 0008. Nobody authors a work item.
- **Letting a predicate read facts.** A fact is a record of an observation
  *then*, not evidence of the world *now*. If you want to gate on a fact,
  ratify it and it becomes canon.
- **Caching standing in the file for display.** A cache that can disagree with
  its source is worse than none. `kept why` answers it.

- **An artifact's identity is authored, and absence is not an act.** The `id`
  in frontmatter is the identity; the filename is not, so renaming a file
  changes nothing. A ratified artifact whose content has vanished is *absent*
  drift — it keeps standing and says so. Never infer a release from a missing
  file: it cannot be told apart from a bad merge or a misconfigured corpus
  root. See 0021.
- **The hash normalizes before it hashes.** NFC, LF, no BOM, no trailing
  whitespace, one final newline — so a CRLF checkout does not silently
  de-ratify the corpus. The rules carry a version in every ratification. See
  0022.
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
  the costs accepted.

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
