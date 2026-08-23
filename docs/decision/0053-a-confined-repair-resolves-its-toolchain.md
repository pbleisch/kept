---
id: 0053-a-confined-repair-resolves-its-toolchain
type: decision
title: 0053 — A confined repair resolves its toolchain and names no policy group
fields:
  statement: >-
    A confined repair reaches its toolchain by asking the tool where it lives, never by
    naming a provider policy group or a path list kept carries; the confined child's search
    path is declared rather than inherited; the network is blocked unless a host is named;
    the box has a temporary directory of its own; and the launch is refused before dispatch
    when the provider does not grant something kept resolved. Only Go, and only on macOS.
links:
  - { target: decision:0017-the-box-is-not-the-jail, relation: extends }
  - { target: decision:0026-nothing-external-is-unbounded, relation: depends-on }
  - { target: decision:0047-a-promise-binds-what-a-repair-may-touch, relation: depends-on }
  - { target: decision:0051-a-gatherers-name-says-what-it-needs, relation: depends-on }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: depends-on }
---

# 0053 — A confined repair resolves its toolchain and names no policy group

## Context

[0017](0017-the-box-is-not-the-jail.md) settled that confinement is kept's own
profile projected onto a provider, and the tree now cuts a box, renders that
projection, and verifies it with a canary. What it could not do was run a
toolchain. A confined `go build` in a real box failed, and each reason was
separate:

- **The child received the operator's entire search path.** The provider
  synthesizes the confined child's `PATH` from the `PATH` of its own process and
  does not filter that one against the profile's variable allowlist. kept merged
  `os.Environ()` into the provider's environment, so a repair held thirty-odd
  directories of the operator's while kept's rendered policy named none of them.
- **Nothing told the toolchain where its parts were.** Measured: Go finds
  neither its module cache nor its build cache with no `HOME` and no variables,
  and it works with **no `HOME` at all** once the variables are set — which is
  the better channel, because `HOME` is an addressing base for a hundred
  programs while the variables name three directories.
- **A box has no writable temporary directory.** The profile takes the host's
  scratch trees back from the provider's baseline, which is right — they were
  the largest grant nothing in kept ever asked for — and it left the child with
  nowhere at all to write a temporary file.
- **A build-only box reached every host.** `network.allow_domain: []` is the
  provider's spelling of *unrestricted*, not of *none*, so kept's guard forced a
  box that wants no network to name a host it does not want.

The tempting answer to the second one is the provider's own toolchain policy
groups, and it was measured and is wrong for kept. `go_runtime` grants `~/go`
and `/usr/local/go` and **does not cover a Homebrew Go**; that toolchain is
reachable only because the baseline grants `/opt`. So a group is neither
necessary nor sufficient, and a profile carrying hard-coded `$HOME`-relative
paths fails the moment `GOPATH` is not `~/go` — measured, with the error
arriving from inside the box as a module cache that could not be stat'd.

**What settles it is that with the resolved paths in hand, the group buys
nothing.** Naming no toolchain group at all, plus the three resolved paths and
the variables, builds, tests and vets the real kept tree in a box. The
eighteen-group deny baseline applies regardless — to a profile naming no group,
and to `groups.include: []`. What a group adds on top of resolution is width
nobody resolved and nothing checked.

Resolution has its own failure, and it is the reason the fourth part exists: a
machine can put a toolchain somewhere the provider reaches through nothing. The
provider also accepts a path it cannot expand and grants the literal string,
reporting the profile valid. Both produce a boundary that is not the one kept
rendered, and both surface from inside a box as a repair that ran and did
nothing.

## Decision

**This decision is macOS and Go, deliberately, and it is expected to be
revised.** It is the narrow thing that makes a confined Go repair work on the
machine kept is being built on. Linux is not covered and is not claimed; no
other language is resolved; there is no capability library, no definition
vocabulary, no toolchain bundle and no way for a workspace to add one. When a
second toolchain or a second platform has to work, this is revisited — and a
revision that changes what is decided here drops its standing and is granted
again, which is the point of writing it down rather than leaving it in a
comment.

