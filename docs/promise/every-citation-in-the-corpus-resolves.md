---
id: every-citation-in-the-corpus-resolves
type: promise
title: Every citation in the corpus resolves
fields:
  statement: >-
    Every reference this corpus makes — a links: entry, an inline [[…]], a manifest edge —
    resolves to an artifact that is here. No policy cites a decision that is not there.
  gather:
    corpus:
      gatherer: corpus
      root: docs
      scope: ["docs/**"]
  hold_when: >
    obs.corpus.ok
    and count(obs.corpus.broken_links, l => true) == 0
  trigger: cadence
  max_age: 7d
  repair:
    scope: ["docs/**"]
links:
  - { target: decision:0030-a-link-manifest-is-one-file-per-subject, relation: cites }
  - { target: decision:0002-artifacts-are-wiki-entities, relation: cites }
---

`docs/self-hosting.md` schedules this as *every policy cites a decision that
exists*, and calls it out in the same breath as vacuous — there were no
policies, so it was a promise nothing could break. There are thirteen now, each
citing the decisions it rests on, and this is what holds those citations to
something.

It is written over the whole corpus rather than over policies alone, and that is
the stronger reading rather than a looser one. A decision citing a decision that
was never written, a promise pointing at a superseded slug, a body link that
went stale when a file was renamed — all of them are the same defect, and none
of them is more acceptable because a policy was not the one that wrote it. The
harder corpus-side promises `self-hosting.md` asks for are all this shape.

**What it does not check, and the instrument it would need.** That every policy
cites *something* is not observable here. `corpus` declares `ok`, `artifacts`,
`findings` and `broken_links`, and none of those carries the edges that were
made — only the ones that point at nothing. A promise saying *every artifact of
this kind has an edge of this relation to that kind* needs a gatherer that
reports the link graph, and no such gatherer exists. Until one does, the
positive half is authored discipline and the negative half is this.

`obs.corpus.ok` is the guard: it is false for a corpus that loaded and holds
nothing, and a corpus that will not load at all is a gather failure that files
no observation — so `unknown`, never a clean sheet. See
[[decision:0002-artifacts-are-wiki-entities]].
