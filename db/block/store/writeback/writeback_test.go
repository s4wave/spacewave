package block_store_writeback

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_testing "github.com/s4wave/spacewave/db/block/store/test"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// testMarkersID is the ID of the object store that holds the test markers.
const testMarkersID = "test/writeback"

// leafOpts declares a block with no children.
var leafOpts = &block.PutOpts{RefsKnown: true}

// ownedStore writes through a Volume under the ownership of a bucket, as the
// account's bucket handle does, so the Volume tracks each block's refs.
type ownedStore struct {
	*volume_kvtx.Volume
}

// PutBlock writes a block owned by the bucket.
func (o ownedStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return o.PrepareOwnedBlock(ctx, "bucket", data, opts)
}

// GetStoredBlock reads a block with the refs the Volume tracks.
func (o ownedStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block_gc.NewGCStoreOps(o.Volume, o.GetRefGraph()).GetStoredBlock(ctx, ref)
}

// PutBlockBatch writes a batch owned by the bucket.
func (o ownedStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	return o.PrepareOwnedBlockBatch(ctx, "bucket", entries)
}

// testEnv is a Volume over an in-memory key-value store, which holds the
// blocks and the markers of a Store. open reopens it over the same key-value
// store, as a restart does.
type testEnv struct {
	kv  *store_kvtx_inmem.Store
	vol *volume_kvtx.Volume
}

// newTestEnv opens a Volume over an empty key-value store.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	env := &testEnv{kv: store_kvtx_inmem.NewStore()}
	env.open(t)
	return env
}

