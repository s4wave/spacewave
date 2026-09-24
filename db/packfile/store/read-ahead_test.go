//go:build !tinygo

package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_order "github.com/s4wave/spacewave/db/packfile/order"
	"github.com/s4wave/spacewave/net/hash"
)

// TestMaterializerReadAheadSharesForegroundCache verifies the source RPC
// service's bulk policy reaches the real pack reader and benefits later reads.
func TestMaterializerReadAheadSharesForegroundCache(t *testing.T) {
	// Build a 16 MiB pack with independently addressable, adjacent blocks.
	items := make([]packItem, 64)
	for i := range items {
		data := bytes.Repeat([]byte{byte(i)}, 256<<10)
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatal(err)
		}
		items[i] = packItem{h: h, data: data}
	}
	data, bloom := packItems(t, items)
	transport := &bytesTransport{data: data}
	store := NewPackfileStore(func(id string, size int64) (*PackReader, error) {
		return NewPackReader(id, size, transport, hash.RecommendedHashType), nil
	}, newMemIndexCache())
	t.Cleanup(store.Close)
	store.SetIndexPromotionEnabled(false)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id: "bulk", BloomFilter: bloom, BlockCount: uint64(len(items)), SizeBytes: uint64(len(data)),
	}})

	// Index discovery remains exact and is independent of payload read-ahead.
	if exists, err := store.GetBlockExists(t.Context(), block.NewBlockRef(items[0].h)); err != nil || !exists {
		t.Fatalf("load index: exists=%v err=%v", exists, err)
	}
	before := transport.callCount()
	bulk := block_rpc_server.NewBlockStoreWithReadAhead(store, 10<<20)
	resp, err := bulk.GetBlock(t.Context(), &block_rpc.GetBlockRequest{Ref: block.NewBlockRef(items[0].h)})
	if err != nil || resp.GetError() != "" || !resp.GetExists() || !bytes.Equal(resp.GetData(), items[0].data) {
		t.Fatalf("bulk read failed: err=%v response error=%s", err, resp.GetError())
	}
	if got := transport.callCount() - before; got != 1 {
		t.Fatalf("bulk payload requests = %d, want 1", got)
	}
	if got := transport.callAt(before).length; got != 10<<20 {
		t.Fatalf("bulk payload range = %d, want 10 MiB", got)
	}

	// A different service reads several MiB ahead without another fetch.
	foreground := block_rpc_server.NewBlockStore(store)
	resp, err = foreground.GetBlock(t.Context(), &block_rpc.GetBlockRequest{Ref: block.NewBlockRef(items[16].h)})
	if err != nil || resp.GetError() != "" || !bytes.Equal(resp.GetData(), items[16].data) {
		t.Fatalf("cached foreground read failed: %v %s", err, resp.GetError())
	}
	if got := transport.callCount() - before; got != 1 {
		t.Fatalf("foreground refetched cached bytes: %d payload requests", got)
	}

	// A distant foreground miss retains the small-read policy.
	resp, err = foreground.GetBlock(t.Context(), &block_rpc.GetBlockRequest{Ref: block.NewBlockRef(items[52].h)})
	if err != nil || resp.GetError() != "" || !bytes.Equal(resp.GetData(), items[52].data) {
		t.Fatalf("distant foreground read failed: %v %s", err, resp.GetError())
	}
	for i := before + 1; i < transport.callCount(); i++ {
		if got := transport.callAt(i).length; got >= 10<<20 {
			t.Fatalf("bulk policy leaked into foreground request: %d", got)
		}
	}
}

