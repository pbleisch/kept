---
id: 0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port
type: decision
title: 0055 — A confined runtime is resolved, homed, and reaches one named port
fields:
  statement: >-
    A confined repair runtime is located by asking its interpreter where it lives and by
    following its own executable to the package that will be read; the child's home
    directory is declared in the profile rather than inherited or omitted; a self-hosted
    endpoint is reached by blocking the network and naming one loopback port, which the
    provider's explain tool cannot answer for and the canary already proves; and the
    adapter owns when the runtime's input closes. Only Pi on Node, and only on macOS.
links:
  - { target: decision:0053-a-confined-repair-resolves-its-toolchain, relation: extends }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: depends-on }
  - { target: decision:0017-the-box-is-not-the-jail, relation: extends }
  - { target: decision:0026-nothing-external-is-unbounded, relation: depends-on }
---

# 0055 — A confined runtime is resolved, homed, and reaches one named port

## Context

[0053](0053-a-confined-repair-resolves-its-toolchain.md) made a confined Go
*toolchain* work: ask the tool where it lives, declare the search path, block
the network, cut a temporary directory beside the box, and refuse before
dispatch when the provider does not grant something kept resolved. A repair
*runtime* is the other program in a box, and [0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)
will not admit one without a confined, deliberately interrupted live session.
Pi could not start. Four reasons, each measured, and none of them Go's.

**Nothing resolves Node, and the pre-flight could not have caught it.** Pi is a
Node script reached through `#!/usr/bin/env node`, so the interpreter's
directory has to be on the child's declared search path. It was not, and the
failure arrived from inside the box as `env: node: No such file or directory`
and exit 127. The pre-flight asks the provider about paths kept *declared*, and
kept declared nothing about Node — so the one mechanism 0053 built to turn this
class of failure into a refusal had nothing to ask about.

**A search path is not a grant, and the file Node reads is not the interpreter.**
`pi` is a symlink into a package tree Node loads. On this machine both the
interpreter and that tree are under `/opt/homebrew`, which the provider's
`homebrew_macos` baseline grants read *recursively* — the same accident 0053
already names for a Homebrew Go. Measured elsewhere: `~/.nvm/versions/node/…`,
`~/.volta/…`, `~/.npm-global/lib/node_modules` and `/usr/local/lib/node_modules`
— where `npm install -g` puts the runtime on a stock machine — are covered by no
capability at all, and a Node script under a directory the provider grants
nothing under fails from inside the box with `MODULE_NOT_FOUND` and exit 1.

**A runtime needs a home and may not inherit one.** 0053 refuses `HOME` on the
child's allowlist, and correctly: the provider fills the child's environment
from its own process, so an allowed `HOME` is the operator's. But a Node program
with no `HOME` calls `getpwuid` and finds the operator's home anyway, and 0052's
Pi credential channel is a projection addressed by `HOME` and
`PI_CODING_AGENT_DIR` *both* — an adapter that set only one would leave the
other resolving somewhere nobody projected. So the child's home has to be a
value kept decides: not inherited, and not absent.

**A self-hosted endpoint is on loopback, and loopback is not a host.** The
provider's domain allowlist is a proxy, and it sets `NO_PROXY=localhost,127.0.0.1`
in the child — measured — so a loopback request bypasses the proxy and is then
refused at the socket. An allowlist naming `localhost` reaches nothing. The
provider's lever for loopback is a port capability, and on macOS the only one is
connect **and** bind.

And one more, which is not confinement at all but arrives in the same session.
**Pi's RPC session ends at input EOF.** Writing the packet and closing the
child's input truncated the run after the user message — exit 0, no assistant
turn, no tool call, no cost, no turn boundary. Measured. A completed session
that did nothing is precisely the receipt 0052's evidence contract exists to
refuse, and it is indistinguishable from a repair that had nothing to do.

## Decision