// open closes the Volume and opens it again over the same key-value store.
func (e *testEnv) open(t *testing.T) {
	// Close the open Volume first, then reopen over the same store.
	t.Helper()
	if e.vol != nil {
		if err := e.vol.Close(); err != nil {
			t.Fatal(err)
		}
	}
	vol, err := volume_kvtx.NewVolume(t.Context(), "test/writeback", store_kvkey.NewDefaultKVKey(), e.kv, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	e.vol = vol
}

// markers returns the object store of markers beside the Volume's blocks.
func (e *testEnv) markers(t *testing.T) kvtx.Store {
	// Read the markers through the Volume.
	t.Helper()
	markers, release, err := e.vol.AccessObjectStore(t.Context(), testMarkersID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return markers
}

// newStore opens a Store over the Volume, counting the markers a previous
// Store left.
func (e *testEnv) newStore(t *testing.T, enabled bool) *Store {
	t.Helper()
	s, err := NewStore(t.Context(), ownedStore{e.vol}, e.vol, e.markers(t), testMarkersID, enabled)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// isMarked reports whether a marker names the block.
func (e *testEnv) isMarked(t *testing.T, ref *block.BlockRef) bool {
	// Look the block up in a read transaction.
	t.Helper()
	tx, err := e.markers(t).NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	found, err := tx.Exists(t.Context(), markedKey(ref.GetHash()))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// failingStore rejects every batch write, standing in for an unreachable bucket.
type failingStore struct {
	block.StoreOps
}

// PutBlockBatch always fails.
func (f *failingStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, errors.New("bucket unreachable")
}

// leafStore tracks every block as a leaf, as a GC store does for a block with
// no outgoing refs. The bare in-memory store holds bytes without refs.
type leafStore struct {
	block.StoreOps
}

// GetStoredBlock reads the block as a known leaf.
func (l *leafStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := block.GetBlockWithoutRefs(ctx, l.StoreOps, ref)
	if stored != nil {
		stored.RefsKnown = true
	}
	return stored, err
}

func newInmemBlockStore() block.StoreOps {
	return &leafStore{StoreOps: newUntrackedBlockStore()}
}

// newUntrackedBlockStore returns a store that holds bytes without refs.
func newUntrackedBlockStore() block.StoreOps {
	return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
}

// drain uploads the Store's queue to remote and returns once nothing is
// pending.
func drain(t *testing.T, s *Store, remote block.StoreOps) {
	// Run Upload in the background and wait for the queue to empty.
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Upload(ctx, remote) }()
	for {
		status, changed := s.GetStatus()
		if status.Err != nil {
			t.Fatalf("upload: %v", status.Err)
		}
		if status.Pending == 0 {
			break
		}
		<-changed
	}
	cancel()
	<-done
}

// TestUploadAfterRestart checks that queued writes survive a failed upload and
// a reopen, then drain to the remote store.
func TestUploadAfterRestart(t *testing.T) {
	// Prepare local block storage and durable upload markers.
	ctx := t.Context()
	env := newTestEnv(t)

	// Unplaced writes are not queued.
	s := env.newStore(t, false)
	if _, _, err := s.PutBlock(ctx, []byte("unplaced"), leafOpts); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}

	// Two distinct writes and one repeat queue two blocks.
	var refs []*block.BlockRef
	for _, data := range []string{"alpha", "beta", "alpha"} {
		ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	if status, _ := s.GetStatus(); status.Pending != 2 || status.PendingBytes != 9 {
		t.Fatalf("pending %d (%d bytes), expected 2 (9 bytes)", status.Pending, status.PendingBytes)
	}

	// A failing remote keeps the markers and reports the failure.
	if err := s.Upload(ctx, &failingStore{StoreOps: newInmemBlockStore()}); err == nil {
		t.Fatal("expected upload failure")
	}
	if status, _ := s.GetStatus(); status.Pending != 2 || status.Err == nil {
		t.Fatalf("after failure: pending %d, err %v", status.Pending, status.Err)
	}

	// A reopened store recounts the markers and drains them.
	s = env.newStore(t, true)
	remote := newInmemBlockStore()
	drain(t, s, remote)
	if status, _ := s.GetStatus(); status.Uploaded != 2 || status.UploadedBytes != 9 || status.UploadReads < 1 {
		t.Fatalf("drain counted %d blocks, %d bytes, %d reads", status.Uploaded, status.UploadedBytes, status.UploadReads)
	}

	// Require the remote store to contain both queued blocks.
	for _, ref := range refs[:2] {
		found, err := remote.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("remote missing %s: %v", ref.MarshalString(), err)
		}
	}
}

// publicationOf builds a publication of blocks that moves head from the
// previous number to number.
func publicationOf(t *testing.T, number int, blocks ...string) (*block.AtomicPublication, []*block.BlockRef) {
	// Build the publication from the blocks in order.
	t.Helper()
	pub := &block.AtomicPublication{BucketID: "bucket", TrackGC: true}
	var refs []*block.BlockRef
	for _, data := range blocks {
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		pub.Entries = append(pub.Entries, &block.PutBatchEntry{Ref: ref, Data: []byte(data), RefsKnown: true})
		refs = append(refs, ref)
	}
	pub.Heads = []*block.AtomicHeadUpdate{{ObjectStoreID: "so", Key: []byte("head"), Replace: func(_ context.Context, current []byte, found bool) ([]byte, error) {
		if want := strconv.Itoa(number - 1); (number > 1) != found || (found && string(current) != want) {
			return nil, errors.Errorf("head %q is not %q", current, want)
		}
		return []byte(strconv.Itoa(number)), nil
	}}}
	return pub, refs
}

// failingMarker marks its blocks, then fails, as a marker that wrote its
// records and then lost the transaction.
type failingMarker struct {
	kvtx.BlockMarker
}

// MarkBlocks marks the blocks, then fails.
func (f *failingMarker) MarkBlocks(ctx context.Context, tx kvtx.Tx, blocks []kvtx.BlockMark) (func(), error) {
	if _, err := f.BlockMarker.MarkBlocks(ctx, tx, blocks); err != nil {
		return nil, err
	}
	return nil, errors.New("marker failed")
}

// TestAtomicPublicationMarks checks that a publication and the markers of its
// blocks commit together: a publication that fails leaves neither, a
// committed one survives a restart marked, and the restarted store uploads it.
func TestAtomicPublicationMarks(t *testing.T) {
	// Publish, crash before upload, reopen, and upload.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)

	// A publication whose marker fails after writing its records commits
	// neither the blocks nor the records.
	failed, failedRefs := publicationOf(t, 1, "failed-a", "failed-b")
	failCtx, _ := kvtx.WithBlockMarker(ctx, &failingMarker{BlockMarker: s})
	if err := env.vol.PublishAtomic(failCtx, failed); err == nil {
		t.Fatal("expected the publication to fail")
	}
	for _, ref := range failedRefs {
		if found, err := env.vol.GetBlockExists(ctx, ref); err != nil || found {
			t.Fatalf("failed block %s = %v/%v, want absent", ref.MarshalString(), found, err)
		}
		if env.isMarked(t, ref) {
			t.Fatalf("failed block %s is marked", ref.MarshalString())
		}
	}
	if status, _ := s.GetStatus(); status.Pending != 0 || status.PendingBytes != 0 {
		t.Fatalf("failed publication left %+v", status)
	}

	// Publish atomically, repeating one block already stored and marked.
	if _, _, err := s.PutBlock(ctx, []byte("shared"), leafOpts); err != nil {
		t.Fatal(err)
	}
	pub, refs := publicationOf(t, 1, "atomic-a", "atomic-b", "shared")
	pubCtx, req := kvtx.WithBlockMarker(ctx, s)
	if err := env.vol.PublishAtomic(pubCtx, pub); err != nil {
		t.Fatal(err)
	}
	if !req.Applied() {
		t.Fatal("the publication did not mark its blocks")
	}
	if status, _ := s.GetStatus(); status.Pending != 3 || status.PendingBytes != int64(len("atomic-a")+len("atomic-b")+len("shared")) {
		t.Fatalf("after publish: %+v", status)
	}

	// Crash before any upload: reopen the Volume and the store.
	env.open(t)
	s = env.newStore(t, true)
	if status, _ := s.GetStatus(); status.Pending != 3 {
		t.Fatalf("after restart pending %d, expected 3", status.Pending)
	}
	for _, ref := range refs {
		if found, err := env.vol.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("block %s = %v/%v after restart, want stored", ref.MarshalString(), found, err)
		}
		if !env.isMarked(t, ref) {
			t.Fatalf("block %s is not marked after restart", ref.MarshalString())
		}
	}

	// The restarted store uploads every block and clears every marker.
	remote := newInmemBlockStore()
	drain(t, s, remote)
	for _, ref := range refs {
		if found, err := remote.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("remote missing %s: %v", ref.MarshalString(), err)
		}
		if env.isMarked(t, ref) {
			t.Fatalf("block %s is marked after upload", ref.MarshalString())
		}
	}
}

