---
id: 0056-a-native-runtime-declares-nothing-and-names-its-own-scratch
type: decision
title: 0056 — A native runtime declares nothing, and names its own scratch directory
fields:
  statement: >-
    A repair runtime shipped as a self-contained native image is resolved by inspecting the
    file rather than by interrogating it, declares no path at all, and is refused by name when
    it turns out to be a script; the box's temporary directory is published under every name a
    runtime reads it by, decided in the one field that also grants it; and a runtime whose
    credential is answered by a hosted endpoint names that host rather than blocking the
    network.
links:
  - { target: decision:0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port, relation: extends }
  - { target: decision:0053-a-confined-repair-resolves-its-toolchain, relation: extends }
  - { target: decision:0052-repair-runtimes-are-closed-adapters-with-native-receipts, relation: depends-on }
  - { target: decision:0017-the-box-is-not-the-jail, relation: extends }
---

# 0056 — A native runtime declares nothing, and names its own scratch directory

## Context

[0055](0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md) made a
confined Pi run: ask the interpreter where it lives, follow the runtime's own
executable to the package Node reads, declare a home, block the network and name
one loopback port. It says, deliberately, that it is Pi on Node and that a
second hosted runtime revisits it. This is that revisit, and almost none of it
transferred. Four findings, each measured against Claude Code 2.1.241 in a real
box under a real provider.

**It is not Node-hosted, and resolving it as though it were grants what nobody
reads.** The installed runtime is a self-contained Mach-O arm64 image linking
only `/usr/lib`; the npm package's `bin` entry *is* that image, hard-linked from
a platform package. `ResolveNode` does not fail on it — it succeeds, by
accident, because the image happens to sit inside an npm package tree with a
`package.json` above it. What it returns is the Node *installation root*, which
this runtime never opens, and it requires a Node interpreter to be installed at
all, which a machine running the native build need not have. Measured the other
way: at a path the provider answers `path_not_granted` for, a full confined
session behaves identically whether the image's directory is granted or nothing
is, because the provider's command policy grants the launched entrypoint a
direct exec bypass. There is nothing to declare.

**But the shape 0055 was written about is one install away.** The same npm
package ships a `#!/usr/bin/env node` fallback launcher, and its own comment
says it is used whenever the postinstall did not run — `--ignore-scripts` is all
it takes. Handed that file, kept would launch a script under a profile that
resolved no interpreter and get `env: node: No such file or directory` from
inside the box: 0055's measured failure exactly, on a runtime whose decision
says it does not have one.

**It does not read `TMPDIR`.** The binary's temporary-directory resolver reads
`CLAUDE_CODE_TMPDIR` and otherwise returns the literal string `/tmp`; `TMPDIR`
is never consulted. A repair profile denies the host's scratch trees by name
([0053](0053-a-confined-repair-resolves-its-toolchain.md)) and denial beats a
grant, so a confined session without that variable opens `/tmp/claude-<uid>` and
dies with `EPERM: operation not permitted`, exit 1, having emitted not one
stream event. The binary's own guard names the remedy in its error text. So
0053's rule — the box cuts a temporary directory and sets `TMPDIR` — is
necessary and not sufficient: it decided *where*, and a runtime that reads some
other name never learns it.

**And its credential is answered by a host, so the network cannot be blocked.**
0055's endpoint was a self-hosted one on loopback. This runtime's subscription
token is answered by `api.anthropic.com` and by nothing else. Measured with the
network blocked: ten retries with exponential backoff, roughly three minutes,
`ConnectionRefused`, and a receipt that reads like a session that ran and did
nothing. Measured with that one host on the provider's domain allowlist: the
upstream answered in two seconds.

One more thing arrived in the same session and is not confinement. The direct
argv this adapter had been pinned to was read off `--help` and never launched.
It cannot start: `--print` with `--output-format stream-json` refuses without
`--verbose`. A pinned argv that has never started a process is a fixture wearing
an invocation's clothes.

## Decision

**This decision is macOS and this one runtime, deliberately**, the way 0053 and
0055 are. It adds no capability library and no runtime-definition vocabulary. A
second native runtime, or a second platform, revisits it.

### A self-contained native runtime is resolved by inspection, and declares nothing

`ResolveNative` resolves the executable's symlinks and then reads the first
bytes of the file. A Mach-O image — thin or universal — resolves, and returns
**no declared path at all**: no search-path entry, because kept execs an
absolute path and there is no shebang to satisfy; no readable grant, because the
entrypoint's exec bypass is what makes the image runnable and the image reads
nothing else. Granting its directory anyway would be a boundary kept invented
for a read that does not happen, which is the thing 0053 refuses when it says
kept grants what the tool named and no more.

The residual is stated rather than hidden, exactly as 0053 and 0055 state
theirs: what makes this safe is the provider's exec bypass for the launched
entrypoint, and that is the provider's rule, not kept's.

**Anything else is a refusal naming what was found.** A script is refused
against its own interpreter line, because that is the shape whose failure is
silent. A file that is not a native image for this machine is refused as such.
Neither is launched and hoped for.

It takes no context and no deadline, because it starts nothing. 0053's rule is
*ask the tool where it lives*; a single image has no elsewhere to be asked
about, and a version flag answers a different question.

### The box's temporary directory is published under every name that reads it

`Profile.TempEnv` names the additional variables the box's temporary directory
is set as. They are rendered into the provider's `set_vars` with `Temp`'s value,
beside `TMPDIR`, from the one field that also grants the directory — the same
shape `Home` has and for the same reason. An invocation may name one of them,
and exactly one value: `Temp`. A disagreement is a refusal before dispatch,
because the adapter's environment and the rendered policy are two statements of
one fact, and a runtime reading a scratch directory nobody granted is a runtime
dying at a denied path with nothing saying why.

