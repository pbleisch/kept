---
id: standing-and-truth-are-never-in-an-artifact-file
type: policy
title: Standing and truth are never written to an artifact file
fields:
  statement: >-
    Standing and truth are ledger records keyed by a content hash, never fields in a file.
    Nothing that may write the corpus may write either, and `.kept/**` is out of scope for
    every repair whatever a promise declares.
  applies_to: land
  on_fail: refuse
links:
  - { target: decision:0016-the-ledger-lives-on-its-own-ref, relation: cites }
  - { target: decision:0032-standing-is-a-fold-over-acts, relation: cites }
---

The repair agent can write the corpus. That is not a concession, it is the
whole point of a repair. What it must never be able to write is the record of
whether anyone approved the result, or the record of whether the result holds.

Two mechanisms enforce that and both are load-bearing, which is why this is a
policy and not a note. The ledger lives on a ref no box checks out, so the box
never has the bytes; and `.kept/**` is out of scope for every repair, so a
promise that declared a scope reaching it would still not reach it. Either one
alone fails the day the other is changed for a good reason.

There is a third thing this forbids that is easier to walk into: a `ratified:`
or `verdict:` field in frontmatter, added to make a listing quicker. It would
be a second place the answer lived, and a second place is a cache that can
disagree with its source. See [[decision:0032-standing-is-a-fold-over-acts]].
