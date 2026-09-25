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

// TestMoveSpaceStorageRoundTrip moves a Space onto a storage backend, loses
// its local blocks, and moves it back: the blocks return from the bucket.
func TestMoveSpaceStorageRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	bucket := &fakeS3{objects: make(map[string][]byte)}
	srv := httptest.NewServer(bucket)
	defer srv.Close()

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
	defer tkrRef.Release()
	bs, err := tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write a root and a child and reference them from the store's bucket
	// node, as the World engine does.
	child := []byte("child block")
	childRef, _, err := bs.placement.local.PutBlock(ctx, child, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := []byte("root block")
	rootRef, _, err := bs.placement.local.PutBlock(ctx, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	rg := acc.GetVolume().GetRefGraph()
	bucketIRI := block_gc.BucketIRI(BlockStoreBucketID(acc.GetProviderID(), acc.GetAccountID(), blockStoreID))
	if err := rg.AddRef(ctx, bucketIRI, block_gc.BlockIRI(rootRef)); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, block_gc.BlockIRI(rootRef), block_gc.BlockIRI(childRef)); err != nil {
		t.Fatal(err)
	}

	// Move onto the backend: the backfill uploads both blocks.
	var phases []MovePhase
	record := func(p MoveProgress) error {
		if !slices.Contains(phases, p.Phase) {
			phases = append(phases, p.Phase)
		}
		return nil
	}
	if err := acc.MoveSpaceStorage(ctx, soID, backendID, record); err != nil {
		t.Fatal(err)
	}
	if n := bucket.count(); n != 2 {
		t.Fatalf("bucket holds %d objects, want 2", n)
	}

	// Lose the local copies, then move back to the account's own storage.
	for _, ref := range []*block.BlockRef{rootRef, childRef} {
		if err := acc.GetVolume().RmBlock(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	bs, err = tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found, err := bs.placement.local.GetBlockExistsBatch(ctx, []*block.BlockRef{rootRef, childRef})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(found, true) {
		t.Fatalf("local blocks remain after removal: %v", found)
	}
	phases = nil
	if err := acc.MoveSpaceStorage(ctx, soID, "", record); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(phases, []MovePhase{MovePhaseFetch, MovePhaseDone}) {
		t.Fatalf("phases = %v, want fetch then done", phases)
	}
	bs, err = tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[*block.BlockRef][]byte{rootRef: root, childRef: child} {
		data, ok, err := bs.placement.local.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if !ok || !bytes.Equal(data, want) {
			t.Fatalf("local block %s = %q, %v; want %q", ref.MarshalString(), data, ok, want)
		}
	}
}
