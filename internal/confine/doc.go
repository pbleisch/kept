// Package confine owns the resource boundary around a repair process.
//
// A repair box is only the tree whose diff may later be considered. This
// package supplies the separate confinement launch which decides what the
// process in that tree can touch. It deliberately owns neither a runtime
// adapter nor packet assembly; those receive only the verified launch
// capability. See docs/decision/0017-the-box-is-not-the-jail.md.
package confine
