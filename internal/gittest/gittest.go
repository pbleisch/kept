// Package gittest keeps the git a test runs to the configuration the test
// wrote.
//
// It is imported by tests and by nothing else. The production code is the
// opposite case: configuration an operator hands git through the environment
// is the operator's, and `kept doctor` reporting a core.hooksPath set that way
// is the code working. A test asserting what kept does in a repository it
// built is asking a different question, and a parent process's configuration
// answers it wrongly.
package gittest

import (
	"os"
	"strings"
)

// Isolate unsets every variable through which a parent process hands git
// configuration on the side, so that git in this process and in everything it
// starts reads only the repository's own files and the machine's.
//
// Two mechanisms, both of which outrank a repository's own config:
// GIT_CONFIG_COUNT with its GIT_CONFIG_KEY_<n> and GIT_CONFIG_VALUE_<n> pairs,
// and GIT_CONFIG_PARAMETERS, which `git -c` exports to whatever git runs.
// Measured on 2026-11-20: a supervisor that sets GIT_CONFIG_COUNT=1 and
// core.hooksPath for every task it starts made fifteen internal/ledger tests
// and six cmd/kept tests fail identically on every tree, because kept asked git
// where hooks live and git answered with the supervisor's directory.
//
// It is meant for TestMain, before m.Run, because it changes the process's
// environment and a parallel test reading it partway would see half of it.
func Isolate() {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if Injected(name) {
			_ = os.Unsetenv(name)
		}
	}
}

// Injected reports whether an environment variable hands git configuration
// outside every config file.
func Injected(name string) bool {
	switch {
	case name == "GIT_CONFIG_COUNT", name == "GIT_CONFIG_PARAMETERS":
		return true
	case strings.HasPrefix(name, "GIT_CONFIG_KEY_"), strings.HasPrefix(name, "GIT_CONFIG_VALUE_"):
		return true
	}
	return false
}
