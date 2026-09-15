package block_rpc_client

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

// TestBlockStoreBatchBounds checks content, refs, and operation order across
// several packet-sized requests, including a tombstone between writes.
func TestBlockStoreBatchBounds(t *testing.T) {
	var entries []*block.PutBatchEntry
	for i := range 12 {
		data := bytes.Repeat([]byte{byte(i)}, 1<<20)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, Refs: []*block.BlockRef{ref}})
		if i == 5 {
			entries = append(entries, &block.PutBatchEntry{Ref: ref, Tombstone: true})
		}
	}

	seen, requests := 0, 0
	client := &testBlockStoreClient{batchHook: func(_ context.Context, req *block_rpc.PutBlockBatchRequest) error {
		requests++
		if req.SizeVT() > 4<<20 {
			t.Fatalf("request contains %d bytes", req.SizeVT())
		}
		for _, entry := range req.Entries {
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
	if err := NewBlockStore(client, 0, false).PutBlockBatch(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if seen != len(entries) || requests < 2 {
		t.Fatalf("sent %d entries in %d requests", seen, requests)
	}
}

// TestBlockStoreBatchStops checks that cancellation or failure after a
// committed prefix prevents later requests from being sent.
func TestBlockStoreBatchStops(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "error"
		if canceled {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			want := errors.New("write failed")
			if canceled {
				want = context.Canceled
			}
			client := &testBlockStoreClient{batchHook: func(context.Context, *block_rpc.PutBlockBatchRequest) error {
				calls++
				if canceled {
					cancel()
					return nil
				}
				return want
			}}
			entry := &block.PutBatchEntry{Data: make([]byte, 3<<20)}
			err := NewBlockStore(client, 0, false).PutBlockBatch(ctx, []*block.PutBatchEntry{entry, entry})
			if !errors.Is(err, want) || calls != 1 {
				t.Fatalf("calls=%d err=%v, want one call and %v", calls, err, want)
			}
		})
	}
}
