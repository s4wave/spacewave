package block_store_writeback

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_client "github.com/s4wave/spacewave/db/block/rpc/client"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_testing "github.com/s4wave/spacewave/db/block/store/test"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/hash"
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
	checkUploadedBefore(t, s, time.Now(), false)
	if err := s.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}

	// Two distinct writes and one repeat queue two blocks.
	start := time.Now()
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

	// The report holds a head back until the blocks before it uploaded.
	written := time.Now()
	checkUploadedBefore(t, s, start, true)
	checkUploadedBefore(t, s, written, false)

	// A reopened store recounts the markers and drains them.
	s = env.newStore(t, true)
	remote := newInmemBlockStore()
	drain(t, s, remote)
	if status, _ := s.GetStatus(); status.Uploaded != 2 || status.UploadedBytes != 9 || status.UploadReads < 1 {
		t.Fatalf("drain counted %d blocks, %d bytes, %d reads", status.Uploaded, status.UploadedBytes, status.UploadReads)
	}

	// The drained queue proves any head, until queueing stops.
	checkUploadedBefore(t, s, written, true)

	// Require the remote store to contain both queued blocks.
	for _, ref := range refs[:2] {
		found, err := remote.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("remote missing %s: %v", ref.MarshalString(), err)
		}
	}
}

