package block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestTransactionWriteAtRoots writes selected values once, leaving the
// enclosing root and unselected values for the later transaction commit.
func TestTransactionWriteAtRoots(t *testing.T) {
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	tx, root := block.NewTransaction(store, nil, nil, nil)
	root.SetBlock(block_mock.NewExample("enclosing"), true)
	first := root.FollowRef(1, nil)
	first.SetBlock(block_mock.NewExample("first"), true)
	second := root.FollowRef(2, nil)
	second.SetBlock(block_mock.NewExample("second"), true)
	omitted := root.FollowRef(3, nil)
	omitted.SetBlock(block_mock.NewExample("omitted"), true)
	writes := 0
	first.SetPreWriteHook(func(any) error { writes++; return nil })

	// Repeated selection must not duplicate a shared subtree's encoding.
	if err := tx.WriteAtRoots(ctx, []*block.Cursor{first, second, first}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || !root.GetRef().GetEmpty() || !omitted.GetRef().GetEmpty() {
		t.Fatalf("writes=%d root=%v omitted=%v", writes, root.GetRef(), omitted.GetRef())
	}
	for index, cursor := range []*block.Cursor{first, second} {
		_, read := block.NewTransaction(store, nil, cursor.GetRef(), nil)
		got, err := block_mock.UnmarshalExample(ctx, read)
		if err != nil || got.GetMsg() != []string{"first", "second"}[index] {
			t.Fatalf("selected value=%v err=%v", got, err)
		}
	}
	if _, _, err := tx.Write(ctx, true); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || root.GetRef().GetEmpty() || omitted.GetRef().GetEmpty() {
		t.Fatal("remaining transaction did not retain selected values")
	}
}

// TestTransactionWriteAtRootsFailure preserves cancellation and hook errors.
func TestTransactionWriteAtRootsFailure(t *testing.T) {
	for _, cancelWrite := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		tx, cursor := block.NewTransaction(block_mock.NewMockStore(0), nil, nil, nil)
		cursor.SetBlock(block_mock.NewExample("value"), true)
		want := errors.New("hook rejected value")
		cursor.SetPreWriteHook(func(any) error { return want })
		if cancelWrite {
			want = context.Canceled
			cancel()
		}
		if err := tx.WriteAtRoots(ctx, []*block.Cursor{cursor}); !errors.Is(err, want) {
			t.Fatalf("error=%v want=%v", err, want)
		}
	}
}
