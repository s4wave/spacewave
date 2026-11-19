package provider_local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// fakeS3 is an in-memory S3-compatible bucket serving object PUT, GET, HEAD,
// DELETE, and ListObjectsV2 on path-style URLs. Like Cloudflare R2, it does
// not list object versions. It ignores signatures.
type fakeS3 struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// objects holds each object by key.
	objects map[string][]byte
	// unavailable fails every request with a server error.
	unavailable bool
}

// ServeHTTP handles one request against the single bucket.
func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Hold the bucket for the request, failing it while unavailable.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.unavailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	// Answer listings, then the object request.
	_, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if r.URL.Query().Has("versions") {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	if key == "" {
		f.list(w, r.URL.Query().Get("prefix"))
		return
	}
	data, ok := f.objects[key]
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.objects[key] = body
	case http.MethodGet, http.MethodHead:
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, key, time.Time{}, bytes.NewReader(data))
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

// list writes a single-page ListObjectsV2 result for prefix.
func (f *fakeS3) list(w http.ResponseWriter, prefix string) {
	// List the fake bucket objects as an S3 listing.
	var b strings.Builder
	b.WriteString("<ListBucketResult><IsTruncated>false</IsTruncated>")
	for key, data := range f.objects {
		if strings.HasPrefix(key, prefix) {
			b.WriteString("<Contents><Key>")
			b.WriteString(key)
			b.WriteString("</Key><Size>")
			b.WriteString(strconv.Itoa(len(data)))
			b.WriteString("</Size></Contents>")
		}
	}
	b.WriteString("</ListBucketResult>")
	_, _ = io.WriteString(w, b.String())
}

// count returns the number of stored objects.
func (f *fakeS3) count() int {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return len(f.objects)
}

// placedStoreTest is a Space whose root and child blocks are uploaded to a
// fake S3 backend.
type placedStoreTest struct {
	acc       *ProviderAccount
	bucket    *fakeS3
	backendID string
	soID      string
	tkr       *bstoreTracker
	blocks    map[*block.BlockRef][]byte
	rootRef   *block.BlockRef
	childRef  *block.BlockRef
	record    func(MoveProgress) error
	phases    []MovePhase
}