// TestDisableDropsMarkers checks that disabling upload clears the queue.
func TestDisableDropsMarkers(t *testing.T) {
	// Create an enabled upload store over durable markers.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)

	// Queue a block and disable uploads to drop its marker.
	if _, _, err := s.PutBlock(ctx, []byte("queued"), leafOpts); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}

	// A write while disabled queues nothing.
	if _, _, err := s.PutBlock(ctx, []byte("unqueued"), leafOpts); err != nil {
		t.Fatal(err)
	}

	// Require a reopened upload store to retain the empty queue.
	reopened := env.newStore(t, true)
	if status, _ := reopened.GetStatus(); status.Pending != 0 {
		t.Fatalf("pending %d after disable, expected 0", status.Pending)
	}
}

// TestBackfillQueuesStoredBlocks checks that choosing a remote queues the
// blocks stored before it, once per remote, across a reopen.
func TestBackfillQueuesStoredBlocks(t *testing.T) {
	// Create local block storage with uploads initially disabled.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, false)

	// Two blocks are stored while uploads are off, and a third ref is listed
	// but not stored.
	var refs []*block.BlockRef
	for _, data := range []string{"before-a", "before-b"} {
		ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	missing, _, err := newInmemBlockStore().PutBlock(ctx, []byte("elsewhere"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var listed int
	list := func(context.Context) ([]*block.BlockRef, error) {
		listed++
		return append(refs, missing), nil
	}

	// Backfill the selected remote target with the existing local blocks.
	if err := s.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Backfill(ctx, "bucket", list); err != nil {
		t.Fatal(err)
	}
	status, _ := s.GetStatus()
	if status.Pending != 2 || status.PendingBytes != 16 || status.Target != "bucket" {
		t.Fatalf("after backfill: %+v", status)
	}

	// A reopened store remembers the completed backfill.
	s = env.newStore(t, true)
	if err := s.Backfill(ctx, "bucket", list); err != nil {
		t.Fatal(err)
	}
	if listed != 1 {
		t.Fatalf("listed %d times, expected 1", listed)
	}

	// Disabling forgets it, so the next remote backfills again.
	if err := s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); status.Target != "" {
		t.Fatalf("target %q after disable", status.Target)
	}
}

// TestDroppedBlockWrittenAgainKeepsMarker checks that a marker whose block was
// swept before its upload read it is cleared, but one whose block was stored
// again after that read stays queued.
func TestDroppedBlockWrittenAgainKeepsMarker(t *testing.T) {
	// Write a block, then sweep it as dropped.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	ref, _, err := s.PutBlock(ctx, []byte("swept"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.local.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}

	// Read the batch as an upload does: the block is gone, so it is dropped.
	head, err := s.scanHead(ctx, batchLimitsOf(nil))
	if err != nil || len(head.entries) != 1 {
		t.Fatalf("scan = %d blocks, %v", len(head.entries), err)
	}
	batch := head.entries
	remote := newInmemBlockStore()
	if err := s.uploadChunk(ctx, remote, batch); err != nil || !batch[0].dropped {
		t.Fatalf("upload chunk = %v, dropped %v", err, batch[0].dropped)
	}

	// The block is written again before the marker clears.
	if _, _, err := s.PutBlock(ctx, []byte("swept"), leafOpts); err != nil {
		t.Fatal(err)
	}
	if err := s.clear(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); status.Pending != 1 {
		t.Fatalf("pending %d, want the rewritten block queued", status.Pending)
	}
	drain(t, s, remote)
	if found, err := remote.GetBlockExists(ctx, ref); err != nil || !found {
		t.Fatalf("remote missing the rewritten block: %v", err)
	}
}

// TestUploadUntrackedBlock checks that a block held without its refs fails the
// upload and stays queued, since the remote would keep it as a leaf.
func TestUploadUntrackedBlock(t *testing.T) {
	// Queue a block that the local store holds without its refs.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	ref, _, err := env.vol.Store.PutBlock(ctx, []byte("untracked"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.queue(ctx, []Mark{{Hash: ref.GetHash(), Size: int64(len("untracked"))}}); err != nil {
		t.Fatal(err)
	}

	// The upload fails naming the block, and nothing reaches the remote.
	remote := newInmemBlockStore()
	err = s.Upload(ctx, remote)
	if !errors.Is(err, block.ErrRefsUnknown) || !strings.Contains(err.Error(), ref.MarshalString()) {
		t.Fatalf("upload = %v, want ErrRefsUnknown naming %s", err, ref.MarshalString())
	}
	found, err := remote.GetBlockExists(ctx, ref)
	if err != nil || found {
		t.Fatalf("remote = %v/%v, want absent", found, err)
	}
	if status, _ := s.GetStatus(); status.Pending != 1 || !errors.Is(status.Err, block.ErrRefsUnknown) {
		t.Fatalf("status = %+v, want one pending block and ErrRefsUnknown", status)
	}
}

// TestMarkingStoreStageHoldsCompleteRoots checks that a stage of the marking
// store holds completion proofs under the inner stage.
func TestMarkingStoreStageHoldsCompleteRoots(t *testing.T) {
	block_store_testing.RequireStageHold(t, func(ops block.StoreOps) block.StoreOps {
		return NewMarkingStore(ops, nil)
	})
}

// sizedStore sizes its batches and records them, standing in for a bucket of
// packfiles. It fails the write numbered failAt, counting from 1, once.
type sizedStore struct {
	block.StoreOps
	limits block.BatchLimits
	mtx    sync.Mutex
	// batches lists the size of each batch written.
	batches []int
	// failAt is the number of the write to fail, or zero.
	failAt int
}

// GetBatchLimits returns the test's limits.
func (r *sizedStore) GetBatchLimits() block.BatchLimits {
	return r.limits
}

// PutBlockBatch records the batch, failing the numbered write.
func (r *sizedStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Record the batch and decide whether this write fails.
	r.mtx.Lock()
	r.batches = append(r.batches, len(entries))
	fail := len(r.batches) == r.failAt
	r.mtx.Unlock()

	// Fail the numbered write, or store the batch.
	if fail {
		return nil, errors.New("bucket unreachable")
	}
	return r.StoreOps.PutBlockBatch(ctx, entries)
}

// getBatches returns the sizes of the batches written so far.
func (r *sizedStore) getBatches() []int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return slices.Clone(r.batches)
}

