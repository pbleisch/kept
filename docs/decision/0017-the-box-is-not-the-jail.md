---
id: 0017-the-box-is-not-the-jail
type: decision
title: 0017 — The box bounds what may land; confinement bounds what may be touched
links:
  - { target: decision:0013-the-gate-makes-concurrency-safe, relation: extends }
---

# 0017 — The box bounds what may land; confinement bounds what may be touched

## Context

A box is a git worktree, and the temptation is to call it the isolation
boundary and stop. It is not one. A worktree is a directory: a process with its
working directory set there can `cd ..`, write `$HOME`, read every credential
on the machine, reach any host, and write to the origin repository directly —
and the box would never know.

The two boundaries do different jobs, and grft — which is in `lineage.md` as a
predecessor — separated them and wrote down why:

> The box decides what may *land*, the jail decides what may be *touched*.

`nono` arrives at the same separation from the other side, and shows where the
confinement half goes when taken seriously: filesystem scoping, network
allowlisting, and a **credential broker** that gates secrets per tool rather
than exposing them to the agent, with policy in a profile rather than in the
prompt, and tools the agent delegates to confined separately from the agent
itself.

There is a sharper reason kept cannot defer this the way grft did. grft parked
everything but a write allow-list, and named the condition for resuming:

> Resume when either becomes true: a session needs to run against a repository
> whose contents the operator is not willing to expose, or **sessions start
> running unattended.**

Unattended is kept's normal mode. The premise is a queue that drains without
you. So kept crosses grft's own resume threshold on day one, and a write
allow-list — under which an agent can still read every credential on the
machine and post it anywhere — is not the finished answer here even though it
was a reasonable place for grft to pause.

## Decision

**The box is a tree. Confinement is a separate, explicitly configured policy
over resources. They compose and neither substitutes for the other.**

| | bounds | mechanism |
|---|---|---|
| **box** | what may reach the repository | a worktree, plus scope enforcement at land, plus the gate |
| **confinement** | what the process may touch | a named provider enforcing a policy |

Confinement policy names its resources separately, because they fail
separately:

- **filesystem** — writable paths; everything else read-only
- **network** — reachable hosts, and whether egress goes through a proxy
- **credentials** — what the process holds, and what is brokered to a
  named tool instead of held
- **process** — what may be executed, and what confinement a delegated tool
  inherits

**Confinement is configured, never inferred, and `unset` is not `none`.** A run
refuses to dispatch an agent until a provider is named. `none` is a legal value
and records a decision; a typo must not silently produce an unconfined agent.
This is grft's answer, arrived at by use, and it is adopted rather than
re-derived.

**A provider is never trusted on its name.** Before an agent runs, a canary
executes inside the jail, attempts each escape the policy claims to prevent,
and requires refusal. This is `AGENTS.md`'s own rule — a check that has never
fired is not a check — applied to the boundary, and it catches the failure that
matters: a profile subtly wrong, or a provider that silently did nothing on the
wrong platform, produces an agent that runs perfectly well and a thread record
saying it was contained.

### Two specifics kept must not get wrong

**No push credentials in a box.** 0015 guarantees every commit on
`origin/main` was gated. An agent holding push access to `origin` walks around
that guarantee entirely, and the guarantee becomes decorative. A box's git
remote is the local repository, and the agent's environment does not carry a
credential that reaches the origin.

**The ledger is not reachable.** [0016](0016-the-ledger-lives-on-its-own-ref.md)
already keeps it off every checked-out ref, so confinement is the second of two
independent mechanisms rather than the only one.

### The provider is `nono`, and kept does not build a jail

Confinement is a solved problem kept has no reason to solve again. `nono`
already does the four resources — filesystem scoping, network allowlisting,
credential brokering through a proxy, and separate policies for the tools an
agent delegates to — on darwin, linux and WSL2, with no daemon and no
container. grft's experience is the argument: the jail is where a project's
time goes to be parked, and grft's own confinement work stalled at a write
allow-list for exactly that reason.

