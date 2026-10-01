package sobject_world_engine

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestProcessSetRetainedRootOp checks setting, replacing, and releasing
// retained roots keeps them sorted by name, and that a stale generation, a bad
// name, or a full set rejects the operation.
func TestProcessSetRetainedRootOp(t *testing.T) {
	// Start from an empty set on generation 2.
	le := logrus.NewEntry(logrus.New())
	peerID := newProcessTestPeerID(t)
	refA := &block.BlockRef{Hash: testRetainedRootHash(t, "a")}
	refB := &block.BlockRef{Hash: testRetainedRootHash(t, "b")}
	state := &InnerState{StorageGeneration: 2}

	// Apply each operation to the state the previous one accepted.
	apply := func(name string, ref *block.BlockRef, generation uint64) bool {
		// Process the operation on the current state.
		t.Helper()
		op := &SetRetainedRootOp{Name: name, RootRef: ref, StorageGeneration: generation}
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
		if !apply(step.name, step.ref, 2) {
			t.Fatalf("set %q rejected", step.name)
		}
		if got := names(); got != step.want {
			t.Fatalf("after set %q: roots %q; want %q", step.name, got, step.want)
		}
	}

	// Reject a stale generation and invalid names.
	if apply("c", refA, 1) {
		t.Fatal("stale generation accepted")
	}
	if apply("", refA, 2) || apply(strings.Repeat("x", maxRetainedRootNameLen+1), refA, 2) {
		t.Fatal("invalid name accepted")
	}

	// Fill the set, then reject one more name but accept a replacement.
	for i := len(state.GetRetainedRoots()); i < maxRetainedRoots; i++ {
		if !apply("root-"+strconv.Itoa(i), refA, 2) {
			t.Fatalf("root %d rejected", i)
		}
	}
	if apply("extra", refA, 2) {
		t.Fatal("root beyond the limit accepted")
	}
	if !apply("a", refB, 2) {
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
// accepted head is refused without queueing, because its generation would not
// prove a reclaim pass left its blocks in storage.
func TestSetRetainedRootRequiresAcceptedHead(t *testing.T) {
	// Retain a root on an engine whose accepted head is empty.
	c := &Controller{le: logrus.NewEntry(logrus.New()), conf: &Config{}}
	so := &testMaintenanceSharedObject{
		snapshot: &testMaintenanceSnapshot{role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
	}
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
