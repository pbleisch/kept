---
id: 0043-the-catalogue-is-closed-and-a-third-party-is-namespaced
type: decision
title: 0043 — The built-in catalogue is closed, a third party is namespaced, and PATH does not win
links:
  - { target: decision:0004-gatherers-return-observations, relation: extends }
  - { target: decision:0005-gatherers-are-a-named-map, relation: extends }
  - { target: decision:0023-what-may-be-captured, relation: extends }
  - { target: decision:0034-the-observation-record, relation: extends }
  - { target: decision:0038-a-run-line-is-argv-and-there-is-no-shell, relation: depends-on }
  - { target: decision:0036-the-predicate-language, relation: depends-on }
---

# 0043 — The built-in catalogue is closed, a third party is namespaced, and PATH does not win

## Context

Open question 8 asked which gatherers are built in, which are subprocesses over
a stdio protocol, and whether a third party resolves ahead of a built-in on
`PATH`. It named grft's plugin protocol as the reference — exec, one JSON object
in, one out, `PATH` wins — and called it deliberately tiny, which it is.

The question has to be answered before increment 4 adds a gatherer, and the
reason is the one [0038](0038-a-run-line-is-argv-and-there-is-no-shell.md) gives
for `run:`. A gather entry lives under `gather:`, and `_schema/promise.yaml`
names `gather` in its `hash` list, so **the name of the instrument binds**. A
promise ratified while `tests` meant one thing and read later as meaning another
is not a promise anyone can rely on, and unlike an edit nothing detects it,
because the bytes did not change.

`Catalogue` and `Gatherer` already anticipate this. `Builtin()` is a function
rather than a package variable *"so that no caller can add to or remove from the
one map every corpus is checked against"*, and `Gatherer` is already the protocol
in Go form: a name carrying a version, a declaration over args, and a
measurement.

### The catalogue is being closed against one language, and that is the risk

Every gatherer anyone has needed here is plausibly built in, because this is a
Go repository holding decisions and promises about itself. A rule drawn from
that sample generalises or it does not, and the place it would break is domain
vocabulary — a kind of noun the core is supposed never to carry.

The manuscript half was written down for exactly that, against no
implementation: prose promises and canon entries for a manuscript, then the
question asked of each — which gatherer does this need, and does that gatherer
belong in the core. **The strongest available candidate for a plugin was
`entities` — characters, scenes, chapters — and it dissolved.** `entities` reads
the corpus's own artifacts and the fields their schemas declare; its args are
`type: scene`, `order_by: reading_order`, `measure: words`, and its output is
`items`, `ids`, `count`, `broken`. Character, scene and plotline appear in
`_schema/` and in the promise, which is where they belong. Nothing about
`entities` mentions a novel, exactly as nothing about `tests` mentions a Go
package beyond an arg the promise writes.

**But it dissolves only under a rule the code does not have yet.** `entities`
cannot declare a fixed field list in code the way `tests` does, because what a
scene *has* is decided by `_schema/scene.yaml` and kept has never seen it. The
two ways out without a rule change are both bad: return every artifact's whole
frontmatter, which copies the corpus into a permanent content-addressed record
on every world and is what
[0023](0023-what-may-be-captured.md)'s size cap exists to stop; or hard-code the
fields, which is the moment domain vocabulary enters the core and `x/` comes
back. So the declaration rule below is not a tidy generalisation — it is the
condition under which the catalogue can close where this decision closes it.

## Decision

### The built-in catalogue is closed, and a bare name always resolves inside it

A promise naming `tests` gets kept's `tests`. Nothing on `PATH` can shadow a
built-in, ever.

### A third-party gatherer is named `x/<name>`, and the prefix binds

`x/` never resolves to a built-in and a bare name never resolves outside one, so
the promise's own text says whether it is trusting kept or trusting the machine
— reviewable in a diff, and covered by the hash.

**That is the load-bearing consequence rather than a side effect.** Because
`gather` is in `_schema/promise.yaml`'s `hash` list, moving a promise from
`tests` to `x/tests` **drops its standing**: the promise's own text now records
which it trusts, and a person has to grant it again. It is the property `PATH`
wins destroys, and it is the whole point.

### Inside the namespace, resolution is grft's

`x/spellcheck` resolves to `kept-gatherer-spellcheck` on `PATH`, exec'd directly.
No shell, ever — 0038's grammar and its reasons apply unchanged. 0026's deadline
and its process-group kill apply unchanged.

### Whoever decides the output shape declares it

A gatherer declares the fields it returns, and the declaration is checked against
`hold_when` when the corpus loads. **Where kept decides the shape the declaration
is in code; where the promise decides it the declaration is in the promise.**

- In code: `tests`, `imports`, `symbols`, `corpus` — kept wrote the instrument
  and knows what it yields.
- In the promise: `command`, whose extractor names fields kept has never seen;
  and `entities`, whose per-item fields are decided by the corpus's own schemas,
  which kept does not ship.

