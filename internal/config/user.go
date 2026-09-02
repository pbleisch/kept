package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// User is what an operator sets once per machine, rather than once per
// repository.
//
// Identity is the whole of it today. It is deliberately not in
// .kept/config.yaml: that file is tracked, so an operator key in it would
// attribute the next person who clones the repository and ratifies anything to
// whoever committed the config — silently, and permanently, in an append-only
// stream. Git decided this the same way and keeps user.email out of the tracked
// tree. See docs/decision/0048.
type User struct {
	// Operator is who ratifies and releases, written into the act as the person
	// who performed it. Empty means nothing here names them, and an act is
	// refused rather than attributed to a guess.
	Operator string

	// Repair is how a repair runtime is reached on this machine. It is here for
	// the reason Operator is, and decision 0048 already settled that reason:
	// .kept/config.yaml is tracked, so a key naming one person's keychain item
	// would be committed to the repository, be wrong on every other clone, and
	// name a second person's credential in a file the first person authored.
	// The runtime's executable is a path on one machine, the credential is one
	// person's, and the lapse date describes one minting of one token.
	Repair Repair

	// Assess is how the adversarial pass reaches a runtime on this machine, and
	// it is a **second block rather than a flag on the first**.
	//
	// Decision 0078 selects the assessor's runtime, model and effort
	// independently of the repairer's, and the reason is the limit it is trying
	// to get past: two instances of one model can find one substitution
	// reasonable, and the answer to that is a different model or more effort. A
	// field shared with the repairer would make that untunable by construction.
	//
	// An unset block is *no assessor is configured on this machine*, which is
	// not an error: a repair still runs and still reports, and the pass says it
	// did not. See docs/decision/0081.
	Assess Assess
}

// Assess is the runtime the adversarial pass spends, and which model answers.
//
// It embeds [Repair] because *which adapter, which executable, whose
// credential, which versions* is the same question for both, and a second copy
// of those fields would be a second thing to keep right. What is not shared is
// the answer: an operator naming one runtime here and another above is doing
// exactly what 0078 asks for.
type Assess struct {
	Repair

	// Model is which model answers, in the runtime's own vocabulary — an alias
	// or a full name — and empty is the runtime's own default. kept validates
	// that it is one argv value and nothing else: the set of names moves with
	// the vendor rather than with this build, so a list here would be the stale
	// copy of an answer that lives somewhere else.
	//
	// It is the one place kept names a model on the way in, and it does not
	// change what a receipt says: the model a receipt projects is read from the
	// runtime's own system/init, so a session that was given a name it did not
	// honour says so. See docs/decision/0077.
	Model string

	// Effort is how hard it tries, from the runtime's own closed set, and empty
	// is the runtime's default. It is checked against that set here rather than
	// left to the runtime, because a value the runtime cannot parse exits before
	// a session exists — which a person meets at dispatch rather than at the
	// moment they wrote it down.
	Effort string
}

// Repair is the runtime a confined repair uses and the credential it spends.
// Nothing here is a secret: Credential is a reference the confinement provider
// resolves, and kept never reads what it points at. See docs/decision/0057.
type Repair struct {
	// Runtime is the adapter name, and Executable is its absolute path. Neither
	// is guessed: nothing here searches PATH.
	Runtime    string
	Executable string

	// Credential is the reference the provider resolves, as a scheme://...
	// URI. A bare account name is refused because it cannot be told apart from
	// a pasted secret, and this file is the last place a secret should be able
	// to arrive by accident.
	Credential string

	// RuntimeVersions is which runtime versions this machine will dispatch on:
	// the versions kept has reviewed, or those plus unreviewed ones on the same
	// line. Empty is the strict answer.
	//
	// It is here rather than in .kept/config.yaml for the reason Executable is,
	// and decision 0048 already settled it: the installed runtime is a fact
	// about one machine, so a policy about which of its versions are acceptable
	// is one person's risk posture and would be committed to every clone. It
	// also travels with what it qualifies — the executable two fields up is the
	// binary whose version this decides about.
	RuntimeVersions string

	// CredentialExpires is when that credential lapses.
	//
	// It is the lapse rather than the minting, and that is the whole of the
	// answer to "where does expiry live". `claude setup-token` takes no
	// options, prints no expiry and has no revoke, so nothing can read the
	// lifetime back. A mint date plus a lifetime kept assumed would be a second
	// place the answer lived — one kept cannot check, and one that goes
	// silently wrong the day the vendor changes the lifetime. The adapter's
	// check wants the lapse, so the operator writes the lapse, and nothing
	// reads the token to work it out.
	CredentialExpires time.Time
}

