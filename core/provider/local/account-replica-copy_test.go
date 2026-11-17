package provider_local

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
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

// TestAccountReplicaCopyFollowsParticipantGrants resumes an incomplete graph
// when the existing peer grants access without publishing another World.
func TestAccountReplicaCopyFollowsParticipantGrants(t *testing.T) {
	// Retain both authenticated providers and their existing DEX stream.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	peers := newDEXRolePeers(t, ctx)
	owner, replica := peers.objects[0], peers.objects[1]
	requester := peers.sessions[1].GetPeerId().String()

	// Revoke only the Session, leaving the replica's storage grant to sync state.
	removed, err := sobject.RemoveSOParticipant(ctx, owner.soHost, requester, owner.GetPrivKey(), nil)
	if err != nil || !removed {
		t.Fatalf("remove requester = %v/%v", removed, err)
	}
	states, releaseStates, err := replica.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return !slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == requester
		}), nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Publish a graph that the revoked requester cannot fetch yet.
	entries := peers.accounts[0].soListCtr.GetValue().GetSharedObjects()
	i := slices.IndexFunc(entries, func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == owner.GetSharedObjectID()
	})
	if i == -1 {
		t.Fatal("source object is absent from account")
	}
	leaf, payload := seedProviderReplicaPayload(ctx, t, peers.accounts[0], peers.accounts[0], entries[i].GetRef())
	state := &p2pSyncState{}
	done := make(chan error, 1)
	go func() {
		done <- peers.accounts[1].runAccountReplicaCopy(ctx, replica, state, peers.exchanges[1])
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Observe the completed denied walk before restoring its authority.
	denied := waitReplicaCopyProgress(ctx, t, state, replica.GetSharedObjectID(), false)
	starts := peers.exchanges[1].GetSessionStarts()

	// Restore the same Session's grant on the same published graph and stream.
	if _, _, _, err := sobject.AddSOParticipant(ctx, owner.soHost, owner.GetSharedObjectID(), owner.GetPrivKey(), owner.GetPeerID().String(), requester, peers.sessions[1].GetPrivKey().GetPublic(), sobject.SOParticipantRole_SOParticipantRole_WRITER, "", ""); err != nil {
		t.Fatal(err)
	}
	completed := waitReplicaCopyProgress(ctx, t, state, replica.GetSharedObjectID(), true)
	if !denied.GetHead().EqualVT(completed.GetHead()) || peers.exchanges[1].GetSessionStarts() != starts {
		t.Fatal("copy required another World or DEX session")
	}
	data, found, err := replica.GetBlockStore().GetBlock(ctx, leaf)
	if err != nil || !found || string(data) != string(payload) {
		t.Fatalf("replica leaf after grant: found=%v err=%v", found, err)
	}
}

