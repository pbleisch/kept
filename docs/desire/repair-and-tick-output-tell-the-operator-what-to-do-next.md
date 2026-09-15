---
id: repair-and-tick-output-tell-the-operator-what-to-do-next
fields:
  want: repair and tick output tell the operator what to do next
---

What the operator wants from `kept repair` and `kept tick` is a clear account of which work has finished, what state it left, and what the operator is now expected to do. That includes when the answer is nothing. Decision 0085 did this for a repair that *fails*. The same standard is missing on the ordinary path.

Observed on 2026-09-14 and 2026-09-15, repairing `promise:each-clause-of-a-predicate-answers-on-its-own`:

- **A repair's phases are invisible.** The repair session (thread `19y86jbnzqjtye6xv1vnxj614s`) exited after 47 minutes. The after-measurement and anchor followed, and then the adversarial pass started under the same `timeouts.agent`. The progress line simply dropped to `~7m of 1h`, and the operator could not tell whether the run had reset, restarted or failed. No line said "the repair finished and was anchored at 85cc990; the adversarial pass is starting".
- **A tick's result does not say what to do.** `promise:the-confinement-suite-passes` went `not-kept` because one live Pi test was flaky. `kept why` showed the verdict and a disposition, `refused · provider … kept repair verifies it and dispatches`, which reads like an error. It is only the tick declining to dispatch, for a promise that has no `repair` block anyway. Nothing named the failing test, or said whether the operator should repair, investigate or ignore it.
- **Trial output buries the answer.** This is recorded separately as `desire:shape-trial-explains-each-clause-instead-of-dumping-instruments`. It is the same failure: the operator gets the data, but not the reading of it.

Each surface should end with what happened and the operator's next action:

- For each finished phase of a repair, one line saying what it established. For example: session exited with result; re-derived `kept`, and which named tests changed; anchored at `<commit>`; adversarial pass started or skipped, and why.
- When a repair ends, the concrete next steps: `kept review <thread>`, reading the diff, `kept land <thread> --onto …`. Or, when nothing should be landed, say so and why.
- For a tick, per drifted promise:
  - whether it is actionable, meaning repairable, or has no repair block so only a person can fix it;
  - what failed, for example the named failing test for a `go/tests` gather;
  - the next command.

  A disposition that means "not dispatched, by design" must not read as a fault.
- When nothing needs attention, say that plainly. `kept status` already does.

This must stay within 0027: the terminal output is for a person, and `--json` stays the stable machine surface.
