package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// TestInlineValuesRetainExistingDataAndCursors verifies old external values,
// packed values, retained readers, and cursor reuse across page replacement.
func TestInlineValuesRetainExistingDataAndCursors(t *testing.T) {
	ctx := t.Context()
	store := newOkraTestStore()
	oldRoot := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.Set(ctx, []byte("old"), []byte("external value")); err != nil {
			t.Fatal(err)
		}
	})
	btx, root := block.NewTransaction(store, nil, oldRoot, nil)
	tx, err := NewTxWithInlineValues(ctx, root, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	values := map[string][]byte{
		"old":   []byte("external value"),
		"small": []byte("packed value"),
		"empty": {},
		"large": bytes.Repeat([]byte("large"), inlineValueLimit),
	}
	for _, key := range []string{"small", "empty", "large"} {
		if err := tx.Set(ctx, []byte(key), values[key]); err != nil {
			t.Fatal(err)
		}
	}
	reader := tx.BlockIterate(ctx, []byte("small"), true, false)
	defer reader.Close()
	if !reader.Next() {
		t.Fatal("missing packed value", reader.Err())
	}
	saved := reader.ValueCursor()
	if err := tx.Set(ctx, []byte("small"), []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	got, err := blob.FetchToBytes(ctx, saved)
	if err != nil || !bytes.Equal(got, values["small"]) {
		t.Fatalf("retained cursor: %q, %v", got, err)
	}
	if err := tx.SetCursorAtKey(ctx, []byte("copy"), saved, true); err != nil {
		t.Fatal(err)
	}
	values["copy"] = values["small"]
	values["small"] = []byte("replacement")
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	read := openOkraRoot(t, ctx, store, ref, false)
	defer read.Discard()
	for key, want := range values {
		got, found, err := read.Get(ctx, []byte(key))
		if err != nil || !found || !bytes.Equal(got, want) {
			t.Fatalf("%s readback: %q, %v, %v", key, got, found, err)
		}
	}
	// An inline cursor from a separate read transaction must become a real
	// value when another tree adopts it; its containing page ref is not a value.
	inline, err := read.GetCursorAtKey(ctx, []byte("small"))
	if err != nil {
		t.Fatal(err)
	}
	copied := writeMutatedOkraRoot(t, ctx, store, func(target *Tx) {
		if err := target.SetCursorAtKey(ctx, []byte("adopted"), inline, true); err != nil {
			t.Fatal(err)
		}
	})
	adopted := openOkraRoot(t, ctx, store, copied, false)
	defer adopted.Discard()
	got, found, err := adopted.Get(ctx, []byte("adopted"))
	if err != nil || !found || !bytes.Equal(got, values["small"]) {
		t.Fatalf("adopted inline cursor: %q, %v, %v", got, found, err)
	}
}

// TestInlineReplaceAllMatchesSet preserves deterministic packed roots across
// bulk construction and incremental insertion, including empty and large values.
func TestInlineReplaceAllMatchesSet(t *testing.T) {
	ctx := t.Context()
	store := newOkraTestStore()
	values := func(yield func([]byte, []byte) bool) {
		for i := range 512 {
			key := []byte(fmt.Sprintf("key-%04d", i))
			value := bytes.Repeat([]byte{byte(i % 256)}, i)
			if !yield(key, value) {
				return
			}
		}
	}
	var want *block.BlockRef
	for _, bulk := range []bool{false, true} {
		ref := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
			tx.inlineValues = true
			if bulk {
				if err := tx.ReplaceAll(ctx, values); err != nil {
					t.Fatal(err)
				}
			} else {
				for key, value := range values {
					if err := tx.Set(ctx, key, value); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
		if want == nil {
			want = ref
		} else if !want.EqualsRef(ref) {
			t.Fatal("packed bulk root differs from incremental root")
		}
		read := openOkraRoot(t, ctx, store, ref, false)
		for key, want := range values {
			got, found, err := read.Get(ctx, key)
			if err != nil || !found || !bytes.Equal(got, want) {
				t.Fatalf("%s readback failed: %v", key, err)
			}
		}
		read.Discard()
	}
}
