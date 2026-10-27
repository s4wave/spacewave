package provider_local

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
)

type replicaDestination struct {
	sobject.SharedObject
	store *BlockStore
}

func (r *replicaDestination) GetBlockStore() bstore.BlockStore {
	return r.store
}

// TestAccountReplicaCopyRetainsCompletedSubtrees copies a complete graph from a
// separate source. Durable completion proofs skip unchanged graphs on later heads.
func TestAccountReplicaCopyRetainsCompletedSubtrees(t *testing.T) {
	// Mount a seeded source Space.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	account, so, head, leaf, payload := mountSeededReplicaSource(ctx, t)

	// Copying twice copies the graph once, in batches.
	local := newBatchForwardTestStore()
	destination := &replicaDestination{SharedObject: so, store: &BlockStore{
		store: local, readStore: so.GetBlockStore(),
	}}
	for i := range 2 {
		progress := &AccountReplicaCopyState{Head: head}
		if err := account.copyAccountWorld(ctx, destination, progress, func() {}); err != nil {
			t.Fatal(err)
		}
		if i == 0 && progress.GetBlocks() == 0 {
			t.Fatal("copy did not visit the source graph")
		}
		if local.putBlockHits.Load() != 0 || (i == 0 && (local.putBlockBatchHits == 0 || uint64(local.putBlockBatchHits) >= progress.GetBlocks())) {
			t.Fatalf("copy of %d blocks used %d individual and %d batched writes", progress.GetBlocks(), local.putBlockHits.Load(), local.putBlockBatchHits)
		}
		if i == 1 && (progress.GetBlocks() != 0 || local.putBlockBatchHits != 0) {
			t.Fatal("completed immutable graph was copied again")
		}
		local.putBlockBatchHits = 0
	}

	// The local store holds the leaf.
	data, found, err := local.GetBlock(ctx, leaf)
	if err != nil || !found || string(data) != string(payload) {
		t.Fatalf("local leaf after copy: found=%v err=%v", found, err)
	}
}

// TestAccountReplicaCopyBatchesProofReads copies a complete graph into a
// separate volume that holds none of it. The copy checks each level of the
// graph for completion proofs, so it reads one batch of proofs per level, not
// one per reference.
func TestAccountReplicaCopyBatchesProofReads(t *testing.T) {
	// Mount a seeded source Space.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	account, so, head, leaf, payload := mountSeededReplicaSource(ctx, t)

	// Mount an empty destination Space in a separate volume.
	dstSO := mountEmptyReplicaDestination(ctx, t)

	// Copy the source graph into the destination, counting proof reads.
	counted := &proofCountingStore{Store: block_store.NewStore("destination", dstSO.GetBlockStore())}
	destination := &replicaDestination{SharedObject: so, store: &BlockStore{
		store: counted, readStore: so.GetBlockStore(),
	}}
	progress := &AccountReplicaCopyState{Head: head}
	if err := account.copyAccountWorld(ctx, destination, progress, func() {}); err != nil {
		t.Fatal(err)
	}

	// The destination holds the leaf, and the proofs came in fewer reads
	// than the copy visited blocks.
	data, found, err := dstSO.GetBlockStore().GetBlock(ctx, leaf)
	if err != nil || !found || string(data) != string(payload) {
		t.Fatalf("destination leaf after copy: found=%v err=%v", found, err)
	}
	reads := counted.reads.Load()
	t.Logf("copied %d blocks with %d proof reads", progress.GetBlocks(), reads)
	if reads == 0 || uint64(reads) >= progress.GetBlocks() {
		t.Fatalf("copy of %d blocks made %d proof reads", progress.GetBlocks(), reads)
	}
}

