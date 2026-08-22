// Package runtimestate materializes the private per-run configuration a repair
// runtime reads, and discards it with the box.
//
// It is a fourth boundary beside the three that already exist, and it exists
// because none of them can hold this without becoming something else. A box
// bounds what may land, so the tree it snapshots must not contain a runtime's
// credential. Confinement bounds what may be touched, and a policy over
// resources does not author content. An adapter owns argv, one packet and the
// native evidence a session produced; giving it a copier would make the one
// component that talks to an external program also the component that reads
// the operator's configuration, which is the coupling decision 0052 keeps out
// of the adapter contract by saying an adapter never reads the operator's home.
//
// What it produces is a projection in decision 0017's sense: a rendered subset
// of what the operator configured, small enough to state, and never the whole
// thing. A runtime reads its own configuration from a directory; kept decides
// which entries that directory is allowed to contain.
//
// Nothing here is a plugin protocol. There is one exported materializer, for
// the one runtime whose credential is a static self-hosted endpoint's.
package runtimestate
