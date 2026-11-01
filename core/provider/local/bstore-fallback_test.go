package provider_local

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
)

// leafStore reports every block as a known leaf, as a GC store does for a block
// with no outgoing refs. The bare in-memory store holds bytes without refs.
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

// newLeafStore returns an empty in-memory store of known leaves.
func newLeafStore() block.StoreOps {
	return &leafStore{StoreOps: block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)}
}

// ownedStore writes through a Volume under the ownership of a bucket, as the
// account's bucket handle does, so the Volume tracks each block's refs, queues
// cache fills and applies the write-back store's in-transaction markers.
type ownedStore struct {
	*volume_kvtx.Volume
}

// PutBlock writes a block owned by the bucket.
func (o ownedStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return o.PrepareOwnedBlock(ctx, "bucket", data, opts)
}

// PutBlockBatch writes a batch owned by the bucket.
func (o ownedStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	return o.PrepareOwnedBlockBatch(ctx, "bucket", entries)
}

// GetStoredBlock reads a block with the refs the Volume tracks.
func (o ownedStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block_gc.NewGCStoreOps(o.Volume, o.GetRefGraph()).GetStoredBlock(ctx, ref)
}

// newOwnedVolume opens a Volume over an in-memory key-value store and returns
// the store of its bucket and the Volume.
func newOwnedVolume(t *testing.T) (ownedStore, *volume_kvtx.Volume) {
	// Report failures at the caller.
	t.Helper()

	// Open the Volume over an in-memory store.
	vol, err := volume_kvtx.NewVolume(t.Context(), "test/fallback", store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Close the Volume when the test ends.
	t.Cleanup(func() { _ = vol.Close() })

	// Return the bucket store beside the Volume it lives in.
	return ownedStore{vol}, vol
}

// countingStore counts the block reads it serves, standing in for the SQL
// fallback, which tests cannot reach.
type countingStore struct {
	block.StoreOps
	// reads counts the block reads and existence checks.
	reads atomic.Int64
	// corrupt serves every block with its first byte flipped.
	corrupt atomic.Bool
}

// GetBlockExists counts the check.
func (c *countingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	c.reads.Add(1)
	return c.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch counts the check.
func (c *countingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	c.reads.Add(1)
	return c.StoreOps.GetBlockExistsBatch(ctx, refs)
}

// GetStoredBlock counts the read.
func (c *countingStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	c.reads.Add(1)
	stored, err := c.StoreOps.GetStoredBlock(ctx, ref)
	if stored != nil && c.corrupt.Load() {
		stored.Data = append([]byte{stored.Data[0] ^ 0xff}, stored.Data[1:]...)
	}
	return stored, err
}

// GetBlock counts the read.
func (c *countingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	c.reads.Add(1)
	return c.StoreOps.GetBlock(ctx, ref)
}

// promptPacks uploads short packs at once, where the pack store holds them for
// a linger time to fill, so a test need not wait out the linger.
type promptPacks struct {
	block.StoreOps
}

// GetBatchLimits returns the pack store's limits without the linger.
func (p promptPacks) GetBatchLimits() block.BatchLimits {
	limits := p.StoreOps.(block.BatchSizer).GetBatchLimits()
	limits.Linger = 0
	return limits
}

// trackedSet is a block tracker standing in for the volume's reference graph.
type trackedSet struct {
	mtx    sync.Mutex
	blocks map[string]bool
}

// track marks the block tracked.
func (t *trackedSet) track(ref *block.BlockRef) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.blocks[ref.MarshalString()] = true
}

// TrackedBlocks reports which refs were tracked.
func (t *trackedSet) TrackedBlocks(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Report each ref under the lock.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	out := make([]bool, len(refs))
	for i, ref := range refs {
		out[i] = t.blocks[ref.MarshalString()]
	}
	return out, nil
}

// fallbackTest is a placed store over a fake S3 pack store, with an SQL
// stand-in as its read fallback.
type fallbackTest struct {
	t     *testing.T
	ctx   context.Context
	local block.StoreOps
	wb    *block_store_writeback.Store
	// bucket is the fake S3 bucket behind the pack store.
	bucket *fakeS3
	srv    *httptest.Server
	packs  backendStore
	// sql is the SQL fallback stand-in.
	sql *countingStore
	// p is the store's placement state.
	p *placementState
	// tracked is the blocks the stand-in graph tracks.
	tracked *trackedSet
	// reads and copies are the store's read chains.
	reads, copies block.StoreOps
	// open opens the pack store; nil opens it on the fake bucket.
	open func() backendStore
}

// newFallbackTest builds a placed store whose reads fall back to the stand-in.
func newFallbackTest(t *testing.T) *fallbackTest {
	return newFallbackTestOn(t, nil)
}

// newFallbackTestOn builds a placed store whose pack store is open(), or the
// fake bucket's when open is nil.
func newFallbackTestOn(t *testing.T, open func() backendStore) *fallbackTest {
	// Open the write-back store over a Volume, which marks in the transaction
	// that stores each block.
	t.Helper()
	ctx := t.Context()
	local, vol := newOwnedVolume(t)
	markers, release, err := vol.AccessObjectStore(ctx, "test/markers", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	wb, err := block_store_writeback.NewStore(ctx, local, vol, markers, "test/markers", true)
	if err != nil {
		t.Fatal(err)
	}

	// Open the pack store on a fake S3 bucket.
	f := &fallbackTest{t: t, ctx: ctx, local: local, wb: wb, open: open, bucket: &fakeS3{objects: make(map[string][]byte)}}
	f.srv = httptest.NewServer(f.bucket)
	t.Cleanup(f.srv.Close)
	f.packs = f.openPacks()
	t.Cleanup(func() { f.packs.Close() })

	// Build the read chains, with an SQL stand-in holding its own blocks.
	f.sql = &countingStore{StoreOps: newLeafStore()}
	f.tracked = &trackedSet{blocks: make(map[string]bool)}
	f.p = &placementState{wb: wb, local: local, tracker: f.tracked}
	f.p.backend.Store(&account_settings.StorageBackend{Id: "packs"})
	recorded := &readRecorder{StoreOps: local, reads: &f.p.reads}
	f.reads, f.copies = f.p.readChains(
		recorded,
		local,
		func() block.StoreOps { return f.packs },
		func() block.StoreOps { return nil },
		f.sql,
	)
	return f
}

// openPacks opens a new pack store on the fake bucket, as a restart would.
func (f *fallbackTest) openPacks() backendStore {
	f.t.Helper()
	if f.open != nil {
		return f.open()
	}
	store, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), &account_settings.S3Location{
		Endpoint:   strings.TrimPrefix(f.srv.URL, "http://"),
		Region:     "us-east-1",
		Bucket:     "spaces",
		DisableSsl: true,
		KeepPacks:  true,
	}, "world", &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"})
	if err != nil {
		f.t.Fatal(err)
	}
	return store
}

// upload uploads every queued block to the pack store.
func (f *fallbackTest) upload() {
	// Run the upload until nothing is pending.
	f.t.Helper()
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- f.wb.Upload(ctx, promptPacks{f.packs}) }()
	for {
		status, changed := f.wb.GetStatus()
		if status.Pending == 0 {
			break
		}
		select {
		case <-changed:
		case <-time.After(20 * time.Second):
			f.t.Fatalf("upload did not finish: %+v", status)
		}
	}
	cancel()
	<-done
}