type runtimeFile struct {
	Runtime           string `yaml:"runtime"`
	Executable        string `yaml:"executable"`
	Credential        string `yaml:"credential"`
	RuntimeVersions   string `yaml:"runtime_versions"`
	CredentialExpires string `yaml:"credential_expires"`
}

// assessFile spells the runtime half out again rather than embedding
// runtimeFile: the YAML decoder refuses to fill an unexported embedded type,
// and exporting one would put a wire shape in this package's vocabulary for no
// reason. The two are held together by [assessFile.runtime], which is the one
// place the fields are carried across.
type assessFile struct {
	Runtime           string `yaml:"runtime"`
	Executable        string `yaml:"executable"`
	Credential        string `yaml:"credential"`
	RuntimeVersions   string `yaml:"runtime_versions"`
	CredentialExpires string `yaml:"credential_expires"`
	Model             string `yaml:"model"`
	Effort            string `yaml:"effort"`
}

func (f assessFile) runtime() runtimeFile {
	return runtimeFile{
		Runtime:           f.Runtime,
		Executable:        f.Executable,
		Credential:        f.Credential,
		RuntimeVersions:   f.RuntimeVersions,
		CredentialExpires: f.CredentialExpires,
	}
}

type userFile struct {
	Operator string      `yaml:"operator"`
	Repair   runtimeFile `yaml:"repair"`
	Assess   assessFile  `yaml:"assess"`
}

// UserPath is where the user's config lives, and it is reported in the message
// an act is refused with, so that being told to set something comes with where.
func UserPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("this machine has no user configuration directory, so --by or KEPT_BY is the way: %w", err)
	}
	return filepath.Join(dir, "kept", "config.yaml"), nil
}

// ReadUser reads the operator's own config. A machine with no such file reads
// as the zero User rather than an error: not having set an identity yet is an
// ordinary state, and the act that needs one says so itself.
func ReadUser() (*User, error) {
	path, err := UserPath()
	if err != nil {
		return &User{}, nil
	}
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &User{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var f userFile
	if err := yaml.Unmarshal(src, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	user := &User{
		Operator: strings.TrimSpace(f.Operator),
		Repair:   readRuntime(f.Repair),
		Assess: Assess{
			Repair: readRuntime(f.Assess.runtime()),
			Model:  strings.TrimSpace(f.Assess.Model),
			Effort: strings.TrimSpace(f.Assess.Effort),
		},
	}
	for _, block := range []struct {
		key     string
		written string
		into    *time.Time
	}{
		{"repair", f.Repair.CredentialExpires, &user.Repair.CredentialExpires},
		{"assess", f.Assess.CredentialExpires, &user.Assess.CredentialExpires},
	} {
		written := strings.TrimSpace(block.written)
		if written == "" {
			continue
		}
		expires, err := parseLapse(written)
		if err != nil {
			return nil, fmt.Errorf("%s: %s.credential_expires: %w", path, block.key, err)
		}
		*block.into = expires
	}
	return user, nil
}

func readRuntime(f runtimeFile) Repair {
	return Repair{
		Runtime:         strings.TrimSpace(f.Runtime),
		Executable:      strings.TrimSpace(f.Executable),
		Credential:      strings.TrimSpace(f.Credential),
		RuntimeVersions: strings.TrimSpace(f.RuntimeVersions),
	}
}

// parseLapse reads the two spellings an operator would reach for and no others.
// A bare date means the end of that day in this machine's zone, because
// somebody writing 2027-08-23 means the credential is good through the 23rd
// rather than dead at midnight as it begins.
func parseLapse(written string) (time.Time, error) {
	if instant, err := time.Parse(time.RFC3339, written); err == nil {
		return instant, nil
	}
	day, err := time.ParseInLocation(time.DateOnly, written, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither a date nor an RFC 3339 instant", written)
	}
	return day.AddDate(0, 0, 1), nil
}