// setupPlacedStore writes a root referencing a child into a new Space, moves
// the Space onto a fake S3 backend, and waits for both uploads.
func setupPlacedStore(ctx context.Context, t *testing.T) *placedStoreTest {
	// Start a local account and release it when the test returns.
	t.Helper()
	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t)
	t.Cleanup(release)

	// Serve a fake S3 bucket for the placed store.
	bucket := &fakeS3{objects: make(map[string][]byte)}
	srv := httptest.NewServer(bucket)
	t.Cleanup(srv.Close)

	// Add the fake backend and require an empty usage check.
	backendID, check, err := acc.AddStorageBackend(ctx, "fake", &account_settings.S3Location{
		Endpoint:     strings.TrimPrefix(srv.URL, "http://"),
		Region:       "us-east-1",
		Bucket:       "spaces",
		ObjectPrefix: "p/",
		DisableSsl:   true,
	}, &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if backendID == "" {
		t.Fatalf("check failed: %v", check)
	}
	if check.GetUsage().GetObjects() != 0 {
		t.Fatalf("new prefix holds %d objects", check.GetUsage().GetObjects())
	}

	// Create a space and wait for its block store.
	soRef, err := acc.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	soID := soRef.GetProviderResourceRef().GetId()
	blockStoreID := acc.lookupSharedObjectBlockStoreID(soID)
	tkrRef, tkr, _ := acc.bstores.AddKeyRef(blockStoreID)
	t.Cleanup(tkrRef.Release)
	bs, err := tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write a root referencing a child and reference the root from the
	// store's bucket node, as the World engine does.
	child := []byte("child block")
	childRef, _, err := bs.placement.local.PutBlock(ctx, child, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	root := []byte("root block")
	rootRef, _, err := bs.placement.local.PutBlock(ctx, root, &block.PutOpts{Refs: []*block.BlockRef{childRef}, RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	bucketID := BlockStoreBucketID(acc.GetProviderID(), acc.GetAccountID(), blockStoreID)
	roots := acc.GetVolume().(interface {
		SetBucketRoots(context.Context, string, []*block.PutBatchEntry, []block.NamedRoot) error
	})
	if err := roots.SetBucketRoots(ctx, bucketID, nil, []block.NamedRoot{{Name: "world", Ref: rootRef}}); err != nil {
		t.Fatal(err)
	}

	// Record move progress and move the space onto the fake backend.
	p := &placedStoreTest{
		acc:       acc,
		bucket:    bucket,
		backendID: backendID,
		soID:      soID,
		tkr:       tkr,
		blocks:    map[*block.BlockRef][]byte{rootRef: root, childRef: child},
		rootRef:   rootRef,
		childRef:  childRef,
	}
	p.record = func(progress MoveProgress) error {
		if !slices.Contains(p.phases, progress.Phase) {
			p.phases = append(p.phases, progress.Phase)
		}
		return nil
	}

	// Move onto the backend: the backfill uploads both blocks.
	if err := acc.MoveSpaceStorage(ctx, soID, backendID, p.record); err != nil {
		t.Fatal(err)
	}
	if n := bucket.count(); n != 2 {
		t.Fatalf("bucket holds %d objects, want 2", n)
	}
	return p
}

// prepareColdMove writes a separate pack set and attaches a real cold reader.
func (p *placedStoreTest) prepareColdMove(ctx context.Context, t *testing.T, bs *BlockStore, gate *coldMoveGate) (*block_store_s3.ColdPackStore, []*block.BlockRef) {
	// Pack the root and child with the same refs the local store recorded.
	t.Helper()
	refs := []*block.BlockRef{p.rootRef, p.childRef}
	dataByRef := map[*block.BlockRef][]byte{p.rootRef: p.blocks[p.rootRef], p.childRef: p.blocks[p.childRef]}
	if gate != nil {
		for i := range cap(gate.started) {
			data := []byte("foreground block " + strconv.Itoa(i))
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			refs = append(refs, ref)
			dataByRef[ref] = data
		}
	}

	// Write disjoint extents so foreground readers cannot share a flight.
	var buf bytes.Buffer
	i := 0
	res, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		// Supply the World edge only on the root.
		if i == len(refs) {
			return nil, nil, nil
		}
		ref := refs[i]
		i++
		stored := &block.StoredBlock{Data: dataByRef[ref], RefsKnown: true}
		if ref == p.rootRef {
			stored.Refs = refs[1:2]
		}
		return ref.GetHash(), stored, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mirror the objects exactly as they exist under the separate bucket prefix.
	id := identity.NewPackID()
	entry := &packfile.PackfileEntry{Id: id, BlockCount: res.BlockCount, SizeBytes: res.BytesWritten, BloomFilter: res.BloomFilter, BloomFormatVersion: packfile.BloomFormatVersionV1}
	encoded, err := entry.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	mirror := t.TempDir()
	for name, data := range map[string][]byte{"packs": buf.Bytes(), "entries": encoded} {
		if err := os.Mkdir(filepath.Join(mirror, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mirror, name, id), data, 0o600); err != nil {
			t.Fatal(err)
		}
		p.bucket.mtx.Lock()
		p.bucket.objects["cold/"+name+"/"+id] = data
		p.bucket.mtx.Unlock()
	}

	// Build the manifest offline, without any bucket access.
	keys := make([][]byte, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, packfile.BlockKey(ref.GetHash()))
	}
	slices.SortFunc(keys, bytes.Compare)
	exported := iter.Seq2[[]byte, error](func(yield func([]byte, error) bool) {
		for _, key := range keys {
			if !yield(key, nil) {
				return
			}
		}
	})
	manifest, err := block_store_s3.BuildColdPackManifest(ctx, mirror, exported)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = manifest.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(mirror, "manifest.pb")
	if err := os.WriteFile(file, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	// Use the placement's own fake-bucket client. The move uses this reader,
	// which the fixture attaches directly instead of restarting the provider.
	opened := p.tkr.remote.Load()
	if opened == nil || opened.s3 == nil {
		t.Fatal("hot placement has no bucket client")
	}
	reader := block_store_s3.ColdPackRangeReader(opened.s3)
	if gate != nil {
		gate.ColdPackRangeReader = reader
		reader = gate
	}
	cold, err := block_store_s3.NewColdPackStore(ctx, nil, reader, "spaces", block_store_s3.ColdPackOptions{Prefix: "cold/", ManifestFile: file})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cold.Close)
	bs.placement.cold = cold

	// Delete only the test objects under the hot placement's prefix.
	p.bucket.mtx.Lock()
	for key := range p.bucket.objects {
		if strings.HasPrefix(key, "p/") {
			delete(p.bucket.objects, key)
		}
	}
	p.bucket.mtx.Unlock()

	// Make sure a cold-only body can actually be read before attempting a move.
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !cold.Stats().Ready {
		select {
		case <-ctx.Done():
			t.Fatal("cold manifest did not load")
		case <-tick.C:
		}
	}
	data, ok, err := cold.GetBlock(ctx, p.childRef)
	if err != nil || !ok || !bytes.Equal(data, p.blocks[p.childRef]) {
		t.Fatalf("cold-only child before move = %q/%t/%v", data, ok, err)
	}
	return cold, refs[2:]
}

// loseLocalBlocks removes the local copies of the blocks.
func (p *placedStoreTest) loseLocalBlocks(ctx context.Context, t *testing.T) *BlockStore {
	// Delete the local blocks and require the cache to miss them.
	t.Helper()
	for ref := range p.blocks {
		if err := p.acc.GetVolume().RmBlock(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	bs, err := p.tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found, err := bs.placement.local.GetBlockExistsBatch(ctx, []*block.BlockRef{p.rootRef, p.childRef})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(found, true) {
		t.Fatalf("local blocks remain after removal: %v", found)
	}
	return bs
}

// unheldPuts returns the count of blocks vol observed put with no holder,
// which a Volume that frees refuses with ErrUnheldPut.
func unheldPuts(vol volume.Volume) uint64 {
	health, _ := vol.(interface {
		GetHealthSnapshotWithWait() (*volume.VolumeHealth, <-chan struct{})
	}).GetHealthSnapshotWithWait()
	return health.GetNotHeldPutBlocks()
}

// checkLocalGraph checks that the local store holds both blocks and the edge
// from the root to the child.
func (p *placedStoreTest) checkLocalGraph(ctx context.Context, t *testing.T, bs *BlockStore) {
	// Require the local graph to hold the root, the child, and their edge.
	t.Helper()
	for ref, want := range p.blocks {
		stored, err := bs.placement.local.GetStoredBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil || !bytes.Equal(stored.Data, want) {
			t.Fatalf("local block %s = %v; want %q", ref.MarshalString(), stored, want)
		}
	}
	lists := p.acc.GetVolume().(interface {
		GetChildren(context.Context, *block.BlockRef) ([]*block.BlockRef, bool, error)
	})
	refs, listed, err := lists.GetChildren(ctx, p.rootRef)
	if err != nil {
		t.Fatal(err)
	}
	if !listed || !slices.ContainsFunc(refs, p.childRef.EqualsRef) {
		t.Fatalf("root child list = %v listed %v, want the child", refs, listed)
	}
}

// dropRootList deletes the root's child list, as a hole or a store written
// before format 7 leaves a block, so the move's walk stops at the root.
func (p *placedStoreTest) dropRootList(ctx context.Context, t *testing.T) {
	// Delete the list in one transaction of the account's volume.
	t.Helper()
	tx, err := p.acc.GetVolume().(volume_kvtx.KvtxVolume).GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := volume_kvtx.OpenRecords(tx).DeleteChildren(ctx, p.rootRef); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestMoveSpaceStorageRoundTrip moves a Space onto a storage backend, loses
// its local blocks, and moves it back: the blocks return from the bucket. When
// the root also lost its child list, the move fetches the child the walk could
// not reach through the root's fetched refs, before the switch releases the
// bucket's copy.
func TestMoveSpaceStorageRoundTrip(t *testing.T) {
	for _, name := range []string{"listless=false", "listless=true", "cold-only"} {
		t.Run(name, func(t *testing.T) {
			// Bound the round trip and move the space back to local storage.
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			// Drop the local cache, move the space home, and require the graph.
			p := setupPlacedStore(ctx, t)
			placed := p.loseLocalBlocks(ctx, t)
			if name == "cold-only" {
				// A disabled or unreachable cold tier must not let the move switch.
				p.prepareColdMove(ctx, t, placed, nil)
				p.phases = nil
				block_store_s3.SetColdPackReads(false)
				err := p.acc.MoveSpaceStorage(ctx, p.soID, "", p.record)
				block_store_s3.SetColdPackReads(true)
				if !errors.Is(err, block.ErrUnavailable) || slices.Contains(p.phases, MovePhaseDone) {
					t.Fatalf("move with cold tier disabled = %v, phases %v", err, p.phases)
				}
			}
			if name == "listless=true" {
				p.dropRootList(ctx, t)
			}
			p.phases = nil
			unheld := unheldPuts(p.acc.GetVolume())
			if err := p.acc.MoveSpaceStorage(ctx, p.soID, "", p.record); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(p.phases, []MovePhase{MovePhaseFetch, MovePhaseDone}) {
				t.Fatalf("phases = %v, want fetch then done", p.phases)
			}
			if n := unheldPuts(p.acc.GetVolume()) - unheld; n != 0 {
				t.Fatalf("the fetch stored %d blocks nothing held", n)
			}
			bs, err := p.tkr.bstoreCtr.WaitValue(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			p.checkLocalGraph(ctx, t, bs)
		})
	}
}

// coldMoveGate holds each of eight foreground GETs after it gained admission.
type coldMoveGate struct {
	// ColdPackRangeReader reads the actual range from the fake bucket.
	block_store_s3.ColdPackRangeReader
	// armed is set only after the fixture has warmed the tail.
	armed atomic.Bool
	// ordinal is the number of armed requests that reached the range reader.
	ordinal atomic.Int32
	// finished counts foreground calls that left the gate.
	finished atomic.Int32
	// started counts requests that hold a demand slot.
	started chan struct{}
	// release lets exactly one admitted read complete per send.
	release chan struct{}
}

// GetColdPackRange keeps the first eight armed calls in the demand lane.
func (g *coldMoveGate) GetColdPackRange(ctx context.Context, bucket, key string, off int64, length int, size uint64) ([]byte, []byte, error) {
	// The fake read never outlives the store's read deadline.
	if g.armed.Load() && int(g.ordinal.Add(1)) <= cap(g.started) {
		g.started <- struct{}{}
		defer g.finished.Add(1)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-g.release:
		}
	}
	return g.ColdPackRangeReader.GetColdPackRange(ctx, bucket, key, off, length, size)
}

// TestMoveSpaceStorageWaitsForColdSlot fills all demand slots with live reads
// and releases only one. The move must copy both cold-only blocks through it.
func TestMoveSpaceStorageWaitsForColdSlot(t *testing.T) {
	// Keep unrelated foreground extents busy on the same loaded pack.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	p := setupPlacedStore(ctx, t)
	placed := p.loseLocalBlocks(ctx, t)
	gate := &coldMoveGate{started: make(chan struct{}, 8), release: make(chan struct{})}
	cold, refs := p.prepareColdMove(ctx, t, placed, gate)
	gate.armed.Store(true)
	defer close(gate.release)
	for _, ref := range refs {
		go func() { _, _ = cold.GetStoredBlock(ctx, ref) }()
	}
	for range refs {
		select {
		case <-gate.started:
		case <-ctx.Done():
			t.Fatal("foreground reads did not occupy all eight slots")
		}
	}

	// A canceled wait must report an unresolved block and claim no permit.
	short, stop := context.WithTimeout(ctx, 25*time.Millisecond)
	stored, err := cold.GetStoredBlockForMove(short, p.rootRef)
	stop()
	if stored != nil || !errors.Is(err, block.ErrUnavailable) || gate.finished.Load() != 0 {
		t.Fatalf("canceled cold read = %v, %v, %d released ranges", stored, err, gate.finished.Load())
	}

	// While the lane is full the move reaches fetch, but cannot finish it.
	fetching, done := make(chan struct{}, 1), make(chan error, 1)
	go func() {
		done <- p.acc.MoveSpaceStorage(ctx, p.soID, "", func(progress MoveProgress) error {
			if progress.Phase == MovePhaseFetch && progress.Fetched == 0 {
				fetching <- struct{}{}
			}
			return nil
		})
	}()
	select {
	case <-fetching:
	case err := <-done:
		t.Fatalf("move did not reach its fetch phase: %v", err)
	case <-ctx.Done():
		t.Fatal("move did not reach its fetch phase")
	}
	select {
	case err := <-done:
		t.Fatalf("move finished with all eight slots held: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	// The other seven reads stay held through the destination check.
	select {
	case gate.release <- struct{}{}:
	case <-ctx.Done():
		t.Fatal("the fake bucket lost its held ranges")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("move did not finish after a slot was released")
	}
	if finished := gate.finished.Load(); finished != 1 {
		t.Fatalf("move finished after %d foreground slots freed, want 1", finished)
	}

	// Confirm the move wrote the two blocks and their edge into the destination.
	bs, err := p.tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.checkLocalGraph(ctx, t, bs)
}

// TestPlacedStoreGraphCopyAfterCacheLoss loses the local blocks of a placed
// Space, then copies its graph through the store: the copy reads the blocks
// back from the bucket, and their child lists stay.
func TestPlacedStoreGraphCopyAfterCacheLoss(t *testing.T) {
	// Bound the cache-loss copy.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Drop the local blocks, then copy the graph back.
	p := setupPlacedStore(ctx, t)
	bs := p.loseLocalBlocks(ctx, t)
	if err := block.CopyGraph(ctx, bs, bs.placement.local, p.rootRef, nil); err != nil {
		t.Fatal(err)
	}
	p.checkLocalGraph(ctx, t, bs)
}

// TestPlacedStoreSettleAndLiveness places a Space with a settle age. The
// bucket reclaim keeps the blocks the volume keeps child lists for after
// their local values are gone, and a pending-bytes cap uploads a fresh block
// before it settles.
func TestPlacedStoreSettleAndLiveness(t *testing.T) {
	// Bound the settle and liveness checks.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Hold writes for an hour before upload.
	p := setupPlacedStore(ctx, t)
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 3600})
	p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.Settle == time.Hour
	})

	// Drop the local values and require the listed blocks to stay live.
	bs := p.loseLocalBlocks(ctx, t)
	refs := []*block.BlockRef{p.rootRef, p.childRef}
	live, err := bs.placement.live(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(live, []bool{true, true}) {
		t.Fatalf("live = %v, want both listed blocks live", live)
	}

	// A one-byte cap uploads a fresh block before its hour settles.
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 3600, MaxPendingBytes: 1})
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.MaxPendingBytes == 1
	})
	if _, _, err := bs.PutBlock(ctx, []byte("capped block"), &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Pending == 0
	})
	if n := p.bucket.count(); n <= 2 {
		t.Fatalf("bucket holds %d objects, want the capped block's upload", n)
	}
}

