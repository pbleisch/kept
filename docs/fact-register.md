# The fact register

A proposal, and its recommendation is **not yet**. `docs/nono-surface.md` is the
only fact register this repository has, it is written as prose, and the question
is whether it becomes artifacts in `docs/fact/`.

The short answer: **the schema is wrong and fixing it is free today; the
migration has no reader and should wait for one.** Those are two different
decisions and only the first is urgent.

## What the source actually is

Measured against this tree, 2026-08-25:

| | |
|---|---|
| rows | **79**, in thirteen sections |
| **distinct `N` ids** | **74** — five ids appear in two rows each |
| classifications per entry | three — kind, evidence, failure mode |
| kind | 59 behaviour · 10 concept · 7 format · **3 compound** |
| failure mode | 51 silent · 22 loud · **6 that will not answer in one word** |
| **declared** evidence values | **3** — measured, documented, assumed |
| **distinct** evidence strings in use | **40+** |

Three findings fall out of that, and none of them is *the file is disorganized*.
It is a well-kept register that has outgrown the shape it is written in.

## The evidence column is two fields, and it has already split

`nono-surface.md` declares a three-value evidence vocabulary and then writes
forty different things in that column, because each row needs to say *what class
of evidence this is* and *who established it*, and there is one column:

```
| N14 | groups.include must be a list, never null | … | format | measured | loud |
| N16 | filesystem.deny beats every grant        | … | behaviour | documented + measured | silent |
| N57 | apple-password:// reads the internet class only | … | concept | measured; guide says the opposite | silent |
| N2  | kept pins no nono version                | … | behaviour | assumed — … checks nothing | silent |
|     | …the tool-policy scout report, §8, not re-verified here |
```

`_schema/fact.yaml` already splits them: **`modality`** for the class and
**`observed_by`** for *"the gatherer and version, or the person"*. So the
migration is not a reformatting. It is a decomposition the prose outgrew on its
own, and the payoff is that *show me every fact established by a named test* and
*show me everything nothing establishes* stop being reading tasks.

**The failure-mode column has split the same way.** Six of 79 rows will not
answer loud-or-silent in one word — *"loud when it arrives, and it has never
arrived"*, *"loud if kept ever tried; silent if the requirement is dropped"*,
*"loud (a removed scheme) / silent (a new one kept does not refuse)"*. Those are
not sloppy cells. They are rows whose failure mode is **conditional on something
else in the register**, which a column cannot hold and a link can.

## The ids are aspiring to be slugs

**Five `N` ids appear in two rows each** — N5, N22, N24, N29, N32 — because a
finding is stated in its home section and restated in a topical one. The
restatements are paraphrases, and at least one pair has already drifted:

```
N22 §5  deny_keychains_macos denies ~/Library/Keychains, which is what
        refuses `security dump-keychain`
N22 §9  The keychain probe's refusal comes from a group kept never named
```

Both are true and they are not the same claim; the second is the one that
matters and the first is the mechanism. Rename that group and both rows are
wrong, and nothing says there are two.

This is the failure `AGENTS.md` names more than once — *a rule written in two
places is a rule that can disagree with itself* — and `internal/thread/claude.go`
says it in a comment about this exact provider: *"Two spellings of one fact is
the failure."* N5 is the pure case: *"empty `allow_domain` means unrestricted"*
and *"empty `allow_domain` is unrestricted"*, two rows, one fact.

**A corpus refuses this and prose cannot.** Two artifacts claiming one slug is a
load error naming both files ([0021](decision/0021-identity-is-authored-absence-is-not-an-act.md)),
and it is checked on every read. So the register already has an identity concept
it does not enforce, which is evidence it is artifact-shaped — and it is the
**one reader that exists today**, which is why it is worth stating plainly rather
than filed under tidiness. It is not enough on its own: the same five duplicates
can be reconciled with a script over the table, and the ongoing value — nobody
adds a sixth — is small against 79 files. What it does is date the trigger
below rather than overturn it.

## The vocabulary does not line up, and two rungs are missing

`fact.yaml` offers `verified · claimed · inferred · asserted · conflicted`.
Mapped against what the register actually uses:

