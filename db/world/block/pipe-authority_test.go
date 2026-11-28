//go:build !js && !wasip1

package world_block

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// pipeAuthority is an in-memory Authority that pipelines, as a Space's does: a
// turn's base is the last admitted candidate, Admit holds the candidate's
// blocks under its pending root and returns undecided, and the test decides
// each candidate in order, optionally with a rebased head. A decision resolves
// the admission before it reports the head to every watcher, as a Space's
// does: the Authority decides without waiting for its watch.
type pipeAuthority struct {
	// store holds the candidates' blocks.
	store block.StoreOps
	// turnMtx serializes turns.
	turnMtx csync.Mutex

	// mtx guards the fields below.
	mtx sync.Mutex
	// head is the decided head.
	head world.Head
	// writer is the head the next turn builds on.
	writer world.Head
	// admitted are the candidates in admission order.
	admitted []*pipeAdmission
	// watchers receive each decided head.
	watchers []func(world.Head)
	// entered is closed and replaced at each admission.
	entered chan struct{}
}

// newPipeAuthority returns an authority at head over store.
func newPipeAuthority(store block.StoreOps, head world.Head) *pipeAuthority {
	head.Durable = true
	return &pipeAuthority{store: store, head: head, writer: head, entered: make(chan struct{})}
}

// Head returns the decided head.
func (a *pipeAuthority) Head(context.Context) (world.Head, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.head, nil
}

// WatchHead reports the decided head, then each later one.
func (a *pipeAuthority) WatchHead(_ context.Context, fn func(world.Head)) (func(), error) {
	a.mtx.Lock()
	a.watchers = append(a.watchers, fn)
	head := a.head
	a.mtx.Unlock()
	fn(head)
	return func() {}, nil
}

// Begin waits for the turn, whose base is the writer head.
func (a *pipeAuthority) Begin(ctx context.Context) (world.Turn, error) {
	release, err := a.turnMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	a.mtx.Lock()
	base := a.writer
	var after *pipeAdmission
	if !base.Durable && len(a.admitted) != 0 {
		after = a.admitted[len(a.admitted)-1]
	}
	a.mtx.Unlock()
	return &pipeTurn{a: a, base: base, after: after, release: release}, nil
}

