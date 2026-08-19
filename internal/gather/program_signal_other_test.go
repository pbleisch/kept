//go:build !unix

package gather

import "os"

// Nothing here can kill itself with a signal, so the Signalled path is
// exercised only where signals exist.
func signalSelf() { os.Exit(9) }
