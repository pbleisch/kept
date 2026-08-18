# Where this came from

`kept` is a synthesis of nine systems built before it, all personal
projects that are not published. Most of the design decisions here are not new; they are
the parts of those systems that survived contact with use, assembled once.

A survey of all nine — architecture, mechanisms, and what each proved — is part
of grft's own documentation, which is not published; the table below is what
kept took from it. Read it before proposing a change that sounds like a return to something one of them already
did.

## What was taken from each

| System | Taken |
|---|---|
| **awiki** | The whole corpus: markdown + frontmatter entities, `type:slug` ids, deterministic slugs, lenient per-type schemas, three-source linking with priority dedup, attachments with a media whitelist, derived SQLite/FTS5 index, the write facade, rename propagation. Its ADRs 001–018 are the reference. |
| **chopworks** | The same content model applied to a real prose corpus. Also: gates that take accept / revise / reject, write scopes validated at acceptance, and `autoAcceptAfter` on non-required gates — a deadline is how a single operator stops being the bottleneck. |
| **backlog** | The thread loop. Per-item worktrees, ground-truth override, two-phase validation, triage as a typed decision (retry / resume / fork), independence scoring, cost per attempt, crash detection with a grace period. The only one of the nine used on real work. |
| **pt** | Provenance and modality on facts, `verified` as harness-only, append-only retraction, scope promotion, evidence quality as a separate axis from pass/fail, typed exit codes, per-thread leases, the notify hook. |
| **adsr** | Tier discipline (declare which store you write to), drift with a cause rather than a boolean, Move-as-post-condition, quarantine that does not halt the loop, intent committed before non-idempotent execution. |
| **grft** | The sandbox box and its dirty-file overlay, scope-limited landing, authority by artifact class, the effect state machine, artifact retention on every exit path, and the measurements that narrowed the claims (below). |
| **harness** | Persona as a binding of role, tools, authority and memory. Evidence manifests. Both deliberately **not** adopted here — see below. |
| **cpmux** | Do not own the processes you supervise. A restart should cost nothing. |
| **flo** | Attention as scarce; a computed answer to "is this worth an interruption". Journal-plus-projections with a `doctor` that proves reconstructability by replaying into a scratch database. |

## Measurements that shaped the decisions

All from grft's evaluation report, which is not published, unless noted.

**A competent model catches epistemic failures unprompted; it has no
representation of standing.** Three samples over eighteen scenarios, 144 model
calls, $5.77. Six false completions, **all six deontic, zero epistemic in 54
scenario-runs**. On a scenario built to isolate standing — check green on
arrival, nothing to repair, authorization withdrawn — the model certified four
times out of five, reasoning impeccably about evidence quality on the way.

*Consequence:* the machinery that forces evidence scrutiny is worth much less
than it was. Standing is the part a model cannot supply. See decision 0006.

**The coverage promise that was satisfied hollowly.** An agent repairing "every
package under `internal/` has a test file" added files and **rewrote an
existing one**, leaving the confinement canary with no coverage. The check was
truthful. The probe was truthful — it correctly reported the check could detect
a regression. Nothing compared the proxy to the intent.

*Consequence:* structured observations, so `the set of tests never shrinks` is
expressible; and the thread assessor, which catches this from the diff. This is
the running example in `model-deck.html`. See decisions 0004 and 0006.

**A sampler that looked like a measurement for months.** `mutate`'s fixed
sample computed `all[i*step]` with `step := len(all)/n`, which for any file with
fewer than `2n` sites drew the *first n* — the top of the file, which is imports
and small helpers. Packages scored 10% under it and 100% under a random sample
of the same code.

*Consequence:* any sampler needs a test that it reaches the whole population,
not that it returns the right count.

**Enumerated repairs did not produce work.** adsr picks the cheapest Move from
an authored list; grft scores candidates from a compiled rule registry. backlog
has no selection scheme at all and produced the most. See decision 0009.

**Rich records of the artifact paid off; rich records of the work did not.**
chopworks' content model earns its keep. adsr, pt and grft have the richest
work-state models in the group and produced the least output. That asymmetry is
why `kept` invests in the corpus and keeps work in the ledger as a derived
thing.

## Deliberately not adopted

**Trust levels and altitude bounds** (harness). Ways of saying standing is
partial and earned. One principal, so there is nothing to bound. If a second
appears, they belong as attributes on the ratification act, not as new types.

**Evidence manifests** (harness). A node declaring which evidence kinds must
exist, produced by which capability, postdating which event. Closer to a
predicate over observations than it looks; the predicate covers it.

**Incremental invalidation** (grft). Measured to work — matcher work flat at
2.0 per delta against the oracle's 645 across a 256× graph, with attribute and
invariant counts exactly constant. It pays above a register size one operator
will not reach. Full re-derivation each tick is simpler and cannot go stale.

**Field-level CAS and leader epochs** (adsr). Not because writers are single —
they are not; runs execute concurrently — but because the layout means no two
writers ever write the same bytes. One file per artifact, one file per
artifact's truth, and observations content-addressed by what they measured. CAS
arbitrates a contended write; this design removes the contention instead. See
decisions 0011 and 0014.

**Quarantine on parse failure** (adsr). Worth adopting later. A corpus that
several agents write will eventually contain a file that does not parse, and
the loop should keep going and say so rather than stop.

**Durable execution and replay** (Temporal-shaped). A crash re-derives from the
corpus and the ledger. There is nothing to resume.

**Learned selection** (ChatDev Puppeteer, MetaGPT AFlow). Needs episode volume
that does not exist, and the cheap version presupposes candidates, which
decision 0009 removes.