// TestAccountReplicaCopyRereadsHolesOnSessionStart copies a graph whose leaf
// no peer holds, then returns the leaf to the owner and restarts the owner's
// transport. The replica's new peer session alone must read the known-absent
// hole again and complete the copy of the same head.
func TestAccountReplicaCopyRereadsHolesOnSessionStart(t *testing.T) {
	// Seed the owner's graph and evict its leaf from both peers' Volumes,
	// keeping the bytes; the owner goes first so the replica cannot fetch it
	// back. A bucket's RmBlock only releases its hold.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	peers := newDEXRolePeers(t, ctx)
	owner, replica := peers.objects[0], peers.objects[1]
	entries := peers.accounts[0].soListCtr.GetValue().GetSharedObjects()
	i := slices.IndexFunc(entries, func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == owner.GetSharedObjectID()
	})
	if i == -1 {
		t.Fatal("source object is absent from account")
	}
	leaf, payload := seedProviderReplicaPayload(ctx, t, peers.accounts[0], peers.accounts[0], entries[i].GetRef())
	ownerStore := owner.GetBlockStore()
	for _, account := range peers.accounts {
		if err := account.vol.RmBlock(ctx, leaf); err != nil {
			t.Fatal(err)
		}
	}

	// Copy until the walk records the leaf as a hole.
	state := &p2pSyncState{}
	done := make(chan error, 1)
	go func() {
		done <- peers.accounts[1].runAccountReplicaCopy(ctx, replica, state, peers.exchanges[1])
	}()
	defer func() {
		cancel()
		<-done
	}()
	holed := waitReplicaCopyProgress(ctx, t, state, replica.GetSharedObjectID(), false)
	starts := peers.exchanges[1].GetSessionStarts()

	// Return the leaf to the owner, then restart the owner's transport so a
	// new peer session starts at the replica.
	ref, _, err := ownerStore.PutBlock(ctx, payload, &block.PutOpts{RefsKnown: true})
	if err != nil || !ref.EqualVT(leaf) {
		t.Fatalf("restore leaf = %v/%v, want %v", ref, err, leaf)
	}
	peers.accounts[0].StopP2PSync()
	peers.accounts[0].StopSessionTransport()
	if err := peers.accounts[0].EnsureConfiguredSessionTransport(ctx, peers.sessions[0].GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if err := peers.accounts[0].StartPersistentP2PSync(ctx, peers.accounts[0].GetSessionTransport()); err != nil {
		t.Fatal(err)
	}

	// The session start completes the copy of the same head.
	completed := waitReplicaCopyProgress(ctx, t, state, replica.GetSharedObjectID(), true)
	if !holed.GetHead().EqualVT(completed.GetHead()) {
		t.Fatal("copy required another World")
	}
	if peers.exchanges[1].GetSessionStarts() == starts {
		t.Fatal("copy completed without a peer session start")
	}
	data, found, err := replica.GetBlockStore().GetBlock(ctx, leaf)
	if err != nil || !found || string(data) != string(payload) {
		t.Fatalf("replica leaf after session start: found=%v err=%v", found, err)
	}
}

// waitReplicaCopyProgress waits until the copy of id publishes a completed
// walk, or a failed one when complete is false, and returns its progress.
func waitReplicaCopyProgress(ctx context.Context, t *testing.T, state *p2pSyncState, id string, complete bool) *AccountReplicaCopyState {
	t.Helper()
	for {
		// Read progress and subscribe to its next change under one lock.
		var progress *AccountReplicaCopyState
		var wait <-chan struct{}
		state.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			progress = state.copyProgress[id].CloneVT()
			wait = getWait()
		})

		// Finish only after the walk publishes the requested outcome.
		if progress != nil && (complete && progress.GetComplete() || !complete && progress.GetError() != "") {
			return progress
		}

		// Await a real progress transition or the test's failure deadline.
		select {
		case <-ctx.Done():
			t.Fatalf("wait for copy complete=%v: %v; progress=%v", complete, ctx.Err(), progress)
		case <-wait:
		}
	}
}