// evict drops every uploaded block from the local store.
func (f *fallbackTest) evict() block_store_writeback.EvictResult {
	f.t.Helper()
	result, err := f.wb.Evict(f.ctx, f.packs, 1<<30, nil, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

// held reports whether the local store holds the block.
func (f *fallbackTest) held(ref *block.BlockRef) bool {
	f.t.Helper()
	found, err := f.local.GetBlockExists(f.ctx, ref)
	if err != nil {
		f.t.Fatal(err)
	}
	return found
}

// waitHeld waits for a read's background fill to land the block locally.
func (f *fallbackTest) waitHeld(ref *block.BlockRef) {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !f.held(ref) {
		if time.Now().After(deadline) {
			f.t.Fatalf("block %s was not filled locally", ref.MarshalString())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitResident waits for background fills to enter the resident byte count.
func (f *fallbackTest) waitResident(want int64) {
	// Wait on the writeback status, including its eviction accounting.
	f.t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	for {
		status, changed := f.wb.GetStatus()
		if status.ResidentBytes() == want {
			return
		}
		select {
		case <-ctx.Done():
			f.t.Fatalf("resident bytes = %d, want %d: %+v", status.ResidentBytes(), want, status)
		case <-changed:
		}
	}
}

// read reads the block through the store's read chain.
func (f *fallbackTest) read(ref *block.BlockRef, want []byte) {
	f.t.Helper()
	data, found, err := f.reads.GetBlock(f.ctx, ref)
	if err != nil || !found || !bytes.Equal(data, want) {
		f.t.Fatalf("read %s = %q, %v, %v; want %q", ref.MarshalString(), data, found, err, want)
	}
}

// putSQL stores a block only the SQL stand-in holds.
func (f *fallbackTest) putSQL(data []byte) *block.BlockRef {
	f.t.Helper()
	ref, _, err := f.sql.PutBlock(f.ctx, data, &block.PutOpts{RefsKnown: true})
	if err != nil {
		f.t.Fatal(err)
	}
	return ref
}

// packHolds reports whether a pack store opened fresh on the bucket holds the
// block, so only uploaded packs can answer.
func (f *fallbackTest) packHolds(ref *block.BlockRef) bool {
	// Ask a pack store with no cached state.
	f.t.Helper()
	fresh := f.openPacks()
	defer fresh.Close()
	found, err := fresh.GetBlockExists(f.ctx, ref)
	if err != nil {
		f.t.Fatal(err)
	}
	return found
}

// setUnavailable makes the bucket fail every request, or serve again.
func (f *fakeS3) setUnavailable(unavailable bool) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.unavailable = unavailable
}

// status returns the write-back status.
func (f *fallbackTest) status() block_store_writeback.Status {
	status, _ := f.wb.GetStatus()
	return status
}

// waitFallback waits until the fallback counters satisfy cond.
func (f *fallbackTest) waitFallback(cond func(FallbackStatus) bool) FallbackStatus {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if status := f.p.fallbackFills.status(); cond(status) {
			return status
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("fallback status %+v did not settle", f.p.fallbackFills.status())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestReadFallbackFillsAndUploads reads blocks the pack store holds and blocks
// only the fallback holds, tracked and untracked. A pack hit refills the disk
// without a marker. A fallback hit lands on the disk with an upload marker that
// keeps eviction off it, eviction never asks the fallback, and after the upload
// the pack store holds the block and a read skips the fallback.
func TestReadFallbackFillsAndUploads(t *testing.T) {
	// Open the placed store.
	f := newFallbackTest(t)

	// Write a tracked and an untracked block, upload them, and evict them.
	trackedPack, untrackedPack := []byte("tracked pack block"), []byte("untracked pack block")
	trackedPackRef, _, err := f.wb.PutBlock(f.ctx, trackedPack, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	untrackedPackRef, _, err := f.wb.PutBlock(f.ctx, untrackedPack, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	f.tracked.track(trackedPackRef)
	f.upload()
	if result := f.evict(); result.Evicted != 2 {
		t.Fatalf("evict = %+v, want both blocks evicted", result)
	}

	// Both read from the pack store and refill the disk without a marker.
	f.read(trackedPackRef, trackedPack)
	f.read(untrackedPackRef, untrackedPack)
	f.waitHeld(trackedPackRef)
	f.waitHeld(untrackedPackRef)
	if status := f.status(); status.Pending != 0 {
		t.Fatalf("pack hits queued %d uploads", status.Pending)
	}
	if n := f.sql.reads.Load(); n != 0 {
		t.Fatalf("pack store reads touched the fallback %d times", n)
	}

	// Blocks only the fallback holds read from it and land on the disk marked.
	trackedSQL, untrackedSQL := []byte("tracked sql block"), []byte("untracked sql block")
	trackedSQLRef, untrackedSQLRef := f.putSQL(trackedSQL), f.putSQL(untrackedSQL)
	f.tracked.track(trackedSQLRef)
	f.read(trackedSQLRef, trackedSQL)
	f.read(untrackedSQLRef, untrackedSQL)

	// Wait for the fills to land and count them.
	f.waitHeld(trackedSQLRef)
	f.waitHeld(untrackedSQLRef)
	fallback := f.waitFallback(func(s FallbackStatus) bool { return s.Filled == 2 })
	if fallback.Hits != 2 || fallback.FillFailed != 0 || fallback.FillSkipped != 0 {
		t.Fatalf("fallback status = %+v, want two clean fills", fallback)
	}
	if status := f.status(); status.Pending != 2 {
		t.Fatalf("pending = %d, want the two fallback blocks", status.Pending)
	}

	// Eviction leaves them on the disk and never asks the fallback.
	reads := f.sql.reads.Load()
	if result := f.evict(); !f.held(trackedSQLRef) || !f.held(untrackedSQLRef) {
		t.Fatalf("evict = %+v, want the marked blocks kept", result)
	}
	if n := f.sql.reads.Load(); n != reads {
		t.Fatalf("eviction asked the fallback %d times", n-reads)
	}
	if f.packHolds(trackedSQLRef) || f.packHolds(untrackedSQLRef) {
		t.Fatal("pack store holds a fallback block before its upload")
	}

	// The upload moves them to a pack, after which they evict like any block.
	f.upload()
	if !f.packHolds(trackedSQLRef) || !f.packHolds(untrackedSQLRef) {
		t.Fatal("pack store lacks a fallback block after the upload")
	}
	if result := f.evict(); result.Evicted != 2 || f.held(trackedSQLRef) || f.held(untrackedSQLRef) {
		t.Fatalf("evict = %+v, want the uploaded blocks evicted", result)
	}

	// Later reads come from the pack store, and the fallback keeps its rows.
	reads = f.sql.reads.Load()
	f.read(trackedSQLRef, trackedSQL)
	f.read(untrackedSQLRef, untrackedSQL)
	if n := f.sql.reads.Load(); n != reads {
		t.Fatalf("reads after the upload touched the fallback %d times", n-reads)
	}
	if found, err := f.sql.StoreOps.GetBlockExists(f.ctx, trackedSQLRef); err != nil || !found {
		t.Fatalf("fallback row = %v, %v; want it kept", found, err)
	}

	// A copy reads the same sources and stores nothing.
	copied := []byte("copy only")
	copiedRef := f.putSQL(copied)
	data, found, err := f.copies.GetBlock(f.ctx, copiedRef)
	if err != nil || !found || !bytes.Equal(data, copied) || f.held(copiedRef) {
		t.Fatalf("copy read = %q, %v, %v, held %v", data, found, err, f.held(copiedRef))
	}
}

// TestReadFallbackFailures checks that a backend error falls through to the
// fallback without counting as absence, that corrupt fallback bytes neither
// serve nor fill, and that a fill with no free slot is counted and retried.
func TestReadFallbackFailures(t *testing.T) {
	// Open the placed store.
	f := newFallbackTest(t)

	// Upload and evict a block both the pack store and the fallback hold.
	both := []byte("held by both")
	bothRef, _, err := f.wb.PutBlock(f.ctx, both, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.sql.PutBlock(f.ctx, both, &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	f.upload()
	if result := f.evict(); result.Evicted != 1 || f.held(bothRef) {
		t.Fatalf("evict = %+v, held %v; want the block evicted", result, f.held(bothRef))
	}

	// An unreachable bucket falls through to the fallback, which serves the
	// block and stores it with a marker for the pack store to take later. A
	// pack store opened fresh holds no cached packs, so the read must go out.
	f.packs.Close()
	f.packs = f.openPacks()
	f.bucket.setUnavailable(true)
	f.read(bothRef, both)
	f.waitFallback(func(s FallbackStatus) bool { return s.Filled == 1 })
	if status := f.p.fallbackFills.status(); status.PackErrors == 0 {
		t.Fatalf("fallback status = %+v, want the bucket error counted", status)
	}

	// A block nobody holds is an error while the bucket is down, never a miss,
	// and queues nothing.
	absent, err := block.BuildBlockRef([]byte("absent everywhere"), nil)
	if err != nil {
		t.Fatal(err)
	}
	pending := f.status().Pending
	if data, found, err := f.reads.GetBlock(f.ctx, absent); err == nil || found {
		t.Fatalf("read of an absent block = %q, %v, %v; want an error", data, found, err)
	}
	if status := f.status(); status.Pending != pending {
		t.Fatalf("pending = %d, want %d", status.Pending, pending)
	}

	// With the bucket back, the same block is a clean miss.
	f.bucket.setUnavailable(false)
	if data, found, err := f.reads.GetBlock(f.ctx, absent); err != nil || found {
		t.Fatalf("read of an absent block = %q, %v, %v; want a miss", data, found, err)
	}

	// Corrupt fallback bytes fail the read and fill nothing.
	corrupt := f.putSQL([]byte("corrupted in the table"))
	f.sql.corrupt.Store(true)
	if data, found, err := f.reads.GetBlock(f.ctx, corrupt); err == nil || found {
		t.Fatalf("corrupt read = %q, %v, %v; want an error", data, found, err)
	}
	f.sql.corrupt.Store(false)
	if status := f.p.fallbackFills.status(); status.FillFailed != 1 || f.held(corrupt) {
		t.Fatalf("fallback status = %+v, held %v; want one failure and no fill", status, f.held(corrupt))
	}

	// A hit with every fill slot busy serves, counts the skip, and fills on
	// the next read.
	busy := []byte("read while the slots are busy")
	busyRef := f.putSQL(busy)
	f.p.fallbackFills.running.Store(fallbackFillSlots)
	f.read(busyRef, busy)
	f.p.fallbackFills.running.Store(0)
	if status := f.p.fallbackFills.status(); status.FillSkipped != 1 || f.held(busyRef) {
		t.Fatalf("fallback status = %+v, held %v; want one skip and no fill", status, f.held(busyRef))
	}
	f.read(busyRef, busy)
	f.waitHeld(busyRef)
}

// TestReadFallbackFillRacesEviction fills blocks from the fallback while
// eviction passes run, and requires every read to succeed and every block to
// reach the pack store.
func TestReadFallbackFillRacesEviction(t *testing.T) {
	// Open the placed store.
	f := newFallbackTest(t)

	// Hold blocks in the fallback only.
	const n = 20
	datas := make(map[*block.BlockRef][]byte, n)
	for i := range n {
		data := []byte{'b', byte(i), 'x'}
		datas[f.putSQL(data)] = data
	}

	// Keep evicting while the blocks read.
	stop := make(chan struct{})
	var evicting sync.WaitGroup
	evicting.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = f.wb.Evict(f.ctx, f.packs, 1<<30, nil, nil)
			}
		}
	})
	for round := range 3 {
		for ref, data := range datas {
			f.read(ref, data)
		}
		if round == 1 {
			f.upload()
		}
	}
	close(stop)
	evicting.Wait()

	// Wait for each fill to land or the block to be packed, then upload. Every
	// block reaches the pack store and still reads.
	for ref := range datas {
		for deadline := time.Now().Add(10 * time.Second); !f.held(ref) && !f.packHolds(ref); {
			if time.Now().After(deadline) {
				t.Fatalf("block %s neither filled nor packed", ref.MarshalString())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	f.upload()
	for ref, data := range datas {
		if !f.packHolds(ref) {
			t.Fatalf("pack store lacks %s", ref.MarshalString())
		}
		f.read(ref, data)
	}
}