// TestAccountReplicaCopySkipsHeldSubtree copies a graph into a volume that
// already holds a complete subtree of it. The copy holds that subtree under
// its stage and skips it, so it visits only the blocks no sibling subtree
// shares with it.
func TestAccountReplicaCopySkipsHeldSubtree(t *testing.T) {
	// Mount a seeded source Space and an empty destination Space.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	account, so, head, _, _ := mountSeededReplicaSource(ctx, t)
	dstSO := mountEmptyReplicaDestination(ctx, t)

	// Hold back the first child of the head root.
	source := so.GetBlockStore()
	root, err := source.GetStoredBlock(ctx, head.GetRootRef())
	if err != nil || len(root.Refs) == 0 {
		t.Fatalf("head root children: %d, %v", len(root.GetRefs()), err)
	}
	subtree := root.Refs[0]
	total := countGraphBlocks(ctx, t, source, head.GetRootRef(), nil)
	want := countGraphBlocks(ctx, t, source, head.GetRootRef(), subtree)

	// Store the subtree in the destination with its completion proof.
	if err := block.CopyGraph(ctx, source, dstSO.GetBlockStore(), subtree, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := dstSO.GetBlockStore().Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := block.MarkRootComplete(ctx, dstSO.GetBlockStore(), subtree); err != nil {
		t.Fatal(err)
	}

	// Copy the graph, counting the blocks it visits.
	destination := &replicaDestination{SharedObject: so, store: &BlockStore{
		store: block_store.NewStore("destination", dstSO.GetBlockStore()), readStore: source,
	}}
	progress := &AccountReplicaCopyState{Head: head}
	if err := account.copyAccountWorld(ctx, destination, progress, func() {}); err != nil {
		t.Fatal(err)
	}
	t.Logf("copied %d of %d blocks", progress.GetBlocks(), total)
	if progress.GetBlocks() != uint64(want) {
		t.Fatalf("copy visited %d blocks, want %d of %d outside the held subtree", progress.GetBlocks(), want, total)
	}
}

// countGraphBlocks counts the blocks a copy of the graph under root visits
// when it skips the subtree under skip, if set.
func countGraphBlocks(ctx context.Context, t *testing.T, src block.StoreOps, root, skip *block.BlockRef) int {
	// Count visited blocks, treating the skipped subtree as already copied.
	t.Helper()
	var n int
	opts := &block.GraphCopyOptions{
		Visited: func(*block.BlockRef, []byte) { n++ },
		Known: func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
			known := make([]bool, len(refs))
			for i, ref := range refs {
				known[i] = skip != nil && ref.EqualVT(skip)
			}
			return known, nil
		},
	}

	// Copy into a scratch store.
	if err := block.CopyGraph(ctx, src, newBatchForwardTestStore(), root, opts); err != nil {
		t.Fatal(err)
	}
	return n
}

// mountEmptyReplicaDestination mounts an empty Space in its own volume.
func mountEmptyReplicaDestination(ctx context.Context, t *testing.T) sobject.SharedObject {
	// Start a provider account and create a Space.
	t.Helper()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	t.Cleanup(release)
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Mount it.
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSO)
	return so
}

