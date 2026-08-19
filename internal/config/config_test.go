package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The three keys a tick honours are read out of the file, because a knob that
// silently does nothing is worse than no knob.
func TestTheKeysATickHonoursAreRead(t *testing.T) {
	cfg, err := Read(write(t, `corpus: docs
exclude:
  - vendor/
  - "*.snapshot"
timeouts:
  gatherer: 90s
  agent: 60m
confine:
  provider: nono
retention:
  observations: 200
`))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cfg.Gatherer != 90*time.Second {
		t.Errorf("gatherer deadline = %s, want 90s", cfg.Gatherer)
	}
	if cfg.Provider != "nono" {
		t.Errorf("provider = %q", cfg.Provider)
	}
	if len(cfg.Exclude) != 2 || cfg.Exclude[0] != "vendor/" {
		t.Errorf("exclude = %q", cfg.Exclude)
	}
}

// An unset provider is not none, and it does not read as one. A run refuses to
// dispatch until one is named, so the difference between "the operator chose
// no confinement" and "the operator has not chosen" has to survive the read.
// See docs/decision/0017.
func TestAnUnsetProviderIsNotNone(t *testing.T) {
	for _, body := range []string{"corpus: docs\n", "confine:\n  provider:\n"} {
		cfg, err := Read(write(t, body))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if cfg.Provider != "" {
			t.Errorf("%q read the provider as %q, and nothing named one", body, cfg.Provider)
		}
	}

	cfg, err := Read(write(t, "confine:\n  provider: none\n"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cfg.Provider != "none" {
		t.Errorf("an explicit none read as %q", cfg.Provider)
	}
}

// A deadline that will not parse is refused rather than defaulted. Silently
// falling back would bound a gather by something other than what the operator
// wrote, which is the failure the deadline exists to prevent.
func TestADeadlineThatWillNotParseIsRefused(t *testing.T) {
	for _, bad := range []string{"soon", "0s", "-5m", "10 minutes"} {
		if _, err := Read(write(t, "timeouts:\n  gatherer: "+bad+"\n")); err == nil {
			t.Errorf("%q was accepted as a deadline", bad)
		}
	}
}

// A workspace with no config reads as the zero value. Every key here has a
// defined meaning when it is absent, and the one that must exist for kept to
// run at all is found before this is reached.
func TestNoConfigIsTheZeroValue(t *testing.T) {
	cfg, err := Read(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cfg.Gatherer != 0 || cfg.Provider != "" || len(cfg.Exclude) != 0 {
		t.Errorf("an absent config read as %+v", cfg)
	}
}
