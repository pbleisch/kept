package thread

import (
	"os"
	"path/filepath"
	"time"

	"github.com/pbleisch/kept/internal/ledger"
)

// ExecutableMoment is when the parent looked at a runtime's executable.
type ExecutableMoment string

const (
	// AtLaunch is the executable an invocation names, looked at immediately
	// before it is handed to confinement.
	AtLaunch ExecutableMoment = "launch"
	// AtExit is the same path looked at again once the process is reaped.
	AtExit ExecutableMoment = "exit"
)

func (m ExecutableMoment) known() bool { return m == AtLaunch || m == AtExit }

// Executable is a runtime's executable as the parent observed it: the path the
// invocation named, what that path resolved to, and the file's identity.
//
// It is the parent's fact rather than the runtime's account of itself, and it
// is recorded twice — at launch and after the reap — so that *the binary was
// replaced during the session* is a comparison of two records rather than an
// inference from timestamps nobody wrote down. A package manager replaces a
// runtime by renaming its directory away and putting a new one in its place, so
// the identity that moves is the resolved file's: its inode, size and
// modification time. See docs/decision/0085.
type Executable struct {
	Moment ExecutableMoment `json:"moment"`
	// Path is what the invocation named, and Resolved is that path with every
	// link followed. Resolved is empty when nothing was there to resolve.
	Path     string `json:"path"`
	Resolved string `json:"resolved,omitempty"`
	// Version is what the runtime reported when it was admitted, and only a
	// launch record carries one: the version kept checked is the version the
	// adapter stamped every native event with. An exit record carries none,
	// because nothing asked — a changed identity is what says to ask.
	Version string `json:"version,omitempty"`
	// Inode, Size and Modified are the resolved file's identity.
	Inode    uint64    `json:"inode,omitempty"`
	Size     int64     `json:"size,omitempty"`
	Modified time.Time `json:"modified,omitzero"`
	// Missing says the path resolved to nothing when it was looked at, which is
	// what a runtime mid-reinstall looks like.
	Missing bool `json:"missing,omitempty"`
}

// IdentifyExecutable looks at an executable. It never runs it.
func IdentifyExecutable(path string, moment ExecutableMoment, version string) Executable {
	observed := Executable{Moment: moment, Path: path, Version: version}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		observed.Missing = true
		return observed
	}
	info, err := os.Stat(resolved)
	if err != nil {
		observed.Missing = true
		return observed
	}
	observed.Resolved = resolved
	observed.Inode = inodeOf(info)
	observed.Size = info.Size()
	observed.Modified = info.ModTime().UTC()
	return observed
}

// SameFile reports whether two observations are of one file: the same resolved
// path, and the same identity at it.
func (e Executable) SameFile(other Executable) bool {
	return e.Missing == other.Missing &&
		e.Resolved == other.Resolved &&
		e.Inode == other.Inode &&
		e.Size == other.Size &&
		e.Modified.Equal(other.Modified)
}

func executableWhy(observed Executable) string {
	switch {
	case !observed.Moment.known():
		return "executable record names no moment it was observed at"
	case observed.Path == "":
		return "executable record names no path"
	case observed.Moment == AtExit && observed.Version != "":
		return "executable record at exit claims a version nothing asked for"
	case observed.Missing && (observed.Resolved != "" || observed.Inode != 0 || observed.Size != 0 || !observed.Modified.IsZero()):
		return "executable record is missing and names an identity anyway"
	case !observed.Missing && observed.Resolved == "":
		return "executable record resolves to nothing and does not say it is missing"
	}
	return ""
}

// Executables is the launch and exit observations of one session, and whether
// each was recorded. A session killed before the parent could look again has a
// launch and no exit, which is not a binary that stayed the same.
func Executables(entries []Entry) (launch, exit Executable, launched, exited bool) {
	for _, entry := range entries {
		if entry.Kind != ledger.ThreadExecutable || entry.Executable == nil {
			continue
		}
		switch entry.Executable.Moment {
		case AtLaunch:
			launch, launched = *entry.Executable, true
		case AtExit:
			exit, exited = *entry.Executable, true
		}
	}
	return launch, exit, launched, exited
}