**This decision is macOS, Node and Pi, deliberately.** It is the narrow thing
that makes a confined Pi repair run on the machine kept is being built on. It
adds no capability library, no runtime definition vocabulary and no way for a
workspace to declare one; a second hosted runtime or a second platform revisits
it.

### A Node-hosted runtime is resolved the way a toolchain is

kept runs `node -p process.execPath` against an explicit absolute Node
executable, under a deadline, and records what comes back. Node is the authority
on where Node is: a Homebrew Cellar path, an official tarball, a version
manager's shim and a Nix store path all answer correctly, because each one execs
a real interpreter that knows where it lives. The directory it names is declared
on the child's search path, so the shebang resolves; the installation root — the
parent of that directory, which is the layout every Node distribution has — is
granted readable.

The runtime's own executable is then followed to the file Node will actually
read: symlinks resolved, then up to the nearest ancestor holding a
`package.json`. That is Node's own definition of a package boundary rather than
a depth kept guessed, and that directory is granted readable. An executable with
no package root above it is refused rather than granted its own directory and
hoped for.

What resolution does not cover is stated rather than assumed: a Homebrew Node's
dynamic libraries live outside its installation root, and the box reaches them
because the baseline grants `/opt`. kept grants what the tool named and no more;
the residual is the baseline's, exactly as it is for a Homebrew Go under 0053.

### The child's home is declared, like its search path and its temporary directory

The profile carries the confined child's home directory. It is rendered into the
provider's `set_vars` as `HOME` and granted writable, in one place, so the grant
and the variable cannot disagree — the same shape 0053 gave the box's temporary
directory. It lies outside the box, because a runtime's private state inside the
tree would arrive in the repair's diff, and a read-only one is not the safer
version: Pi takes a lock to *read* its own stores, and the lock is a directory
beside the file.

`HOME` stays refused on the child's allowlist, and is now refused in the
toolchain variables as well, because one field decides it. A box that needs no
home declares none, which is what a Go build box does.

An invocation may name `HOME`, and exactly one value: the one the profile
declares. A disagreement is a refusal rather than a silent loss, because the
adapter's environment and the rendered policy are two statements of one fact and
a runtime that reads a home nobody projected is a runtime reading the operator's.

### A self-hosted endpoint is one named loopback port, and the canary verifies it

A box that must reach a self-hosted endpoint blocks the network and names the
endpoint's loopback port. Measured, against the rendered profile: the named port
answers, another loopback port does not, and no external host does.

**The provider's explain tool cannot answer for this, so kept does not ask it.**
`nono why --host 127.0.0.1 --port 1234` reports `network_blocked` against the
very profile that reaches it — measured — so a pre-flight question about the
endpoint would refuse a dispatch the sandbox permits. What proves the boundary is
the canary's own `host_outside_allowlist` probe, which dials a loopback port kept
never named and must be refused. That probe already existed; naming one port is
what gives it something to tell apart.

The capability grants connect **and** bind, because macOS has no connect-only
lever — the provider's is Landlock and Linux-only. A confined repair can
therefore listen on the port it may reach. That is accepted width, named here so
it is not discovered later as an oversight.

A private-address endpoint is not this. It is a host, it needs the allowlist and
the proxy, and nothing here decides it.

### The adapter owns when the runtime's input closes

The confinement launch hands the child's input to the adapter and closes it only
if the adapter did not take it. A runtime whose protocol ends at input EOF holds
it open and closes it on the event its own vocabulary calls settled — for Pi,
`agent_settled`, which is documented as the point after which no retry,
compaction or queued continuation remains.

This is the same seam that already existed for a runtime needing a response
before its next request; what changes is that the adapter receives a closer
rather than a writer, so the decision to end the session belongs to the
component that can read the runtime's own events. The parent's deadline, kill
and reap are untouched: they remain the supervisor's, and an adapter that never
closes the channel is bounded by them rather than by hope.

## Alternatives

