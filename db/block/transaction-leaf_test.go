package block_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
)

// BenchmarkTransactionLeaf measures a complete one-block transaction, including
// encoding, hashing, and an in-memory storage batch.
func BenchmarkTransactionLeaf(b *testing.B) {
	store := block_mock.NewMockStore(0)
	body := block_mock.NewExample(strings.Repeat("block contents", 32))
	b.ReportAllocs()
	for b.Loop() {
		tx, cursor := block.NewTransaction(store, nil, nil, nil)
		cursor.SetBlock(body, true)
		if _, _, err := tx.Write(b.Context(), true); err != nil {
			b.Fatal(err)
		}
	}
}

// TestTransactionLeaf preserves hooks, transforms, readback, and borrowed-buffer
// ownership when there is no child node to schedule.
func TestTransactionLeaf(t *testing.T) {
	// Borrow the caller's buffer and transform the hook's final block content.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	buffer := block.NewBufferedStore(ctx, store)
	xfrm, err := transform_gzip.NewGzip(&transform_gzip.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx, cursor := block.NewTransaction(store, xfrm, nil, nil)
	tx.SetWriteBuffer(buffer)
	cursor.SetBlock(block_mock.NewExample("before hook"), true)
	cursor.SetPreWriteHook(func(body any) error {
		body.(*block_mock.Example).Msg = "after hook"
		return nil
	})
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// The transaction must leave the borrowed buffer's durability fence to its caller.
	if exists, err := store.GetBlockExists(ctx, ref); err != nil || exists {
		t.Fatalf("borrowed buffer drained early: exists=%v err=%v", exists, err)
	}
	if _, err := buffer.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	_, read := block.NewTransaction(store, xfrm, ref, nil)
	body, err := block_mock.UnmarshalExample(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if body.GetMsg() != "after hook" {
		t.Fatalf("stored content = %q", body.GetMsg())
	}
}

// TestTransactionLeafFailure propagates hook, storage, and cancellation errors.
func TestTransactionLeafFailure(t *testing.T) {
	for _, failure := range []string{"hook", "storage", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			// Select one failing boundary on the ordinary transaction write path.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			store := block_mock.NewMockStore(0)
			want := errors.New("hook failed")
			if failure == "storage" {
				store = block.NopStoreOps{}
				want = block.ErrBlockStoreUnavailable
			}
			if failure == "cancel" {
				cancel()
				want = context.Canceled
			}
			tx, cursor := block.NewTransaction(store, nil, nil, nil)
			cursor.SetBlock(block_mock.NewExample("leaf"), true)
			if failure == "hook" {
				cursor.SetPreWriteHook(func(any) error { return want })
			}

			// A failed write must never report a successfully written root.
			ref, _, err := tx.Write(ctx, true)
			if !errors.Is(err, want) {
				t.Fatalf("write error = %v, want %v", err, want)
			}
			if !ref.GetEmpty() {
				t.Fatal("failed write returned a root")
			}
		})
	}
}

// TestTransactionParentError joins sibling encoders after a parent rejects a ref.
func TestTransactionParentError(t *testing.T) {
	// Two siblings must both leave the parent-update lock after rejection.
	want := errors.New("parent rejected reference")
	tx, cursor := block.NewTransaction(block_mock.NewMockStore(0), nil, nil, nil)
	cursor.SetBlock(&rejectingParent{err: want}, true)
	cursor.FollowRef(1, nil).SetBlock(block_mock.NewExample("first"), true)
	cursor.FollowRef(2, nil).SetBlock(block_mock.NewExample("second"), true)
	if _, _, err := tx.Write(t.Context(), true); !errors.Is(err, want) {
		t.Fatalf("write error = %v, want %v", err, want)
	}
}

// rejectingParent exercises failure while applying a child's encoded reference.
type rejectingParent struct {
	// err is returned by every reference update.
	err error
}

// MarshalBlock encodes a nonempty root payload.
func (*rejectingParent) MarshalBlock() ([]byte, error) { return []byte{1}, nil }

// UnmarshalBlock accepts the root payload.
func (*rejectingParent) UnmarshalBlock([]byte) error { return nil }

// GetBlockRefs declares the two sibling dependencies.
func (*rejectingParent) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{1: nil, 2: nil}, nil
}

// GetBlockRefCtor constructs either sibling.
func (*rejectingParent) GetBlockRefCtor(uint32) block.Ctor { return block_mock.NewExampleBlock }

// ApplyBlockRef rejects a child update while its sibling may be encoding.
func (p *rejectingParent) ApplyBlockRef(uint32, *block.BlockRef) error { return p.err }

// _ is a type assertion
var (
	_ block.Block         = (*rejectingParent)(nil)
	_ block.BlockWithRefs = (*rejectingParent)(nil)
)
