---
id: a-tick-says-when-a-promise-it-measured-has-lost-its-standing
fields:
  want: a tick says when a promise it measured has lost its standing
---

When a ratified promise is edited, its standing lapses with nothing written: the ledger holds a grant for the old wording's hash, and the file now hashes to something else, so the register derives `unratified` for the reason `revised`. That is the model working as intended (0032). What is missing is anyone saying so where the operator is looking.

Today only two surfaces report it:

- `kept status` lists it as needing attention, names both hashes ("granted at sha256:…, and it now says sha256:…"), and exits 1 (`cmd/kept/status.go`).
- `kept why <promise>` shows the old grant against the current hash.

The surfaces an operator runs every day go quiet instead:

- `kept drift` drops it entirely, because drift is `standing != none and truth != kept`. A promise that is now failing and no longer ratified disappears from the drift list, rather than appearing with a reason.
- `kept` (the tick) still gathers and evaluates it — it is still predicated — and records a verdict for the new wording, but prints nothing about the lapse.
- `kept repair` refuses it for want of standing, which is correct and is the first moment the operator learns anything happened.

So an operator whose routine is tick-then-drift edits a promise, and it vanishes from everything they read. The edit may have been deliberate and awaiting ratification, or it may have been an accident (a reflow, a typo fix in a bound field, a merge). Both deserve one line.

What would be enough:

- The tick names, beneath its verdict table, each promise it measured whose standing lapsed by revision since it was granted, with the command that ends it: `kept ratify <promise>` to grant the new wording, or `kept retire <promise>` to let it go.
- `kept drift` says how many ratified-then-revised promises it is not listing, and points at `kept status`, the way `status` already counts what it hides.

Constraints worth keeping:

- Nothing is written to make this visible. Standing stays derived on read; this is presentation over the register, not a new record.
- No exit code moves. 0027's contract for the tick and for drift is unchanged; `kept status` already carries the exit code for this case.
- `--json` already carries standing per artifact; this is about the terminal surface.

Related: `desire:repair-and-tick-output-tell-the-operator-what-to-do-next`, and the note in `AGENTS.md` that ratifying on a branch leaves `main` reading the old wording as ratified, which is the same silence from the other side.
