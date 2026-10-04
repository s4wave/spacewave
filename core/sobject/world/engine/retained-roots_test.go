package sobject_world_engine

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
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

// TestCopyWorldsCopiesFromStorage checks holdRootSet names a root without
// copying its graph, copyWorlds copies the graph from storage into the local
// store and then succeeds from the local store alone, and a root missing from
// both stores reports block.ErrNotFound.
func TestCopyWorldsCopiesFromStorage(t *testing.T) {
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

	// Holding the root through a store that reads storage on a local miss
	// leaves the leaf in storage.
	overlay := block.NewOverlay(ctx, le, source.GetBucket(), target.GetBucket(), block.OverlayMode_UPPER_WRITE_CACHE, 0, nil)
	so := &testSharedObject{
		blockStore: newTestBlockStore("retained-roots-test", overlay),
		localStore: store_kvtx_inmem.NewStore(),
	}
	if err := holdRootSet(ctx, so, retainedRootsName, []*RetainedRoot{{Name: "backup", RootRef: root}}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := target.GetBucket().GetBlock(ctx, leaf); err != nil || found {
		t.Fatalf("hold copied the leaf: found=%v, err=%v", found, err)
	}

	// Copying the World brings the leaf into the local store.
	roots := []*block.BlockRef{root}
	if err := copyWorlds(ctx, so, backfillProofStoreID, roots); err != nil {
		t.Fatal(err)
	}
	if _, found, err := target.GetBucket().GetBlock(ctx, leaf); err != nil || !found {
		t.Fatalf("leaf not copied locally: found=%v, err=%v", found, err)
	}

	// Without storage, the local copy still satisfies the root.
	so.blockStore = newTestBlockStore("retained-roots-test", target.GetBucket())
	if err := copyWorlds(ctx, so, backfillProofStoreID, roots); err != nil {
		t.Fatalf("copy from the local store: %v", err)
	}

	// A root in neither store is lost.
	lost := []*block.BlockRef{{Hash: testRetainedRootHash(t, "lost")}}
	if err := copyWorlds(ctx, so, backfillProofStoreID, lost); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("copy lost root = %v; want block.ErrNotFound", err)
	}
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
	// Open an engine whose accepted World is empty.
	ctx := t.Context()
	c, so, head := newProcessTestWorld(t, ctx)
	so.localStore = store_kvtx_inmem.NewStore()
	so.snapshot = &testSharedObjectSnapshot{}
	blk, err := buildBlockEngine(ctx, c.le, c.bus, c.sfs, so, head.GetHeadRef(), head.GetHeadRef().GetTransformConf(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(blk.Release)
	e := newSoEngine(c, so, blk.bengine, newReplayer(c, so))

	// Retaining another root is refused before anything is queued.
	ref := &block.BlockRef{Hash: testRetainedRootHash(t, "old")}
	if err := e.SetRetainedRoot(ctx, "backup", ref); !errors.Is(err, errRetainedRootNotHead) {
		t.Fatalf("retain non-head root = %v; want errRetainedRootNotHead", err)
	}
	if len(so.queued) != 0 {
		t.Fatalf("queued %d operations", len(so.queued))
	}
}