// checkUploadedBefore requires the store's report for at.
func checkUploadedBefore(t *testing.T, s *Store, at time.Time, expected bool) {
	t.Helper()
	uploaded, err := s.UploadedBefore(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	if uploaded != expected {
		t.Fatalf("uploaded before %v: %v, expected %v", at, uploaded, expected)
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
	head, err := s.scanHead(ctx, batchLimitsOf(nil), nil)
	if err != nil || len(head.entries) != 1 {
		t.Fatalf("scan = %d blocks, %v", len(head.entries), err)
	}
	batch := head.entries
	remote := newInmemBlockStore()
	if err := s.uploadChunk(ctx, remote, batch); err != nil || !batch[0].dropped {
		t.Fatalf("upload chunk = %v, dropped %v", err, batch[0].dropped)
	}

	// A rescan of the unchanged queue reuses the parsed entry, not its mark.
	same, err := s.scanHead(ctx, batchLimitsOf(nil), head)
	if err != nil || len(same.entries) != 1 || same.entries[0].ref != batch[0].ref || same.entries[0].dropped {
		t.Fatalf("rescan of the unchanged queue = %d blocks, %v", len(same.entries), err)
	}

	// The block is written again before the marker clears.
	if _, _, err := s.PutBlock(ctx, []byte("swept"), leafOpts); err != nil {
		t.Fatal(err)
	}

	// A scan reusing the earlier head reads the rewrite, not the dropped entry.
	again, err := s.scanHead(ctx, batchLimitsOf(nil), head)
	if err != nil || len(again.entries) != 1 {
		t.Fatalf("rescan = %d blocks, %v", len(again.entries), err)
	}
	if got := again.entries[0]; got.dropped || bytes.Equal(got.value, batch[0].value) {
		t.Fatalf("rescan reused the dropped entry: dropped %v", got.dropped)
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

// recordingStore records the batch entries it stores.
type recordingStore struct {
	block.StoreOps
	got []*block.PutBatchEntry
}

// PutBlockBatch records the entries and stores them.
func (r *recordingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	r.got = append(r.got, entries...)
	return r.StoreOps.PutBlockBatch(ctx, entries)
}

// TestUploadUntrackedBlock checks that a block held without its refs uploads
// with its refs unknown, so the remote does not keep it as a leaf.
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

	// The upload drains, and the remote receives the block marked unknown.
	remote := &recordingStore{StoreOps: newInmemBlockStore()}
	drain(t, s, remote)
	if len(remote.got) != 1 || !remote.got[0].Ref.EqualsRef(ref) || remote.got[0].RefsKnown {
		t.Fatalf("remote got %+v, want the block with its refs unknown", remote.got)
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

// waitStatus waits until the status satisfies done, and returns it.
func waitStatus(t *testing.T, s *Store, done func(Status) bool) Status {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		status, changed := s.GetStatus()
		if done(status) {
			return status
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
	// The linger clock starts at each block's write time, so start the test's
	// clock before the first write.
	start := time.Now()
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

	// The drained queue raised its scan floor past every entry. A new write
	// and a backfilled block dated before every cleared entry lower it, so
	// both still upload.
	ref, _, err := s.PutBlock(ctx, []byte("after the drain"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := s.PutBlock(ctx, []byte("backfilled"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, func(st Status) bool { return st.Pending == 0 })
	if err := s.queue(ctx, []Mark{{Hash: old.GetHash(), Size: 10}}); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, func(st Status) bool { return st.Pending == 0 })
	for _, r := range []*block.BlockRef{ref, old} {
		if env.isMarked(t, r) {
			t.Fatalf("block %s still queued after the drain", r.MarshalString())
		}
	}
	if sizes := remote.getBatches(); !slices.Equal(sizes, []int{10, 10, 10, 5, 2, 1}) {
		t.Fatalf("batches %v, want the new write and the backfill uploaded", sizes)
	}
	cancel()
	<-done
}

// TestRPCWritesRetainUploadMarkers checks that writes through the real block
// RPC persist their markers before success and drain after the store reopens.
func TestRPCWritesRetainUploadMarkers(t *testing.T) {
	// Serve blocks over RPC under an independent server connection context.
	ctx := t.Context()
	env := newTestEnv(t)
	mux := srpc.NewMux()
	if err := mux.Register(block_rpc.NewSRPCBlockStoreHandler(
		block_rpc_server.NewBlockStore(ownedStore{env.vol}), "")); err != nil {
		t.Fatal(err)
	}

	// Connect the client without carrying its write contexts into the server.
	open := srpc.NewServerPipe(srpc.NewServer(mux))
	client := srpc.NewClient(func(_ context.Context, handler srpc.PacketDataHandler, closed srpc.CloseHandler) (srpc.PacketWriter, error) {
		return open(ctx, handler, closed)
	})
	local := block_rpc_client.NewBlockStore(block_rpc.NewSRPCBlockStoreClient(client), 0, false)

	// Open the upload queue over the remote block store.
	s, err := NewStore(ctx, local, env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}

	// Queue both unary and batch writes through the proxy.
	first, _, err := s.PutBlock(ctx, []byte("unary proxy block"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("batch proxy block")
	second, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: second, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*block.BlockRef{first, second} {
		if !env.isMarked(t, ref) {
			t.Fatalf("acknowledged block %s has no marker", ref.MarshalString())
		}
	}

	// Reopen from durable markers and upload both values through the proxy.
	s, err = NewStore(ctx, local, env.vol, env.markers(t), testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); status.Pending != 2 {
		t.Fatalf("reopened pending=%d, want both proxy writes", status.Pending)
	}
	remote := newInmemBlockStore()
	drain(t, s, remote)
	for _, ref := range []*block.BlockRef{first, second} {
		if found, err := remote.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("uploaded block %s exists=%v err=%v", ref.MarshalString(), found, err)
		}
	}
}

// locatingStore is a remote whose index answers from located, or fails.
type locatingStore struct {
	block.StoreOps
	located   map[string]bool
	err       error
	existsErr error
	puts      int
	checks    int
	reclaims  bool
}

// GetBlockExistsBatch counts the batched checks, or fails with existsErr.
func (l *locatingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	l.checks++
	if l.existsErr != nil {
		return nil, l.existsErr
	}
	return l.StoreOps.GetBlockExistsBatch(ctx, refs)
}

// LocateBlocks answers from located.
func (l *locatingStore) LocateBlocks(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	if l.err != nil {
		return nil, l.err
	}
	out := make([]bool, len(refs))
	for i, ref := range refs {
		out[i] = l.located[ref.MarshalString()]
	}
	return out, nil
}

// KeepsBlocks reports the store's setting.
func (l *locatingStore) KeepsBlocks() bool { return !l.reclaims }

// PutBlockBatch counts the blocks written.
func (l *locatingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	l.puts += len(entries)
	return l.StoreOps.PutBlockBatch(ctx, entries)
}

// TestUploadSkipsLocatedBlocks drains blocks another writer uploaded without
// reading them, and tries to lose one: a block the index does not name, one
// it names that the remote lacks, a lookup or a confirm that fails, and the
// switch turned off all still upload.
func TestUploadSkipsLocatedBlocks(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	put := func(data string) *block.BlockRef {
		ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	remote := &locatingStore{StoreOps: newInmemBlockStore(), located: map[string]bool{}}

	// With the switch off, a located block uploads as before.
	var on atomic.Bool
	s.SetLocateUploaded(&on)
	off := put("switch off")
	remote.located[off.MarshalString()] = true
	drain(t, s, remote)
	if remote.puts != 1 {
		t.Fatalf("switch off wrote %d blocks", remote.puts)
	}
	on.Store(true)

	// A located block the remote holds clears unread, and one beside it
	// still uploads, with one batched confirm for the chunk.
	remote.puts, remote.checks = 0, 0
	held, missing := put("held"), put("missing")
	if _, _, err := remote.PutBlock(ctx, []byte("held"), leafOpts); err != nil {
		t.Fatal(err)
	}
	remote.located[held.MarshalString()] = true
	drain(t, s, remote)
	if remote.puts != 1 {
		t.Fatalf("wrote %d blocks, want only the unlocated one", remote.puts)
	}
	if remote.checks != 1 {
		t.Fatalf("confirmed in %d checks, want one batch", remote.checks)
	}
	if found, err := remote.GetBlockExists(ctx, missing); err != nil || !found {
		t.Fatalf("unlocated block not uploaded: %v", err)
	}
	if env.isMarked(t, held) || env.isMarked(t, missing) {
		t.Fatal("a marker stayed after the drain")
	}

	// A block the index names that the remote lacks, as a row naming a pack
	// without the block does, uploads.
	remote.puts = 0
	wrongRow := put("wrong row")
	remote.located[wrongRow.MarshalString()] = true
	drain(t, s, remote)
	if remote.puts != 1 {
		t.Fatalf("wrong row wrote %d blocks, want the unconfirmed one", remote.puts)
	}
	if found, err := remote.GetBlockExists(ctx, wrongRow); err != nil || !found {
		t.Fatalf("unconfirmed block not uploaded: %v", err)
	}

	// A failed confirm uploads, though the remote holds the block.
	remote.puts = 0
	unconfirmed := put("confirm failed")
	if _, _, err := remote.PutBlock(ctx, []byte("confirm failed"), leafOpts); err != nil {
		t.Fatal(err)
	}
	remote.located[unconfirmed.MarshalString()] = true
	remote.existsErr = errors.New("packs unavailable")
	drain(t, s, remote)
	remote.existsErr = nil
	if remote.puts != 1 {
		t.Fatalf("failed confirm wrote %d blocks", remote.puts)
	}

	// A failed lookup uploads everything, though the index would have said yes.
	remote.puts = 0
	failed := put("lookup failed")
	remote.located[failed.MarshalString()] = true
	remote.err = errors.New("index unavailable")
	drain(t, s, remote)
	if remote.puts != 1 {
		t.Fatalf("failed lookup wrote %d blocks", remote.puts)
	}

	// A remote that reclaims needs the upload, located or not.
	remote.puts, remote.err, remote.reclaims = 0, nil, true
	reclaimed := put("reclaiming remote")
	remote.located[reclaimed.MarshalString()] = true
	drain(t, s, remote)
	if remote.puts != 1 {
		t.Fatalf("reclaiming remote wrote %d blocks", remote.puts)
	}

	// A lookup answering for the wrong number of blocks is not trusted either.
	if got := s.locatedBlocks(ctx, shortLocator{remote}, make([]pendingBlock, 2)); got != nil {
		t.Fatalf("short answer trusted: %v", got)
	}
}

// TestClearLocatedPassesTheBacklog clears a block another writer uploaded while
// an older block waits at the head of the queue, then tries to lose either: a
// block the index does not name, a failed lookup, the switch off and a remote
// that reclaims all leave their entries, the head still reads as not uploaded,
// and eviction drops only the block the remote holds.
func TestClearLocatedPassesTheBacklog(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	put := func(data string) *block.BlockRef {
		ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	clearRefs := func(remote block.StoreOps, refs ...*block.BlockRef) {
		hashes := make([]*hash.Hash, len(refs))
		for i, ref := range refs {
			hashes[i] = ref.GetHash()
		}
		if err := s.ClearLocated(ctx, remote, hashes); err != nil {
			t.Fatal(err)
		}
	}
	pending := func(want int) {
		t.Helper()
		if status, _ := s.GetStatus(); status.Pending != want {
			t.Fatalf("pending=%d, want %d", status.Pending, want)
		}
	}

	// The head is an old block the remote lacks; the rest are newer.
	remote := &locatingStore{StoreOps: newInmemBlockStore(), located: map[string]bool{}}
	var on atomic.Bool
	on.Store(true)
	s.SetLocateUploaded(&on)
	head := put("backlog head")
	mid := time.Now()
	pushed, unnamed := put("pushed"), put("in the report, not the index")
	if _, _, err := remote.PutBlock(ctx, []byte("pushed"), leafOpts); err != nil {
		t.Fatal(err)
	}
	remote.located[pushed.MarshalString()] = true

	// Nothing clears with the switch off, on a reclaiming remote, or when the
	// lookup fails, though the index would say yes. A failed lookup says so,
	// so a caller answering for the clear knows it did not finish.
	on.Store(false)
	clearRefs(remote, pushed)
	on.Store(true)
	remote.reclaims = true
	clearRefs(remote, pushed)
	remote.reclaims, remote.err = false, errors.New("index unavailable")
	if err := s.ClearLocated(ctx, remote, []*hash.Hash{pushed.GetHash()}); err == nil {
		t.Fatal("a clear whose lookup failed returned nil")
	}
	remote.err = nil
	pending(3)

	// Only the block the index names clears, past the head. A block with no
	// entry changes nothing, and neither does clearing twice.
	clearRefs(remote, head, unnamed)
	pending(3)
	absent, err := hash.Sum(hash.RecommendedHashType, []byte("never written"))
	if err != nil {
		t.Fatal(err)
	}
	clearRefs(remote, pushed, pushed, block.NewBlockRef(absent))
	pending(2)
	if env.isMarked(t, pushed) || !env.isMarked(t, head) || !env.isMarked(t, unnamed) {
		t.Fatal("cleared the wrong markers")
	}

	// The head still says the remote may lack what was written before mid.
	if done, err := s.UploadedBefore(ctx, mid); err != nil || done {
		t.Fatalf("uploaded before mid=%v err=%v with the head queued", done, err)
	}

	// A clear says nothing about the upload's health.
	failing := errors.New("upload failing")
	s.SetError(failing)
	clearRefs(remote, pushed)
	if status, _ := s.GetStatus(); status.Err != failing {
		t.Fatalf("status error = %v after a clear", status.Err)
	}
	s.SetError(nil)

	// Eviction drops the pushed block, whose entry the clear queued.
	if result, err := s.Evict(ctx, remote, 1<<30, nil, nil); err != nil || result.Evicted != 1 {
		t.Fatalf("evict = %+v, %v", result, err)
	}
	if held, err := s.values.GetBlockExists(ctx, pushed); err != nil || held {
		t.Fatalf("pushed block held=%v err=%v after eviction", held, err)
	}

	// The upload takes the rest, and not the pushed block, which no longer
	// queues.
	remote.puts = 0
	drain(t, s, remote)
	if remote.puts != 2 {
		t.Fatalf("drain wrote %d blocks, want the two still queued", remote.puts)
	}
	pending(0)
}

// slowMarkers delays every read transaction while armed, so the count of the
// queue after an open spans many clears.
type slowMarkers struct {
	kvtx.Store
	armed atomic.Bool
}

// NewTransaction delays an armed read.
func (g *slowMarkers) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write && g.armed.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	return g.Store.NewTransaction(ctx, write)
}

// countQueue counts the queue's entries and their bytes in one snapshot.
func countQueue(t *testing.T, markers kvtx.Store) (int, int64) {
	t.Helper()
	var n int
	var size int64
	err := kvtx.RunTransaction(t.Context(), false, func(ctx context.Context) (kvtx.Tx, error) { return markers.NewTransaction(ctx, false) }, func(ctx context.Context, tx kvtx.Tx) error {
		n, size = 0, 0
		return scanQueue(ctx, tx, nil, func(item queueItem) error {
			n++
			size += item.size()
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return n, size
}

// readCount reads the queue's kept count.
func readCount(t *testing.T, markers kvtx.Store) queueCount {
	t.Helper()
	var count queueCount
	err := kvtx.RunTransaction(t.Context(), false, func(ctx context.Context) (kvtx.Tx, error) { return markers.NewTransaction(ctx, false) }, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		count, _, err = readQueueCount(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// TestQueueCountWalk walks the queue of a store that keeps no count, with the
// upload, clears and writes running from the open on, reopens mid-walk with
// the first store still walking, crashes it and a write mid-transaction, and
// resumes. The walk must resume from its cursor, and once it
// ends the kept count and the status must equal a count of the queue: a
// write or clear that adjusted the count for a key the walk had yet to reach
// would leave it high or low.
func TestQueueCountWalk(t *testing.T) {
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)

	// Queue many walk slices of entries, then reopen with the switch that
	// drops the count, as on a store that never kept one, and slow reads.
	total := 40 * walkSliceLimit
	entries := make([]*block.PutBatchEntry, total)
	for i := range total {
		data := []byte(strconv.Itoa(i))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries[i] = &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true}
	}
	if _, err := s.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); status.Counting || status.Pending != total {
		t.Fatalf("status before the reopen = %+v, want %d counted", status, total)
	}
	g := &slowMarkers{Store: env.markers(t)}
	g.armed.Store(true)
	openCtx, crash := context.WithCancel(ctx)
	SetRecountAtOpen(true)
	s, err := NewStore(openCtx, ownedStore{env.vol}, env.vol, g, testMarkersID, true)
	SetRecountAtOpen(false)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); !status.Counting || status.Pending != 0 || status.Oldest.IsZero() {
		t.Fatalf("status after the open = %+v, want counting from zero", status)
	}

	// The upload starts at once and uploads from the head while the walk
	// runs.
	uploadCtx, stopUpload := context.WithCancel(openCtx)
	uploaded := make(chan error, 1)
	go func() { uploaded <- s.Upload(uploadCtx, newInmemBlockStore()) }()
	waitStatus(t, s, func(st Status) bool { return st.Uploaded != 0 })
	stopUpload()
	<-uploaded

	// Clear a spread of blocks behind and past the cursor, with a write
	// queued after the open among them.
	put := func(s *Store, data string) *block.BlockRef {
		ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	clearSpread := func(s *Store, from int, extra *block.BlockRef) {
		located := map[string]bool{extra.MarshalString(): true}
		hashes := []*hash.Hash{extra.GetHash()}
		for i := from; i < total; i += 997 {
			located[entries[i].Ref.MarshalString()] = true
			hashes = append(hashes, entries[i].Ref.GetHash())
		}
		remote := &locatingStore{StoreOps: newInmemBlockStore(), located: located}
		var on atomic.Bool
		on.Store(true)
		s.SetLocateUploaded(&on)
		for chunk := range slices.Chunk(hashes, 5) {
			if err := s.ClearLocated(ctx, remote, chunk); err != nil {
				t.Fatal(err)
			}
		}
	}
	clearSpread(s, 0, put(s, "written after the first open"))

	// A write that fails mid-transaction queues and counts nothing.
	err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) { return g.NewTransaction(ctx, true) }, func(ctx context.Context, tx kvtx.Tx) error {
		h, err := hash.Sum(hash.RecommendedHashType, []byte("discarded"))
		if err != nil {
			return err
		}
		if _, err := queueMarks(ctx, tx, []Mark{{Hash: h, Size: 9, At: time.Unix(0, 0)}, {Hash: h, Size: 9, At: time.Now()}}); err != nil {
			return err
		}
		return errors.New("crash mid-write")
	})
	if err == nil {
		t.Fatal("the discarded write committed")
	}

	// Reopen mid-walk, as a remount does, and crash the first store after:
	// the walk resumes from its cursor, and the first store's slices count
	// nothing into the count the second read.
	status := waitStatus(t, s, func(st Status) bool { return st.Pending > 3*walkSliceLimit })
	if !status.Counting {
		t.Fatal("the walk ended before the crash; slow it further")
	}
	s, err = NewStore(ctx, ownedStore{env.vol}, env.vol, g, testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	crash()
	if status, _ := s.GetStatus(); !status.Counting || status.Pending < 3*walkSliceLimit {
		t.Fatalf("status after the crash = %+v, want the walk resumed past %d", status, 3*walkSliceLimit)
	}

	// Writes and clears run on while the resumed walk ends.
	clearSpread(s, 500, put(s, "written after the crash"))
	put(s, "written again after the crash")
	status = waitStatus(t, s, func(st Status) bool { return !st.Counting })
	g.armed.Store(false)

	// The kept count and the status equal the queue's count.
	n, size := countQueue(t, env.markers(t))
	count := readCount(t, env.markers(t))
	if !count.done || count.n != n || count.bytes != size {
		t.Fatalf("kept count = %+v, queue holds %d, %d", count, n, size)
	}
	status, _ = s.GetStatus()
	if status.Pending != n || status.PendingBytes != size {
		t.Fatalf("status = %d entries, %d bytes; queue holds %d, %d", status.Pending, status.PendingBytes, n, size)
	}

	// The upload drains the rest to an empty count.
	drain(t, s, newInmemBlockStore())
	if count := readCount(t, env.markers(t)); !count.done || count.n != 0 || count.bytes != 0 {
		t.Fatalf("kept count after the drain = %+v", count)
	}
	if status, _ := s.GetStatus(); status.Pending != 0 || status.PendingBytes != 0 || !status.Oldest.IsZero() {
		t.Fatalf("status after the drain = %+v", status)
	}
}

// shortLocator answers with one result, whatever it is asked.
type shortLocator struct{ block.StoreOps }

// LocateBlocks answers for one block.
func (shortLocator) LocateBlocks(context.Context, []*block.BlockRef) ([]bool, error) {
	return []bool{true}, nil
}

// KeepsBlocks says the remote keeps what it holds.
func (shortLocator) KeepsBlocks() bool { return true }