```yaml
gather:
  scenes: { gatherer: entities, type: scene, fields: [pov, stage, act] }
```

Both are checked against `hold_when` at corpus load, at the same moment, by the
same call. That is what keeps a promise reading a field its gatherer never
declares an **authoring error a person fixes** rather than an `unknown` in the
code path that decides a verdict — [0036](0036-the-predicate-language.md)'s
seam, which does not move.

This generalises [0034](0034-the-observation-record.md)'s asymmetry. 0034 stated
it as *in code for built-ins; in the promise for `command` alone*, and called
that asymmetry the answer rather than an inconsistency. The reason it gave was
never about `command` — it was about **who can name the fields**. Restated over
that, the rule covers the same two cases it covered before and one it could not:

- **What it preserves.** The check still happens at load, before the world is
  looked at, for every gatherer. Nothing may declare fields at gather time,
  where a declaration would arrive too late to be an authoring error. A
  built-in whose shape kept decides may not push its declaration into the
  promise to save writing it down.
- **What it gives up.** *Is this the escape hatch?* was a one-bit test anyone
  could apply by looking at the name. *Who decides the output shape?* is a
  judgement, made per gatherer, and it can be got wrong in the direction that
  hurts: a built-in that declares in the promise when it did not have to has
  moved part of its contract out of the code, where no compiler checks it and
  no reviewer of the gatherer sees it. The bound is the sentence itself, and
  `kept doctor` is where a violation would be reported.

**It is also what keeps `entities` core rather than `x/entities`.** Written the
narrow way, `entities` either bloats the ledger or forces the plugin boundary
this decision is closing — and the manuscript case is the best evidence
available that a built-in-only catalogue generalises past Go. Written this way,
`entities` needs no amendment later.

### A gatherer returning per-item records returns both `id` and `path`

A convention, not a mechanism, and it belongs here because it is
catalogue-wide.

The alternative to one gatherer answering a question is a semi-join in the
predicate, and a semi-join joined on the wrong key **reports a verdict rather
than a refusal**:

```
kept       all(filter(obs.meta.items, c => c.stage == "copy"), c => c.path in obs.scan.clean)
not-kept   all(filter(obs.meta.items, c => c.stage == "copy"), c => c.id   in obs.scan.clean)
```

Same world, same data, one clean copy-stage chapter. `Expr.Check` cannot catch
it: it checks `obs.<name>.<field>` at the top level only, and `c.id` inside a
lambda is unchecked by construction, because a gatherer may return less than it
declared. This is the `command` false-green one layer in — an authoring mistake
that produces an answer instead of an error — and it is worse where `entities`
speaks in artifact ids and a source scan speaks in file paths, which
[0021](0021-identity-is-authored-absence-is-not-an-act.md) says are not the same
thing. One rule, no machinery: the join key is always available and always the
same.

### The wire — **NOT YET BINDING**

**Everything in this section is a sketch, and the first real plugin may revise
it.** Nothing here has been implemented and nothing has ever been typed against
it. It is written down because a protocol is a compatibility surface and 0031
and 0034's rigour means deciding one before the first plugin rather than after —
but an unimplemented spec binds nobody, and written in the same voice as the
rest of this decision someone will read it as a contract. It is not one. When
the first plugin exists, this section is revised against it and stops being a
sketch; until then a disagreement between this section and a working plugin is
resolved in the plugin's favour.

Two calls, one JSON object in on stdin, one out on stdout.

`declare` is made when the corpus loads, before the world has been looked at,
because that is where `Catalogue.Check` lives:

```json
→ {"protocol":1,"call":"declare","args":{"scope":"docs/**","dict":"en_GB"}}
← {"protocol":1,"name":"spellcheck","version":3,"fields":["misspellings","hits"]}
```

```json
→ {"protocol":1,"call":"gather","root":"/abs/path","globs":["docs/**"],
   "paths":["docs/a.md",…],"scope":"sha256:…","world":"sha256:…",
   "args":{…},"deadline_ms":300000}
← {"protocol":1,"data":{"hits":0,"misspellings":[]}}
  or
← {"protocol":1,"error":{"kind":"not-runnable","message":"aspell is not installed"}}
```

Declaring per gather would be wrong twice over: `Declares` must be answerable
**without the world**, and it must be answerable for a promise that is not due
and will never gather this tick. Caching a declaration across runs would also be
wrong — the binary can be replaced between runs, and a stale declaration would
let a gatherer answer with a field the corpus was checked against a different
version of. Cache it for the life of one process and no longer.

`version` is mandatory and it is the one field to be strict about. 0034 makes
`gatherer@version` part of what an observation is addressed by, so a plugin that
changes what it measures without bumping `version` serves a pre-change
observation out of content addressing and **nothing reports it**. A subprocess
that does not return an integer `version` is a `BadSpec` at load, not an
`unknown` at gather.

