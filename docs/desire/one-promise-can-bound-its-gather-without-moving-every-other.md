---
id: one-promise-can-bound-its-gather-without-moving-every-other
fields:
  want: one promise can bound its gather without moving every other bound
  ripe_when: >-
    One promise's gather needs a deadline that would be wrong for the rest: a
    slow suite that forces timeouts.gatherer up for every gather in the
    workspace, or a fast instrument whose hang goes unnoticed for as long as
    the slowest suite is allowed.
  shaped_from:
    - docs/decision/0026-nothing-external-is-unbounded.md
---

Today a gather has one deadline. It is `timeouts.gatherer` in
`.kept/config.yaml`, or `gather.DefaultDeadline` when that is unset. Every gather
in a tick shares it through `gather.Requests`, and it is the `of 10m` in the
progress line. 0026 configures deadlines per kind of external call, and this want
is finer than that, so realizing it amends 0026.

Where the override lives is the decision. An arg under `gather:` is bound by
`promise.yaml`'s hash list and is part of the observation's address, so tuning a
deadline would de-ratify the promise and start a new series. A field beside
`max_age` binds nothing but travels with the corpus, while the right deadline
depends on the machine running it. An override in config keyed by artifact id
is local and touches no hash. It does mean the progress line and
`Requests.Bound()` carry a bound per request rather than one.
