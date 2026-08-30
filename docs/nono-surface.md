# The nono surface

Every hard dependency kept has on `nono`, the confinement provider
[0017](decision/0017-the-box-is-not-the-jail.md) admits. It is the companion to
[`dependencies.md`](dependencies.md), which carries the module table and one
line for this: a binary rather than a module, so it has no `go.mod` entry and
would otherwise have nothing written down at all.

`nono` is not linked, imported or vendored. kept renders a JSON policy file,
asks `nono why` about it, and runs `nono run`. That is the whole mechanical
seam and it is small. The coupling is not: the file's field names, the
provider's own vocabulary, and a dozen behaviours nobody promised are load
bearing, and the last group is the one this document exists for.

Everything below was verified on **nono 0.74.0** (Homebrew, darwin 25.3.0,
arm64) on 2026-08-23, against the installed binary, its embedded JSON Schema
(`nono profile schema`) and its embedded authoring guide (`nono profile
guide`). Nothing here is repeated from a report without saying so.

## How to read an entry

Each entry carries three classifications, because they fail differently.

**Kind.**

- **format** — a field name or JSON shape kept writes or parses. A rename
  breaks it loudly.
- **concept** — a name kept borrows: a policy group, a URI scheme, an
  operation word. These can be renamed, or keep their name and change meaning.
- **behaviour** — something kept relies on that nono does and may never have
  promised. These are the dangerous ones.

**Evidence.**

- **measured** — a command was run here, on this machine, and its output is
  what the entry says. The command is named.
- **documented** — nono's own schema or authoring guide says so, and the line
  is cited.
- **assumed** — kept relies on it and nothing establishes it.

**Failure mode.** Whether an upstream change breaks kept **loudly** — a parse
error, a refusal, a nonzero exit, a failed canary — or **silently**: a grant
that stops being granted, a denial that stops denying, a check that starts
passing vacuously. A silent one is worth more attention than a loud one.

---

## 1. The assumed-and-silent set

These are the findings, and they are set out in full here rather than left to
be picked out of the tables below. The test for membership is not the evidence
label alone: an entry belongs here when a change upstream would **not announce
itself** — because nothing promises the behaviour, because the documentation
says the opposite of what the binary does, or because what is documented would
fail quietly if it moved. Entries that are merely assumed but fail loudly stay
in their section's table, marked there.

