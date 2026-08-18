package corpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir holds a repository's kept state, and ConfigFile is where the corpus
// root is declared. Both are world rather than corpus — .kept is privileged
// world, since it decides what is enforced — and they are read here because
// finding the corpus is the corpus package's business and nothing else's. See
// docs/decision/0028.
const (
	ConfigDir  = ".kept"
	ConfigFile = "config.yaml"
)

// ErrNoConfig is returned when no .kept/config.yaml is found in a directory or
// any of its parents.
var ErrNoConfig = errors.New("no " + ConfigDir + "/" + ConfigFile + " here or in any parent directory")

// FindRoot resolves the corpus root for a directory, by walking up to the
// nearest .kept/config.yaml and reading its corpus: key. It also returns the
// directory holding .kept, which is the repository the corpus describes.
//
// A config with no corpus: key resolves to the directory holding .kept, which
// is what a manuscript wants: there the corpus is the content, so the root is
// the repository root and nothing else changes. See docs/layout.md.
func FindRoot(start string) (root, base string, err error) {
	base, err = findBase(start)
	if err != nil {
		return "", "", err
	}

	var cfg struct {
		Corpus string `yaml:"corpus"`
	}
	path := filepath.Join(base, ConfigDir, ConfigFile)
	src, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}
	if err := unmarshalYAML(src, &cfg); err != nil {
		return "", "", fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Corpus == "" {
		return base, base, nil
	}
	return filepath.Join(base, filepath.FromSlash(cfg.Corpus)), base, nil
}

func findBase(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ConfigDir, ConfigFile)); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoConfig
		}
		dir = parent
	}
}
