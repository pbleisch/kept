// Package repair turns an operator's configuration into the two things a
// confined repair needs: the runtime configuration an adapter is handed, and
// the credential route the confinement profile carries.
//
// It exists because decision 0052 forbids an adapter from reading a keychain,
// an auth database or the parent environment to obtain a credential, and a rule
// like that is worth more as a structure than as a comment. internal/thread has
// no route to configuration at all — it is handed a struct — and this package
// is where the struct is built. Under the brokered channel decision 0057
// chooses, the guarantee is stronger than a boundary: the credential never
// enters this process either, because what kept assembles is a reference the
// confinement provider resolves for itself.
//
// It is deliberately not a secret-provider framework. There are three runtimes
// and each needs one credential; 0043 and 0052 both refuse the general shape.
package repair
