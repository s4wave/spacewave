package kvtx_block_iavl

import (
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestIteratorStackGrowth visits every key once when traversal grows its stack.
// A small initial capacity exercises the same growth as a deeply nested tree.
func TestIteratorStackGrowth(t *testing.T) {
	// Build a tree deep enough to grow the iterator stack.
	ctx := t.Context()
	_, cursor, err := BuildTree(&block.NopStoreOps{}, nil, nil, func(yield func([]byte, *block.BlockRef) bool) {
		for i := range 64 {
			if !yield([]byte(fmt.Sprintf("key-%02d", i)), nil) {
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Open the constructed tree for iterator checks.
	tx, err := NewTx(ctx, cursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Exercise each traversal direction and starting position.
	for _, reverse := range []bool{false, true} {
		for _, seek := range []string{"next", "", "key-32"} {
			t.Run(fmt.Sprintf("reverse=%v/seek=%s", reverse, seek), func(t *testing.T) {
				// Create an iterator with a deliberately small stack capacity.
				iterator := NewIterator(ctx, tx, nil, true, reverse)
				defer iterator.Close()
				iterator.stack = iterator.stack[:1:1]

				// Choose the expected first key and traversal direction.
				start, step := 0, 1
				if reverse {
					start, step = 63, -1
				}

				// Position the iterator by advancing or seeking.
				if seek == "next" {
					iterator.Next()
				} else {
					if seek != "" {
						start = 32
					}
					if err := iterator.Seek([]byte(seek)); err != nil {
						t.Fatal(err)
					}
				}

				// Verify every remaining key while traversal grows the stack.
				for i := start; i >= 0 && i < 64; i += step {
					want := fmt.Sprintf("key-%02d", i)
					if !iterator.Valid() || string(iterator.Key()) != want {
						t.Fatalf("key=%q want=%q err=%v", iterator.Key(), want, iterator.Err())
					}
					iterator.Next()
				}
				if iterator.Valid() || iterator.Err() != nil {
					// Verify the iterator finishes without a key or error.
					t.Fatalf("iteration did not finish: key=%q err=%v", iterator.Key(), iterator.Err())
				}
			})
		}
	}
}