// TestAccountReplicaCopyWaitsOnUnknownRefs copies from a source that holds
// every block without its refs. Retrying the same head cannot succeed, so the
// copy makes one attempt and waits for the next head, which ends it.
func TestAccountReplicaCopyWaitsOnUnknownRefs(t *testing.T) {
	// Mount a seeded source Space.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	account, so, head, _, _ := mountSeededReplicaSource(ctx, t)

	// Read the source through a store that drops every block's refs.
	source := &refsUnknownStore{StoreOps: so.GetBlockStore()}
	destination := &replicaDestination{SharedObject: so, store: &BlockStore{
		store: newBatchForwardTestStore(), copyStore: source,
	}}
	local, releaseLocal, err := so.AccessLocalStateStore(ctx, "account-replica-copy-test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLocal()

	// Without a peer exchange, the copy waits until the next head arrives.
	cur := &replicaHead{head: head}
	latest := ccontainer.NewCContainer(cur)
	done := make(chan error, 1)
	go func() {
		done <- account.copyAccountWorldHead(ctx, destination, &p2pSyncState{}, local, latest, cur, nil)
	}()
	select {
	case err := <-done:
		t.Fatalf("copy returned %v before the next head", err)
	case <-time.After(500 * time.Millisecond):
	}
	latest.SetValue(&replicaHead{head: head.CloneVT()})
	if err := <-done; err != nil {
		t.Fatalf("copy returned %v after the next head", err)
	}
	if reads := source.reads.Load(); reads != 1 {
		t.Fatalf("copy read the source %d times, want one attempt", reads)
	}
}

// TestAccountReplicaCopyKeepsProofsAroundHoles copies from a source missing one
// block, or whose peers all refuse it. The walk finishes and names the block,
// and the next walk of the same head reads only the path to it.
func TestAccountReplicaCopyKeepsProofsAroundHoles(t *testing.T) {
	for name, holeErr := range map[string]error{
		"missing": block.ErrNotFound,
		"refused": fmt.Errorf("%w: no peer served block: shared object block access denied", block.ErrUnavailable),
	} {
		t.Run(name, func(t *testing.T) {
			// Mount a seeded source Space that cannot serve its leaf payload.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			account, so, head, leaf, _ := mountSeededReplicaSource(ctx, t)
			source := &holeStore{StoreOps: so.GetBlockStore(), hole: leaf, err: holeErr}
			destination := &replicaDestination{SharedObject: so, store: &BlockStore{
				store: newBatchForwardTestStore(), copyStore: source,
			}}

			// Each walk finishes and names the leaf.
			walk := func() int32 {
				// Copy the head, counting the source reads.
				t.Helper()
				source.reads.Store(0)
				progress := &AccountReplicaCopyState{ObjectId: so.GetSharedObjectID(), Head: head}
				err := account.copyAccountWorld(ctx, destination, progress, func() {})
				var holes *worldHolesError
				if !errors.As(err, &holes) || !errors.Is(err, block.ErrNotFound) {
					t.Fatalf("copy returned %v, want the leaf as a hole", err)
				}
				if len(holes.refs) != 1 || !holes.refs[0].EqualVT(leaf) {
					t.Fatalf("copy named holes %v, want the leaf", holes.refs)
				}
				return source.reads.Load()
			}
			first, second := walk(), walk()
			if second >= first {
				t.Fatalf("second walk read %d blocks, first read %d", second, first)
			}
		})
	}
}

// holeStore serves its inner store's blocks except one.
type holeStore struct {
	block.StoreOps
	// hole is the block the store does not serve.
	hole *block.BlockRef
	// err is the error a read of hole returns.
	err error
	// reads counts the stored block reads.
	reads atomic.Int32
}

// GetStoredBlock reads the inner block unless it is the hole.
func (s *holeStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.reads.Add(1)
	if ref.EqualVT(s.hole) {
		return nil, s.err
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// refsUnknownStore serves its inner store's blocks without their refs.
type refsUnknownStore struct {
	block.StoreOps
	// reads counts the stored block reads.
	reads atomic.Int32
}

// GetStoredBlock reads the inner block and drops its refs.
func (s *refsUnknownStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.reads.Add(1)
	stored, err := s.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}
	return &block.StoredBlock{Data: stored.Data}, nil
}

// mountSeededReplicaSource starts a provider account, seeds a Space with one
// leaf payload, mounts it, and returns its World head with the leaf.
func mountSeededReplicaSource(ctx context.Context, t *testing.T) (*ProviderAccount, sobject.SharedObject, *bucket.ObjectRef, *block.BlockRef, []byte) {
	// Start a provider account and create a seeded Space.
	t.Helper()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	t.Cleanup(release)
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, account, ref)

	// Mount it.
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSO)
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()

	// Decode its World head.
	inner, err := states.GetValue().GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := &sobject_world_engine.InnerState{}
	if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatal(err)
	}
	return account, so, head.GetHeadRef(), leaf, payload
}

// TestSharedObjectBlockStoreRetainsRoots checks that the write-back layer
// exposes the volume's root retention, so replacing a World head releases the
// superseded graph to the collector.
func TestSharedObjectBlockStoreRetainsRoots(t *testing.T) {
	// Start a provider account on a test volume.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, account, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Create and mount a Space shared object.
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()

	// Its block store must reach the volume's root retention.
	if !block.SupportsRootRetention(so.GetBlockStore()) {
		t.Fatal("shared object block store hides root retention")
	}
}
