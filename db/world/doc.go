// Package world defines the World: a transactional store of typed objects and
// a quad graph between them, sequenced by a change log.
//
// An Engine opens transactions on a World. A read transaction holds one
// immutable revision until Discard. A write transaction's commit seals its
// blocks into a Candidate and proposes it to the engine's Authority, which
// alone moves the head: an accepted candidate becomes the head, and a
// superseded one fails with ErrSuperseded so the writer reruns its change from
// a new transaction (IsSuperseded). ErrDurabilityUnknown after acceptance
// means the outcome is unknown, not failed. The writer reconciles from
// Authority.Head. Sync and SyncThrough fence durability, and a head the
// Authority accepted survives a restart because the Authority holds it.
//
// Each commit advances the World seqno. WorldWaitSeqno, ChangeWatcher and
// WaitObjectRev wake a waiter for the changes it reads, so a consumer watches
// the World rather than polls it.
//
// Package block implements the Engine over a block graph, one engine for
// every World: a local World's Authority is a coordinator lease and a
// compare-and-swap on the World's named root in its Volume, and a Space
// World's is the SharedObject that agrees on its operations.
package world
