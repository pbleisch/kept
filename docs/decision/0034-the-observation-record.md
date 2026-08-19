# 0034 — What an observation is addressed by, and what it merely records

**Status** proposed · does for observations what
[0031](0031-the-binding-preimage.md) did for artifacts · implements
[0014](0014-an-observation-names-its-world.md)'s second keying rule · narrows
`model.Observation`, which carries three fields the record must not · settles
which of [0023](0023-what-may-be-captured.md)'s four layers ship with `command`

## Context

0014 says the content-addressed observation path includes the world, and stops
there. `layout.md` says `observations/sha256/ab/cdef…`, one file per pinned
observation, and that *"two runs that observe the same thing write identical
bytes to one path"*. Between them, everything about the address is settled
except the one thing an address is: which bytes are hashed.

The gap has to close before the first tick, and it closes forever. Every truth
transition names the observations it read, so a preimage that changes later does
not mean recomputing some values — it means every transition in every ledger
citing a file that never existed. This is a one-way door in exactly the sense
[0013](0013-the-gate-makes-concurrency-safe.md) means, and 0031 is the worked
precedent for closing one.

Four sub-questions arrive with it, and each has an answer that looks obviously
right and is wrong.

**Whether to reuse 0031's preimage.** It is right there, it is deterministic and
unambiguous, and reusing it would be one line. The encoding should indeed be
reused. The *text rule* must not be, for a reason that is not visible from either
decision on its own.

**What identifies a measurement.** The obvious list is what `model.Observation`
already holds. Three of its fields are wrong to record and one it needs is
missing.

**What a failed gather leaves.** `model.Observation` has an `Err`, which makes a
failure a kind of observation. 0004 says it is not one.

**What stops a secret becoming a declared field.** 0023 built four layers against
that — declared fields, an explicit extractor, a size cap, and redaction — and
increment 3 ships `command`, which is the escape hatch all four were written for.
Two of the four have a home; two do not.

## Decision

### The encoding is 0031's; the text rule is not, and this is the sharp edge

The preimage is the same tagged, length-prefixed encoding 0031 fixed, for the
same two reasons — deterministic, so two machines agree; unambiguous, so no two
values encode alike — with one difference: **text is hashed as it was measured,
and never normalized.**

0022's five rules exist because a *person* wrote the file and a *checkout* may
rewrite it, so a CRLF working copy must not de-ratify a corpus. Neither half is
true of a gatherer's output. Applying them to a measurement collides
measurements that genuinely differ. Run against this repository's own
`canonical.Normalize`, all four of these produce identical bytes:

| | |
|---|---|
| `"FAIL\ttrailing   \n"` and `"FAIL\ttrailing\n"` | trailing whitespace is data in a log |
| `"line\r\nline2\r\n"` and `"line\nline2\n"` | a tool that emits CRLF and one that does not |
| `"out"` and `"out\n\n\n"` | how many newlines a program actually printed |
| `"café"` in NFD and in NFC | a filename a tool printed back |

Because observations are addressed by content and deduplicated by path, the
consequence is not a wrong digest. It is **the second measurement being
discarded as a duplicate of the first** — the observation cache serving a stale
answer, which is the failure 0014 was written to prevent, arriving through a door
0014 had no reason to check. There is a test that produces exactly that
collision.

So the domain is its own and the version is its own:

```
domain   "kept/observation\nobs:1\n"
fields   gatherer  s<len>:<name@version>
         args      <encoded map, sorted by encoded key>
         world     <encoded map: remote, ref, commit, tree, thread>
         scope     s<len>:<the scope digest, 0033>
         data      <encoded map of the declared fields>
```

`obs:1` rather than `norm:1`, because the two answer different questions and a
record that claimed `norm:1` while not normalizing would be a lie a reader
cannot detect.

### An observation is addressed by what it measured, and records when

