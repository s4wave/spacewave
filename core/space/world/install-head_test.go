package space_world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// flakyRootStore holds named roots in memory and fails the next fail root
// moves.
type flakyRootStore struct {
	block.StoreOps
	// roots are the held roots by name.
	roots map[string]*block.BlockRef
	// fail counts the root moves left to fail.
	fail int
}

// GetRoot returns the block name holds.
func (s *flakyRootStore) GetRoot(_ context.Context, name string) (*block.BlockRef, error) {
	return s.roots[name], nil
}

// SetRoots stores entries and moves the roots, unless a failure is due.
func (s *flakyRootStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if s.fail > 0 {
		s.fail--
		return errors.New("injected root move failure")
	}
	if _, err := s.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	for _, root := range roots {
		if root.Ref.GetEmpty() {
			delete(s.roots, root.Name)
			continue
		}
		s.roots[root.Name] = root.Ref
	}
	return nil
}

// TestInstallHeadRetriesRetainedHold checks that a head whose retained-root
// hold failed is held and reported to the watchers by the next install, and
// that its placed candidate is decided only then.
func TestInstallHeadRetriesRetainedHold(t *testing.T) {
	// Build an adapter whose replay head retains a root, over a store that
	// fails the first root move.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef), fail: 1}
	retained, err := block.BuildBlockRef([]byte("retained World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err := block.BuildBlockRef([]byte("head World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &Adapter{
		le:    logrus.NewEntry(logrus.New()),
		pages: block.NewPendingStore(store, 1<<20),
		held:  make(map[string]*block.BlockRef),
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{
		HeadRef:       &bucket.ObjectRef{RootRef: head},
		RetainedRoots: []*RetainedRoot{{Name: "export", RootRef: retained}},
	}}
	adm := &spaceAdmission{state: admissionPlaced, done: make(chan struct{})}
	a.pending = []*spaceAdmission{adm}
	var reports []world.Head
	a.watchers = []*headWatch{{fn: func(h world.Head) { reports = append(reports, h) }}}

	// The first install fails its hold, reports nothing and decides nothing.
	if err := a.installHead(ctx); err == nil {
		t.Fatal("the first install held the retained roots through a failing store")
	}
	if len(reports) != 0 || adm.state == admissionDecided {
		t.Fatalf("a failed install reported %d heads and decided %v", len(reports), adm.state == admissionDecided)
	}

	// The next install holds them, reports the head once, then decides.
	if err := a.installHead(ctx); err != nil {
		t.Fatal(err)
	}
	if store.roots[RootRetained].GetEmpty() {
		t.Fatal("the retry did not hold the retained roots")
	}
	if len(reports) != 1 || !reports[0].Root.EqualsRef(head) || adm.state != admissionDecided {
		t.Fatalf("the retry reported %v and decided %v", reports, adm.state == admissionDecided)
	}
}

// TestResetPayloadsSeesAdmissions checks that a payload reset waiting on the
// root lock holds the payloads of an admission that added them while it
// waited, so a queued operation never loses its payload hold.
func TestResetPayloadsSeesAdmissions(t *testing.T) {
	// Build an adapter with no replayed positions.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		le:    logrus.NewEntry(logrus.New()),
		pages: block.NewPendingStore(store, 1<<20),
		held:  make(map[string]*block.BlockRef),
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{}}
	payload, err := block.BuildBlockRef([]byte("payload"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start a reset while an admission holds the root lock.
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.resetPayloads(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// The admission is pending and holds its payloads, as Admit leaves it.
	adm := &spaceAdmission{state: admissionQueued, payloads: []*block.BlockRef{payload}, done: make(chan struct{})}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.pending = append(a.pending, adm)
	})
	a.payloads = append(a.payloads, payload)
	release()

	// The reset holds them.
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(a.payloads) != 1 || !a.payloads[0].EqualsRef(payload) {
		t.Fatalf("payloads after the reset = %v, want the admission's", a.payloads)
	}
}

// TestInstallKeepsLaterCheckpointWorlds checks that an install of an earlier
// checkpoint keeps holding a later World Root returned while a kept position
// still is that World: Root stored its pages and dropped them from memory, so
// releasing it would leave the position naming unheld pages.
func TestInstallKeepsLaterCheckpointWorlds(t *testing.T) {
	// Build an adapter whose checkpoint root holds an earlier and a later
	// World, with a position at the later one.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef)}
	a := &Adapter{
		le:               logrus.NewEntry(logrus.New()),
		pages:            block.NewPendingStore(store, 1<<20),
		held:             make(map[string]*block.BlockRef),
		checkpointWorlds: worldHistory{name: RootCheckpoint},
		cursorWorlds:     worldHistory{name: RootReplayCursor},
		pendingWorlds:    worldHistory{name: RootPending},
	}
	earlier, err := block.BuildBlockRef([]byte("earlier World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	later, err := block.BuildBlockRef([]byte("later World"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: earlier}}}
	a.replay.positions = []spacePosition{{state: &InnerState{HeadRef: &bucket.ObjectRef{RootRef: later}}}}
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addHistory(ctx, &a.checkpointWorlds, earlier); err != nil {
		t.Fatal(err)
	}
	if err := a.addHistory(ctx, &a.checkpointWorlds, later); err != nil {
		t.Fatal(err)
	}
	release()

	// Install the earlier one as the replay base.
	if err := a.holdBase(ctx, earlier); err != nil {
		t.Fatal(err)
	}
	if err := a.trimHistories(ctx); err != nil {
		t.Fatal(err)
	}

	// The checkpoint root still holds the later World, and only it.
	if store.roots[RootCheckpoint].GetEmpty() || !a.checkpointWorlds.holds(later) || a.checkpointWorlds.holds(earlier) {
		t.Fatalf("checkpoint history after the install = %v", a.checkpointWorlds.roots)
	}
	if !store.roots[RootReplayBase].EqualsRef(earlier) {
		t.Fatal("the install did not hold the replay base")
	}
}