// waitStatus waits until the status satisfies done.
func waitStatus(t *testing.T, s *Store, done func(Status) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		status, changed := s.GetStatus()
		if done(status) {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatalf("status never satisfied the wait: %+v", status)
		}
	}
}

// TestUploadFillsBatches checks that a remote that sizes its batches gets full
// ones at once and the short rest only after the linger time, and that a failed
// batch repeats alone.
func TestUploadFillsBatches(t *testing.T) {
	// Open a store that queues its writes.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)

	// Queue 25 blocks of 100 bytes, which fill two batches of ten and leave five.
	for i := range 25 {
		data := []byte(strings.Repeat(strconv.Itoa(i%10), 99) + string(rune('a'+i)))
		if _, _, err := s.PutBlock(ctx, data, leafOpts); err != nil {
			t.Fatal(err)
		}
	}
	const linger = 400 * time.Millisecond
	remote := &sizedStore{
		StoreOps: newInmemBlockStore(),
		limits:   block.BatchLimits{Bytes: 1000, Blocks: 10, Workers: 1, Linger: linger},
		failAt:   2,
	}

	// The first full batch uploads, then the second fails and repeats alone.
	start := time.Now()
	if err := s.Upload(ctx, remote); err == nil {
		t.Fatal("expected the second batch to fail")
	}
	if status, _ := s.GetStatus(); status.Pending != 15 || status.Uploaded != 10 {
		t.Fatalf("after failure: pending %d, uploaded %d, want 15 and 10", status.Pending, status.Uploaded)
	}

	// The retry uploads the second full batch at once.
	uploadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Upload(uploadCtx, remote) }()
	waitStatus(t, s, func(st Status) bool { return st.Pending == 5 })
	if sizes := remote.getBatches(); !slices.Equal(sizes, []int{10, 10, 10}) {
		t.Fatalf("batches %v, want 10, the failed 10 and its retry", sizes)
	}

	// The rest of five uploads only after the linger time.
	waitStatus(t, s, func(st Status) bool { return st.Pending == 0 })
	if waited := time.Since(start); waited < linger {
		t.Fatalf("the short batch uploaded after %v, before its linger time", waited)
	}
	if sizes := remote.getBatches(); !slices.Equal(sizes, []int{10, 10, 10, 5}) {
		t.Fatalf("batches %v, want a final batch of 5", sizes)
	}
	cancel()
	<-done
}