| | addressed | why |
|---|---|---|
| `gatherer` — name and version | yes | a gatherer's bug fix must not reuse pre-fix observations |
| `args` | yes | 0014 keys the cache on `(gatherer, args, world)`, and two `command` gathers differing only in `run` must not collide |
| `world` | yes | 0014's rule 2, unchanged |
| `scope` — the digest of [0033](0033-due-is-a-digest-over-the-scope.md) | yes | it is part of what the gatherer saw, and it makes the record verifiable by re-hashing its own fields |
| `data` — the declared fields, and only those | yes | it is the measurement |
| `at` | **no** | see below |

**`args` rather than `scope` as a string.** `model.Observation` carries a
`Scope` and no `Args`, which cannot key what 0014 says the cache is keyed by. A
scope becomes a projection of the args — the same relation `Artifact.Statement`
has to `Fields` — and the whole args map is what binds.

**`at` is recorded and not addressed, and it means first-seen.** Putting a
timestamp in the address would mean no two runs ever produce one file, which
throws away the property `layout.md` says content addressing is for. Leaving it
out entirely would leave nothing to fold by, and 0011 requires derived reads to
fold by event time. So it is in the file, outside the preimage, and the first
writer wins: a second run measuring the same thing finds the path taken and does
not rewrite it.

That has one consequence, and it is load-bearing rather than incidental: **a
promise's freshness cannot come from its observation**, because `at` never
advances for an unchanging world. It comes from the truth stream's heartbeat —
*a record is appended when the verdict changes, or when a gather ran and the last
record is older than `max_age`* — which is what makes `max_age` terminate rather
than firing forever. Anyone reading this rule and thinking the timestamp should
be refreshed is about to break content addressing to fix a problem the heartbeat
already solves.

### An observation names no run, and this is 0013's rule rather than an exception to it

No `writer`, and no `seq`.

0013 argued it already, about a different question, and the argument transfers
whole: *"which machine wrote this record stops mattering once observations are
keyed by world: the same tree measured by the same gatherer is the same
observation, whoever ran it, which is what content addressing already asserts."*

A `writer` would also make two runs write **different bytes** for one
measurement, which is the property `layout.md` claims and would no longer have.
And `seq` recovers order among a run's appends; an observation is written whole
to a path of its own, so there is no order among them to recover.

Attribution is not lost, it is relocated to where it means something. A verdict
is an act performed by a run and its transition names that run; a measurement is
a fact about a tree, and a fact does not have an author. Where two machines
genuinely disagree about one tree, 0013 already says the cause belongs in the
observation's data where a predicate can read it.

### A failed gather leaves no observation

`Err` is not on the record. 0004 is unambiguous — *a failed gatherer produces no
observation, so no predicate runs* — and an error field would make a failure a
kind of observation, which is the collapse the three-valued verdict exists to
prevent. It survives as an in-process value on `model.Observation` and never
reaches the ledger.

What a failure leaves is three things, each already decided elsewhere: a
`gather.receipt` artifact, bounded and redacted (0023); an `unknown` verdict
naming that receipt; and, when the cause was a deadline, a record saying **timed
out** rather than **failed**, because
[0026](0026-nothing-external-is-unbounded.md) requires the two to be tellable
apart from the record.

### The size cap ships with `command`; redaction is deferred on purpose

0023 built four layers and increment 3 ships the escape hatch they exist for.
Two of them are covered by the shape above: **declared fields**, because `data`
carries what the gatherer declared and nothing else, and **the explicit
extractor**, because `command` has no default stdout capture and must name what
it pulls out. The other two need saying rather than assuming.

**The size cap lands with `command`.** 0023: *"An observation has a size cap.
Over it, the gather fails and the verdict is `unknown`, because a payload that
large is the shape a leak arrives in and a predicate that needed it was reading
the wrong thing."* It needs no dependency, it is a comparison, and it is the
layer that bounds the damage the other three do not catch. Two bounds, because
they fail differently: the **raw output** an extractor may read, so that a
runaway program cannot exhaust memory before any layer runs, and the **encoded
observation**, so that a well-formed extractor cannot write a hundred megabytes
into a permanent, tracked, append-only record. Exceeding either is a gather
failure, so the verdict is `unknown`, and the receipt says which bound and by
how much — a cap that reports `not-kept` would be the very confusion the
three-valued verdict exists to prevent.