### A toolchain is located by asking it, and the answer is a fact about the machine

kept runs `go env GOROOT GOPATH GOMODCACHE GOCACHE` against an explicit absolute
`go` executable, under a deadline, and records what comes back. The tool is the
authority on its own location: a Homebrew Cellar path, an official tarball under
`/usr/local/go`, a version manager's shim and a Nix store path all answer
correctly, because each one execs a real toolchain that knows where it lives.

What comes back is a **fact about this machine** and never a capability claim.
It is not written down in kept, it is not configuration, and it is not carried
between machines. What kept declares is structural and true everywhere:
a toolchain root and a module cache that must be readable, a build cache that
must be writable, and the variables that name them.

Two of those variables are not locations at all and could not come from a path
list. `GOTOOLCHAIN=local` keeps a module's `toolchain` line from sending a box
to fetch a compiler it cannot reach. `GOFLAGS=-buildvcs=false` is required
because a box is a git worktree whose `.git` is a *file* pointing at the
repository outside the box — so a default build stamps VCS information it cannot
read, and fails.

### kept names no provider policy group

`groups.include` is rendered as an empty list. Not omitted: an omission says
nothing, and an empty list says kept decided. The baseline the provider applies
to every profile is unaffected by this and is what kept relies on for `/usr/bin`,
`/bin` and the credential, keychain, browser-data and shell-configuration
denials.

If a toolchain ever needs an auxiliary path no resolver can find, naming its
group is the escape hatch — and it requires a measured need and is recorded as
unverified width, because that is exactly what it is.

### The child's search path is declared, and it grants nothing

The provider ignores its own variable allowlist for `PATH` and there is no
profile field for it, so the **only** lever is the environment of the provider's
process. kept constructs that environment rather than inheriting one: the
child's search path is exactly what the profile declares, and `HOME` is given to
the provider — which resolves its configuration and every `~`-rooted policy path
against it, and refuses to start without one — while the child's allowlist may
not carry `HOME` or `PATH` at all.

A search path is **resolution, not containment**. A name on it grants nothing
and a name off it forbids nothing, both measured. So kept renders no grant for
these directories, narrows the search path for determinism and legibility only,
and no record may claim a search path contained anything.

### The box has a temporary directory of its own

It is created with the box, removed with the box, and lies **beside** the
worktree rather than inside it, because a temporary directory in the tree puts
every scratch file a build writes into the repair's diff. The profile grants it
and sets `TMPDIR` to it, in one place, so the grant and the variable cannot
disagree.

### A build-only box blocks the network rather than naming a host

`network.block: true` denies everything, and it wins over an allowlist standing
beside it. So a profile either blocks the network or names at least one host,
and doing both is refused: an allowlist under a block is a record saying hosts
are reachable that the sandbox refuses. Measured, all four ecosystems build
offline against a warm cache; network is an install-time need, and no install
capability is decided here.

### kept does not trust its own rendering, and the pre-flight is what makes resolution safe

Before a process exists, kept asks the provider — `nono why --json --profile
<rendered> --path <p> --op <read|write>` — whether the profile it just rendered
actually grants every path kept declared: the box root, the repair scope, each
resolved toolchain path, the box's temporary directory, and every directory on
the declared search path. A denial **refuses the dispatch, naming the path, the
operation, and what asked for it.**

This is the term on which resolution is acceptable. It converts every way a
resolution can be wrong — a toolchain the provider reaches through nothing, a
shim whose own directory is ungranted, a grant the profile's own denials take
back, an unexpanded variable granted as a literal, a provider release that
resolves its baseline differently — into a refusal before dispatch, which
[0017](0017-the-box-is-not-the-jail.md) already provides for: *"Confinement can
refuse to run … That is a dispatch refusal, and it is reported as such rather
than as drift that could not be repaired."* Without it each one is an obscure
failure from inside a box that reads exactly like a repair that did not work.

It is also not kept's invention. `nono why` is the provider's own explain tool,
and asking it is cheaper and more truthful than kept reasoning about what its
own JSON meant.

### What decides that a repair needs Go is the promise, and it already says so

