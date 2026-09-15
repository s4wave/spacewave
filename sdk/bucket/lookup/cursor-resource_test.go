//go:build !js && !wasip1

package s4wave_bucket_lookup_test

import (
	"bytes"
	"testing"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	sdk_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestCursorResourceEncodedBlocks checks transformed and stored bytes through
// the real Resource pipe, including a batch larger than one decoded packet.
func TestCursorResourceEncodedBlocks(t *testing.T) {
	cursor, tb := newResourceCursor(t)
	ctx := t.Context()
	data := bytes.Repeat([]byte("compressed and encrypted block "), 40000)
	ref, _, err := cursor.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := cursor.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("decoded read: found=%v err=%v", found, err)
	}

	// Both local and remote StoreOps must return the exact content-addressed
	// ciphertext. The enclosing cursor alone applies the storage transform.
	stored, found, err := tb.Volume.GetBlock(ctx, ref)
	if err != nil || !found || bytes.Equal(stored, data) || len(stored) >= len(data) {
		t.Fatalf("stored payload: found=%v bytes=%d err=%v", found, len(stored), err)
	}
	if err := ref.VerifyData(stored, true); err != nil {
		t.Fatal(err)
	}
	raw := cursor.GetBucket()
	remote, found, err := raw.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(remote, stored) {
		t.Fatalf("encoded read: found=%v err=%v", found, err)
	}

	entries := make([]*block.PutBatchEntry, 12)
	for i := range entries {
		plain := bytes.Repeat([]byte{byte(i)}, 1<<20)
		encoded, err := cursor.GetTransformer().EncodeBlock(plain)
		if err != nil {
			t.Fatal(err)
		}
		blockRef, err := block.BuildBlockRef(encoded, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries[i] = &block.PutBatchEntry{Ref: blockRef, Data: encoded, Refs: []*block.BlockRef{ref}}
	}
	if err := raw.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if fenced, err := raw.Sync(ctx); err != nil || !fenced {
		t.Fatalf("durability fence: fenced=%v err=%v", fenced, err)
	}
	for i, entry := range entries {
		got, found, err := cursor.GetBlock(ctx, entry.Ref)
		if err != nil || !found || !bytes.Equal(got, bytes.Repeat([]byte{byte(i)}, 1<<20)) {
			t.Fatalf("batch entry %d: found=%v err=%v", i, found, err)
		}
	}
}

// BenchmarkCursorResourceRoundTrip includes the real Resource transport,
// compression, encryption, storage deduplication, and decoded readback.
func BenchmarkCursorResourceRoundTrip(b *testing.B) {
	cursor, _ := newResourceCursor(b)
	data := bytes.Repeat([]byte("transformed Resource payload "), 40000)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		ref, _, err := cursor.PutBlock(b.Context(), data, nil)
		if err != nil {
			b.Fatal(err)
		}
		got, found, err := cursor.GetBlock(b.Context(), ref)
		if err != nil || !found || !bytes.Equal(got, data) {
			b.Fatalf("round trip: found=%v err=%v", found, err)
		}
	}
}

// newResourceCursor uses the ordinary encrypted World engine and releases all
// acquired handles before closing the testbed's Resource connection.
func newResourceCursor(t testing.TB) (*bucket_lookup.Cursor, *world_testbed.Testbed) {
	t.Helper()
	tb, client, cleanup := resource_testbed.SetupTestbedWithClient(t.Context(), t)
	t.Cleanup(cleanup)
	root := client.AccessRootResource()
	t.Cleanup(root.Release)
	rpc, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	created, err := sdk_testbed.NewSRPCTestbedResourceServiceClient(rpc).CreateWorld(t.Context(), &sdk_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ref := client.CreateResourceReference(created.ResourceId)
	t.Cleanup(ref.Release)
	engine, err := sdk_world_engine.NewSDKEngine(client, ref)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := engine.BuildStorageCursor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	if cursor.GetTransformer() == nil {
		t.Fatal("test cursor has no storage transform")
	}
	return cursor, tb
}
