package s4wave_bucket_lookup

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
)

// TestSDKCursorPreservesServerBucketIDOverride keeps mirrored references in
// the mounted bucket when the stored author bucket differs.
func TestSDKCursorPreservesServerBucketIDOverride(t *testing.T) {
	ctx := context.Background()
	ref := &bucketLookupCursorRef{client: &bucketLookupCursorClient{
		response: &GetRefResponse{
			Ref:              &bucket.ObjectRef{BucketId: "device-mirror"},
			BucketIdOverride: "device-mirror",
		},
	}}
	cursor, err := NewCursor(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cursor.Release()
	if got := cursor.GetBucketIDOverride(); got != "device-mirror" {
		t.Fatalf("bucket override = %q, want device-mirror", got)
	}

	child, err := cursor.FollowRef(ctx, &bucket.ObjectRef{BucketId: "author-bucket"})
	if err != nil {
		t.Fatalf("follow mirrored author reference: %v", err)
	}
	defer child.Release()
	if got := child.GetOpArgs().GetBucketId(); got != "device-mirror" {
		t.Fatalf("child bucket = %q, want device-mirror", got)
	}
}

// bucketLookupCursorRef supplies one scripted Resource reference.
type bucketLookupCursorRef struct {
	client   srpc.Client
	released bool
}

// GetResourceID identifies the scripted cursor resource.
func (r *bucketLookupCursorRef) GetResourceID() uint32 { return 1 }

// GetClient returns the scripted RPC client.
func (r *bucketLookupCursorRef) GetClient() (srpc.Client, error) { return r.client, nil }

// Release records that the caller released its reference.
func (r *bucketLookupCursorRef) Release() { r.released = true }

var _ resource_client.ResourceRef = (*bucketLookupCursorRef)(nil)

// bucketLookupCursorClient returns the scripted cursor binding.
type bucketLookupCursorClient struct {
	response *GetRefResponse
}

// ExecCall copies the binding into a GetRef response.
func (c *bucketLookupCursorClient) ExecCall(
	_ context.Context,
	_, _ string,
	_ srpc.Message,
	out srpc.Message,
) error {
	response, ok := out.(*GetRefResponse)
	if !ok {
		return errors.Errorf("unexpected response type %T", out)
	}
	*response = *c.response.CloneVT()
	return nil
}

// NewStream rejects streaming calls outside the binding test.
func (c *bucketLookupCursorClient) NewStream(
	context.Context,
	string,
	string,
	srpc.Message,
) (srpc.Stream, error) {
	return nil, errors.New("unexpected stream")
}

// TestSDKBucketLookupStoreGetBlockRecordsResourceCounter checks transport byte accounting.
func TestSDKBucketLookupStoreGetBlockRecordsResourceCounter(t *testing.T) {
	ctx := context.Background()
	data := []byte("resource block data")
	store := &cursorStore{StoreOps: block_mock.NewMockStore(0)}
	ref, _, err := store.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	opCtx, counter := block.WithReadCounter(ctx)
	got, found, err := store.GetBlock(opCtx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("GetBlock found=%v data=%q, want true %q", found, got, data)
	}
	snapshot := counter.Snapshot()
	if snapshot.ResourceGetBlockCount != 1 ||
		snapshot.ResourceGetBlockRefCount != 1 ||
		snapshot.ResourceGetBlockBytes != uint64(len(data)) ||
		snapshot.ResourceGetBlockMissCount != 0 {
		t.Fatalf("unexpected resource GetBlock counters: %+v", snapshot)
	}
}

// TestSDKBucketLookupStoreReadOperationReusesDecodedBlocks preserves cloned
// decoded values across reads within one borrowed scope.
func TestSDKBucketLookupStoreReadOperationReusesDecodedBlocks(t *testing.T) {
	ctx := context.Background()
	encoded, err := (&block_mock.Example{Msg: "resource decoded"}).MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	store := &cursorStore{StoreOps: block_mock.NewMockStore(0)}
	ref, _, err := store.PutBlock(ctx, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, first := block.NewTransaction(store, nil, ref, nil)
	_, second := block.NewTransaction(store, nil, ref, nil)
	ctor := func() block.Block { return &block_mock.Example{} }

	opCtx, counter := block.WithReadCounter(ctx)
	scopedStore, release, err := store.BeginReadOperation(opCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	opCtx = block.WithReadOperationStore(opCtx, scopedStore)

	firstBlock, err := first.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstExample := firstBlock.(*block_mock.Example)
	firstExample.Msg = "mutated"

	secondBlock, err := second.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}
	secondExample := secondBlock.(*block_mock.Example)
	if secondExample.GetMsg() != "resource decoded" {
		t.Fatalf("cached resource block message = %q, want resource decoded", secondExample.GetMsg())
	}
	if firstExample == secondExample {
		t.Fatal("resource cache hit returned the first decoded block instance")
	}
	snapshot := counter.Snapshot()
	if snapshot.ResourceGetBlockCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 ||
		snapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected resource decoded cache counters: %+v", snapshot)
	}
}
