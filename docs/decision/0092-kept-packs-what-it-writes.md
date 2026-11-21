---
id: 0092-kept-packs-what-it-writes
type: decision
title: 0092 — kept packs what it writes, because nothing else will
fields:
  statement: >-
    Every object kept writes to the ledger and sessions refs is written through
    plumbing, and plumbing never runs git's own automatic maintenance. So kept
    runs it: once a process has handed git 256 MiB of content for one
    repository, the store runs `git repack -d --geometric=2` in that repository
    after the write that crossed the bound. The bound counts bytes, not objects.
    A repository whose operator set gc.auto to 0 is never packed. A packing that
    fails or is cut off loses nothing, because the write it follows is already
    durable.
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: extends }
  - { target: decision:0089-session-records-live-on-their-own-ref, relation: extends }
  - { target: decision:0026-nothing-external-is-unbounded, relation: depends-on }
  - { target: decision:0087-a-receipt-is-journaled-and-sealed-into-the-ledger, relation: depends-on }
---

# 0092 — kept packs what it writes, because nothing else will

## Context

The two repair runs that validated 0090 and 0091 repaired
`promise:a-receipt-costs-the-ledger-about-what-it-holds` in a scratch clone. The
first run left **156,526 loose objects, 101.9 GiB**, and the second left
**45,664, 53.6 GiB, in one hour**. Free space on the machine went from 801 GB to
693 GB during the first run. A `git gc` afterwards packed each into a pack that
grew by 136 MiB and 71 MiB. It took 10m44s and 6m57s of wall time, with peak
resident sets of 6.9 GB and 10.2 GB.

Two things together produce those numbers.

The first is the subject of the promise. On `main` a receipt line is its own
commit, and each commit rewrites the whole receipt. An episode therefore writes
one copy of the receipt per line, and the receipt reached 34.9 MB. That is
0087's to fix, and the promise is broken so that a repair can fix it.

The second is not the promise's subject, and it would stay after 0087 lands.
**kept never packs anything.** `RefStore` writes with `hash-object -w`,
`update-index`, `write-tree`, `commit-tree` and `update-ref`. Git's automatic
maintenance (`git maintenance run --auto`, which runs `gc --auto`) is triggered
by porcelain such as commit, merge, fetch and am. It is never triggered by
plumbing. A repository that only kept writes to accumulates loose objects until
a person runs `git gc`. Git's own trigger would not have helped much anyway:
`gc.auto` counts objects, 6,700 by default, and a receipt write adds about four.
That is roughly 1,600 writes between packings, which is several gigabytes at the
receipt sizes measured.

## Decision

**kept is its own porcelain for maintenance.** After a ledger or sessions write
lands, the store adds the bytes it handed git to a process-wide count for that
repository. When the count reaches `ledger.PackThreshold`, 256 MiB, the writer
resets it and runs:

```
git repack -d -q --geometric=2
```

That one command does both halves of the job. It puts every loose object into a
pack, and it merges the smallest packs until each pack holds at least twice as
many objects as the next. The number of packs therefore grows with the logarithm
of what was written, not with the number of packings. Merging is also what lets
git store each version of a receipt as a delta of the one before it, rather than
once whole in every small pack.

**The bound counts bytes, uncompressed, as kept handed them to git.** Bytes are
what fill a disk, and kept knows them at the write without asking git. Loose
objects are zlib-compressed, so the loose bytes on disk at any moment stay below
the bound.

**It runs in the writer, after the ref has moved, and nothing about it can undo
the write.** If a packing fails, is cut off at its five-minute deadline (0026), or
is skipped, the objects stay loose, exactly as they did before this decision.
The next crossing tries again. Git writes a new pack under a temporary name and
removes nothing until the pack is complete, so a packing that is killed leaves
the repository as it found it. One process runs one packing at a time per
repository. A second writer that crosses the bound while one runs does not start
another.

**`gc.auto = 0` is honoured.** An operator who set it has said the repository is
never packed automatically. Git's porcelain respects that, and kept does too.

## Consequences

**Measured locally, the bound costs no write time.** Three thousand
receipt-shaped appends through `RefStore` hand git about 1.5 GB. Without packing
they leave 12,000 loose objects, 86 MiB. With a 256 MiB bound, the most loose
bytes seen at any sample was 22 MiB, and the run ended with 7.4 MiB loose and
1.9 MiB in three packs. The mean write took 47 ms against 48 ms without packing,
and the slowest single write, the one that packed, took 0.85 s. A 64 MiB bound
held loose bytes to 8.5 MiB and cost about 10% in write time, so 256 MiB is the
chosen bound. The real-run before and after figures are in the parity report for
the repair runs that followed.

**The CPU that `git gc` used to spend at the end is now spent during the run.**
For a receipt on `main` that is a lot of CPU, because every version of a 30 MB
file is still written and then deltified away. The work is roughly the same as
the gc it replaces. The difference is that disk use is bounded throughout,
instead of growing until someone notices. Once 0087 lands, a write hands git
about what the line holds, and packings become rare.

**It packs repositories kept does not own the history of.** `repack -d` only
packs loose objects and merges packs. It does not prune unreachable objects, and
it does not touch a cruft pack or a reflog, so nothing a person could still
recover is lost. A packing does change which files are in `.git/objects`. A
person who relies on loose objects staying loose has gc.auto set to 0, or would
have it set, for git's own porcelain.

**A test that writes a lot through a store may pack.** The suites write well
under 256 MiB per repository. The promise's named tests take the same time with
this decision as without it, measured at 12.5 and 12.7 seconds. A test that needs
to see a packing lowers its own store's bound. The bound is a field on the store
for that reason, and the process-wide default is a constant.

## Alternatives

**Run `git maintenance run --auto` after every write.** This was rejected
because the bound would then be git's, and git's bound counts objects. At receipt
sizes, 6,700 loose objects is gigabytes. It would also cost a git process on
every write, which is a measurable share of a write.

**Lower `gc.auto` for kept's own calls with `-c gc.auto=…`.** This was rejected
for the same reason, because it is still a count. It would also override an
operator's setting rather than honour it.

**Run `git gc --auto` detached, in the background.** Git's own detach leaves a
process running after kept exits, under no deadline that kept holds. A synchronous
packing in the writer is bounded, is observed, and is cheap at this bound.

**Write packs directly with `git fast-import` or `pack-objects --stdin`.** This
would avoid loose objects altogether. It was rejected because it would be a
second write path beside the one every ledger guarantee is proven against. The
compare-and-swap, the reflog witness of 0054 and the crash tests all rest on the
plumbing sequence this decision leaves alone.

**Wait for 0087.** 0087 removes the receipt's O(n²) writes, and it should land.
But it removes them only for the receipt. Every ledger stream is still rewritten
whole on every append, and nothing would ever pack any of it. The two decisions
are independent, so this one does not wait.
