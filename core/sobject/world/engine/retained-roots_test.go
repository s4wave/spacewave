package sobject_world_engine

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestProcessSetRetainedRootOp checks setting, replacing, and releasing
// retained roots keeps them sorted by name, and that a bad name or a full set
// rejects the operation.
func TestProcessSetRetainedRootOp(t *testing.T) {
	// Start from an empty set.
	le := logrus.NewEntry(logrus.New())
	peerID := newProcessTestPeerID(t)
	refA := &block.BlockRef{Hash: testRetainedRootHash(t, "a")}
	refB := &block.BlockRef{Hash: testRetainedRootHash(t, "b")}
	state := &InnerState{}

	// Apply each operation to the state the previous one accepted.
	apply := func(name string, ref *block.BlockRef) bool {
		// Process the operation on the current state.
		t.Helper()
		op := &SetRetainedRootOp{Name: name, RootRef: ref}
		next, res, err := processSetRetainedRootOp(le, op, state, peerID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if next != nil {
			state = next
		}
		return res.GetSuccess()
	}
	names := func() string {
		var out []string
		for _, root := range state.GetRetainedRoots() {
			label := "b"
			if root.GetRootRef().EqualVT(refA) {
				label = "a"
			}
			out = append(out, root.GetName()+"="+label)
		}
		return strings.Join(out, ",")
	}

	// Set, replace, and release.
	steps := []struct {
		name string
		ref  *block.BlockRef
		want string
	}{
		{"b", refB, "b=b"},
		{"a", refB, "a=b,b=b"},
		{"a", refA, "a=a,b=b"},
		{"b", nil, "a=a"},
		{"missing", nil, "a=a"},
	}
	for _, step := range steps {
		if !apply(step.name, step.ref) {
			t.Fatalf("set %q rejected", step.name)
		}
		if got := names(); got != step.want {
			t.Fatalf("after set %q: roots %q; want %q", step.name, got, step.want)
		}
	}

	// Reject invalid names.
	if apply("", refA) || apply(strings.Repeat("x", maxRetainedRootNameLen+1), refA) {
		t.Fatal("invalid name accepted")
	}

	// Fill the set, then reject one more name but accept a replacement.
	for i := len(state.GetRetainedRoots()); i < maxRetainedRoots; i++ {
		if !apply("root-"+strconv.Itoa(i), refA) {
			t.Fatalf("root %d rejected", i)
		}
	}
	if apply("extra", refA) {
		t.Fatal("root beyond the limit accepted")
	}
	if !apply("a", refB) {
		t.Fatal("replacement in a full set rejected")
	}
}

// TestRetainRootsCopiesFromStorage checks retainRoots copies a root's graph
// from storage into the local store, then holds it from the local store alone,
// and reports a root missing from both as block.ErrNotFound.
func TestRetainRootsCopiesFromStorage(t *testing.T) {
	// Build the storage side.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	remote, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(remote.Release)
	source, err := remote.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(source.Release)

	// Build the local side.
	local, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(local.Release)
	target, err := local.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(target.Release)

	// Write a root and its leaf only to the storage side.
	leaf, _, err := block.PutBlock(ctx, source.GetBucket(), &block_mock.Example{Msg: "retained payload"})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := block.PutBlock(ctx, source.GetBucket(), &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: leaf}})
	if err != nil {
		t.Fatal(err)
	}

	// Retain it through a store that reads storage on a local miss.
	overlay := block.NewOverlay(ctx, le, source.GetBucket(), target.GetBucket(), block.OverlayMode_UPPER_WRITE_CACHE, 0, nil)
	so := &retainedRootsTestSharedObject{
		testSharedObject: testSharedObject{blockStore: newTestBlockStore("retained-roots-test", overlay)},
		proofs:           newTestRejectedCandidateStore(),
	}
	c := &Controller{le: le}
	roots := []*RetainedRoot{{Name: "backup", RootRef: root}}
	if err := c.retainRoots(ctx, so, roots); err != nil {
		t.Fatal(err)
	}
	if _, found, err := target.GetBucket().GetBlock(ctx, leaf); err != nil || !found {
		t.Fatalf("leaf not copied locally: found=%v, err=%v", found, err)
	}

	// Without storage, the local copy still satisfies the root.
	so.blockStore = newTestBlockStore("retained-roots-test", target.GetBucket())
	if err := c.retainRoots(ctx, so, roots); err != nil {
		t.Fatalf("retain from the local store: %v", err)
	}

	// A root in neither store is lost.
	lost := []*RetainedRoot{{Name: "lost", RootRef: &block.BlockRef{Hash: testRetainedRootHash(t, "lost")}}}
	if err := c.retainRoots(ctx, so, lost); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("retain lost root = %v; want block.ErrNotFound", err)
	}
}

