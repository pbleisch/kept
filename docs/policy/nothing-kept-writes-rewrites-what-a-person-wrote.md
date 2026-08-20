---
id: nothing-kept-writes-rewrites-what-a-person-wrote
type: policy
title: Nothing kept writes rewrites what a person wrote
fields:
  statement: >-
    kept writes only what is missing. An unknown field survives a read and is never
    re-emitted, and a file somebody else authored is reported rather than edited — including
    when kept knows exactly what line it wants in there.
  applies_to: write
  on_fail: refuse
links:
  - { target: decision:0002-artifacts-are-wiki-entities, relation: cites }
  - { target: decision:0042-doctor-adopts-a-corpus, relation: cites }
---

Two halves, and the second is the one that is hard to hold.

The first is frontmatter. A field kept does not know is legal, and it survives
a read-and-write cycle untouched, because a corpus is a person's and a tool
that silently drops what it did not recognise is a tool nobody can extend. The
schema says what binds; it does not say what may be present.

The second is whole files. `kept init` writes the files that are missing and
stops there. It prints the `.gitattributes` rule rather than appending it to
one somebody else authored, and the reason is not caution — `* text=auto`
decides how git stores every file in the tree, and that is not kept's decision
to make once somebody has made it. The same line bounds `kept doctor --fix`,
which writes identity and only identity: what a file is called and what it
claims to be, never what it says.

The tempting version is always a small one — one line appended, one field
normalised, one heading tidied. See [[decision:0042-doctor-adopts-a-corpus]].
