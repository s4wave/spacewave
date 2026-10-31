package world

import (
	"context"
)

// AdmitCommitter is a write Tx whose commit can return once the engine admits
// the write, before it is durable.
type AdmitCommitter interface {
	// CommitAdmit commits the write and returns once the engine admitted it
	// and installed the World after it, which later writes fork from. Readers
	// see that World once it is durable. On error the write was not admitted,
	// or it failed before CommitAdmit returned. A write whose base the engine
	// drops for a failed publication during its commit fails with that error
	// and is never replayed onto the reinstalled World. Call Release on the
	// Admission.
	CommitAdmit(ctx context.Context) (Admission, error)
}

// Admission is an admitted write whose durable result may be pending.
type Admission interface {
	// Seqno returns the World seqno after the write.
	Seqno() uint64
	// WaitDurable waits until the write is durable and readers see it, and
	// returns Seqno. When the write fails, the engine installs the World
	// without it and the writes admitted after it, and WaitDurable returns
	// the seqno of that World with the error. The wait ends with the engine's
	// lifetime, not with ctx: the outcome is then unknown.
	WaitDurable(ctx context.Context) (uint64, error)
	// Release releases the write. Call it once, after WaitDurable returns:
	// it drops the write's hold on the World after it, which the durable head
	// may not hold yet before then, so a sweep could free that World's blocks.
	Release()
}

// ReinstallNotifier is an engine that reports the World it installs after
// dropping failed writes.
type ReinstallNotifier interface {
	// OnReinstall calls fn with the World seqno each time the engine drops
	// failed writes and installs the World without them, and when a new
	// engine opens in their place, before a later write is admitted. The
	// returned func unregisters fn.
	//
	// fn runs under the engine's writer lock, in settle order, and must not
	// open a World transaction. It runs on the receipt's watcher, on a
	// writer's goroutine as it forks before the write's body runs, on a
	// goroutine entering another engine call that changes its state, or on
	// the one opening a new engine; never during a commit. The lock order is
	// the engine's writer lock first, the consumer's second: fn may block on
	// a consumer lock that is taken only inside a write's body, after the
	// fork, and released after CommitAdmit returns. WaitDurable takes no
	// engine lock and never waits for fn: it returns a failed receipt's error
	// without waiting for the reinstall.
	OnReinstall(ctx context.Context, fn func(seqno uint64)) (func(), error)
}

// OnReinstall registers fn with eng's reinstall hooks. An engine that does
// not report reinstalls admits no write before it is durable and never drops
// one, so fn is not registered and the returned func does nothing.
func OnReinstall(ctx context.Context, eng Engine, fn func(seqno uint64)) (func(), error) {
	if rn, ok := eng.(ReinstallNotifier); ok {
		return rn.OnReinstall(ctx, fn)
	}
	return func() {}, nil
}

// CommitAdmit commits tx and returns its Admission. A Tx that cannot commit
// at admission commits in full and returns an Admission that is durable. Its
// seqno is tx's GetSeqno read after the writes and before Commit, which a
// world_block Tx advances with each change. That is the seqno after the
// write when the engine serializes its write transactions, as a world_block
// Engine does; a Tx whose engine applies concurrent writes at commit cannot
// report it, and an AdmitCommitter must be used there.
func CommitAdmit(ctx context.Context, tx Tx) (Admission, error) {
	// Let a Tx that admits commit itself.
	if ac, ok := tx.(AdmitCommitter); ok {
		return ac.CommitAdmit(ctx)
	}

	// Otherwise read the seqno and commit in full.
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		tx.Discard()
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return DurableAdmission(seqno), nil
}

// DurableAdmission is the Admission of a write that is already durable at
// seqno.
type DurableAdmission uint64

// Seqno returns the World seqno after the write.
func (a DurableAdmission) Seqno() uint64 { return uint64(a) }

// WaitDurable returns Seqno.
func (a DurableAdmission) WaitDurable(context.Context) (uint64, error) { return uint64(a), nil }

// Release does nothing.
func (a DurableAdmission) Release() {}

// releaseAdmission calls release after the Admission's Release.
type releaseAdmission struct {
	Admission
	release func()
}

// Release releases the Admission, then calls release.
func (a *releaseAdmission) Release() {
	a.Admission.Release()
	a.release()
}

// _ is a type assertion
var _ Admission = DurableAdmission(0)