**A subprocess gatherer cannot report `not-kept` by failing.** The wire form
carries `data` or `error` and no verdict of any kind. Every one of these is an
`error`, and an error means no observation, and no observation means `unknown`:
a non-zero exit, empty stdout, unparseable stdout, a missing or unknown
`protocol`, a field in `data` the declaration did not name, a declared field
absent from `data`, a timeout, or a signal. `gather.Record` is where that rule
lives and it does not move. A half-answering plugin cannot manufacture a false
verdict either, because a missing field is `Unknown` in the predicate language
rather than false (0036).

**A subprocess gatherer sets a pattern gate, never a structural one.**
`model.md`'s autonomy table grades by what the instrument is, and kept did not
write this one. Weakest sets the gate, so a promise naming one is assessor-gated
and does not auto-land.

### The wire is specified now and implemented when something needs it

No third-party gatherer exists or is wanted, and every gatherer increment 4 adds
belongs inside. Implementing before the first plugin is the speculative half,
and 0036's bound applies to protocols as much as to builtins: *a builtin is
added when a promise in this repository or in a manuscript needs one, never in
advance.*

## Alternatives

**`PATH` wins, as grft has it.** The obvious one, and the reason to write this
down at all. Rejected because the two systems' outputs do different work:
**grft's plugin output was advisory to a person, who saw it before acting;
kept's gatherer output is the truth that gates an automated land** in increment
6. `tests` is graded structural — auto-land, inside scope — so a shadowing
`tests` that always returns `ok: true` turns every test-gated promise green and
opens that gate, with nothing anywhere in the record naming which binary
answered. A gatherer also runs outside every box, as the operator, with the
operator's credentials: [0017](0017-the-box-is-not-the-jail.md) confines the
repair agent and says nothing about the instrument. `PATH` wins would make an
arbitrary binary a code-execution primitive reached by running `kept` in a
directory.

The deeper objection is 0038's. `command` already resolves its program through
`PATH`, and that is acceptable because the program is **authored**: `run: "go vet
./..."` is read in review and covered by the hash. Under `PATH` wins the promise
says `gatherer: tests` and which instrument that is comes from the environment —
invisible in the text, invisible in the diff, uncovered by the hash. That is a
`World` that is not the tree, arriving through a door
[0033](0033-due-is-a-digest-over-the-scope.md) closed everywhere else.

**A trusted-plugin list in `.kept/config.yaml`.** Rejected as a second place the
answer lives. A promise would name `tests` and mean different instruments in two
checkouts of one corpus, and the config is not hashed, so nothing would drop
standing when it changed.

**Signed plugins.** Rejected as premature:
[0024](0024-signing-is-designed-and-deferred.md) defers signing generally, and a
namespace that says *this is not kept's* costs nothing and gets most of the
value.

**Keep the narrow declaration rule — `command` alone declares in the promise.**
Rejected, and it is the alternative that would have been taken by default,
because it is what 0034 already says and it is a sharper test. Rejected because
`entities` fails it while satisfying every reason behind it, and the two ways of
making `entities` fit under the narrow rule are the two this decision is trying
to avoid: copy the corpus into the ledger, or put scene, chapter and plotline
into `internal/gather`.

**Build the wire in increment 4, as `increments.md` §4 says.** Rejected, and the
counter-argument is real: this repository's method is to decide by building
against a real need, and that method produced 0036. It is declined here because
the real-need test has no candidate — no third-party gatherer exists or is
wanted — and **building a compatibility surface with no consumer is the one case
where building teaches nothing.** The compromise is that what is written down
instead is marked as not binding, so the half that was not built does not
masquerade as the half that was.

## Consequences

- **A third party cannot ship a better `tests` and have existing promises
  silently pick it up.** They must be re-authored and re-ratified, by a person.
  That is the cost, it is accepted, and it is the mechanism rather than a side
  effect.
- **`x/` is reserved in the gather namespace forever**, and a built-in can never
  be called `x/anything`. The refusal for an unknown gatherer names both halves
  — that no built-in answers to this name, and that a third party is written
  `x/<name>` — the way 0036's unknown-function message names the bound it is
  enforcing.
- **Adding a built-in is now the only way to widen what a bare name can mean**,
  which puts every widening through 0036's bound and through review.
- **The declaration rule is a judgement rather than a test**, and the judgement
  can be got wrong quietly. A built-in that declares in the promise without
  needing to has moved part of its contract out of the code. `kept doctor` is
  where that is reported.
- **Two guards that must be able to fire**, per `AGENTS.md`: a promise naming
  `x/tests` does not resolve to the built-in `tests`, and a promise naming
  `tests` does not resolve to a `kept-gatherer-tests` placed on `PATH`.
- **The wire section is not a contract and says so.** The first plugin revises
  it. If that revision is substantial it is a decision of its own rather than an
  edit to this one, per `AGENTS.md`'s supersede-don't-edit rule — the sketch is
  what is being replaced, and what this decision actually decided is the
  boundary above it.
