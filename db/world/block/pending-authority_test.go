//go:build !js && !wasip1

package world_block

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// pendingRootName returns the root a pendingAuthority holds a candidate's
// blocks under.
func pendingRootName(seqno uint64) string {
	return "pending:" + strconv.FormatUint(seqno, 10)
}

// pendingAuthority is an Authority that answers later, as a Space's does: its
// Admit stores the candidate's blocks under pending:<seqno> and accepts the
// candidate at once, and the head becomes durable only when the test calls
// durable, which swaps the World root and releases the pending root in one
// transaction. Head and the turn come from the local authority it wraps.
type pendingAuthority struct {
	*LocalAuthority
	// mtx guards admitted.
	mtx sync.Mutex
	// admitted is the candidate waiting to become durable, or nil.
	admitted *pendingAdmission
	// entered is closed and replaced each time a candidate is admitted.
	entered chan struct{}
}

// newPendingAuthority wraps a.
func newPendingAuthority(a *LocalAuthority) *pendingAuthority {
	return &pendingAuthority{LocalAuthority: a, entered: make(chan struct{})}
}

// Begin opens the local turn and makes its Admit answer later.
func (a *pendingAuthority) Begin(ctx context.Context) (world.Turn, error) {
	turn, err := a.LocalAuthority.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &pendingTurn{Turn: turn, a: a}, nil
}

// waitAdmitted waits for a candidate and returns it.
func (a *pendingAuthority) waitAdmitted(t *testing.T) *pendingAdmission {
	// Wait for the next admission.
	t.Helper()
	a.mtx.Lock()
	entered := a.entered
	adm := a.admitted
	a.mtx.Unlock()
	if adm != nil {
		return adm
	}
	select {
	case <-entered:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	// Return it.
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.admitted
}

// durable makes the waiting candidate the durable head: one transaction that
// swaps the World root and releases the pending root.
func (a *pendingAuthority) durable(t *testing.T) {
	// Take the waiting candidate.
	t.Helper()
	adm := a.waitAdmitted(t)
	a.mtx.Lock()
	a.admitted = nil
	a.mtx.Unlock()

	// Swap the World root and release the pending root.
	c := adm.candidate
	err := a.store.SetRoots(t.Context(), nil, []block.NamedRoot{
		{Name: a.name, Ref: c.Root, Previous: c.Base},
		{Name: pendingRootName(c.Seqno), Previous: c.Root},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.observe(world.Head{Root: c.Root, Seqno: c.Seqno, Durable: true})
	adm.resolve(nil)
}

// durableLost makes the waiting candidate the durable head, as durable does,
// but ends its wait with err, as an authority whose report of the record was
// lost does.
func (a *pendingAuthority) durableLost(t *testing.T, err error) {
	// Swap the root as durable does.
	t.Helper()
	adm := a.waitAdmitted(t)
	a.mtx.Lock()
	a.admitted = nil
	a.mtx.Unlock()
	c := adm.candidate
	if serr := a.store.SetRoots(t.Context(), nil, []block.NamedRoot{
		{Name: a.name, Ref: c.Root, Previous: c.Base},
		{Name: pendingRootName(c.Seqno), Previous: c.Root},
	}); serr != nil {
		t.Fatal(serr)
	}

	// Report the wait failed.
	adm.resolve(err)
}

// fail ends the waiting candidate with err, as an authority that stopped
// before the checkpoint ends it, and leaves its pending root held.
func (a *pendingAuthority) fail(t *testing.T, err error) *pendingAdmission {
	t.Helper()
	adm := a.waitAdmitted(t)
	a.mtx.Lock()
	a.admitted = nil
	a.mtx.Unlock()
	adm.resolve(err)
	return adm
}

// pendingTurn is a turn of a pendingAuthority.
type pendingTurn struct {
	world.Turn
	// a is the authority.
	a *pendingAuthority
}

// Admit stores the candidate's blocks under its pending root and accepts it.
func (t *pendingTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	// Hold the candidate's blocks under its pending root.
	err := t.a.store.SetRoots(ctx, candidate.Entries, []block.NamedRoot{{
		Name: pendingRootName(candidate.Seqno),
		Ref:  candidate.Root,
	}})
	if err != nil {
		return nil, err
	}

	// Record the admission and wake the test.
	adm := &pendingAdmission{candidate: candidate, done: make(chan struct{})}
	t.a.mtx.Lock()
	t.a.admitted = adm
	close(t.a.entered)
	t.a.entered = make(chan struct{})
	t.a.mtx.Unlock()
	return adm, nil
}

// pendingAdmission is an accepted candidate waiting to become durable.
type pendingAdmission struct {
	// candidate is the candidate.
	candidate world.Candidate
	// done closes once the candidate is durable or failed.
	done chan struct{}
	// err is why it failed, set before done closes.
	err error
}

// resolve ends the wait with err.
func (a *pendingAdmission) resolve(err error) {
	a.err = err
	close(a.done)
}

// Seqno returns the candidate's seqno.
func (a *pendingAdmission) Seqno() uint64 { return a.candidate.Seqno }

// Head returns the accepted head.
func (a *pendingAdmission) Head() world.Head {
	return world.Head{Root: a.candidate.Root, Seqno: a.candidate.Seqno}
}

// WaitDecided accepts the candidate.
func (a *pendingAdmission) WaitDecided(context.Context) (world.Decision, error) {
	return world.Accepted, nil
}

// WaitDurable waits for the test to make the head durable or fail it.
func (a *pendingAdmission) WaitDurable(ctx context.Context) (uint64, error) {
	select {
	case <-a.done:
		return a.candidate.Seqno, a.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Release does nothing: an accepted candidate stays the authority's.
func (a *pendingAdmission) Release() {}

// _ is a type assertion
var (
	_ world.Authority = (*pendingAuthority)(nil)
	_ world.Admission = (*pendingAdmission)(nil)
)
