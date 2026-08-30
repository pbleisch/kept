---
id: everything-tracked-in-git-merges-without-a-human
type: policy
title: Everything tracked in git must merge without a human
fields:
  statement: >-
    No tracked file is read-modify-written in place, and every tracked path is keyed by
    something already unique — the artifact, the thread, the observation's content hash, the
    run — so two concurrent runs never choose one file and nothing has to arbitrate.
  applies_to: write
  on_fail: refuse
links:
  - { target: decision:0011-tracked-formats-must-merge, relation: cites }
  - { target: decision:0012-a-run-id-is-opaque, relation: cites }
  - { target: decision:0068-an-anchor-is-a-base-and-a-change, relation: cites }
---

There is one operator and there are many concurrent runs of `kept`. That
asymmetry is the whole problem: nobody is at the keyboard when two ticks decide
to write, and a conflict that needs a person to resolve it is a conflict that
sits there until somebody notices the queue has stopped.

Two rules together make it impossible rather than unlikely. Append, never
rewrite — a line added to a stream merges with a line added to the same stream
by union, whoever wrote first. And key every path by something already unique,
so that two runs writing "at the same time" are writing to different files and
there is nothing to merge at all.

The second rule is what makes the first one safe at the directory level, and it
is the reason an id is minted rather than counted: two runs allocating from a
counter choose the same name, and their records merge into one apparent thing
with nothing reporting it. See [[decision:0011-tracked-formats-must-merge]].