| in `nono-surface.md` | `fact.yaml` | |
|---|---|---|
| by construction | `inferred` | ✓ |
| *"guide says the opposite"*, *"supersedes the earlier reading in 0057"* | `conflicted` | ✓ — and `conflicts_with` is already a field |
| *"from the tool-policy scout report, §8, not re-verified here"* | `claimed` | ✓ |
| **documented** (nono's schema or guide says so) | `asserted`? | the vendor asserts it — workable, and it reads oddly |
| **measured** — 32 rows, the backbone | **nothing** | `verified` is *"produced by the system only"* |
| **assumed** — the subject of section 1 | **nothing** | |

**`measured` has no rung.** A person ran a command on this machine and wrote
down what it printed. That is the strongest evidence in this register — it is
what `AGENTS.md` means by *"`nono profile show` on a rendered profile is the
authority, not the guide"* — and `fact.yaml` reserves `verified` for the system,
correctly, on pt's lineage. Filing 32 measured findings as `claimed` or
`asserted` inverts the register's whole ordering.

**`assumed` has no rung either**, and it is the one that matters most. Section 1
is *The assumed-and-silent set*; `AGENTS.md` sends a reader there first; N2 —
*kept pins no nono version and has no compatibility check* — is named there as
**the largest assumed dependency in the surface**. There is no value in
`fact.yaml` meaning *kept relies on this and nothing establishes it*, and
`claimed` is not it: a claim has a claimant, and an assumption has nobody.

Both belong in the ladder:

```
verified   a gatherer produced it, and a tick reproduces it
measured   a person or an agent ran it once and named the command
documented the vendor's own schema or guide says so
inferred   derived from something else here
claimed    a third party said so and nobody re-ran it
asserted   stated without any of the above
assumed    kept relies on it and nothing establishes it
conflicted two sources disagree, and conflicts_with names the other
```

**This costs nothing to fix, today.** `fact.yaml` has `hash: []` — a fact bears
no standing, so nothing is keyed to it — and `docs/fact/` is empty. **Zero
re-ratifications, zero migrations, and it is free exactly once.** After the
first fact exists, changing the vocabulary means rewriting every one.

## What a migration would buy, and what is actually built

Four payoffs are usually claimed for this. Measured against the code:

| | state |
|---|---|
| **a packet carries the facts about what a repair touches** | **not built.** `thread.PacketInput` is `Promise · Why · Observations · PriorAttempts · Policies`. `model.md` says *"packets read facts"*; nothing does |
| conflicts become enumerable | real, and small — four or five rows |
| modality becomes queryable | real, and it is the decomposition above |
| staleness becomes visible | **not available.** A fact has no route to truth, so nothing computes it — and a fact about nono 0.73 cannot even be told from one about 0.80, because N2 |

The first row is the one worth having and the only one that justifies 79 files.
It is the same payoff `self-hosting.md` claims for the policies: today
*"read `docs/nono-surface.md` before changing anything under `internal/confine`"*
is a sentence in `AGENTS.md` that an agent may or may not honour, and as facts
the relevant ones would be **assembled into the packet** rather than hoped for.

It requires a packet change, which is an ADR, and it is a good one to write. It
does not exist now.

## Why not to migrate yet

`nono-surface.md` **works.** 869 lines, ordered by risk rather than by number,
with the silent-and-assumed findings deliberately first; `AGENTS.md` points at
it and it is read. Seventy-nine files lose that ordering, and the ordering is
editorial judgment nothing reconstructs.

Against that, the repository's own habit is explicit and applies exactly:
0043 leaves the plugin wire a sketch because *"building a compatibility surface
with no consumer is the one case where building teaches nothing"*; 0044 refuses
the index on a measurement; 0024 defers signing because the adversary does not
exist. **A fact register whose only reader is a person is a document with
frontmatter.**

So: the test for a fact artifact is the same one `self-hosting.md` used to move
thirteen of `AGENTS.md`'s invariants and leave the rest — **does anything read
it other than a person?** Three readers would qualify: a packet carries it, it
holds a conflict edge worth enumerating, or something can re-derive it. Today
the first does not exist, the third does not exist, and the second is five rows.
The loader's identity check is a fourth and it does exist — it is the only one
that does, and it buys a one-time reconciliation rather than a standing reason.

## The trigger

Stated as [`desire-pipeline.md`](desire-pipeline.md) asks a retirement to state
it, so this does not have to be re-argued from scratch:

> **Migrate `nono-surface.md` into `docs/fact/` when a packet carries facts.**
> That is the reader. Until then the register is a document and the schema fix
> is the whole of the work.

A second, weaker trigger: if the conflict set grows past a handful — kept's
measurement disagreeing with the vendor's documentation is the most valuable
thing in the file and the easiest to lose — `conflicts_with` alone may earn the
migration for that subset.

## What in the file is not a fact

Four sections, and they should not be swept along:

| section | is | becomes |
|---|---|---|
| **How to read an entry** | the register's own schema | `_schema/fact.yaml`, and the classifications above |
| **11 · Rejected: things that look like this surface and are not** | the boundary of the register | stays — and it is a precedent, since the document already discriminates |
| **12 · Reading a nono changelog** | a procedure | a document, or `AGENTS.md` |
| **13 · What a different provider would cost** | an analysis | a document |

Section 11 is worth noticing on its own: the file already keeps an explicit
not-this set, which is the same instinct as `lineage.md`'s *deliberately not
adopted*. Both are registers of what was considered and excluded, and both are
in prose.

## The adjacent want

`observed_by` carries the whole weight here because **`observation` is
unfillable for all 79** — they were established by a person at a shell or by a
Go test, and neither writes a ledger observation. The field exists for a world
where a gatherer measures the provider.

That world is one `command` gatherer away: `nono why --json` against a rendered
profile already answers, before a process exists, about every path kept
declared, and the pre-flight already calls it. A promise naming it would make
provider facts **re-derivable on a tick** rather than measured once and trusted
forever — and it would give N2 its answer, because a gatherer that asks the
provider anything can report what version answered.

That is a desire, not part of this brief, and it belongs in the pipeline the
other brief proposes. It is the shortest path from this register's weakest
property to its strongest.

## Deltas

**Now, and free:**

- `_schema/fact.yaml` — add `measured` and `assumed` to `modality`; document the
  ladder; keep `verified` system-only. Zero re-ratifications, and free exactly
  once.
- One ADR: *the modality ladder, and why `measured` is not `verified`*. Short,
  and it is the one-way part — the enum is what every future fact is written
  against.

**Later, on the trigger:**

- An ADR adding facts to `PacketInput`, with the rule for which facts a packet
  carries. That rule is the hard part and it is not obvious: *every fact* is a
  packet nobody reads, and *the facts about the paths in scope* needs a relation
  from a fact to the code it is about, which nothing currently records.
- The migration itself, and a decision on whether `nono-surface.md` survives as
  the reading surface or is generated from the artifacts. **It should survive.**
  A generated document loses the risk ordering, and a register nobody reads is
  worse than one nothing queries.