No workspace configuration is added. [0051](0051-a-gatherers-name-says-what-it-needs.md)
put the toolchain requirement in the gatherer's own name — `go/tests` means the
Go toolchain — in a field `_schema/promise.yaml` already binds. A second
per-promise toolchain field would be a second place the same fact lives, and a
workspace-level one would be a knob for something the corpus already states.

## Alternatives

**Name the provider's `go_runtime` groups.** Rejected on measurement: with the
resolved paths present the groups change nothing that is enforced, and they
grant `~/go` and `/usr/local/go` — directories nobody resolved and the pre-flight
never checked. They are also neither necessary nor sufficient: a Homebrew Go is
outside both.

**Extend the provider's built-in `go-dev` profile.** Rejected, and not narrowly.
`extends` imports scalars a child cannot clear. `workdir` cannot be tightened
through inheritance, so a profile extending `go-dev` either refuses to start
non-interactively when the working directory is granted read-only — which is
exactly a kept box — or, with the flag that gets past that, **writes into a path
the profile declared `filesystem.read`**. Both branches were measured. It also
imports `network_profile: developer`, which reaches the model provider and
GitHub and denies `proxy.golang.org`: simultaneously too wide for a repair box
and too narrow for Go.

**Declare the paths in a table kept carries, per platform.** Rejected: it is a
list that is wrong the moment a machine differs, and its being wrong is silent —
the provider validates a profile full of paths that are not there. Resolution
plus a pre-flight is fewer lines and fails loudly.

**Build the capability library — a closed set of definitions, a resolver per
ecosystem, install capabilities, `private-temp` and `ipc` as opt-in
capabilities.** Rejected here, not on its merits but on its size: nothing in the
tree yet dispatches a repair, one toolchain on one platform is what has to work,
and a vocabulary decided before its second user is the framework
[0043](0043-the-catalogue-is-closed-and-a-third-party-is-namespaced.md) refuses
to build ahead of evidence. The four-part shape it argued for — declare,
resolve, render, pre-flight — is adopted; the library is not.

**Pin the toolchain as a provider command policy.** Rejected: a declared command
gets its own sandbox that does not inherit the session's, so making it work
meant restating the toolchain's entire capability set inside the command policy.
That is the same rule in two places, which this repository refuses by name.

## Consequences

- **A toolchain reached only through a shim directory the provider does not
  grant is refused, by name, before dispatch.** That is the correct failure and
  it is a real cost: on a machine whose `go` is a version manager's shim under
  the operator's home, a confined repair does not start until that directory is
  granted. Nothing here adds a way for an operator to grant it, because nothing
  here dispatches a repair yet.
- **A repair may write two paths outside its box**: the toolchain's build cache
  and the box's own temporary directory. Both are named, both are checked by the
  pre-flight, and the thread record should say so. The build cache is what makes
  a repair build in a second instead of downloading the module graph, and that
  trade is deliberate.
- **The box's temporary directory does not redirect Apple's `/usr/bin`
  wrappers.** They find the per-user temporary directory through `confstr` and
  not through `TMPDIR`, so with the host trees denied every one of them becomes
  noisy and a **cgo link fails** — measured, and with a message naming the wrong
  thing. Pure-Go builds are unaffected, and the real kept tree builds, tests and
  vets in a box. A repair whose tree needs cgo on macOS is the first thing that
  re-opens this.
- **Linux is not covered.** The host temporary roots, the baseline group set and
  the `workdir` behaviour are all provider data that may differ, and none of it
  was measured there. On a platform with no verified profile, 0017's existing
  rule applies: the provider stays unset and a run refuses to dispatch.
- **The pre-flight costs one provider invocation per declared path**, about five
  milliseconds each, bounded by the same deadline as the canary. It runs on
  every launch rather than once, so no code path can dispatch a profile nothing
  asked the provider about.
- **`Profile` grows five fields and the renderer four.** The rendered projection
  now carries `groups.include`, `network.block`, `environment.set_vars` and a
  writable temporary directory. The whitelist discipline is unchanged: the
  renderer emits the fields it names and nothing else, because the provider is
  pre-1.0 and its next release may add another way to subtract.
