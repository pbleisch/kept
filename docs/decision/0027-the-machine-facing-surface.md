---
id: 0027-the-machine-facing-surface
type: decision
title: 0027 — The machine-facing surface is a contract, and kept explains itself
---

# 0027 — The machine-facing surface is a contract, and kept explains itself

## Context

kept is used by three kinds of caller and only one of them is a person. A
script or a CI job needs an exit code it can branch on. An agent driving `kept`
needs structured output rather than a table meant for a terminal. And a coding
agent working in a repository that *contains* kept — Claude Code, Codex,
anything else the operator runs — needs to know which parts of that repository
it must not touch, which it will not discover by reading the code.

None of the three is specified. The verb surface exists and every verb exits 2.

The third is the one with teeth. `.kept/` holds the ledger, the artifacts and
the config. An ordinary coding agent asked to "clean up stale files" or "fix the
config" will happily edit state that kept's guarantees depend on, and it will
have no reason to suspect otherwise. Confinement
([0017](0017-the-box-is-not-the-jail.md)) bounds the agents *kept* dispatches.
It does nothing about the agents the operator runs.

## Decision

### Exit codes distinguish "not kept" from "could not tell"

Following the `grep` and `diff` convention, with the addition kept specifically
needs:

| | |
|---|---|
| `0` | every artifact with standing is kept |
| `1` | **known false** — something with standing is `not-kept`, `regressed`, `absent` or `conflicted` |
| `2` | usage error — bad flags, unknown verb, unparseable corpus |
| `3` | **no answer** — something with standing is `unknown`, `unverified` or `stale`, and nothing is known false |
| `4` | refused — stale view, unconfigured provider, failed canary, budget |

**`1` is not "drift exists", and the first draft of this table said it was.**
Every kind in `3` is also a drift kind, so that wording made the two rows
overlap and left the split to whoever wrote the code. The line is not drift
versus no drift; it is **the world disagrees with a commitment** versus **there
is no evidence either way**, because those want different responses from
whoever is reading the code.

`absent` sits with the known-false half rather than with the unanswered one,
which is worth saying because it could go either way. A ratified artifact whose
content has vanished is a definite finding — 0021 refuses to read it as a
release precisely so that it stays loud — and a corpus root pointed at nothing
is caught before it gets here by that decision's bulk-absence guard, not by
this table.

`3` is the one worth arguing for. A CI job that treats "the toolchain was
missing" as "the promise is broken" produces exactly the false report the
three-valued verdict exists to prevent. The distinction is in the model already;
it has to survive the exit code.

It is the same rule as `AGENTS.md`'s *a failed gatherer yields `unknown`, never
`not-kept` — a machine that could not measure does not record what it thinks it
saw*, applied one layer up. Exit `1` for an unknown would be the CLI recording
what it thinks it saw, at the boundary where every script that consumes kept
learns the lesson.

`4` keeps a refusal from reading as a pass. A run that did nothing because it
was stale must not exit `0`.

**Precedence is `2 > 4 > 1 > 3 > 0`**, and only the placement of `4` is not
obvious. A run that refused because its view was stale is a run whose standing
may be wrong — the promise it calls `not-kept` may be one the operator has
already released — so a degraded run reports that it was degraded rather than
reporting findings it cannot vouch for.

### Output is human by default and structured on demand

`--json` for a document, JSON Lines for anything streaming. Every structured
payload carries a schema version, for the same reason the ledger envelope does
([0013](0013-the-gate-makes-concurrency-safe.md)): a consumer that cannot tell
which shape it is holding will guess wrong exactly once.

The human format is not a contract and may change. The structured format is,
and changes to it are versioned.

### `kept agent` generates the instructions other agents need

```
kept agent rules       what an agent must not touch, and why
kept agent brief       how an agent uses kept in this repository
kept agent install     write or refresh the block in AGENTS.md / CLAUDE.md
```

Generated, not hand-written, because the rules are derived from the same config
that enforces them — the corpus root, which paths are machine-written, which
verbs are a person's alone — and a hand-written copy drifts from the thing it
describes. `kept init` runs `install`; `kept doctor` notices when the block is
stale.

The block says at minimum:

- `.kept/ledger/**` and `.kept/artifacts/**` are machine-written and off-limits;
  they are not on this branch ([0016](0016-the-ledger-lives-on-its-own-ref.md),
  [0023](0023-what-may-be-captured.md)) and should not be added to it
- `.kept/config.yaml` is the operator's; changing it changes what is enforced
- an artifact's `id` is never edited by hand
  ([0021](0021-identity-is-authored-absence-is-not-an-act.md)), and because it
  never changes, it must not name a moment — a slug like `phase-2-tests-pass`
  is a permanent record of a temporary plan
- retiring a promise is `kept release`, never deleting the file
- `ratify` and `release` are the operator's acts and an agent does not run them
  ([0024](0024-signing-is-designed-and-deferred.md))

**This is documentation, not enforcement**, and the difference is the point. It
makes a well-behaved agent behave, which is most of them most of the time, and
it costs nothing. The agents kept dispatches are confined; the agents the
operator runs are informed.

## Alternatives

**Exit `1` for anything wrong.** Rejected: it merges `not-kept`, `unknown` and
`refused`, which is the collapse the three-valued verdict exists to prevent, and
CI is exactly where that collapse does damage.

**Only structured output, formatted for humans by a wrapper.** Rejected: the
default invocation is a person typing `kept`, and it should be readable without
a pipe.

**Hand-write the agent block once.** Rejected: it is a copy of derived facts and
will be wrong within a release. Generating it means `kept doctor` can tell when
it is out of date.

**Enforce it instead of documenting it.** Rejected as insufficient rather than
wrong — kept cannot confine processes it did not start. A `.gitignore`-style
marker and a pre-commit check are worth adding later; neither replaces telling
the agent.

## Consequences

- **Exit `3` will surprise someone** whose CI treats non-zero as failure. That
  is the correct surprise, and the documentation leads with it.
- **The structured schema is a compatibility surface** from the first release,
  which is the cost of being usable by agents at all.
- **`kept agent install` writes to a file the operator owns.** It appends a
  marked block and refreshes only that block, never the surrounding prose.
- **kept's own `AGENTS.md` carries the same block**, since it self-hosts, and
  that is the first test that the generator produces something worth reading.
