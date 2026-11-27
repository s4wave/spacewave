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
	// Prepare a server cursor reference with a mirrored bucket override.
	ctx := context.Background()
	ref := &bucketLookupCursorRef{client: &bucketLookupCursorClient{
		response: &GetRefResponse{
			Ref:              &bucket.ObjectRef{BucketId: "device-mirror"},
			BucketIdOverride: "device-mirror",
		},
	}}

	// Wrap the server cursor for local bucket operations.
	cursor, err := NewCursor(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cursor.Release()

	// Verify the SDK cursor preserves the server bucket override.
	if got := cursor.GetBucketIDOverride(); got != "device-mirror" {
		t.Fatalf("bucket override = %q, want device-mirror", got)
	}

	// Follow an author bucket reference through the mirrored cursor.
	child, err := cursor.FollowRef(ctx, &bucket.ObjectRef{BucketId: "author-bucket"})
	if err != nil {
		t.Fatalf("follow mirrored author reference: %v", err)
	}
	defer child.Release()

	// Verify child operations still target the mirrored bucket.
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

// Done returns a nil channel: the scripted resource is never released.
func (r *bucketLookupCursorRef) Done() <-chan struct{} { return nil }

// ReleasedByServer reports false: the scripted resource is never released.
func (r *bucketLookupCursorRef) ReleasedByServer() bool { return false }

// ReleaseReason reports the scripted reference's local release state.
func (r *bucketLookupCursorRef) ReleaseReason() resource_client.ResourceReleaseReason {
	if r.released {
		return resource_client.ResourceClientReleased
	}
	return resource_client.ResourceLive
}

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
	// Store a block behind the cursor Resource accounting wrapper.
	ctx := context.Background()
	data := []byte("resource block data")
	store := &cursorStore{StoreOps: block_mock.NewMockStore(0)}
	ref, _, err := store.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Read the stored block within a Resource read counter.
	opCtx, counter := block.WithReadCounter(ctx)
	got, found, err := store.GetBlock(opCtx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the cursor store returns the original block bytes.
	if !found || !bytes.Equal(got, data) {
		t.Fatalf("GetBlock found=%v data=%q, want true %q", found, got, data)
	}

	// Verify Resource accounting records one successful block read.
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
	// Encode the block whose decoded value will be shared within a read scope.
	ctx := context.Background()
	encoded, err := (&block_mock.Example{Msg: "resource decoded"}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the encoded Resource block for both cursors to read.
	store := &cursorStore{StoreOps: block_mock.NewMockStore(0)}
	ref, _, err := store.PutBlock(ctx, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Open two cursors on the same Resource block reference.
	_, first := block.NewTransaction(store, nil, ref, nil)
	_, second := block.NewTransaction(store, nil, ref, nil)
	ctor := func() block.Block { return &block_mock.Example{} }

	// Borrow one Resource read scope for both block decodes.
	opCtx, counter := block.WithReadCounter(ctx)
	scopedStore, release, err := store.BeginReadOperation(opCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()
	opCtx = block.WithReadOperationStore(opCtx, scopedStore)

	// Decode and mutate the first cursor value within the read scope.
	firstBlock, err := first.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstExample := firstBlock.(*block_mock.Example)
	firstExample.Msg = "mutated"

	// Decode the same block through the second cursor.
	secondBlock, err := second.Unmarshal(opCtx, ctor)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the cached block is cloned with its original message intact.
	secondExample := secondBlock.(*block_mock.Example)
	if secondExample.GetMsg() != "resource decoded" {
		t.Fatalf("cached resource block message = %q, want resource decoded", secondExample.GetMsg())
	}
	if firstExample == secondExample {
		t.Fatal("resource cache hit returned the first decoded block instance")
	}

	// Verify the read scope reused the decoded block with one clone.
	snapshot := counter.Snapshot()
	if snapshot.ResourceGetBlockCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 ||
		snapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected resource decoded cache counters: %+v", snapshot)
	}
}

// TestSDKBucketLookupStoreReadOnlyTransactionReusesDecodedBlocks checks that a
// read-only transaction over the remote cursor store keeps the blocks it
// decodes, so a second cursor of that transaction reads without decoding.
func TestSDKBucketLookupStoreReadOnlyTransactionReusesDecodedBlocks(t *testing.T) {
	// Store an encoded block behind the remote cursor store, over a store
	// that marks no reads, as the RPC client marks none.
	ctx := context.Background()
	encoded, err := (&block_mock.Example{Msg: "remote decoded"}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	store := &cursorStore{StoreOps: &unmarkedStore{StoreOps: block_mock.NewMockStore(0)}}
	ref, _, err := store.PutBlock(ctx, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Open a read-only transaction with two cursors at the block.
	tx, first := block.NewTransaction(store, nil, ref, nil)
	tx.SetReadOnly()
	second := first.Detach(false)
	ctor := func() block.Block { return &block_mock.Example{} }

	// Decode the block through each cursor.
	opCtx, counter := block.WithReadCounter(ctx)
	for _, cs := range []*block.Cursor{first, second} {
		blk, err := cs.Unmarshal(opCtx, ctor)
		if err != nil {
			t.Fatal(err.Error())
		}
		if msg := blk.(*block_mock.Example).GetMsg(); msg != "remote decoded" {
			t.Fatalf("decoded message = %q, want remote decoded", msg)
		}
	}

	// Verify the second cursor took the decoded block from the transaction.
	snapshot := counter.Snapshot()
	if snapshot.ResourceGetBlockCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 {
		t.Fatalf("unexpected remote decoded cache counters: %+v", snapshot)
	}
}

// unmarkedStore serves blocks without marking the reads durable, as the RPC
// block store client does.
type unmarkedStore struct {
	block.StoreOps
}

// GetBlock reads ref without the caller's read mark.
func (s *unmarkedStore) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.StoreOps.GetBlock(context.Background(), ref)
}
