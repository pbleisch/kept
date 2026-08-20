package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
}

type userFile struct {
	Operator string `yaml:"operator"`
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
	return &User{Operator: strings.TrimSpace(f.Operator)}, nil
}