**Declare the interpreter's directory and stop.** It is what works on this
machine, and it works for the wrong reason: `/opt/homebrew` is baseline-granted
read recursively. Rejected because the failure it leaves is silent and
unbounded — a `npm install -g` runtime under `/usr/local/lib/node_modules`, which
the provider grants nothing under, produces `MODULE_NOT_FOUND` from inside a box
and reads as a repair that ran and did nothing.

**Name the provider's `node_runtime` policy group.** Rejected on measurement,
the same way 0053 rejected `go_runtime`. It grants `~/.nvm`, `~/.fnm`, `~/.npm`,
`~/.node`, `~/.local/share/fnm`, `/usr/local/lib/node_modules` and two pnpm
directories — eight paths nobody resolved and the pre-flight never checks — and
it still covers neither a Homebrew Node, nor `~/.volta`, nor a Nix store, nor an
`npm config set prefix` install. Neither necessary nor sufficient, and its width
is invisible in the record.

**Read the interpreter's Mach-O headers and grant its libraries.** Rejected: it
makes kept a linker, it is platform-specific in a second way, and the thing it
would buy on this machine is already covered by the baseline. The honest version
is to say which grant the baseline is making, which is what the consequences do.

**Put the child's home in the toolchain variables.** Rejected: the variable would
be in one field and the grant it must agree with in another, which is the two
places 0053's temporary directory exists to avoid. The refusal of `HOME` there is
what keeps that true.

**Allow `HOME` on the child's environment allowlist.** Rejected, and it is not a
matter of taste: the provider fills an allowed name from its own process, and
that process is kept's, whose `HOME` is the operator's. The allowlist is the one
channel that cannot carry this value.

**Reach the endpoint through the domain allowlist.** Rejected on measurement:
the allowlist runs a proxy and exempts loopback from it, so naming `localhost`
grants nothing and the connection is refused at the socket. It would also start
a proxy — a whole outbound path — for an endpoint on the same machine.

**Ask the provider about the port in the pre-flight.** Rejected on measurement:
it answers `network_blocked` for a port the same profile reaches, so the
pre-flight would refuse every dispatch that works. 0053's rule is that kept does
not trust its own rendering; where the provider cannot answer, the check moves to
the canary rather than being dropped or faked.

**Let the launch close the child's input after writing the packet.** Rejected on
measurement: Pi exits at EOF, so the session ended after the user message with a
clean exit code and an empty receipt.

## Consequences

- **A runtime the provider reaches through nothing is refused by name, before
  dispatch.** The interpreter's directory, its installation root and the
  runtime's package root are all declared, so all three go through the
  pre-flight. On a machine whose Pi is an `npm install -g` under a prefix nobody
  granted, a confined repair does not start until that path is granted, and
  nothing here adds a way to grant it.
- **A repair may write one more path outside its box**: the runtime's home,
  which is the configuration projection 0052 requires and Pi's stores write into
  while reading them. It joins the toolchain build cache and the box's temporary
  directory, it is named, it is pre-flighted, and the thread record should say so.
- **A confined repair can bind the loopback port it may reach.** macOS offers no
  connect-only capability, so the endpoint's port is bidirectional. Nothing in a
  repair box listens today, and a future one that did would be reachable only
  from this machine's own loopback.
- **The provider's `nono why` is no longer a complete account of a rendered
  profile.** It answers for filesystem paths and for hosts, and it is wrong about
  a declared loopback port. Anything added to the pre-flight from here is checked
  against what the provider actually enforces before it is believed.
- **Linux is not covered**, for the same reasons 0053 gives: the baseline group
  set, the loopback capability and the temporary roots are all provider data that
  differ, and none of it was measured there.
- **`Profile` grows two fields and the rendered projection two.** The projection
  now carries `network.open_port` and a `HOME` entry in `environment.set_vars`,
  and the writable set grows the child's home. The whitelist discipline is
  unchanged: the renderer emits the fields it names and nothing else.