// TestReadAheadRespectsUncoveredGapsAndTransportCap verifies a bulk hint never
// refetches resident bytes or exceeds a constrained transport's payload cap.
func TestReadAheadRespectsUncoveredGapsAndTransportCap(t *testing.T) {
	transport := &bytesTransport{data: make([]byte, 16<<20)}
	reader := NewPackReader("bounded-bulk", int64(len(transport.data)), transport, hash.RecommendedHashType)
	t.Cleanup(reader.Close)
	reader.setTransportFetchMaxBytes(2 << 20)
	ctx := block.WithReadAhead(t.Context(), 10<<20)
	buf := make([]byte, 1)
	if _, err := reader.ReaderAt(ctx).ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if got := transport.callAt(0).length; got != 2<<20 {
		t.Fatalf("constrained range = %d, want 2 MiB", got)
	}

	// Prefill the far side, then request the uncovered interval between them.
	if err := reader.ensureExactRangeResident(context.Background(), 3<<20, 4<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReaderAt(ctx).ReadAt(buf, 2<<20); err != nil {
		t.Fatal(err)
	}
	got := transport.callAt(2)
	if got.off != 2<<20 || got.length != 1<<20 {
		t.Fatalf("gap request = %+v, want [2 MiB, 3 MiB)", got)
	}
}

// TestReadAheadDoesNotOverlapInflightNeighbor verifies different starting
// offsets cannot cause overlapping bulk and foreground network requests.
func TestReadAheadDoesNotOverlapInflightNeighbor(t *testing.T) {
	started := make(chan fetchCall, 2)
	release := make(chan struct{})
	reader := NewPackReader("shared-inflight", 16<<20, TransportFunc(func(ctx context.Context, off int64, length int) ([]byte, error) {
		started <- fetchCall{off: off, length: length}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return make([]byte, length), nil
		}
	}), hash.RecommendedHashType)
	t.Cleanup(reader.Close)
	done := make(chan error, 2)

	// Keep a foreground request in flight beyond the bulk caller's target.
	go func() {
		_, err := reader.ReaderAt(t.Context()).ReadAt(make([]byte, 1), 8<<20)
		done <- err
	}()
	foreground := <-started
	go func() {
		ctx := block.WithReadAhead(t.Context(), 10<<20)
		_, err := reader.ReaderAt(ctx).ReadAt(make([]byte, 1), 0)
		done <- err
	}()
	bulk := <-started
	if bulk.off != 0 || int64(bulk.length) != foreground.off {
		t.Fatalf("bulk range %+v overlaps or leaves a gap before in-flight foreground %+v", bulk, foreground)
	}

	// Both callers complete from their disjoint requests.
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// TestReadAheadRespectsResidentBudget bounds speculative bulk windows.
func TestReadAheadRespectsResidentBudget(t *testing.T) {
	transport := &bytesTransport{data: make([]byte, 16<<20)}
	reader := NewPackReader("budgeted-bulk", int64(len(transport.data)), transport, hash.RecommendedHashType)
	t.Cleanup(reader.Close)
	reader.maxBytes = 2 << 20
	ctx := block.WithReadAhead(t.Context(), 10<<20)
	if _, err := reader.ReaderAt(ctx).ReadAt(make([]byte, 1), 0); err != nil {
		t.Fatal(err)
	}
	if got := transport.callAt(0).length; got != 2<<20 {
		t.Fatalf("budgeted range = %d, want 2 MiB", got)
	}
}

// TestPackLocalityReducesRelatedRangeReads compares identical content and
// reader policy while changing only physical order. The workload reads one
// eight-block file among 64 independent files, each containing 32 KiB chunks.
func TestPackLocalityReducesRelatedRangeReads(t *testing.T) {
	graph := packfile_order.NewGraph()
	items := make(map[string]packItem)
	refs := make([]*block.BlockRef, 64*8)
	for i := range refs {
		data := make([]byte, 32<<10)
		binary.LittleEndian.PutUint32(data, uint32(i))
		h, err := hash.Sum(hash.RecommendedHashType, data)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = block.NewBlockRef(h)
		items[h.MarshalString()] = packItem{h: h, data: data}
	}
	for i, ref := range refs {
		var children []*block.BlockRef
		if i%8 == 0 {
			children = refs[i+1 : i+8]
		}
		graph.Add(ref, children)
	}

	// Preserve exact content and count only payload fetches, after index load.
	measure := func(ordered []*block.BlockRef) (int, int64) {
		t.Helper()
		orderedItems := make([]packItem, 0, len(ordered))
		for _, ref := range ordered {
			orderedItems = append(orderedItems, items[ref.GetHash().MarshalString()])
		}
		data, bloom := packItems(t, orderedItems)
		transport := &bytesTransport{data: data}
		store := NewPackfileStore(func(id string, size int64) (*PackReader, error) {
			return NewPackReader(id, size, transport, hash.RecommendedHashType), nil
		}, newMemIndexCache())
		defer store.Close()
		store.SetTransportMinWindow(128 << 10)
		store.SetTransportQuantum(128 << 10)
		store.SetTransportMaxWindow(128 << 10)
		store.SetIndexPromotionEnabled(false)
		store.UpdateManifest([]*packfile.PackfileEntry{{
			Id: "layout", BloomFilter: bloom, BlockCount: uint64(len(ordered)), SizeBytes: uint64(len(data)),
		}})
		if exists, err := store.GetBlockExists(t.Context(), refs[0]); err != nil || !exists {
			t.Fatalf("load index: exists=%v err=%v", exists, err)
		}
		before := transport.callCount()
		for _, ref := range refs[:8] {
			got, found, err := store.GetBlock(t.Context(), ref)
			if err != nil || !found || !bytes.Equal(got, items[ref.GetHash().MarshalString()].data) {
				t.Fatalf("content changed: found=%v err=%v", found, err)
			}
		}
		var fetched int64
		for i := before; i < transport.callCount(); i++ {
			fetched += int64(transport.callAt(i).length)
		}
		return transport.callCount() - before, fetched
	}

	hashOrder, err := packfile_order.BlockRefs(t.Context(), nil, refs)
	if err != nil {
		t.Fatal(err)
	}
	structuralOrder, err := graph.Order(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hashCalls, hashBytes := measure(hashOrder)
	structuralCalls, structuralBytes := measure(structuralOrder)
	t.Logf("hash order: %d requests, %d bytes; structural order: %d requests, %d bytes; useful content: %d bytes", hashCalls, hashBytes, structuralCalls, structuralBytes, 8*(32<<10))
	if structuralCalls >= hashCalls || structuralBytes >= hashBytes {
		t.Fatal("structural layout did not improve related-content range reads")
	}
}
