---
id: a-promise-is-read-for-whether-it-can-be-kept-before-a-repair
fields:
  want: a promise is read for whether it can be kept before a repair
---

What the operator wants is to be told, before a repair spends anything, when a promise cannot be kept as it is written. Today the only reader who finds that out is the adversarial pass, and it finds it one attempt at a time, at the end of each, after the money is gone.

Observed on 2026-09-15 and 2026-09-18, repairing `promise:each-clause-of-a-predicate-answers-on-its-own` in two consecutive episodes. Both escalated to a person, and in neither was the repairing agent at fault:

- **Thread `19y86jbnzqjtye6xv1vnxj614s`: four attempts, eight sessions, about 31.57 USD.** Every attempt made `hold_when` read `kept`, and the adversarial pass rejected the last three on the same gap. The gap was in the promise, not the work: the instrument and the tests its body named admitted a shape — a step budget of its own for each answering of a clause — that the statement forbade. A repair that met the instrument was a repair the statement ruled out, and no rework could close that.
- **Thread `58pn1kewp155vmnjt3wp2kja40`: three attempts, about 22.08 USD, all three rejected.** The statement made two claims that collide in one corner. One said a mistake refuses only the clause that makes it; the other said that passing the bound anywhere refuses the whole answering and returns no answers. When a mistake is recorded in an early clause and a later clause passes the bound, one of the two sentences has to give. The attempts took opposite sides of that corner, and the passes argued against whichever side each one took. The last pass said in as many words that the text was in tension with itself.

Both defects were readable from the promise alone — its statement, its body, `hold_when` and the instruments it names — before any box was cut. Each was found only by spending a whole attempt budget and escalating. Decision 0086 already says what to do once one is found: the answer is a revised promise and a new episode, not another attempt. What is missing is finding it first.

## What was decided

A **promise-review pass**: one confined, read-only session, on the same machinery as the adversarial pass. The operator's `assess:` block already names its runtime, model and effort, and nothing new is configured. The session is handed the promise's statement, body, `hold_when`, every instrument it gathers with, and the current tree, and it is asked three things and nothing wider:

1. Can this promise be kept as it is written?
2. Where do its own sentences conflict with each other?
3. Does an instrument admit, or require, a shape the statement forbids?

It produces an **argument a person reads**, and that is all it produces.

There are two ways in, and both are wanted:

- **Explicit: `kept shape <promise> --review`**, beside `--trial`. This is for a person shaping a new promise or revising one. `--trial` asks whether the instrument measures the world; `--review` asks whether the words can be met. Both are cheap compared with an episode, and both matter most before `kept ratify`.
- **Automatic: on the first repair of a promise whose current wording has never been repaired successfully.** This catches the promise nobody thought to review, and both episodes above were exactly that. Nobody runs a check they have not thought of, which is the same reason 0078 keeps the adversarial pass unenumerated.

"Wording" is the ratified content: the hash a grant in force names. "Never repaired successfully" should be read from what the ledger already records. A `landed` act carries the promise it was landed for and the hash of that promise's standing when it landed. So a wording has been repaired successfully when some `landed` act names this promise at the current ratified hash, and it has not when none does. Two limits of that reading, stated so shaping does not rediscover them:

- It counts a **land**, not a repair. A repair that held and was never landed, or a drift a person fixed by hand, leaves no `landed` act, so the automatic form would still run for that wording. That errs toward reviewing too often, not too rarely.
- The thread a `landed` act landed is in its note, as prose, not in a field. Answering this question does not need the thread, so that is not a gap here. It becomes one if shaping decides the answer should depend on which thread landed.

## What it must not do

- **Gate nothing.** It does not refuse a repair, hold one back, or make one wait. A repair runs whether the pass ran, errored, or found nothing, and whatever it argued.
- **Vote on nothing.** A verdict is the re-derivation's, as it is for the adversarial pass. This pass reads no measurement toward a verdict and moves none.
- **Change nothing.** It does not edit the promise, propose a replacement wording into the corpus, or write anything under `docs/`. Revising a promise is a person's act, and ratifying the revision is a person's act.
- **Become a checklist.** Like the adversarial pass, what it argues is unbounded and fits no slot. When something it says turns out to be derivable, that is a derivation for the closed half, not a question for this one.

## Open questions for shaping

- **What does it cost per promise, and who pays?** One session per wording, on the operator's `assess:` credential. Does it count against open question 4's budget, and against the same window as the repair it precedes?
- **Does the automatic form run before the first attempt, or instead of it?** Before means the repair proceeds regardless and the argument sits beside it. Instead means a person reads the argument and starts the repair themselves, which is a gate by another name and is ruled out above unless shaping reopens it deliberately.
- **What happens when the pass cannot run?** An unset `assess:` block, a missing credential, a refused provider, a timeout, or a session that errors. It must not block the repair. How is each of those reported, so that "not reviewed" never reads as "reviewed and found nothing"?
- **Is its argument retained, and under what?** The adversarial pass's argument is a receipt entry on the repair's thread. A review of a wording is about the promise at a hash, not about any one thread. Does it get a thread of its own, a record keyed by artifact and hash, or neither? And does `kept why` show it?
- **Does the automatic form run once per wording or once per episode?** If a review ran and the repair still escalated, does the next episode at the same hash review again?
- **Is `--review` meaningful on an unratified promise?** Shaping is when it is cheapest to act on, and there is no ratified hash yet. If it runs, what does it read against, and is anything retained at all?
- **What does the pass see of the tree?** The captain's decision gives it the current tree. Does it also get prior episodes' arguments for the same wording? Those would have made the second collision above obvious, and they would also make its reading depend on what earlier sessions said.

Nearby, and not duplicated here: `desire:shape-trial-explains-each-clause-instead-of-dumping-instruments` is about how `--trial` reports what an instrument measured. This desire is about whether the words can be met at all, which no measurement answers.
