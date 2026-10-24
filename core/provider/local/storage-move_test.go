package provider_local

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
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
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
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
	childRef, _, err := bs.placement.local.PutBlock(ctx, child, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := []byte("root block")
	rootRef, _, err := bs.placement.local.PutBlock(ctx, root, &block.PutOpts{Refs: []*block.BlockRef{childRef}})
	if err != nil {
		t.Fatal(err)
	}
	bucketIRI := block_gc.BucketIRI(BlockStoreBucketID(acc.GetProviderID(), acc.GetAccountID(), blockStoreID))
	if err := acc.GetVolume().GetRefGraph().AddRef(ctx, bucketIRI, block_gc.BlockIRI(rootRef)); err != nil {
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
	refs, err := p.acc.GetVolume().GetRefGraph().GetOutgoingRefs(ctx, block_gc.BlockIRI(p.rootRef))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(refs, block_gc.BlockIRI(p.childRef)) {
		t.Fatalf("root edges = %v, want the child", refs)
	}
}

// TestMoveSpaceStorageRoundTrip moves a Space onto a storage backend, loses
// its local blocks, and moves it back: the blocks return from the bucket.
func TestMoveSpaceStorageRoundTrip(t *testing.T) {
	// Bound the round trip and move the space back to local storage.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Drop the local cache, move the space home, and require the graph.
	p := setupPlacedStore(ctx, t)
	p.loseLocalBlocks(ctx, t)
	p.phases = nil
	if err := p.acc.MoveSpaceStorage(ctx, p.soID, "", p.record); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.phases, []MovePhase{MovePhaseFetch, MovePhaseDone}) {
		t.Fatalf("phases = %v, want fetch then done", p.phases)
	}
	bs, err := p.tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.checkLocalGraph(ctx, t, bs)
}

// TestPlacedStoreGraphCopyAfterCacheLoss loses the local blocks and edges of
// a placed Space, then copies its graph through the store: the copy reads the
// blocks and their edges back from the bucket.
func TestPlacedStoreGraphCopyAfterCacheLoss(t *testing.T) {
	// Bound the cache-loss copy.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Drop the local blocks and the root edge, then copy the graph back.
	p := setupPlacedStore(ctx, t)
	bs := p.loseLocalBlocks(ctx, t)
	if _, err := p.acc.GetVolume().GetRefGraph().RemoveNodeRefs(ctx, block_gc.BlockIRI(p.rootRef), false); err != nil {
		t.Fatal(err)
	}
	if err := block.CopyGraph(ctx, bs, bs.placement.local, p.rootRef, nil); err != nil {
		t.Fatal(err)
	}
	p.checkLocalGraph(ctx, t, bs)
}

// TestPlacedStoreSettleAndLiveness places a Space with a settle age and
// writes a block it then drops: the GC sweep frees it before it settles, so it
// never reaches the bucket. The bucket reclaim keeps blocks the graph tracks
// after their local values are gone, and drops them once a block pass sweeps
// them. A pending-bytes cap uploads a fresh block before it settles.
func TestPlacedStoreSettleAndLiveness(t *testing.T) {
	// Bound the settle and liveness checks.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Hold writes for an hour before upload.
	p := setupPlacedStore(ctx, t)
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 3600})
	bs := p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.Settle == time.Hour
	})

	// Write a block and drop it before it settles.
	garbage, _, err := bs.PutBlock(ctx, []byte("settling garbage"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := bs.GetUploadStatus(); status.Pending != 1 {
		t.Fatalf("pending = %d, want the settling block", status.Pending)
	}
	if err := bs.RmBlock(ctx, garbage); err != nil {
		t.Fatal(err)
	}

	// Drop the local values and require the tracked blocks to stay live.
	bs = p.loseLocalBlocks(ctx, t)
	refs := []*block.BlockRef{p.rootRef, p.childRef}
	live, err := bs.placement.live(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(live, []bool{true, true}) {
		t.Fatalf("live = %v, want both tracked blocks live", live)
	}

	// Release the root and sweep: the reclaim may now drop both, and the
	// dropped block is gone before it settled.
	rg := p.acc.GetVolume().GetRefGraph()
	bucketIRI := block_gc.BucketIRI(BlockStoreBucketID(p.acc.GetProviderID(), p.acc.GetAccountID(), p.tkr.id))
	if err := rg.RemoveRef(ctx, bucketIRI, block_gc.BlockIRI(p.rootRef)); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(rg, p.acc.GetVolume(), nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	live, err = bs.placement.live(ctx, append(refs, garbage))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(live, []bool{false, false, false}) {
		t.Fatalf("live = %v, want every swept block dead", live)
	}

	// Upload at once: the swept block's marker clears without an upload, and
	// re-placing with other cache settings releases nothing.
	p.place(ctx, t, nil)
	p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.Settle == 0 && status.Pending == 0
	})
	if n := p.bucket.count(); n != 2 {
		t.Fatalf("bucket holds %d objects, want the 2 from setup", n)
	}

	// A one-byte cap uploads a fresh block before its hour settles.
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 3600, MaxPendingBytes: 1})
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.MaxPendingBytes == 1
	})
	if _, _, err := bs.PutBlock(ctx, []byte("capped block"), nil); err != nil {
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
	if _, _, err := bs.PutBlock(ctx, p.blocks[p.childRef], nil); err != nil {
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

	// Read the root back: it refills the disk and stages no candidate.
	data, found, err := bs.GetBlock(ctx, p.rootRef)
	if err != nil || !found || !bytes.Equal(data, p.blocks[p.rootRef]) {
		t.Fatalf("read evicted root = %q/%v/%v", data, found, err)
	}
	if found, err := vol.GetBlockExists(ctx, p.rootRef); err != nil || !found {
		t.Fatalf("refilled root on disk = %v/%v", found, err)
	}
	rg := vol.GetRefGraph()
	staged, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(staged, block_gc.BlockIRI(p.rootRef)) {
		t.Fatal("refill staged the root as a garbage candidate")
	}

	// A block pass keeps both blocks, and the refill queued the root again.
	if _, err := block_gc.NewCollector(rg, vol, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
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