// waitAdmitted waits until n candidates were admitted and returns the nth
// candidate.
func (a *pipeAuthority) waitAdmitted(t *testing.T, n int) *pipeAdmission {
	t.Helper()
	for {
		a.mtx.Lock()
		if len(a.admitted) >= n {
			adm := a.admitted[n-1]
			a.mtx.Unlock()
			return adm
		}
		entered := a.entered
		a.mtx.Unlock()
		select {
		case <-entered:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
}

// decide accepts the nth candidate with the head at its root, or at rebased
// when set, resolves the admission, then reports the head to every watcher.
func (a *pipeAuthority) decide(t *testing.T, n int, rebased *block.BlockRef, seqno uint64) {
	t.Helper()
	adm := a.waitAdmitted(t, n)
	head := world.Head{Root: adm.candidate.Root, Seqno: adm.candidate.Seqno, Durable: true}
	if rebased != nil {
		head = world.Head{Root: rebased, Seqno: seqno, Durable: true}
	}
	a.mtx.Lock()
	if adm.candidate.ConfirmDurable != nil {
		adm.candidate.ConfirmDurable(head)
	}
	a.head = head
	if adm == a.admitted[len(a.admitted)-1] {
		a.writer = head
	}
	watchers := a.watchers
	a.mtx.Unlock()
	adm.resolve(head, world.Accepted)
	for _, fn := range watchers {
		fn(head)
	}
}

// refuse supersedes the nth candidate without moving the decided head.
func (a *pipeAuthority) refuse(t *testing.T, n int) {
	// Reset the writer head if it was the refused candidate.
	t.Helper()
	adm := a.waitAdmitted(t, n)
	a.mtx.Lock()
	head := a.head
	if adm == a.admitted[len(a.admitted)-1] {
		a.writer = head
	}
	a.mtx.Unlock()

	// Release the refused candidate's pending root, so a later candidate at
	// its seqno can take the name, then return the unchanged head to the
	// superseded writer.
	err := a.store.SetRoots(t.Context(), nil, []block.NamedRoot{
		{Name: pendingRootName(adm.candidate.Seqno), Previous: adm.candidate.Root},
	})
	if err != nil {
		t.Fatal(err)
	}
	adm.resolve(head, world.Superseded)
}

// report makes root the decided head, as another writer's operation does, and
// reports it to every watcher.
func (a *pipeAuthority) report(root *block.BlockRef, seqno uint64) {
	head := world.Head{Root: root, Seqno: seqno, Durable: true}
	a.mtx.Lock()
	a.head = head
	if len(a.admitted) == 0 || a.admitted[len(a.admitted)-1].Head().Root != nil {
		a.writer = head
	}
	watchers := a.watchers
	a.mtx.Unlock()
	for _, fn := range watchers {
		fn(head)
	}
}

// pipeTurn is a turn of a pipeAuthority.
type pipeTurn struct {
	// a is the authority.
	a *pipeAuthority
	// base is the writer head when the turn began.
	base world.Head
	// after is the undecided admission whose candidate base is, or nil.
	after *pipeAdmission
	// release ends the turn.
	release func()
}

// Base returns the writer head when the turn began, durable only when no
// candidate waited.
func (t *pipeTurn) Base() world.Head {
	return t.base
}

// Admit holds the candidate's blocks under its pending root and makes it the
// writer head.
func (t *pipeTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	err := t.a.store.SetRoots(ctx, candidate.Entries, []block.NamedRoot{{Name: pendingRootName(candidate.Seqno), Ref: candidate.Root}})
	if err != nil {
		return nil, err
	}
	opHash := []byte("pipe-op-" + strconv.FormatUint(candidate.Seqno, 10))
	adm := &pipeAdmission{candidate: candidate, decided: make(chan struct{}), opHash: opHash}
	t.a.mtx.Lock()
	t.a.admitted = append(t.a.admitted, adm)
	t.a.writer = world.Head{Root: candidate.Root, Seqno: candidate.Seqno}
	close(t.a.entered)
	t.a.entered = make(chan struct{})
	t.a.mtx.Unlock()
	return adm, nil
}

// Release ends the turn.
func (t *pipeTurn) Release() {
	t.release()
}

// WaitBase waits until the test decides the admission base is the candidate of.
func (t *pipeTurn) WaitBase(ctx context.Context) error {
	if t.after == nil {
		return nil
	}
	select {
	case <-t.after.decided:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// pipeAdmission is a candidate the test decides.
type pipeAdmission struct {
	// candidate is the candidate.
	candidate world.Candidate
	// decided closes once the test decided it.
	decided chan struct{}
	// head is the decided head, set before decided closes.
	head world.Head
	// decision is the Authority's answer, set before decided closes.
	decision world.Decision
	// releases counts attempts to abandon this admission.
	releases atomic.Int32
	// opHash names the candidate's operation, as a signed one would.
	opHash []byte
}

// resolve records the decision and wakes the writer.
func (a *pipeAdmission) resolve(head world.Head, decision world.Decision) {
	a.head = head
	a.decision = decision
	close(a.decided)
}

// Seqno returns the candidate's seqno.
func (a *pipeAdmission) Seqno() uint64 { return a.candidate.Seqno }

// Head returns the decided head, or the zero Head before the decision.
func (a *pipeAdmission) Head() world.Head {
	select {
	case <-a.decided:
		return a.head
	default:
		return world.Head{}
	}
}

// WaitDecided waits for the test's decision.
func (a *pipeAdmission) WaitDecided(ctx context.Context) (world.Decision, error) {
	select {
	case <-a.decided:
		return a.decision, nil
	case <-ctx.Done():
		return world.Undecided, ctx.Err()
	}
}

// WaitDurable waits for the decision: a decided candidate is durable.
func (a *pipeAdmission) WaitDurable(ctx context.Context) (uint64, error) {
	// Return the decided head's sequence only for an accepted candidate.
	decision, err := a.WaitDecided(ctx)
	if err != nil {
		return 0, err
	}
	if decision == world.Superseded {
		return 0, world.ErrSuperseded
	}
	return a.head.Seqno, nil
}

// Release records that the caller released the admission.
func (a *pipeAdmission) Release() { a.releases.Add(1) }

// OpHash returns the name the pipe gave the candidate's operation.
func (a *pipeAdmission) OpHash() []byte { return a.opHash }

// _ is a type assertion
var (
	_ world.Authority = (*pipeAuthority)(nil)
	_ world.Turn      = (*pipeTurn)(nil)
	_ world.Admission = (*pipeAdmission)(nil)
)