So kept **shells out** rather than writing profiles of its own. The `Jail`
interface stays, because `none` must remain expressible and a native provider
must remain possible, but no native provider is built.

**kept invokes `nono run`, and never `nono wrap`.** `wrap` is the mode nono's
own documentation offers for programmatic embedding, which is this use case, and
it **silently discards a profile's domain allowlist while printing `net proxy`**
— giving the child unrestricted network. That is a released regression, present
from v0.66.0 through v0.73.0, nine consecutive releases with no upstream issue.
`run` does not have it: a proxy that fails to start aborts the launch before the
child is exec'd, and a proxy killed mid-run fails closed, because the kernel
rule outlives it.

**A profile always declares domains.** A profile whose only network stanza is a
credential route starts no proxy, keeps `AllowAll`, and prints `net proxy`
anyway — on `run` as well as `wrap`. Any non-empty domain rule forces the proxy
to start or the launch to abort, so the domains are what make the network
claim true.

**macOS and Linux only.** There is no Windows binary and the project does not
build one; the Windows story is WSL2, which is Linux. On a platform with no
provider, `unset` stays unset and a run refuses to dispatch — which is this
decision's existing rule, arriving where it is most needed.

**A confined agent is orphaned rather than killed** if the supervisor dies. It
reparents and keeps running, still confined. Whatever dispatches it reaps it;
[0018](0018-what-survives-a-crash.md)'s dead-agent path is what reads the
remains.

### The profile is kept's; nono receives a projection of it

Delegating the mechanism is not delegating the model. kept's confinement
profile is its own data structure, and `nono`'s configuration is a **subset**
rendered from it, because kept has to express things that are not a sandbox's
business:

- a **derived** writable set — a promise's `RepairScope` bounds what a repair
  may change, so the writable paths follow from the artifact rather than from
  static config
- confinement for **gatherers**, not only agents; a gatherer is a subprocess
  running project code and it is not automatically more trusted than a repair
- the **canary probe set** that verifies this particular profile
- the profile's own **content hash**, so a thread record can say which profile
  was in force and mean it

**A provider that cannot render some part of the profile refuses**, and the run
does not dispatch. This is the same reasoning as `unset` not meaning `none`: a
profile silently narrowed to what the provider happens to support produces an
agent that runs perfectly well and a record claiming a boundary that was not
there.

### The v1 bar

Unattended is day one, so two are not negotiable and two are staged:

| | |
|---|---|
| **must** | writes confined to the box |
| **must** | no credential in the box that reaches `origin` |
| **must** | `kept` itself not executable in the box ([0024](0024-signing-is-designed-and-deferred.md)) |
| **must** | a named domain allowlist, because it is what starts the proxy |
| **must** | no keychain grant, ever — see below |
| **must for a broker-capable runtime** | credential brokering per runtime; the child holds only a per-run phantom |
| permitted only when a runtime cannot use a broker | a direct named environment value carrying a documented derived credential, tested as an explicit accepted exposure |

The first three are what the rest of the model already depends on. An agent that
can write outside its box has bypassed the scope check; an agent that can push
to `origin` has bypassed the gate, and [0015](0015-landing-is-a-merge-queue.md)
becomes decorative.

**The keychain rule is not a preference.** On macOS, *any* keychain grant —
even read-only on the single file `login.keychain-db` — makes nono drop all four
keychain-daemon denials at once. Measured from inside such a sandbox: 121 items
enumerable and an unrelated secret retrieved in cleartext. With no grant, zero.
So kept's profile grants no keychain, ever. A runtime authenticates only through
the versioned channel its adapter declares: a broker-capable runtime receives a
brokered per-run phantom, while a runtime that cannot use a broker may receive a
documented derived credential through a named child environment value. The
latter is an explicit accepted exposure: a tool-capable agent inside the box
can read its own credential from its environment.

**Which credential and upstream are the operator's, not this decision's.**
kept requires only the declared credential channel; whether it authenticates a
subscription runtime or a static self-hosted endpoint is configuration. That is
a billing and authorization question rather than an architectural one, and it
does not make a provider, model, or per-repair selection mechanism implicit.