They may not also be on the child's variable allowlist or in the toolchain
variables. A variable with two sources has one value and two policies, which is
the disagreement the field exists to prevent.

### A runtime answered by a hosted endpoint names that host, and only that host

A repair box for a runtime whose credential is answered upstream blocks nothing
and names the one host. It is the provider's domain allowlist, which runs a
proxy, and the proxy works: the upstream's own `401` came back through it
unaltered, which is also what proves the credential channel carries what it is
supposed to carry. Nothing else is named — not a telemetry host, not a
documentation host — and what proves that boundary is the canary's existing
`host_outside_allowlist` probe together with a socket measurement from inside
the box.

A blocked network is still right for a box that only builds. The two are chosen
by what the runtime needs, not by preference, and the profile already refuses a
block standing beside an allowlist.

**And the allowlist grants a proxy, not a socket.** Measured from inside the
box: a proxy-aware client reaches the declared host and gets the upstream's own
reply, while a raw connection to that same host's port 443 is refused, and a
host nobody named is refused through the proxy. It is the fact
[0055](0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md)
records from the loopback side, seen from the other one: naming a domain starts
a proxy and sets `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` in the child rather
than opening anything. The adapter still refuses those names in the *parent*
environment — an operator's proxy is an unreviewed redirect of the credential —
and the two do not collide, because kept never sets them and the provider
always does.

### A pinned argv is measured by starting it

An adapter's fixed argv is admitted only after a real process has accepted it.
`--verbose` joins this runtime's argv because the session refuses to start
without it, and `--replay-user-messages` joins it because it makes the packet's
arrival evidence rather than inference: the runtime re-emits the message it read
as a native event, so a receipt holds the runtime's own account of the bytes it
received even when the session is killed before an assistant turn exists.
`--safe-mode` stays, and its reason is now measured rather than assumed — a
`SessionStart` hook planted in the box's own `.claude/settings.json` runs
without it and does not run with it.

## Alternatives

**Resolve the native runtime with `ResolveNode`.** It is what the installed
layout allows, and it works for the wrong reason: the image sits under a
`package.json` that belongs to the wrapper package rather than to anything it
reads. Rejected because it requires an interpreter this runtime does not use, on
a machine that need not have one, and grants that interpreter's installation
root — eight hundred megabytes nobody resolved a read for.

**Declare the image's directory anyway, for safety.** Rejected on measurement: a
full confined session is identical with the grant and without it. A grant that
changes nothing is a boundary in the record and none in the kernel, which is
worse than saying plainly that the entrypoint's exec bypass is what runs it.

**Grant `/tmp/claude-<uid>` instead of redirecting the scratch directory.**
Rejected, and it does not work: the profile denies the host's scratch roots by
name, denial beats a grant, and 0053 refuses a box inside one for the reason
that a boundary asserted there proves nothing.

**Let the adapter set its scratch variable through the child's allowlist.** It
would function — the provider fills an allowed name from kept's own process, and
kept's is where the adapter put it. Rejected because nothing would then relate
the variable to the grant: an adapter could name a directory the profile denies
and the failure would be an `EPERM` from inside the box, which is the class of
failure this decision exists to convert into a refusal.

**Add a general variable-projection field.** Rejected: it would be a
runtime-definition vocabulary with one user, which is what 0055 declines to
start. `TempEnv` names one directory that already exists in the profile, and it
grows nothing that is not that directory.

**Block the network and reach the upstream some other way.** There is no other
way: the subscription credential is answered by one host on the public internet.
A broker route is the alternative [0052](0052-repair-runtimes-are-closed-adapters-with-native-receipts.md)
also permits, and it is a different channel that this decision does not build.

**Keep the argv that was read off `--help`.** Rejected on measurement: it exits
1 before a session exists. It had been pinned, fixture-tested and reviewed
without ever being started, which is the argument for the rule above rather than
against it.

## Consequences

- **A runtime that is a script is refused by name, before dispatch**, on a
  runtime whose decision says it is not one. That refusal is the only thing
  standing between an `--ignore-scripts` install and 0055's measured failure,
  and it is why resolution exists here at all when it declares nothing.
- **`nono why` is no longer a complete account of what a box can run.** It
  reports `path_not_granted` for an executable the same profile launches
  successfully, because the entrypoint's exec bypass is outside what it answers
  for. That joins the loopback port 0055 already names. Anything added to the
  pre-flight from here is checked against what the provider actually enforces
  before it is believed.
- **`Profile` grows one field and the rendered projection grows one entry per
  name.** The whitelist discipline is unchanged: the renderer emits the fields it
  names and nothing else.
- **A Claude repair box reaches the public internet at one host.** That is wider
  than a Go build box, which reaches nothing, and wider than a Pi box, which
  reaches one loopback port. It is the width the credential requires, it is
  named, and the canary's existing `host_outside_allowlist` probe is what keeps
  it to one. It is narrower than it looks in one respect: nothing in the box
  gets a socket to the internet, because the allowlist is a proxy.
- **The adapter stays unadmitted.** Everything above is what a confined session
  needs in order to start, and all of it is measured. What 0052 requires beyond
  it is a deliberately interrupted live session, which needs a valid
  subscription setup token; `claude setup-token` takes no options and has no
  revoke, so minting one is an act only the operator can perform. `claudeAdmitted`
  is false until that session has been run and recorded.
- **Linux is not covered**, for the reasons 0053 and 0055 give, and the image
  check is Mach-O, which is a second way this is macOS.
