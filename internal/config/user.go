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

type userFile struct {
	Operator string `yaml:"operator"`
	Repair   struct {
		Runtime           string `yaml:"runtime"`
		Executable        string `yaml:"executable"`
		Credential        string `yaml:"credential"`
		RuntimeVersions   string `yaml:"runtime_versions"`
		CredentialExpires string `yaml:"credential_expires"`
	} `yaml:"repair"`
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
		Repair: Repair{
			Runtime:         strings.TrimSpace(f.Repair.Runtime),
			Executable:      strings.TrimSpace(f.Repair.Executable),
			Credential:      strings.TrimSpace(f.Repair.Credential),
			RuntimeVersions: strings.TrimSpace(f.Repair.RuntimeVersions),
		},
	}
	if written := strings.TrimSpace(f.Repair.CredentialExpires); written != "" {
		expires, err := parseLapse(written)
		if err != nil {
			return nil, fmt.Errorf("%s: repair.credential_expires: %w", path, err)
		}
		user.Repair.CredentialExpires = expires
	}
	return user, nil
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
