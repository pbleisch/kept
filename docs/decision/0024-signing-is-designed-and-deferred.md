# 0024 — Ratification signing is designed now and implemented later

**Status** accepted · deferred by decision, not by omission · closes a gap
[0011](0011-tracked-formats-must-merge.md) opened

## Context

Standing is created only by a person. That is the model's central claim, and
today it rests on layout: standing is not in the artifact
([0001](0001-standing-and-truth-live-in-the-ledger.md)), the ledger is not in
the box ([0016](0016-the-ledger-lives-on-its-own-ref.md)), and `.kept/**` is
out of scope for every repair.

0011 created a gap nobody followed up. **`merge=union` keeps every line, so
anything that can write the ledger ref can inject a ratification and merge will
preserve it.** There is no way to tell a forged record from a real one. The
integrity of the whole model reduces to write access on a file.

Signing closes that. It also costs UX — key selection, agents that cannot
prompt, providers, verification failure modes — and it defends against an
adversary this system does not yet have.

**The adversary here is an agent gone wild**, on the operator's own machine,
with one principal. Against that, the mechanisms already decided are the ones
that work, plus one that is missing and free. Signing defends against a second
principal or an untrusted contributor, and neither exists yet.

## Decision

**Signing is specified now, implemented when there is a second principal.**

### What is missing and free, and is adopted immediately

**`kept` itself is denied in the box's process policy.** 0017 names `process`
as a confined resource and does not say what it excludes; an agent that can
exec `kept ratify` walks around every layout protection above. This is the
actual defence against the actual adversary, it needs no crypto, and it joins
0017's v1 bar.

### What is decided now so that turning signing on costs nothing

**What is signed:** the canonical serialization of one ratification record —
artifact id, content hash, canonicalization version
([0022](0022-canonical-content.md)), signer, and time. One record, not the file
and not the stream, so a signature survives union merge reordering the lines
around it.

**The byte sequence is pinned now.** This is the part that cannot be retrofitted
without changing what an old record would have meant, and it is why this ADR
exists rather than a note in a backlog.

**Envelope fields are reserved and unused:** `sig`, `sig_alg`, `sig_by`. 0013
already requires a version on the ledger record envelope, so adding them later
is additive.

**Verification policy has a shape:** unsigned records are accepted today; a
future `require_signature: ratify` rejects unsigned ratifications on read. Truth
and dispositions are machine output and are never signed — signing them would
assert that a machine is a person.

### What is deliberately not decided

The provider — SSH signatures via `gpg.format=ssh`, GPG, or something else —
the key discovery story, and what an unattended run does when it holds no key.
That last one is the real design problem, and it is the reason to wait: the
answer depends on whether ratification ever happens outside an interactive
session, and today it never does.

## Alternatives

**Implement signing now.** Rejected: it buys nothing against the current
adversary, and it introduces a failure mode — a run that cannot ratify because
key material is unavailable — into the one act that must never be blocked by
tooling.

**Rely on file permissions.** Rejected as a substitute and adopted as a
supplement. Permissions do not survive a clone, and the ledger is meant to be
cloned.

**Sign every ledger record.** Rejected: it asserts personhood for machine
output, and it puts a signature operation in the tick's hot path.

## Consequences

- **The ledger's central claim is conventional, not verifiable, and this ADR
  says so out loud.** That is better than an unexamined assumption, and it names
  the trigger: a second principal, or a repository more than one person writes.
- **Denying `kept` in the box** is the concrete outcome of this decision, and it
  gets the test `AGENTS.md` requires — a canary probe that attempts `kept
  ratify` from inside the box and requires refusal.
- **A record written before signing exists can never be signed**, because the
  key was not held at the time. Retro-signing would be a lie about when the
  operator agreed.