### The canary's probes

Each is the negation of a guarantee stated elsewhere:

| Probe | Guarantee it defends |
|---|---|
| write outside the box | scope enforcement at land |
| write `.kept/**` | standing is minted only by a person ([0016](0016-the-ledger-lives-on-its-own-ref.md)) |
| push to `origin` | every commit on `main` was gated (0015) |
| read a credential path, and enumerate the keychain | the agent holds no secret it was not given |
| reach a Unix domain socket — the Docker socket, the SSH agent | a jail that leaves these open is host root by another route |
| reach a host outside the allowlist | the network claim is true |
| run `kept ratify` | standing is minted only by a person (0024) |

**A probe that cannot be attempted counts as a failure, not as a pass** — and
that rule needs a mechanism, because on its own it is a hope. The canary carries
a **positive control**: a loopback sentinel the parent holds open that no
correct policy grants. If the sentinel is unreachable *and* every escape is
refused, the jail holds. If the sentinel is unreachable and the probes are too,
the canary itself failed to run and the result is not a pass. Without it a dead
probe harness makes an unconfined sandbox look confined, which is the exact
shape of the failure this section exists to catch.

**This is the part of the decision that evidence made load-bearing rather than
prudent.** nono's own reporting cannot be trusted to say when confinement was
not applied — it printed `net proxy` over an unrestricted network for nine
releases — and the vendor's published profile for a coding agent, run under a
gate of this shape, comes back **not confined**: Docker socket, SSH agent,
port 22 and raw TCP all reachable. The canary is what stands between a
configured provider and a confined one.

## Alternatives

**The box is the boundary; confine nothing.** Rejected: it protects the
repository and nothing else, and the thread record would claim containment that
does not exist.

**Confine, and skip the box.** Rejected: they are orthogonal. A perfectly
jailed agent that writes the working tree directly has still bypassed the gate.

**Ask per run.** Rejected with grft: one explicit setting, and unattended runs
have nobody to ask.

**Build a native jail.** Rejected: `sandbox-exec`, namespaces and seccomp are
each a project, and grft demonstrates what happens to that project when it
competes with the work the tool actually exists to do.

**Adopt nono's configuration as kept's model.** Rejected: kept's profile has to
express a writable set derived from a promise's scope, and confinement for
gatherers as well as agents. Those are kept's concepts and would have nowhere
to live.

**Defer confinement to a later increment, as grft did.** Rejected on the
condition grft itself named. Its reasoning was sound for a tool with a person
watching every session; kept's premise removes the person.

## Consequences

- **A thread record carries the provider and the canary's report.** A repair
  contained by a jail that was not actually confining is a different fact from
  one that was, and the assessor should be able to see which.
- **Confinement can refuse to run**, on an unnamed provider, an unavailable
  provider, or a failed canary. That is a dispatch refusal, and it is reported
  as such rather than as drift that could not be repaired.
- **Credential delivery is explicit.** A broker-capable runtime uses a
  per-runtime broker route; its child receives only a non-secret phantom and
  the route injects the credential on the enumerated upstream request. A direct
  named environment value remains permitted only for a documented derived
  credential when a runtime cannot use a broker, and its exposure to a
  tool-capable agent is accepted explicitly rather than hidden by the contract.
- **kept depends on `nono`, at pre-1.0, knowingly.** `AGENTS.md` requires a
  dependency to be a decision with an ADR, and this is it. The project is
  pre-1.0 and says its API may still change; it has shipped this class of
  fail-open twice. That is accepted, and the canary above is the term on which
  it is accepted — not a mitigation added afterwards but the reason the
  dependency is safe to hold at all. The exposure is bounded by the `Jail`
  interface and by the profile being kept's own: replacing the provider means
  writing a second renderer, not rethinking the model.
- **`none` remains legal and remains a decision.** A repository whose operator
  accepts an unconfined agent records that choice rather than defaulting into
  it.
