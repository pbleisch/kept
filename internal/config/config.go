// Package config reads .kept/config.yaml: the knobs an operator sets, and
// nothing else.
//
// It is deliberately narrow. Five of the keys in that file decide what kept is
// allowed to do — how long a gatherer, an agent and a Git call may run
// (docs/decision/0026), what a workspace excludes beyond its ignore files
// (docs/decision/0033), and which confinement provider is named
// (docs/decision/0017) — and a run that ignored them would enforce something
// other than what the operator wrote. Everything else in the file belongs to
// increments that have not arrived, and reading a key nothing honours would be
// a knob that silently does nothing.
//
// The two deadlines beside the gatherer's arrived with the verb that spends
// them. `kept repair` runs an agent and several Git calls, and a repair that
// invented its own bounds would be a second place the operator's answer lived —
// which is the one thing this package exists to prevent.
//
// The corpus root is read by internal/corpus and not here. Finding the corpus
// is that package's business, and it needs the answer before anything else can
// run; this reads the same file for the keys that are the world's rather than
// the corpus's.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
)

// Config is what the file says, as far as anything reads it.
type Config struct {
	// Exclude is what this workspace excludes beyond .git, .kept and its
	// ignore files, in the ignore format. See docs/decision/0033.
	Exclude []string

	// Gatherer is how long a gather may run. Zero means the file named none,
	// and gather.DefaultDeadline applies — never unbounded, because nothing
	// external runs without a deadline. See docs/decision/0026.
	Gatherer time.Duration

	// Agent is how long one confined repair session may run, and Git bounds
	// every Git call a repair makes — cutting the box, snapshotting it, and
	// reading back what changed. Zero means the file named neither, and the
	// caller supplies its own bound; neither is ever unbounded.
	Agent time.Duration
	Git   time.Duration

	// Provider is the confinement provider. Empty is **unset**, and unset is
	// not none: a run refuses to dispatch until one is named, and a named one
	// is verified by a canary rather than trusted. See docs/decision/0017.
	Provider string
}

// file is the on-disk shape, so that no type from the YAML decoder appears in
// this package's own signatures.
type file struct {
	Exclude  []string `yaml:"exclude"`
	Timeouts struct {
		Gatherer string `yaml:"gatherer"`
		Agent    string `yaml:"agent"`
		Git      string `yaml:"git"`
	} `yaml:"timeouts"`
	Confine struct {
		Provider string `yaml:"provider"`
	} `yaml:"confine"`
}

// Read reads a workspace's .kept/config.yaml. A workspace with no config reads
// as the zero Config rather than an error: every key here has a defined
// meaning when it is absent, and the one key that must exist for kept to run at
// all — the corpus root — is found by internal/corpus before this is reached.
func Read(kept string) (*Config, error) {
	src, err := os.ReadFile(filepath.Join(kept, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var f file
	if err := yaml.Unmarshal(src, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(kept, "config.yaml"), err)
	}

	cfg := &Config{Exclude: f.Exclude, Provider: f.Confine.Provider}
	for _, deadline := range []struct {
		key     string
		written string
		into    *time.Duration
	}{
		{"timeouts.gatherer", f.Timeouts.Gatherer, &cfg.Gatherer},
		{"timeouts.agent", f.Timeouts.Agent, &cfg.Agent},
		{"timeouts.git", f.Timeouts.Git, &cfg.Git},
	} {
		if deadline.written == "" {
			continue
		}
		d, err := time.ParseDuration(deadline.written)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("%s: %q is not a duration an external call can be bounded by", deadline.key, deadline.written)
		}
		*deadline.into = d
	}
	return cfg, nil
}
