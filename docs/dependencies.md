# Dependencies

Every non-stdlib module is listed here with its reason. A module in `go.mod`
with no line here is a finding. The rule and the test are
[decision 0029](decision/0029-dependencies-pass-a-test.md); a dependency in the
trust path gets an ADR of its own as well.

| Module | Does | Why not the stdlib | Seam | Replaced by |
|---|---|---|---|---|
| `github.com/goccy/go-yaml` | reads frontmatter and `_schema/*.yaml` | Go has no YAML, and writing one is a tarpit | `internal/corpus`, `internal/schema`. No type of its own appears in a signature outside them | `gopkg.in/yaml.v3`, or any decoder that fills a `map[string]any` |
| `golang.org/x/text` | Unicode NFC normalization, inside the binding hash | Go has no normalization in its standard library, and the tables are not something to write | `internal/canonical`. Only `unicode/norm` is used, and no type of its own appears anywhere — it takes a string and returns one | nothing, short of vendoring the same tables. A change would be a `norm` bump under [0022](decision/0022-canonical-content.md), not a swap |

`golang.org/x/text` is the same case as the parser on criterion 4, and it is
worth saying why rather than leaving it to be re-derived. It normalizes text
that feeds a content hash; it sees no secret, decides no authority, confines
nothing, and writes nothing. What it *would* change if it were wrong is which
bytes are hashed — and that is exactly the failure
[0022](decision/0022-canonical-content.md)'s `norm` version exists to absorb, by
re-hashing old records under the rules they were written with. Criterion 2 names
Unicode outright as something we would get wrong by hand, and it is a Go
sub-repository with no module dependencies of its own.

`goccy/go-yaml` over `gopkg.in/yaml.v3` on criterion 3 alone: yaml.v3 is archived
upstream, so the abandonment the criterion asks about has already happened, and
goccy carries no module dependencies of its own. It is not in the trust path —
it parses content that feeds a content hash and decides no authority, which is
the case 0029 works through — so this line is the whole record.

Expected, and not yet added:

| Module | Does | Admitted because | Seam |
|---|---|---|---|
| `nono` (a binary, not a module) | confines the repair agent | [0017](decision/0017-the-box-is-not-the-jail.md) — trust path, so it has an ADR | `internal/confine` |