**N1 · The eighteen-group baseline is undocumented, and the documentation says
the opposite.** kept renders `groups.include: []` and relies on nono applying
its full deny-and-system baseline anyway. nono's own authoring guide states, at
the line that would settle it: *"A profile with no `groups.include` has no deny
rules. Always include appropriate deny groups for untrusted workloads."*
Measured, that is false — a profile naming no group is resolved with eighteen
groups, six of them required denials. kept is relying on the undocumented side
of a documented contradiction. If nono ever makes its guide true, kept's boxes
lose the keychain denial, the credential denial, the shell-history denial and
the system read/write grants at once, and **nothing in kept's rendered profile
would change**. What catches it is the canary's `keychain` probe and nothing
else. See [N20](#n20), [N21](#n21), [N22](#n22).

**N2 · kept pins no nono version and has no compatibility check.** There is no
`nono --version` call anywhere in the tree — `grep -rn '"--version"'
internal/confine` finds nothing — and `Profile.Nono`
(`internal/confine/nono.go`) is only required to be an absolute path.
`.kept/config.yaml`'s `confine.provider` is the string `nono` and
`internal/config.Config.Provider` carries it, but nothing dispatches on the
value: `internal/tick/drift.go`'s `Tick.refusal` refuses either way, because no
provider is verified yet. So the version this whole document was verified
against is recorded nowhere a program reads. Contrast
`internal/thread/claude.go`'s `claudeSupport`, whose reviewed versions
`internal/repair.runtimeVersion` asks the binary for and the adapter refuses a
version outside of. There is no equivalent for the provider. **This is the single
largest assumed dependency in the surface**, and it is what makes every other
entry here unenforced at runtime.

**N3 · `nono why --profile` accepting a file path is undocumented.** The whole
pre-flight (`internal/confine/preflight.go`, `askProvider`) passes the rendered
profile's path. `nono why --help` documents `-p, --profile <NAME>` — "Use a
named profile for query context" — where `nono run --help` documents
`<NAME_OR_PATH>`. Measured, `why` does read a path: `nono why --json --profile
./onedomain.json --host example.com` answers from that file's allowlist. If
`why` ever resolved a path as a *name* instead, it would either fail to find it
(loud) or answer about some other profile (silent, and the pre-flight would
pass vacuously about a policy nobody rendered).

**N4 · `nono` synthesizes the child's `PATH` from its own process and ignores
the profile's variable allowlist for it.** There is no field and no flag. This
is why `internal/confine/nono.go`'s `childEnvironment` constructs nono's
environment rather than merging `os.Environ()`, and why `Profile.ToolPath`
exists at all. It has already bitten once: merging the operator's environment
handed every repair the operator's entire search path while kept's rendered
policy named none. It is measured — `TestConfinedChildReceivesExactlyTheDeclared
SearchPath` in `internal/confine/goinbox_test.go` — but it is measured by kept,
not promised by nono. See [N29](#n29).

**N5 · An empty `network.allow_domain` is unrestricted, not none.** Measured:
against a profile whose only network stanza is `"allow_domain": []`, `nono why
--json --host example.com` answers `{"status":"allowed","reason":
"network_allowed"}`. `validateProfile` refuses that shape outright, and it is
the only thing standing between a box and the open internet. Nothing in the
schema states the emptiness rule; it follows from `network.block`'s "Network is
allowed by default". A profile that renders it by accident reaches everything
and reports nothing. See [N13](#n13).

**N6 · An empty `environment.allow_vars` is every variable, not none.** The
same trap by a second door, and this one *is* documented — schema:
`allow_vars` … "When empty or absent, all variables are allowed." kept refuses
an empty `Profile.Env` in `validateProfile`. It is here rather than only in the
table because it is the same silent shape as N5 and a reader scanning for one
should find the other.

**N7 · `filesystem.deny` is a no-op on Linux.** Documented, in the guide:
"Landlock cannot express deny-within-allow, so `filesystem.deny` is a no-op on
Linux", and separately, a `deny` overlapping an `allow`/`read`/`write` makes
nono "refuse to start on Linux rather than silently leave the deny unenforced
… This is a hard error, not a warning". kept renders four denials plus the host
scratch roots (`renderNonoProfile`) and its entire host-temp carve-out
([N24](#n24)) rests on them. On Linux the carve-out either does nothing or
refuses the launch. **kept's confinement is macOS-only in practice today, and
nothing in the code says so** — `nono.go`, `preflight.go` and `toolchain.go`
carry `//go:build !windows`, not `//go:build darwin`. The canary would catch
the no-op half: its `write_host_temp` and `write_kept` probes would report
`reached`, so the failure is loud when it arrives. Two silences remain. Nobody
has run it on Linux, so which of the two failures happens is unknown. And
`canaryReport.valid` requires the `keychain` probe only when `runtime.GOOS ==
"darwin"`, so on Linux the keychain guarantee has no probe behind it at all —
it rests entirely on the group baseline of [N20](#n20).

**N8 · `nono run` and never `nono wrap`.** 0017 records `wrap` as silently
discarding a profile's domain allowlist while printing `net proxy`, measured
across v0.66.0–v0.73.0. kept never invokes `wrap`, so this is a dependency on
`run` *not* acquiring the same regression. Nothing tests for it. Re-verifying
would mean measuring `run`'s allowlist from inside a box, which
`TestConfinedGoBoxReachesNoHost` and `TestConfinedClaudeReachesOnlyItsOne
DeclaredHost` do — so the guard exists, but it is a live-provider test that
skips when nono is not installed. **Not re-verified in this pass** (the `wrap`
half; the `run` half is measured by those tests).

**N9 · The `<SERVICE>_BASE_URL` convention is not in nono's documentation.**
`internal/repair/dispatcher.go`'s `claudeCredentialService = "anthropic"` is
chosen precisely because Claude Code reads `ANTHROPIC_BASE_URL`, and the
comment on `CredentialRoute.Service` states the provider publishes a route's
local address as `<SERVICE>_BASE_URL`. The guide documents `base_url_env_var`
only for `credential_providers` — a different mechanism — and says nothing
about it for `custom_credentials`. The convention *is* measured, but by kept:
`internal/repair/confined_test.go` reads `$ANTHROPIC_BASE_URL` from inside a
box. A bare `_BASE_URL` string in the 0.74.0 binary is consistent with an
uppercase-and-append. If the convention changed, the route would still start,
the phantom would still be published, and the runtime would talk to the real
upstream directly and be refused at the socket — silent, and it would read as
a network problem.

**N10 · The phantom's shape and lifetime are unspecified for
`custom_credentials`.** kept never parses it, which is the mitigation. But
`internal/confine` cannot redact it either, because nono mints it and never
tells kept what it is — so a session that echoes its own environment into a
tool result puts it in the receipt. The guide documents `nono_<64hex>` phantoms
for `credential_providers` only. Measured here only as "a phantom of some
length that is not the credential" (`TestAConfinedClaudeSendsAPhantomAndThe
UpstreamReceivesTheCredential`).

**N11 · A keychain-backed credential reference does *not* block on the
system's approval dialog — for `apple-password://`, and for the reason that
decides it.** `AGENTS.md` and
[0057](decision/0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md)
record the opposite, from a run that hung until it was answered. Re-measured
here deliberately, across five confined runs and three keychain items: no
dialog, and resolution in under two seconds once warm. What decides it is the
item's access entry rather than the scheme. nono resolves `apple-password://`
by running `/usr/bin/security`, an item written by `security add-*-password`
carries partition id `apple-tool:` and a `decrypt` requirement of `identifier
"com.apple.security" and anchor apple`, and `/usr/bin/security` satisfies it —
even for an item created with `-T ''`, whose ACL silently **gained**
`/usr/bin/security` on first read.

So the operator-facing claim is narrower than it was and points the other way:
an unattended repair against a `security`-written item is not blocked. It is
still not general. An item written by a non-Apple-signed program has a
partition list that does not admit `security`, and that case is **not
measured**; nono carries an `Is Keychain access waiting for user approval?`
diagnostic, so it expects to block somewhere. Credential tests still use
`file://`, for the different reason that resolving the real reference would
pull a live secret into the test process.

**N12 · `filesystem.*` path fields are globs.** Documented — "Supports `*`
(single path segment) and `**` (any depth) glob patterns, expanded at sandbox
start". kept renders literal absolute paths and escapes nothing:
`renderNonoProfile` copies `Profile.Read`/`Write`/`ToolRead`/`ToolWrite`
through unchanged, and `validateBoxPath` checks containment, not metacharacters.
Measured: a `read` entry of `<dir>/*` grants every child of `<dir>`, and a query
about the literal path `<dir>/*` answers `path_not_granted`. So a repair scope
whose real path contains an asterisk produces a **wider grant than kept
intended** — silently — while the pre-flight refuses the dispatch on the
literal path, loudly. The wide grant is written to a file that is then not
used, so the standing risk is small; the entry is here because nothing in kept
knows the field is a glob.

---

## 2. The rendered profile

`internal/confine/nono.go` — `nonoProfile`, `nonoCustomCredential`,
`nonoEndpointRule`, `renderNonoProfile`, `writeNonoProfile`. One JSON file per
launch, written 0600 into a fresh temporary directory and removed when the
child is reaped.

<a id="n13"></a>

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N13 | The profile is JSON and `--profile` takes a path | `writeNonoProfile`, `Nono.start` | format | documented (`run --help`: `<NAME_OR_PATH>`) | loud |
| N14 | `groups.include` must be a list, never `null` | `renderNonoProfile` (`= []string{}`) | format | measured | loud |
| N15 | `filesystem.read` / `.allow` / `.deny`, arrays of path strings | `nonoProfile.Filesystem` | format | documented (schema) | loud |
| N16 | `filesystem.deny` beats every grant | `renderNonoProfile`, `hostTemporaryRoots` | behaviour | documented + measured | silent |
| N17 | nono resolves symlinked path entries itself | `hostTemporaryRoots` (renders both forms) | behaviour | measured | silent |
| N12 | `filesystem.*` entries are globs | `renderNonoProfile` | behaviour | documented + measured | silent |
| N18 | `environment.allow_vars` / `set_vars`; `PATH` and `NONO_*` reserved in `set_vars` | `renderNonoProfile`, `validateToolPolicy` | format + behaviour | measured | loud |
| N19 | `set_vars` values undergo nine-variable expansion; an unrecognised `$` is kept literal | `validateToolPolicy` (refuses `$`) | behaviour | documented | silent |
| N6 | empty `allow_vars` means *all* variables | `validateProfile` | behaviour | documented | silent |
| N5 | empty `allow_domain` means *unrestricted* | `validateProfile` | behaviour | measured | silent |

**N14 detail.** Measured: `nono profile validate` on a profile with
`"include": null` answers `JSON syntax error: Profile parse error: invalid
type: unit value, expected a sequence`, exit 1. With `[]` it answers `All 18
group references valid`, exit 0. The comment on `renderNonoProfile` says the
provider rejects `include: null`; that is now measured on 0.74.0.

**N16 detail.** Schema, `filesystem.deny`: "Denies apply regardless of other
allow rules." Measured: with `allow: ["<dir>/tree"]` and `deny:
["<dir>/tree/sub"]`, `nono why --path <dir>/tree/sub --op write` answers
`{"status":"denied","reason":"filesystem_deny","policy_source":
"filesystem.deny"}`. This is what makes the `.kept` tier unreachable inside a
readable box, and what takes the host scratch trees back from the baseline.
See [N7](#1-the-assumed-and-silent-set) for the Linux half.

**N17 detail.** Measured: a profile denying only `/tmp` refuses
`/private/tmp/x` — the answer names `/private/tmp`. A profile denying only
`/var/folders` refuses a path under `/private/var/folders/…`. kept renders both
the written and the resolved form anyway (`hostTemporaryRoots`), which is
belt-and-braces rather than load-bearing; the load-bearing half is that a policy
kept writes and a policy nono enforces relate the same way.

**N19 detail.** Guide §7 names exactly nine: `$HOME`, `$WORKDIR`, `$TMPDIR`,
`$UID`, `$XDG_CONFIG_HOME`, `$XDG_DATA_HOME`, `$XDG_STATE_HOME`,
`$XDG_CACHE_HOME`, `$XDG_RUNTIME_DIR`. Measured: `set_vars` of
`{"GOMODCACHE": "$GOPATH/pkg/mod"}` validates clean — the profile is *valid*
and names a directory that does not exist. `validateToolPolicy` refuses any `$`
in a `ToolEnv` value for exactly this. Also measured: `set_vars` keys `PATH`
and `NONO_THING` are refused at load with named errors
(`PATH is reserved; use allow_vars/deny_vars to control it`; `the NONO_*
prefix is reserved`), which is loud.

### Command policies

`renderNonoProfile` writes two entries under `command_policies`.

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N30 | `command_policies.entrypoint` plus `commands.<name>` with `executable`, `sandbox`, `allow_writable_executable`, `allow_direct_exec_bypass` | `renderNonoProfile` | format | documented (schema) | loud |
| N31 | `commands.<kept>.from.session: "deny"` refuses re-exec of kept from the session edge | `renderNonoProfile`; canary `kept_executable` | concept + behaviour | documented (schema: reserved caller `session`) + measured by canary | silent |
| N32 | A command policy whose executable is missing is **skipped with a warning**, not an error | `execProbe` stats before exec | behaviour | measured | silent |
| N33 | The launched entrypoint gets a direct exec bypass, so a native image needs no filesystem grant | `ResolveNative` declares nothing | behaviour | measured ([0056](decision/0056-a-native-runtime-declares-nothing-and-names-its-own-scratch.md)) | silent |

**N32 detail.** Measured: a profile whose second command policy names a
nonexistent executable runs anyway and prints `[nono] Warning: 1 command policy
not found on PATH, skipping: ghost` to stderr — **even under `--silent`** — and
exits 0. So a wrong `Profile.Kept` would leave the `from.session: deny` policy
silently absent. That is precisely why `execProbe`
(`internal/confine/canary.go`) stats the path first and reports `absent` rather
than folding a missing binary into `refused`. The guard is real and the
upstream behaviour behind it is now measured rather than asserted.

**N30 detail.** Also measured: `allow_writable_executable` and
`allow_direct_exec_bypass` each produce a **validation warning** rather than an
error (`writable_executable_trust_downgrade`, `direct_exec_bypass`). Every
kept-rendered profile carries `allow_writable_executable` on both its commands
and `allow_direct_exec_bypass` on the entrypoint, so `nono profile validate` on
one is expected to report warnings rather than to be clean; a release turning
either warning into an error breaks every launch loudly.

**N33 detail.** The consequence is stated in
[0056](decision/0056-a-native-runtime-declares-nothing-and-names-its-own-scratch.md)
and worth repeating here: `nono why` is **not** a complete account of what a box
can run.

---

## 3. Invocation

`internal/confine/nono.go` — `Nono.start`, `childEnvironment`, `child.Wait`,
`processExit`, `killProcessGroup`.

The exact argv, from `Nono.start`:

```
nono run --silent --no-audit --no-rollback --profile <rendered.json> -- <entrypoint> [args...]
```

<a id="n29"></a>

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N34 | Every flag above exists and means what it says | `Nono.start` | concept | documented (`run --help`) | loud |
| N35 | `nono run` propagates the child's exit code | `processExit` | behaviour | measured | silent |
| N36 | A signalled child arrives as 128+N, not as a signalled nono | `processExit` | behaviour | measured | silent |
| N37 | Under `--silent`, nono's stdout carries only the child's stdout | `Nono.start` (copies stdout to the adapter's stream) | behaviour | measured | loud |
| N38 | nono's own diagnostics go to stderr | `child.diagnostic`, `boundedBuffer` (8 KiB) | behaviour | measured | silent |
| N39 | Killing nono's process group kills the confined tree | `killProcessGroup`, `SysProcAttr{Setpgid: true}` | behaviour | measured by `TestNonoSupervisorDeadlineKillsAndReapsTheRuntimeGroup` | loud |
| N40 | nono refuses to start without `HOME` in **its own** process | `childEnvironment` | behaviour | measured | loud |
| N29 | nono ignores `allow_vars` for the child's `PATH` and derives it from its own | `Profile.ToolPath`, `childEnvironment` | behaviour | measured | silent |
| N41 | nono prepends its own shim directories, named `nono-*`, ahead of the declared search path | `TestConfinedChildReceivesExactlyTheDeclaredSearchPath` | behaviour | measured | loud |
| N8 | `run` does not have `wrap`'s allowlist regression | `Nono.start` (never `wrap`) | behaviour | measured for `run` by live tests; `wrap` from 0017 | silent |
| N42 | The child's working directory is `cmd.Dir` and the box root must therefore be readable | `Nono.start`, `validateProfile` | behaviour | assumed | loud |

**N35/N36 detail.** Measured: `nono run … -- /bin/sh -c "exit 3"` exits 3; `exit
42` exits 42; `kill -9 $$` inside the box makes nono exit **137**. So a runtime
killed *inside* the box reaches `processExit` as `Exit{Code: 137}` and never as
`Exit{Signal: …}`; only kept killing the nono process group itself produces the
signalled shape. Both paths are exercised, and a change to the propagation rule
would rewrite what a receipt says a session did without anything failing.

**N37 detail.** Measured: `nono run --silent … -- /bin/echo HELLO` with stderr
discarded yields exactly `HELLO\n` — six bytes, no banner. Without `--silent`
it also yields exactly those six bytes, so the banner is on stderr either way.
kept pipes this straight into the adapter's stream decoder, so a byte of
nono's own on stdout would corrupt a receipt.

**N40 detail.** Measured: `env -u HOME nono run …` fails with `nono:
Environment variable 'HOME' validation failed: not set`. This is why
`childEnvironment` passes the operator's `HOME` to nono and why
`validateProfile` refuses `HOME` on the *child's* allowlist — it is the
provider's variable, not the box's.

**N42 is assumed, and marked so.** `validateProfile` refuses a readable set
that omits the box root, and its comment states what that prevents: a child
whose `getcwd` is refused, reading downstream as a build that cannot find its
own module. Nothing has measured that failure — the guard has always been
there. It is loud when it arrives (every confined toolchain fails at once), so
it is not in §1.

### The audit family, and why `--no-audit` stays

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N77 | The audit trail records **no filesystem event**, granted or denied | `Nono.start` (`--no-audit`); decision 0059 | behaviour | measured | silent |
| N78 | A denial is reported nowhere else either — not on stderr, not in the trace log | decision 0059 | behaviour | measured | silent |
| N79 | `--audit-integrity` hashes only the **writable** set, before and after, and names no path | decision 0059 | behaviour | measured | silent |
| N80 | With the audit on, `session_started` durably records the entrypoint's **full argv**, unredacted | decision 0059; decision 0023 | behaviour | measured | silent |

**N77 detail.** Measured with `--no-audit` removed from `Nono.start`, against
`TestConfinedRepairReadsItsBoxAndWritesOnlyItsScope` — a session that performs
two writes the rendered profile denies. Read at the raw event log,
`~/.local/state/nono/audit/<session>/audit-events.ndjson`, the session holds
exactly three events: `session_started` (timestamp, argv), `sandbox_runtime`
(timestamp, platform, `tool_sandbox_active`) and `session_ended` (timestamp,
exit code). `session.json` adds `executable_identity` (resolved path + sha256),
`tracked_paths` (the resolved grants), `network_events`, `command_policy_summary`,
`merkle_roots` and `audit_integrity`. Neither denied write appears — not as an
event, not in a summary, not as a count. There is no filesystem event
vocabulary at all: no path, no operation, no time.

**N78 detail.** Measured three ways on the same denial. With `--silent` off,
nono's stderr carries the capability banner and nothing else. With
`RUST_LOG=trace NONO_LOG=trace --log-file`, the 286-line log mentions the
denied path exactly once — inside the argv nono echoes when it execs. The
reason is structural rather than an omission: on macOS the policy is enforced
by the kernel's Seatbelt, so the refusal happens in a process nono is not in.

**N79 detail.** Measured: `--audit-integrity` adds two entries to
`merkle_roots`, one before and one after, and narrows `tracked_paths` to the
writable set alone. So it attests, as one opaque number, that the in-scope
writable tree changed — the set that is not the question, named to no path and
over no operation — and says nothing about anything outside it, which is where
an out-of-scope write would be. `box.Change` already answers the same question
per path and with content.

**N80 detail, and the decision.** kept keeps passing `--no-audit`. Turning it on
buys no evidence for
[`policy:out-of-scope-paths-are-never-landed`](policy/out-of-scope-paths-are-never-landed.md)
and costs something under 0023: the trail durably records the entrypoint's full
argv and the resolved grant paths, unredacted, in a store outside kept's ledger
that kept does not own, does not bound and cannot prune, with no redaction pass
and no record that redaction occurred. Today nothing sensitive is in that argv
— the Claude argv is a pinned constant list plus the reviewed constant ask of
0058, and the packet travels on stdin — but that holds by construction and
nothing enforces it. Cost, measured: about 8 KB and 40 ms per session; the
store on this machine held 14 MB across 21 sessions. See
[decision 0059](decision/0059-an-out-of-scope-path-is-found-in-the-diff.md).

---

## 4. The pre-flight

`internal/confine/preflight.go` — `Nono.preflight`, `askProvider`,
`declaredGrants`, `providerAnswer`, `grant.operation`.

```
nono why --json --profile <rendered.json> --path <p> --op <read|write>
```

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N3 | `why --profile` accepts a **path** | `askProvider` | behaviour | measured; `why --help` documents `<NAME>` only | silent |
| N43 | `why --json` writes one JSON object to stdout with `status`, `reason`, `details` | `providerAnswer` | format | measured | loud |
| N44 | `status == "allowed"` is the only pass; `why` exits **0** for allowed and denied alike | `Nono.preflight`, `providerAllowed` | behaviour | measured | loud |
| N45 | `--op` takes `read` and `write` | `grant.operation` | concept | documented (enum `read`\|`write`\|`readwrite`) | loud |
| N46 | An unreadable profile makes `why` exit 1 with no JSON | `askProvider` (treats it as "could not ask") | behaviour | measured | loud |
| N47 | `why --host` disagrees with `open_port`, so the pre-flight must not ask about hosts | `declaredGrants` asks about paths only | behaviour | measured | silent |

**N43/N44 detail.** Measured shapes on 0.74.0:

```json
{"status":"allowed","reason":"granted_path","granted_path":"/private/tmp","access":"write","source":"group:system_write_macos"}
{"status":"denied","reason":"path_not_granted","details":"Path is not covered by any capability: …","suggested_flag":"--read …"}
{"status":"denied","reason":"filesystem_deny","details":"…","policy_source":"filesystem.deny"}
{"status":"allowed","reason":"network_allowed","access":"Connection to example.com:443 would be allowed"}
{"status":"denied","reason":"proxy_filtered","details":"Domain filtering is active. host example.com is not in the allowlist","policy_source":"proxy domain filter"}
{"status":"denied","reason":"network_blocked","details":"Network access is fully blocked. …","suggested_flag":"--allow-domain …"}
```

`providerAnswer` reads three of those keys and ignores the rest. An **allowed**
answer carries `access` and no `details`, so kept's refusal message — which
prints `reason` and `details` — is only ever built from a denial, where both are
present. Exit status is 0 for every one of these; a release that made a denial
exit nonzero would turn every refusal into "could not ask about", which is a
worse message for the same outcome — loud, and misleading.

**N47 detail.** Measured: against a profile with `"block": true` and
`"open_port": [8901]`, `nono why --json --host 127.0.0.1 --port 8901` answers
`{"status":"denied","reason":"network_blocked"}`. The port *is* reachable from
inside such a box — `TestConfinedPiReachesOnlyTheDeclaredLoopbackPort` measures
that through a socket — so asking the provider would refuse a dispatch that
works. This is [0055](decision/0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md)'s
rule, re-measured. It is silent in the sense that matters: if `why` ever
started answering correctly for ports, nothing in kept would notice or change,
because kept does not ask.

---

## 5. Policy groups and the baseline

kept names **no** group: `renderNonoProfile` writes `groups.include: []` and
[0053](decision/0053-a-confined-repair-resolves-its-toolchain.md) explains why
— with a toolchain's paths resolved by interrogation, a group grants
directories nobody resolved and the pre-flight never checked. What kept does
rely on is the baseline nono applies regardless.

<a id="n20"></a><a id="n21"></a><a id="n22"></a><a id="n24"></a>

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N20 | A profile naming no group is resolved with **eighteen** groups | `renderNonoProfile`; canary | behaviour | measured; guide says the opposite | silent |
| N21 | Six of those are marked **required** and cannot be excluded | nothing in kept excludes one | concept + behaviour | measured | loud if kept ever tried; silent if the requirement is dropped |
| N22 | `deny_keychains_macos` denies `~/Library/Keychains`, which is what refuses `security dump-keychain` | canary `keychain` probe | behaviour | measured | silent |
| N23 | `homebrew_macos` read-grants `/opt/homebrew`, which is why a Homebrew Node interpreter's dynamic libraries resolve with no grant kept wrote | `ResolveNode` doc comment | behaviour | documented (group exists) + argued | silent |
| N24 | `system_write_macos` write-grants the host scratch trees **recursively** — the grant kept takes back | `hostTemporaryRoots`, `NewNono` refusing a box inside one | behaviour | measured | silent |

**N20 detail.** Measured: `nono profile show ./min.json` on a profile with
`"include": []` lists eighteen security groups — `deny_credentials`,
`deny_keychains_macos`, `deny_keychains_linux`, `deny_browser_data_macos`,
`deny_browser_data_linux`, `deny_macos_private`, `deny_shell_history`,
`deny_shell_configs`, `system_read_macos`, `system_read_linux_core`,
`system_write_macos`, `system_write_linux`, `user_tools`, `homebrew_macos`,
`homebrew_linux`, `dangerous_commands`, `dangerous_commands_macos`,
`dangerous_commands_linux` — and `nono profile validate` on the same file
reports `All 18 group references valid`. The number matters because it is what
a changelog entry would move.

**N21 detail.** Measured: `nono profile groups` marks six as `required` on this
platform — `deny_browser_data_macos`, `deny_credentials`,
`deny_keychains_macos`, `deny_macos_private`, `deny_shell_configs`,
`deny_shell_history` — and a profile with `groups.exclude:
["deny_keychains_macos"]` is refused at load with `Cannot exclude required
groups`, exit 1, from `validate`, `show` and `why` alike. Note that
`dangerous_commands_macos` is in the baseline but is **not** required; the
distinction is real and a changelog could move a group across it. (An earlier
brief put the required count at four; on nono 0.74.0 on macOS it is six.)

**N22 detail.** Measured: `nono profile groups deny_keychains_macos` lists
`~/Library/Keychains`, `/Library/Keychains`, `~/.password-store`, `~/.1password`
and three 1Password container paths under `deny.access`; and `nono why --json
--profile ./min.json --path ~/Library/Keychains --op read` answers
`{"status":"denied","reason":"filesystem_deny","policy_source":
"filesystem.deny"}`. So the canary's `keychain` probe fails not because
`/usr/bin/security` is a denied *command* — it is not, `dangerous_commands_macos`
denies only `srm`, `brew` and `launchctl` — but because the keychain **files**
are denied. That is the mechanism behind 0017's categorical rule, and it lives
entirely in a group kept does not name.

**N24 detail.** Measured: against a profile with an empty `deny`, `nono why
--json --path <TMPDIR>/deep/deeper/file --op write` answers `allowed`, `source:
group:system_write_macos`. With kept's `hostTemporaryRoots` list in `deny` the
same query answers `filesystem_deny`. This is the measurement `AGENTS.md`
records as the reason a box may not sit in a temporary directory: a child
confined to a box under `t.TempDir()` writes wherever it likes and every
assertion still passes.

---

## 6. Network

`internal/confine/nono.go` — `Profile.BlockNetwork`, `Profile.Domains`,
`Profile.LoopbackPorts`, `validateProfile`, `renderNonoProfile`.

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N5 | empty `allow_domain` is unrestricted | `validateProfile` | behaviour | measured | silent |
| N48 | `network.block: true` denies every host, and beats an allowlist beside it | `validateProfile` refuses the pair | behaviour | measured | silent |
| N49 | An allowlist starts a **proxy**; it is not a socket grant | canary `host_outside_allowlist`; `Profile.Domains` doc | behaviour | measured by `TestConfinedClaudeReachesOnlyItsOneDeclaredHost` | silent |
| N50 | nono sets `HTTP_PROXY`/`HTTPS_PROXY`/`http_proxy`/`https_proxy` (and `NODE_USE_ENV_PROXY`) in the child, and generates `NO_PROXY` | `internal/thread/claude.go` refuses those names in the **parent**; `TestConfinedClaudeReachesOnlyItsOneDeclaredHost` asserts `env HTTPS_PROXY=http://` in the child | behaviour | measured | silent |
| N51 | Loopback is exempt from the proxy, so naming `localhost` in the allowlist grants nothing | `Profile.LoopbackPorts`, `CheckCredentialRoute` exempting a loopback upstream | behaviour | measured ([0055](decision/0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md)) | silent |
| N52 | `network.open_port` grants **connect and bind** on macOS; there is no connect-only capability | `Profile.LoopbackPorts` doc | concept | documented (schema: "Localhost TCP IPC (connect+bind)") | silent |
| N53 | A configured loopback credential route clears the loopback `NO_PROXY` entry | `internal/repair/confined_test.go` routes to a loopback listener and it works | behaviour | measured (by that test); binary carries `NO_PROXY: clearing loopback via config route match` | silent |

**N48/N5 detail.** Measured, four profiles:

| `network` stanza | `why --host example.com` | `why --host api.anthropic.com` |
|---|---|---|
| `{"block": true}` | `denied` · `network_blocked` | `denied` · `network_blocked` |
| `{"allow_domain": []}` | **`allowed` · `network_allowed`** | **`allowed` · `network_allowed`** |
| `{"allow_domain": ["api.anthropic.com"]}` | `denied` · `proxy_filtered` | `allowed` · `proxy_allowed` |
| `{"block": true, "allow_domain": ["api.anthropic.com"]}` | `denied` · `network_blocked` | `denied` · `network_blocked` |

The second row is [N5](#1-the-assumed-and-silent-set). The fourth is the
precedence: a block beside an allowlist **validates clean** — `nono profile
validate` reports `valid` — and the allowlist then decides nothing, so the
record would name hosts the sandbox refuses. That is the whole of the rule
`validateProfile` enforces when it refuses the pair, measured rather than
argued. The proxy half of N49 is measured from inside a box rather than from
the profile: through the proxy the declared host answers `401` itself, while a
raw socket to the same host's port 443 is refused.

---

## 7. The brokered credential

`internal/confine/nono.go` — `CredentialRoute`, `CredentialEndpoint`,
`CheckCredentialRoute`, `validateCredentialRoute`, `validateCredentialReference`,
`validateCredentialFile`, `validateCredentialPhantom`, `credentialUpstreamHost`,
`nonoCustomCredential`. Assembled in `internal/repair/dispatcher.go` —
`assembleClaude`, `claudeEndpoints`, `claudeCredentialService`,
`ClaudeUpstream`. See
[0057](decision/0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md).

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N54 | `network.custom_credentials.<service>` with `upstream`, `credential_key`, `env_var`, `inject_mode`, `inject_header`, `credential_format`, `endpoint_rules[].method`/`.path` | `nonoCustomCredential`, `nonoEndpointRule` | format | documented (schema + guide table) | loud |
| N55 | A route does nothing unless its service name also appears in `network.credentials` | `renderNonoProfile` writes both | behaviour | documented ("**Important**" in the guide) + measured by `TestARenderedProfileCarriesTheReferenceAndNeverAValue` | silent |
| N56 | `credential_key` accepts a **bare keystore account name**, indistinguishable from a pasted secret | `validateCredentialReference` refuses one; `looksLikeAValue` refuses it a layer earlier | concept | documented | silent |
| N57 | The URI schemes `credential_key` accepts are `op://`, `bw://`, `apple-password://`, `file://`, `env://`, `cmd://` — and **not** `keyring://`, whatever nono's own README shows | `credentialReferenceRefused` refuses `env://`, `cmd://` and `keyring://` | concept | measured (every keyring spelling; see detail) | loud (a removed scheme) / silent (a new one kept does not refuse) |
| N74 | `apple-password://<server>/<account>` reads the login keychain's **internet password** class and no other | `validateCredentialReference` accepts the scheme; nothing in kept picks the class, so the operator's config does | behaviour | measured (see detail) | silent |
| N75 | A reference that resolves to nothing does not refuse the launch: the route is disabled, the phantom is empty, the child gets 503 and nono exits 0 | nothing in kept; there is no pre-flight for resolution | behaviour | measured (see detail) | silent |
| N76 | `keyring://<service>/<account>` is real on the provider's *other* credential surface — `env_credentials` / `--env-credential-map` — where it reads a **generic** password | nothing; that is the direct channel [0052](decision/0052-repair-runtimes-are-closed-adapters-with-native-receipts.md) refuses | concept | measured (see detail) | silent |
| N58 | `env_var` is **required** when `credential_key` is a URI reference | `validateCredentialPhantom` requires `Phantom` | concept | documented | loud |
| N59 | `inject_mode: "header"` and `credential_format` carrying one `{}` | `renderNonoProfile`, `CheckCredentialRoute` | format | documented | loud |
| N60 | `endpoint_rules` are **default-deny when non-empty** and **permit-all when empty or absent** | `CheckCredentialRoute` refuses an empty list | behaviour | documented (schema) + measured by `TestABrokeredClaudeReachesNoEndpointNobodyMeasured` | silent |
| N61 | A rule matches with a query string present; `*` is one segment, `**` is zero or more | `CredentialEndpoint` doc; `claudeEndpoints` names `/v1/messages` only | behaviour | measured ([0057](decision/0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md), with `?beta=true`) | silent |
| N62 | `network.block: true` and a credential route are refused as **contradictory** | `validateCredentialRoute` refuses the pair earlier | behaviour | measured | loud |
| N63 | `upstream` must be HTTPS, or HTTP for loopback only | `credentialUpstreamHost` | concept | documented | loud |
| N64 | The proxy refuses a wrong or absent phantom with 401 before reaching the upstream | nothing in kept; it is the property the channel rests on | behaviour | measured by `TestABrokeredClaudeReachesNoEndpointNobodyMeasured` (`nophantom=401`) | silent |
| N9 | The route's local address is published as `<SERVICE>_BASE_URL` | `claudeCredentialService = "anthropic"` | behaviour | measured by kept's test; **not documented** | silent |
| N10 | The phantom is minted opaquely and never disclosed to kept | `CredentialRoute` has no value field | behaviour | measured | silent |
| N11 | A keychain reference raises the system approval dialog on first resolve — **for `apple-password://` this is now measured false** | tests use `file://` anyway, because resolution reaches a real secret | behaviour | measured (see detail); supersedes the earlier reading in [0057](decision/0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md) | silent (reads as a hang if it returns) |

**N62 detail.** Measured: a profile with `"block": true` beside a
`custom_credentials` route *validates clean* — `nono profile validate` reports
`valid` — and `nono run --dry-run` on the same file fails with `nono:
Configuration parse error: --block-net and --credential are contradictory:
credential injection requires the proxy to be reachable`, exit 1. Two things
follow. The contradiction is a **run-time** check, not a profile-validation
check; and `nono why` answers happily against such a profile, so the pre-flight
would pass on a profile no launch can use. `validateCredentialRoute` refuses it
before either, which is why kept never sees the run-time error.

**N60 detail.** Measured from inside a box against a recording upstream:
`POST /v1/messages` → 200 and reaches the upstream; `POST /v1/organizations` →
403; `GET /v1/messages` → 403; and neither refused request appears in the
upstream's log. The schema states the rule in as many words: "When non-empty,
only requests matching at least one rule are allowed (default-deny). When empty
or absent, all endpoints are permitted."

**N57 detail: `keyring://` does not parse on a credential route.** nono's
README shows `"credential_key": "keyring://gh:github.com/example?decode=go-keyring"`
inside a worked `custom_credentials` entry, and the guide's own field table for
the same key does not list the scheme. The table is right and the example is
wrong. Measured, every spelling — `keyring://service/account`,
`keyring://service:account`, either with `?decode=go-keyring` appended — is
refused identically, before any process exists:

```
nono: Profile parse error: credential_key 'keyring://...' for custom credential
'anthropic' must contain only alphanumeric characters and underscores
(or use op:// / bw:// / apple-password:// / file:// / env:// / cmd:// URI)
```

That message is also where the **bare** name's rule is visible: a keystore
account name must be alphanumerics and underscores, so a hyphenated one is
refused by nono as well as by kept. `credentialReferenceRefused` now names
`keyring` so kept refuses it where it can say which field is wrong.

**N74 detail: `apple-password://` is the internet-password class.** nono
resolves it by shelling out to `/usr/bin/security find-internet-password -s
<server> -a <account> -w`. The guide writes the shape as
`apple-password://account/name`; the binary's own error string says
`apple-password://server/account`, and the binary is right. Both orderings were
measured against a login-keychain **generic** password stored as service
`kept-claude-setup-token`, account `paul`, and neither resolved. The same
reference form against an **internet** password resolved: written to a listener
standing in for the upstream, the request arrived carrying
`Authorization: Bearer <the stored value>` while the child held a 64-character
phantom, which is [N9](#n9) and [N10](#n10) read from the other end.

An operator whose token is a generic password therefore has no reference form
kept can use, and re-storing it is the fix:

```
security add-internet-password -U -s api.anthropic.com -a <account> -w
```

`-w` with no value prompts twice, hidden, so the token reaches neither `argv`
nor shell history.

**N11 detail: no dialog, and the ACL is rewritten silently.** Five confined
runs against three keychain items raised no approval dialog and resolved in
under two seconds once warm. The reason is in the item's own access entry: an
item created by `security add-*-password` carries partition id `apple-tool:`
and a `decrypt` entry whose requirement is `identifier "com.apple.security" and
anchor apple`, and `/usr/bin/security` — what nono invokes — satisfies it. An
item created with no trusted application at all (`-T ''`) resolved too, and its
`decrypt` entry **gained** `/usr/bin/security` between creation and first read,
without anything being answered. So the earlier reading is corrected: for this
scheme, against items the `security` CLI wrote, an unattended repair is not
blocked. A dialog would still be expected for an item written by a
non-Apple-signed program, whose partition list does not admit `security`; that
case is **not measured**, and nono's own `Is Keychain access waiting for user
approval?` diagnostic says it expects to block there. If it ever blocks, kept
sees it as a launch that runs to [0026](decision/0026-nothing-external-is-unbounded.md)'s
deadline rather than as a refusal.

**N75 detail: a reference that resolves to nothing fails quietly.** Measured
with `apple-password://` pointed at a name no item matches: `nono run` exits
**0**, the box comes up, the child's phantom variable is set but **empty**, and
the request is answered `503` by the proxy with `managed credential unavailable
for service 'anthropic': route is configured for proxy-supplied auth`. The
warning `credential_not_found` appears on nono's stderr and in its proxy
warnings block. Nothing refuses the dispatch, and kept has no pre-flight for
resolution — `nono why` answers about paths and hosts, not about whether a
credential reference names anything. A repair against a mistyped reference
therefore looks like a runtime that could not authenticate rather than like a
misconfiguration, which is worth knowing before reading such a receipt.

**A 401 from `api.anthropic.com` is not evidence of a rewrite.** The upstream
answers `OAuth access token is invalid` for any bearer value. Only reading the
request at the upstream separates a phantom passed through from a credential
injected, which is why `internal/repair/confined_test.go` stands a listener up
instead. That is a property of the *upstream*, not of nono, and it is recorded
here because it is the trap a reader re-verifying N9 or N64 will fall into.

---

## 8. Toolchain resolution and the child's environment

`internal/confine/toolchain.go` — `ResolveGo`, `ResolveNode`, `ResolveNative`,
`Toolchain`. These interrogate the *tool*, not nono, so most of the file is not
a nono dependency at all. What is:

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N29 | The child's `PATH` comes from nono's own process, unfiltered | `Toolchain.Path`, `Profile.ToolPath`, `childEnvironment` | behaviour | measured | silent |
| N65 | A directory on the search path grants nothing; the pre-flight asks about it as a **read** anyway, because an unreadable directory cannot be executed from | `declaredGrants` (`"a directory on the confined child's search path"`) | behaviour | measured ([0055](decision/0055-a-confined-runtime-is-resolved-homed-and-reaches-one-port.md): an undeclared interpreter directory reports `env: node: No such file or directory` from inside the box) | loud |
| N66 | The box needs a writable temporary directory or a toolchain fails wherever its own scratch file was used | `Profile.Temp`, `internal/box.Box.Temp` | behaviour | measured ([0053](decision/0053-a-confined-repair-resolves-its-toolchain.md)) | silent |
| N67 | `TMPDIR` is not universal — a runtime reading its own name needs it declared | `Profile.TempEnv` | behaviour | measured ([0056](decision/0056-a-native-runtime-declares-nothing-and-names-its-own-scratch.md): `EPERM … open '/tmp/claude-501'`, exit 1, no stream output) | silent |
| N68 | A path declared in `filesystem.read` that does not exist is accepted silently | `ResolveGo` stats `GOROOT`/`GOMODCACHE`/`GOCACHE`; `validateToolPolicy` stats `Temp` and `Home` | behaviour | from the tool-policy scout report, §8, not re-verified here | silent |

**N68 is taken from a report.** `data/kept-confinement-tool-policy/report.md`
§8 records that the provider accepts a `filesystem.read` entry naming a
directory that is not there, exit 0. kept's mitigation is to stat what it
declares, and it does — but only for the paths listed above, not for
`Profile.Read`/`Write` (which are inside a box kept just cut) or for
`Profile.ToolRead`/`ToolWrite` in general. **Not re-verified in this pass.**

---

## 9. The canary

`internal/confine/canary.go` — `makeCanaryState`, `runCanaryProbes`,
`requiredCanaryProbes`, `canaryReport.valid`, `execProbe`, `RunCanary`,
`CanaryArgs`; and `internal/confine/nono.go` — `Nono.verify`, `sealedCanary`.

Most of the canary depends on the *boundary*, not on nono: it writes, reads and
dials, and a refusal is a refusal whoever enforced it. These are the parts that
depend on nono's behaviour specifically.

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N69 | The canary runs the **same rendered policy** a repair does; only the child environment differs | `launchKind`, `Nono.start` | behaviour | by construction, and it is what makes the canary evidence about a repair | silent |
| N32 | A missing command-policy executable is skipped with a warning | `execProbe` stats first | behaviour | measured | silent |
| N70 | nono's tool sandbox refuses an entrypoint whose parent directory is writable by the launching process | `sealedCanary` chmods the copy and its directory to 0500 | behaviour | documented (schema, `allow_writable_executable`: "trust downgrade for a pinned executable **or parent directory** writable through the sandbox capability set") | loud |
| N22 | The keychain probe's refusal comes from a group kept never named | `keychainProbe` | behaviour | measured | silent |
| N71 | AF_UNIX connect mediation refuses a socket the profile does not name | `unixProbe`; the `unix_socket` probe | behaviour | documented (`--allow-unix-socket` family; `linux.af_unix_mediation` on Linux) | silent |
| N24 | Sentinels must go where the baseline grants nothing — the operator's home, not a temporary directory | `makeCanaryState`, `proveWritable` | behaviour | measured | silent |

**N69 is the reason the canary is worth anything**, and it is also the reason
nothing about a *profile-specific* provider change would be caught: the canary
measures the profile in force, so it catches a baseline that moved and a denial
that stopped denying. It does not catch a grant that quietly widened somewhere
no probe points.

**N71 detail.** kept does not set `linux.af_unix_mediation`. On Linux the
`unix_socket` probe would be refused by Landlock's filesystem rules instead
(the sentinel sits under the operator's home, which the profile grants nothing
under), so the probe should still pass — **argued, not measured**, and it is
part of the same Linux gap as [N7](#1-the-assumed-and-silent-set).

---

## 10. Version and platform

| # | Depends on | kept | Kind | Evidence | Fails |
|---|---|---|---|---|---|
| N2 | No nono version is pinned, asked for, or recorded | `Profile.Nono` (absolute path only) | behaviour | assumed — kept depends on whatever version is installed behaving as this document records, and checks nothing. That there is no such call is measured: `grep -rn '"--version"' internal/confine` finds nothing | silent |
| N72 | `confine.provider: nono` is the config spelling | `.kept/config.yaml`, `internal/config.Config.Provider` | concept | measured (`internal/config/config_test.go`) | loud, and only in a test — nothing dispatches on the value yet |
| N73 | macOS and Linux only; no Windows provider | `//go:build !windows` on `nono.go`, `preflight.go`, `toolchain.go` | concept | documented ([0017](decision/0017-the-box-is-not-the-jail.md)) | loud |
| N7 | `filesystem.deny` is a no-op on Linux, and a deny overlapping a grant is a hard start error there | `renderNonoProfile` renders four denials plus the scratch roots | behaviour | documented (guide) | loud when it arrives, and it has never arrived |

**N72 detail.** The string is documentation today, not dispatch:
`internal/tick/drift.go`'s `Tick.dispose` writes `ledger.Provider` as the
refusal condition whether or not a provider is named, and `Tick.refusal`
distinguishes only "unset" from "not verified by a canary". So renaming the
provider changes a comment in `.kept/config.yaml` and a test fixture, and
nothing else — for now.

---

## 11. Rejected: things that look like this surface and are not

**`core.logAllRefUpdates=always` covering `refs/kept/*` where `true` does
not.** This is a **git** dependency, not a nono one. It lives in
`internal/ledger/ref.go` (`LogAllRefUpdates`, `logAllRefUpdatesKey`,
`ConfigureFetch`) and its measurement is `internal/ledger/reflog_test.go`; see
[0054](decision/0054-a-ledger-that-went-backwards-is-a-refusal.md). It is
listed here only so that a reader who came looking for it stops looking. kept
has a real and unwritten git-behaviour surface — that rule, `FetchRefspec`'s
upstream destination and its glob, union merge on `ledger/**/*.jsonl`, the
reflog's two-ended read — and this document is not it.

**Claude Code's environment variables, argv and version constraint.**
`internal/thread/claude.go`'s `claudeRefusedEnvironment`, `claudeSupport` and
`claudeTempEnvironment` are dependencies on the *runtime*, not the provider.
They appear here only where they touch nono: [N50](#6-network) (kept refuses
`HTTPS_PROXY` in the parent precisely because nono always sets it in the child)
and [N67](#8-toolchain-resolution-and-the-childs-environment).

**Go, Node and Mach-O resolution.** `ResolveGo`, `ResolveNode` and
`ResolveNative` interrogate toolchains. Only what they hand to a profile is a
nono dependency, and that is [N29](#8-toolchain-resolution-and-the-childs-environment)
and [N65](#8-toolchain-resolution-and-the-childs-environment).

**String-matching on nono's messages.** Looked for specifically, because it is
the worst kind of behaviour dependency there is, and **production code does not
do it.** `providerDiagnostic` (`internal/confine/toolchain.go`) captures an
`*exec.ExitError`'s stderr and puts it in kept's own error text without
inspecting it; `askProvider` matches on the `status` **field**, not on prose.
No `strings.Contains` outside a test is applied to anything nono wrote.

Two tests do match on something the provider produces, and neither is a
message. `internal/confine/goinbox_test.go` asserts that every search-path
entry kept did not declare contains `nono-`, which is how it recognises the
provider's shim directories ([N41](#3-invocation)); and
`internal/confine/claudeinbox_test.go` asserts the confined child's environment
carries `HTTPS_PROXY=http://`, which is the proxy of [N50](#6-network). Both
are deliberate, both are in the layer whose job is to measure the provider, and
both fail loudly. That is the right place for a coupling of this kind and the
only place it appears.

---

## 12. Reading a nono changelog

The sections above are grouped by the part of nono they touch, so a release
note maps onto them directly. For each, re-test:

| A release that mentions | Re-test |
|---|---|
| policy groups, the baseline, "required" groups | N20, N21, N22, N24 — `nono profile show` on a `groups.include: []` profile, count the groups, and run the canary |
| profile schema, field renames, validation | N13–N19, N30, N54, N59 — `nono profile schema`, then `go test ./internal/confine/` |
| `nono why`, the explain tool, JSON output | N3, N43, N44, N46, N47 — the pre-flight is the whole exposure |
| network, proxy, allowlists | N5, N48, N49, N50, N51, N53 — and re-measure the empty-allowlist case, which is the one that fails silently |
| loopback, ports, `open_port` | N47, N51, N52 |
| credentials, phantoms, endpoint rules | N54–N64, N9, N10, N11 — `internal/repair/confined_test.go` is the whole measurement and needs no valid credential |
| `run`, `wrap`, exit codes, process supervision | N8, N34–N39, N42 |
| `PATH`, environment, shims | N29, N41, N18, N19, N40 |
| Linux, Landlock, seccomp, AF_UNIX | N7, N71 — the untested half of the surface |
| command policies, tool sandbox, ETI | N30–N33, N70 |

Two habits make this cheap. The measurements in §§2–10 are all one-line
commands against a throwaway profile file, so they can be re-run in a few
minutes. And `go test ./internal/confine/ ./internal/repair/` runs the live
half against a real provider whenever nono is installed, and skips when it is
not — so a green suite on a machine without nono has verified none of this.

## 13. What a different provider would cost

[0017](decision/0017-the-box-is-not-the-jail.md) names the provider as
configurable and keeps `none` expressible. This inventory is what says what
that configurability is actually worth today.

**What would port.** `Profile` is kept's own shape and deliberately not nono's:
`Read`, `Write`, `Domains`, `Env`, `Temp`, `Home`, `ToolPath`, `ToolRead`,
`ToolWrite`, `ToolEnv`, `LoopbackPorts` and `Credential` are stated in kept's
vocabulary, and `validateProfile` enforces rules that are about the *boundary*
rather than about nono. The canary's ten probes — eleven on macOS — are about
the boundary too. So
is `internal/box`, `internal/thread` and the whole packet-and-receipt path. A
second provider would reimplement `renderNonoProfile`, `Nono.start` and
`Nono.preflight`, and about a third of `validateProfile`.

**What would not.** Roughly, the assumed-and-silent set:

- **The baseline.** kept grants nothing outside the box except a resolved
  toolchain, a temporary directory and a home. Everything else a repair
  touches — `/usr/bin`, `/opt/homebrew`, `/System`, and the denials on
  keychains, credentials, shell history and browser data — comes from nono's
  eighteen groups. A provider without an equivalent baseline needs kept to
  *write* that policy, which is a policy library kept deliberately does not
  have ([0053](decision/0053-a-confined-repair-resolves-its-toolchain.md)).
  This is the largest single item.
- **The brokered credential.** [0057](decision/0057-the-credential-is-brokered-and-the-box-holds-a-phantom.md)'s
  whole design — a reference kept never reads, a phantom in the box, a rewrite
  on egress, default-deny endpoint rules — is nono's reverse proxy. A provider
  without one puts kept back on the direct channel, which is the one 0057 was
  written to leave behind.
- **The pre-flight.** `nono why` is an unusual thing for a sandbox to have:
  ask, before a process exists, whether a specific path would be permitted.
  Without it, every resolution error arrives from inside a box as an obscure
  exit.
- **`allow_writable_executable` and the direct exec bypass.** The command
  policy that denies re-exec of the kept binary from the session edge, and the
  bypass that lets a native runtime start with no filesystem grant, are both
  nono's tool sandbox.

**The honest answer**, then: the *interface* is portable and the *policy* is
not. Swapping providers would not be a matter of writing a second
`ConfinementLaunch`; it would be a matter of deciding, for the first time, what
a repair box may reach — because today nono decides most of that and kept
records only the part it added.

---

*Verified against nono 0.74.0 on darwin 25.3.0 (arm64), 2026-08-23. Every entry
marked **measured** was run here against the installed binary, or is a live
test in this repository that runs against it; every entry marked **documented**
cites `nono profile schema` or `nono profile guide` at that version. Four
entries could not be re-verified in this pass and say so where they sit: N8
(`wrap`'s allowlist regression, from 0017), N11 (the keychain approval dialog —
verifying it means raising it), N68 (a nonexistent declared path accepted
silently, from a scout report) and N71 (Linux AF_UNIX, argued, with no Linux
host). They are marked rather than dropped, which is the rule this document is
held to.*