// place re-places the Space on its backend with the cache settings.
func (p *placedStoreTest) place(ctx context.Context, t *testing.T, cache *account_settings.BlockStoreCache) {
	t.Helper()
	err := p.acc.PlaceBlockStore(ctx, &account_settings.BlockStorePlacement{
		BlockStoreId:     p.tkr.id,
		StorageBackendId: p.backendID,
		Cache:            cache,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// waitUpload waits for the store's upload status to satisfy cond and returns
// the store.
func (p *placedStoreTest) waitUpload(ctx context.Context, t *testing.T, cond func(UploadStatus) bool) *BlockStore {
	t.Helper()
	bs, err := p.tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		status, changed := bs.GetUploadStatus()
		if status.Err != nil {
			t.Fatal(status.Err)
		}
		if cond(status) {
			return bs
		}
		select {
		case <-ctx.Done():
			t.Fatalf("upload status %+v never matched", status)
		case <-changed:
		}
	}
}

// TestPlacedStoreEvictAndRefill evicts the uploaded blocks of a placed store,
// refuses a block whose rewrite awaits upload, reads the evicted block back,
// and finds the refill staged no garbage candidate.
func TestPlacedStoreEviction(t *testing.T) {
	// Bound the eviction checks.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Hold the child's rewrite for an hour, so its marker stays queued.
	p := setupPlacedStore(ctx, t)
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 3600})
	bs := p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.Settle == time.Hour && status.Pending == 0
	})
	if _, _, err := bs.PutBlock(ctx, p.blocks[p.childRef], &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}

	// Evict: the root leaves the disk and the marked child stays.
	result, err := bs.placement.evict(ctx, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evicted != 1 || result.Refused != 1 {
		t.Fatalf("evict = %+v, want the root evicted and the child refused", result)
	}
	vol := p.acc.GetVolume()
	held, err := vol.GetBlockExistsBatch(ctx, []*block.BlockRef{p.rootRef, p.childRef})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(held, []bool{false, true}) {
		t.Fatalf("held = %v, want only the child on disk", held)
	}

	// Read the root back: it refills the disk in the background and stages
	// no candidate.
	data, found, err := bs.GetBlock(ctx, p.rootRef)
	if err != nil || !found || !bytes.Equal(data, p.blocks[p.rootRef]) {
		t.Fatalf("read evicted root = %q/%v/%v", data, found, err)
	}
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool { return status.Refills == 1 })
	if found, err := vol.GetBlockExists(ctx, p.rootRef); err != nil || !found {
		t.Fatalf("refilled root on disk = %v/%v", found, err)
	}
	// The refill keeps both blocks listed, and queued the root again.
	p.checkLocalGraph(ctx, t, bs)
	result, err = bs.placement.evict(ctx, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evicted != 1 {
		t.Fatalf("second evict = %+v, want the refilled root evicted", result)
	}

	// Refill the root, then hold eviction off while a budget that fits only
	// the root uploads the child early.
	if _, found, err := bs.GetBlock(ctx, p.rootRef); err != nil || !found {
		t.Fatalf("refill root = %v/%v", found, err)
	}
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool { return status.Refills == 2 })
	rootSize, childSize := int64(len(p.blocks[p.rootRef])), int64(len(p.blocks[p.childRef]))
	release, err := bs.placement.evictGate.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p.place(ctx, t, &account_settings.BlockStoreCache{
		SettleSeconds:     3600,
		MaxPendingBytes:   1,
		ResidentHighBytes: uint64(rootSize + childSize - 1),
		ResidentLowBytes:  uint64(rootSize),
	})
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.ResidentLow == rootSize && status.Pending == 0
	})

	// Read the root, then let the budget pass run: it keeps the root it saw
	// read and evicts the child down to the low mark.
	if _, found, err := bs.GetBlock(ctx, p.rootRef); err != nil || !found {
		t.Fatalf("read hot root = %v/%v", found, err)
	}
	release()
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.ResidentBytes() <= rootSize
	})
	final, _ := bs.placement.wb.GetStatus()
	if final.EvictedBytes != 2*rootSize+childSize || final.Refills != 2 {
		t.Fatalf("status = %+v, want the child evicted and the hot root never refilled", final)
	}
	held, err = vol.GetBlockExistsBatch(ctx, []*block.BlockRef{p.rootRef, p.childRef})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(held, []bool{true, false}) {
		t.Fatalf("held = %v, want only the hot root on disk", held)
	}

	// Read the child back: the read that refilled it keeps it through the
	// next pass, though the store is over its budget again.
	if _, found, err := bs.GetBlock(ctx, p.childRef); err != nil || !found {
		t.Fatalf("refill child = %v/%v", found, err)
	}
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool { return status.Refills == 3 })
	if result, err := bs.placement.evict(ctx, math.MaxInt64); err != nil || result.Evicted != 0 {
		t.Fatalf("evict after refill = %+v/%v, want both blocks kept", result, err)
	}
	held, err = vol.GetBlockExistsBatch(ctx, []*block.BlockRef{p.rootRef, p.childRef})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(held, []bool{true, true}) {
		t.Fatalf("held = %v, want the refilled child kept on disk", held)
	}
}

