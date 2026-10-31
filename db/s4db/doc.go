// Package s4db is a single-file embedded key-value engine.
//
// This is a stub while the engine is rewritten. It declares the surface the
// rest of the module builds against: the kvtx.Store contract with ordered,
// atomic and writer-queue commits, the commit sequence and its watch, named
// leases across processes, and the size reports the Volume and the CLI read.
// Every operation returns ErrUnimplemented.
package s4db
