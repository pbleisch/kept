# Dependencies

Every non-stdlib module is listed here with its reason. A module in `go.mod`
with no line here is a finding. The rule and the test are
[decision 0029](decision/0029-dependencies-pass-a-test.md); a dependency in the
trust path gets an ADR of its own as well.

| Module | Does | Why not the stdlib | Seam | Replaced by |
|---|---|---|---|---|
| _none yet_ | | | | |

Expected, and not yet added:

| Module | Does | Admitted because | Seam |
|---|---|---|---|
| a YAML parser | reads frontmatter and `_schema/*.yaml` | Go has no YAML, and writing one is a tarpit | `internal/corpus`, `internal/schema` |
| `nono` (a binary, not a module) | confines the repair agent | [0017](decision/0017-the-box-is-not-the-jail.md) — trust path, so it has an ADR | `internal/confine` |