// TestAccountReplicaCopyRetainsCompletedSubtrees copies a complete graph from a
// separate source into an empty volume and holds it under the account World
// root. Durable completion proofs skip the unchanged graph on a later copy.
func TestAccountReplicaCopyRetainsCompletedSubtrees(t *testing.T) {
	// Mount a seeded source Space and an empty destination Space.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	account, so, head, leaf, payload := mountSeededReplicaSource(ctx, t)
	dstSO := mountEmptyReplicaDestination(ctx, t)

	// Copying twice copies the graph once.
	local := dstSO.GetBlockStore()
	destination := &replicaDestination{SharedObject: so, store: &BlockStore{
		store: block_store.NewStore("destination", local), readStore: so.GetBlockStore(),
	}}
	for i := range 2 {
		progress := &AccountReplicaCopyState{Head: head}
		if err := account.copyAccountWorld(ctx, destination, progress, &knownAbsent{}, func() {}); err != nil {
			t.Fatal(err)
		}
		if i == 0 && progress.GetBlocks() == 0 {
			t.Fatal("copy did not visit the source graph")
		}
		if i == 1 && progress.GetBlocks() != 0 {
			t.Fatalf("completed immutable graph was copied again: %d blocks", progress.GetBlocks())
		}
	}

	// The account World root holds the head, and the local store holds the
	// leaf.
	held, err := local.GetRoot(ctx, accountWorldRoot)
	if err != nil || !held.EqualsRef(head.GetRootRef()) {
		t.Fatalf("account World root after copy: %v, %v", held, err)
	}
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
	if err := account.copyAccountWorld(ctx, destination, progress, &knownAbsent{}, func() {}); err != nil {
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
	if err := account.copyAccountWorld(ctx, destination, progress, &knownAbsent{}, func() {}); err != nil {
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
		done <- account.copyAccountWorldHead(ctx, destination, &p2pSyncState{}, local, latest, cur, &knownAbsent{}, nil)
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
// and the next walk of the same head reads only the path to it. The block is
// then known absent: the next walk names it without reading it, until a reset,
// as on a peer session start, after which the probe reads it once.
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
			dstSO := mountEmptyReplicaDestination(ctx, t)
			source := &holeStore{StoreOps: so.GetBlockStore(), hole: leaf, err: holeErr}
			destination := &replicaDestination{SharedObject: so, store: &BlockStore{
				store: block_store.NewStore("destination", dstSO.GetBlockStore()), copyStore: source,
			}}

			// Each walk finishes and names the leaf.
			absent := &knownAbsent{}
			walk := func() int32 {
				// Copy the head, counting the source reads.
				t.Helper()
				source.reads.Store(0)
				progress := &AccountReplicaCopyState{ObjectId: so.GetSharedObjectID(), Head: head}
				err := account.copyAccountWorld(ctx, destination, progress, absent, func() {})
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
			if got := source.holeReads.Load(); got != 1 {
				t.Fatalf("two walks read the hole %d times, want once", got)
			}

			// A peer session start resets the set; the probe reads the hole
			// once and the walk after it not at all.
			absent.reset()
			holes := &worldHolesError{refs: []*block.BlockRef{leaf}}
			if arrived, err := holes.probe(ctx, source, absent, absent.generation()); err != nil || arrived {
				t.Fatalf("probe returned arrived=%v err=%v, want the hole still absent", arrived, err)
			}
			walk()
			if got := source.holeReads.Load(); got != 2 {
				t.Fatalf("hole read %d times after one reset, want twice", got)
			}

			// A reset that lands while a walk reads the hole drops the walk's
			// holes, so the next walk reads the hole again.
			absent.reset()
			source.onHole = absent.reset
			walk()
			source.onHole = nil
			if absent.has(leaf) {
				t.Fatal("a walk begun before a reset left its hole in the set")
			}
			walk()
			if got := source.holeReads.Load(); got != 4 {
				t.Fatalf("hole read %d times after a reset during a walk, want four", got)
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
	// holeReads counts the reads of hole, stored or plain.
	holeReads atomic.Int32
	// onHole, when set, runs on each stored read of hole.
	onHole func()
}

// GetStoredBlock reads the inner block unless it is the hole.
func (s *holeStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.reads.Add(1)
	if ref.EqualVT(s.hole) {
		s.holeReads.Add(1)
		if s.onHole != nil {
			s.onHole()
		}
		return nil, s.err
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// GetBlock reads the inner block unless it is the hole, which a missing store
// reports as not found and a refusing one as its error.
func (s *holeStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if !ref.EqualVT(s.hole) {
		return s.StoreOps.GetBlock(ctx, ref)
	}
	s.holeReads.Add(1)
	if errors.Is(s.err, block.ErrNotFound) {
		return nil, false, nil
	}
	return nil, false, s.err
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
	head := &space_world.InnerState{}
	if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatal(err)
	}
	return account, so, head.GetHeadRef(), leaf, payload
}

// TestSharedObjectBlockStoreRetainsRoots checks that the write-back layer
// exposes the volume's leased roots, so replacing a World head releases the
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

	// Its block store must reach the volume's leased roots.
	if !block.HoldsRoots(so.GetBlockStore()) {
		t.Fatal("shared object block store hides the volume's roots")
	}
}