// TestMoveSpaceStorageUnreachableBackend loses the local blocks of a placed
// Space and moves it off a backend that cannot be reached: the move fails and
// keeps the placement and the bucket's objects.
func TestMoveSpaceStorageUnreachableBackend(t *testing.T) {
	// Bound the unreachable-backend move.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Move the space out, then mark the fake bucket unavailable.
	p := setupPlacedStore(ctx, t)
	p.loseLocalBlocks(ctx, t)
	p.bucket.mtx.Lock()
	p.bucket.unavailable = true
	p.bucket.mtx.Unlock()

	// Require the failed move to keep the backend placement and remote objects.
	if err := p.acc.MoveSpaceStorage(ctx, p.soID, "", p.record); err == nil {
		t.Fatal("move off an unreachable backend succeeded")
	}
	settings, err := p.acc.readAccountSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := settings.FindBlockStorePlacement(p.tkr.id).GetStorageBackendId(); got != p.backendID {
		t.Fatalf("placement = %q, want %q", got, p.backendID)
	}
	if n := len(settings.GetStorageReleases()); n != 0 {
		t.Fatalf("settings record %d releases, want none", n)
	}
	if n := p.bucket.count(); n != 2 {
		t.Fatalf("bucket holds %d objects, want 2", n)
	}
}

// TestMoveSpaceStorageClosedRemote loses the local blocks of a placed Space
// and closes the tracker's backend store before moving it back: the move reads
// the blocks from the bucket rather than skipping them.
func TestMoveSpaceStorageClosedRemote(t *testing.T) {
	// Bound the closed-remote move.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Drop the local cache and close the remote store.
	p := setupPlacedStore(ctx, t)
	p.loseLocalBlocks(ctx, t)
	p.tkr.swapRemote(nil)

	// Move the space home and require the local graph.
	if err := p.acc.MoveSpaceStorage(ctx, p.soID, "", p.record); err != nil {
		t.Fatal(err)
	}
	bs, err := p.tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.checkLocalGraph(ctx, t, bs)
}
