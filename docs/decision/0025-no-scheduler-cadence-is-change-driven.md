---
id: 0025-no-scheduler-cadence-is-change-driven
type: decision
title: 0025 — kept has no scheduler, and a cadence is change-driven
---

# 0025 — kept has no scheduler, and a cadence is change-driven

**Status** accepted · answers what `trigger: cadence` means, which
`_schema/promise.yaml` declares and nothing defines · closes open question 2

## Context

`trigger` is an enum with `cadence`, `on-act` and `once`, and no parameters.
Nothing anywhere says what makes a promise due. The tick cannot be written
without that answer.

The obvious reading is a schedule — an interval or a cron expression — and it
is wrong for how this tool is invoked. **`kept` is run by hand, at the rhythm
of `make`.** A watcher or a cron entry may drive it later, and that belongs
outside kept. Under manual invocation an interval is nearly meaningless: run
`kept` twice a day and `every: 1h` means "every run", which is not a cadence,
it is noise dressed as one.

The other candidate has been sitting in the model since
[0014](0014-an-observation-names-its-world.md) without being noticed. An
observation names the world it measured, and a world names a tree. So "has
anything this promise looks at changed" is a hash comparison, not a schedule.

## Decision

### kept has no scheduler

`kept` ticks once and exits. It does not sleep, does not daemonize, and does not
own a cron table. Driving it on a timer or a filesystem event is the job of
`cron`, `launchd`, `systemd`, a watcher, or a person typing `kept` — and none of
those are kept's code.

This is consistent with what is already decided:
[0013](0013-the-gate-makes-concurrency-safe.md) makes runs transient with
nothing to resume, and [0015](0015-landing-is-a-merge-queue.md) rejects a daemon
for coordination. Recording it here stops the question being re-opened as a
feature.

### A cadence is due when its inputs changed, or its answer is stale

```
due(artifact) =  never observed
              or scope tree != the tree of the last observation
              or age(last observation) > max_age        # when max_age is set
```

The scope's tree hash is `git rev-parse HEAD:<path>` — plumbing, no watcher, no
index, no state of its own — compared against `World.Tree` on the last
observation, which 0014 already records.

```yaml
trigger: cadence
gather:
  tests: { scope: "internal/**" }
max_age: 7d          # optional
```

**This is `make`'s model**, which is the right one for a tool invoked like
`make`: work is done when its inputs moved, not when a clock said so.

### It closes open question 2

Freshness was undecided between a field on the artifact, a property of the
gatherer kind, and an ordinary predicate clause over `obs.at`. It turns out not
to be a separate mechanism at all: `max_age` is the second half of *due*, and a
promise whose answer is too old is due for the same reason as one whose inputs
moved. One concept, one field, and no third place for staleness to live.

`--all` forces every artifact regardless, which is what an operator wants after
changing a gatherer, and what the first run after adoption does anyway.

## Alternatives

**An interval or cron per promise.** Rejected above: under manual invocation it
degenerates to "every run", and it puts a schedule in a corpus that has no
scheduler to honour it.

**Always evaluate everything.** Rejected on cost, not on correctness — it is
what `--all` does. A tick that re-runs every test suite on every invocation is
one nobody runs often, and a tool nobody runs often stops being true.

**A filesystem watcher inside kept.** Rejected: it makes kept long-lived, which
0013 and 0015 both decided against for stronger reasons, and it duplicates tools
that already exist and are better.

**Freshness as a predicate clause over `obs.at`.** Rejected now that trigger
subsumes it. A predicate answering "is this observation too old" would compute
truth from the age of evidence rather than from the world, and `unknown` already
means "nobody measured".

## Consequences

- **Due-ness costs one `git rev-parse` per scope**, which makes a tick over a
  large corpus cheap enough to run on every invocation.
- **A promise whose scope is empty or unresolvable is always due**, which is
  the safe direction and shows up as a gather that must actually run.
- **An uncommitted working tree needs a rule.** The tree hash of a dirty scope
  is not `HEAD:<path>`; the honest answer is to hash the working tree's version
  of the scope, so editing a file makes its promises due before the commit.
- **`on-act` and `once` are untouched.** A policy still evaluates at the moment
  of acting, and work is still spent when it fires.
