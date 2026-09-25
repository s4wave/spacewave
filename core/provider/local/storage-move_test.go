package provider_local

import (
	"bytes"
	"context"
	"io"
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
// DELETE, and ListObjectsV2 on path-style URLs. It ignores signatures.
type fakeS3 struct {
	mtx     sync.Mutex
	objects map[string][]byte
}

// ServeHTTP handles one request against the single bucket.
func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	_, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
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
	t.Helper()
	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t)
	t.Cleanup(release)

	bucket := &fakeS3{objects: make(map[string][]byte)}
	srv := httptest.NewServer(bucket)
	t.Cleanup(srv.Close)

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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

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