// retainedRootsTestSharedObject serves one local proof store.
type retainedRootsTestSharedObject struct {
	testSharedObject
	proofs kvtx.Store
}

func (s *retainedRootsTestSharedObject) AccessLocalStateStore(context.Context, string, func()) (kvtx.Store, func(), error) {
	return s.proofs, func() {}, nil
}

// testRetainedRootHash returns a distinct hash for a test block ref.
func testRetainedRootHash(t *testing.T, seed string) *hash.Hash {
	t.Helper()
	h, err := hash.Sum(hash.RecommendedHashType, []byte(seed))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestSetRetainedRootRequiresAcceptedHead checks a root other than the
// accepted head is refused without queueing.
func TestSetRetainedRootRequiresAcceptedHead(t *testing.T) {
	// Retain a root on an engine whose accepted head is empty.
	c := &Controller{le: logrus.NewEntry(logrus.New()), conf: &Config{}}
	so := &testMaintenanceSharedObject{snapshot: &testMaintenanceSnapshot{}}
	e := newSoEngine(c, so, nil)
	ref := &block.BlockRef{Hash: testRetainedRootHash(t, "old")}
	if err := e.SetRetainedRoot(t.Context(), "backup", ref); !errors.Is(err, errRetainedRootNotHead) {
		t.Fatalf("retain non-head root = %v; want errRetainedRootNotHead", err)
	}

	// Check nothing was queued.
	if len(so.queueOps) != 0 {
		t.Fatalf("queued %d operations", len(so.queueOps))
	}
}

// TestCommitMaintenanceOpWaitsForWriteTransaction verifies that a maintenance
// operation cannot advance the SharedObject root while a write transaction
// holds its base.
func TestCommitMaintenanceOpWaitsForWriteTransaction(t *testing.T) {
	// Build a controller.
	c := &Controller{
		le:   logrus.NewEntry(logrus.New()),
		conf: &Config{},
	}
	so := &testMaintenanceSharedObject{snapshot: &testMaintenanceSnapshot{}}

	// Hold the write lock as an open write transaction does.
	unlockWriteMtx, err := c.writeMtx.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer unlockWriteMtx()

	// A maintenance operation gives up without queueing.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	build := func(state *InnerState) (*SOWorldOp, error) {
		return &SOWorldOp{Body: &SOWorldOp_SetRetainedRoot{
			SetRetainedRoot: &SetRetainedRootOp{Name: "backup"},
		}}, nil
	}
	if _, err := c.commitMaintenanceOp(ctx, so, build); !errors.Is(err, context.Canceled) {
		t.Fatalf("maintenance during write transaction: got %v, want context.Canceled", err)
	}
	if len(so.queueOps) != 0 {
		t.Fatal("maintenance op queued while a write transaction held its base")
	}
}

type testMaintenanceSharedObject struct {
	snapshot   sobject.SharedObjectStateSnapshot
	blockStore bstore.BlockStore
	queueOps   [][]byte
}

func (s *testMaintenanceSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testMaintenanceSharedObject) GetPeerID() peer.ID {
	return ""
}

func (s *testMaintenanceSharedObject) GetSharedObjectID() string {
	return ""
}

func (s *testMaintenanceSharedObject) GetBlockStore() bstore.BlockStore {
	return s.blockStore
}

func (s *testMaintenanceSharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	return nil, nil, nil
}

func (s *testMaintenanceSharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snapshot, nil
}

func (s *testMaintenanceSharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, nil
}

func (s *testMaintenanceSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	s.queueOps = append(s.queueOps, bytes.Clone(op))
	return "maintenance-op", nil
}

func (s *testMaintenanceSharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	return 0, false, nil
}

func (s *testMaintenanceSharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	return nil
}

func (s *testMaintenanceSharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	return nil
}

type testMaintenanceSnapshot struct{}

func (s *testMaintenanceSnapshot) GetParticipantConfig(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	return &sobject.SOParticipantConfig{}, nil
}

func (s *testMaintenanceSnapshot) GetParticipantConfigForPeer(ctx context.Context, _ string) (*sobject.SOParticipantConfig, error) {
	return s.GetParticipantConfig(ctx)
}

func (s *testMaintenanceSnapshot) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) GetTransformInfo(ctx context.Context) (*sobject.TransformInfo, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) GetOpQueue(ctx context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, nil, nil
}

func (s *testMaintenanceSnapshot) GetRootInner(ctx context.Context) (*sobject.SORootInner, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) GetRootState(ctx context.Context) (*sobject.SORoot, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) ProcessOperations(
	ctx context.Context,
	ops []*sobject.SOOperation,
	cb sobject.SnapshotProcessOpsFunc,
) (
	nextRoot *sobject.SORoot,
	rejectedOps []*sobject.SOOperationRejection,
	acceptedOps []*sobject.SOOperation,
	err error,
) {
	return nil, nil, nil, nil
}
