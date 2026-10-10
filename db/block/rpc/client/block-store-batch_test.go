package block_rpc_client

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

// TestBlockStoreBatchBounds checks content, refs, and operation order across
// several packet-sized requests, including a tombstone between writes.
func TestBlockStoreBatchBounds(t *testing.T) {
	// Prepare large block writes with an intervening tombstone for the batch.
	var entries []*block.PutBatchEntry
	for i := range 12 {
		// Build a content reference for each distinct block payload.
		data := bytes.Repeat([]byte{byte(i)}, 1<<20)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Preserve write order with a tombstone after the sixth block.
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, Refs: []*block.BlockRef{ref}})
		if i == 5 {
			entries = append(entries, &block.PutBatchEntry{Ref: ref, Tombstone: true})
		}
	}

	// Inspect remote requests for packet bounds and unchanged entry order.
	seen, requests := 0, 0
	client := &testBlockStoreClient{batchHook: func(_ context.Context, req *block_rpc.PutBlockBatchRequest) error {
		// Require each remote request to fit within the packet limit.
		requests++
		if req.SizeVT() > 4<<20 {
			t.Fatalf("request contains %d bytes", req.SizeVT())
		}

		// Compare each received block operation with its original batch entry.
		for _, entry := range req.Entries {
			// Verify the block payload, tombstone, and outgoing references.
			want := entries[seen]
			if !entry.Ref.EqualVT(want.Ref) || !bytes.Equal(entry.Data, want.Data) || entry.Tombstone != want.Tombstone {
				t.Fatalf("request changed entry %d", seen)
			}
			if len(entry.Refs) != len(want.Refs) || len(want.Refs) != 0 && !entry.Refs[0].EqualVT(want.Refs[0]) {
				t.Fatalf("request changed outgoing refs at entry %d", seen)
			}
			seen++
		}

		return nil
	}}

	// Send the batch through the remote block store adapter.
	if _, err := NewBlockStore(client, 0, false).PutBlockBatch(t.Context(), entries); err != nil {
		t.Fatal(err)
	}

	// Require every entry to arrive across multiple bounded requests.
	if seen != len(entries) || requests < 2 {
		t.Fatalf("sent %d entries in %d requests", seen, requests)
	}
}

// TestBlockStoreBatchReportsExistence checks that per-entry existence survives
// splitting a batch across remote requests, in entry order.
func TestBlockStoreBatchReportsExistence(t *testing.T) {
	// Build entries that each fill a request, so they travel separately.
	var entries []*block.PutBatchEntry
	existing := make(map[string]bool)
	want := []bool{false, true, true, false}
	for i, stored := range want {
		data := bytes.Repeat([]byte{byte(i)}, 3<<20)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data})
		existing[string(data)] = stored
	}

	// The adapter returns the remote answers in entry order.
	client := &testBlockStoreClient{existing: existing}
	got, err := NewBlockStore(client, 0, false).PutBlockBatch(t.Context(), entries)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("existed = %v, want %v", got, want)
	}
}

// TestBlockStoreBatchStops checks that cancellation or failure after a
// committed prefix prevents later requests from being sent.
func TestBlockStoreBatchStops(t *testing.T) {
	// Exercise remote failure and cancellation after the first batch request.
	for _, canceled := range []bool{false, true} {
		// Label the batch stop condition for the subtest.
		name := "error"
		if canceled {
			name = "cancel"
		}

		// Verify the batch stops after the first request under either condition.
		t.Run(name, func(t *testing.T) {
			// Prepare a cancellable batch context and its expected failure.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			want := errors.New("write failed")
			if canceled {
				want = context.Canceled
			}

			// Stop the batch by canceling its context or failing the remote write.
			client := &testBlockStoreClient{batchHook: func(context.Context, *block_rpc.PutBlockBatchRequest) error {
				// Record the request and apply the selected stop condition.
				calls++
				if canceled {
					cancel()
					return nil
				}

				return want
			}}

			// Submit two entries that require separate remote requests.
			entry := &block.PutBatchEntry{Data: make([]byte, 3<<20)}
			_, err := NewBlockStore(client, 0, false).PutBlockBatch(ctx, []*block.PutBatchEntry{entry, entry})

			// Require the selected failure and no request after the first.
			if !errors.Is(err, want) || calls != 1 {
				t.Fatalf("calls=%d err=%v, want one call and %v", calls, err, want)
			}
		})
	}
}