**Redaction is deferred, and this says so deliberately rather than by
omission.** An extractor can capture a secret straight into a declared field:
`find_all(stdout, "(?P<token>gh[ps]_[A-Za-z0-9]{36})")` is a well-formed
extractor producing a well-declared field under the cap, and the record is
permanent and unrewritable. That is a real exposure and it is being accepted for
now, for the reason 0023 gives — redaction is best-effort by nature and needs a
detector, which is a dependency and an ADR of its own — and bounded by what
0023 already establishes: the residue is a secret in the operator's own
side-ref history, which *"stops being an acceptable residue the moment the
repository is shared, and that is when redaction stops being optional."*

Two things make the deferral honest rather than a hole. The extractor is
**authored**, so a person wrote the capture and can read it in review, which is
not true of the raw-stdout default 0023 refused. And it **binds**: `gather` is in
`_schema/promise.yaml`'s `hash` list, so an extractor cannot be changed without
dropping the promise's standing and requiring a person to grant it again.

### The envelope

`v`, and then the fields above. `v` is on the record from the first write for
0013's reason. It is the **ledger** envelope's version and not `--json`'s: the
two change for unrelated reasons, and coupling them would make a record-shape
bump break every script.

## Alternatives

**Reuse 0031's preimage unchanged.** Rejected above, and it is the one that
would have been taken by default. The four collisions are the argument.

**Address the observation by hashing its serialized JSON.** Rejected for 0031's
reason, restated: a read-back format brings its own decisions about number
formatting and key order into the middle of an identity mechanism, and map
iteration order alone makes it non-deterministic.

**Put `at` in the address.** Rejected: nothing would ever dedup, `layout.md`'s
one-file-per-measurement claim would be false, and the retention window would
fill with identical measurements.

**Keep `writer` for consistency with every other record.** Rejected on 0013's
argument, and on the concrete ground that it breaks identical-bytes. Consistency
that costs the property the design was built for is not consistency.

**Record a failed observation with an error field.** Rejected by 0004. It also
makes every predicate site responsible for checking a flag, and a site that
forgets reports `not-kept` for a machine that could not measure.

**Cap the observation and not the raw output.** Rejected: the extractor runs
first, so a program emitting gigabytes wedges the tick before the cap on the
encoded record is ever consulted.

**Defer the size cap too, since redaction is deferred.** Rejected. The two are
not the same kind of thing: redaction needs a detector and can only ever be
best-effort, while a cap is a comparison that always works and bounds what the
absence of redaction can cost. Deferring the layer that needs nothing, because a
different layer needs something, is how both end up missing.

## Consequences

- **`model.Observation` changes**: `Args` added, `Scope` becomes a projection,
  `Err` stops being durable, and there is no `Writer` or `Seq` to add.
- **Two versions now travel together and mean different things.** `norm:` is
  what an artifact's hash was computed under; `obs:` is what a measurement's
  address was computed under. A reader who conflates them will look for an edit
  that never happened, which is the confusion 0022 was written to prevent.
- **Observations are verifiable by re-deriving the preimage from the record's
  own fields**, which restores the property
  [0018](0018-what-survives-a-crash.md) claims for them — *a partial write is
  detectable by hashing what is there* — that a non-addressed `at` would
  otherwise have taken away.
- **Changing what the scope digest covers is an `obs:` bump**, not an edit,
  because the digest is in the preimage. That is the insurance 0033's ignore
  rules need if they are ever refined.
- **A promise can still put a secret in the ledger**, through an authored
  extractor, until redaction exists. That is written down here so it is a known
  accepted cost rather than something discovered later, and it is the first
  thing to revisit before this repository is shared.
- **Both caps get a test that fires**, per `AGENTS.md`: a gatherer that emits
  past the raw bound, and an extractor that produces past the record bound, each
  asserting `unknown` and a receipt naming which bound.
