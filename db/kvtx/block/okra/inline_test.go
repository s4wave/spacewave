package kvtx_block_okra

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// TestInlineValuesRetainExistingDataAndCursors verifies old external values,
// packed values, retained readers, and cursor reuse across page replacement.
func TestInlineValuesRetainExistingDataAndCursors(t *testing.T) {
	// Publish an existing external value before enabling inline values.
	ctx := t.Context()
	store := newOkraTestStore()
	oldRoot := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		if err := tx.Set(ctx, []byte("old"), []byte("external value")); err != nil {
			t.Fatal(err)
		}
	})

	// Open the existing tree with the inline value layout.
	btx, root := block.NewTransaction(store, nil, oldRoot, nil)
	tx, err := NewTxWithInlineValues(ctx, root, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Insert small, empty and large values alongside the external value.
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

	// Retain the small inline value through an iterator cursor.
	reader := tx.BlockIterate(ctx, []byte("small"), true, false)
	defer reader.Close()
	if !reader.Next() {
		t.Fatal("missing packed value", reader.Err())
	}
	saved := reader.ValueCursor()

	// Replace the small value while retaining its original cursor.
	if err := tx.Set(ctx, []byte("small"), []byte("replacement")); err != nil {
		t.Fatal(err)
	}

	// Require the retained cursor to preserve the original small value.
	got, err := blob.FetchToBytes(ctx, saved)
	if err != nil || !bytes.Equal(got, values["small"]) {
		t.Fatalf("retained cursor: %q, %v", got, err)
	}

	// Copy the retained value cursor into a second tree entry.
	if err := tx.SetCursorAtKey(ctx, []byte("copy"), saved, true); err != nil {
		t.Fatal(err)
	}
	values["copy"] = values["small"]
	values["small"] = []byte("replacement")

	// Commit and publish the tree with its copied and replaced values.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Require all expected values to read back from the published tree. The
	// reader is read-only, so adopting its inline cursor below must still write.
	read := openOkraRoot(t, ctx, store, ref, false)
	defer read.Discard()
	read.bcs.GetTransaction().SetReadOnly()
	for key, want := range values {
		got, found, err := read.Get(ctx, []byte(key))
		if err != nil || !found || !bytes.Equal(got, want) {
			t.Fatalf("%s readback: %q, %v, %v", key, got, found, err)
		}
	}

	// An inline cursor from a separate read transaction must become a real
	// value when another tree adopts it. Its containing page ref is not a
	// value.
	inline, err := read.GetCursorAtKey(ctx, []byte("small"))
	if err != nil {
		t.Fatal(err)
	}

	// Adopt the inline cursor into a separate target tree.
	copied := writeMutatedOkraRoot(t, ctx, store, func(target *Tx) {
		if err := target.SetCursorAtKey(ctx, []byte("adopted"), inline, true); err != nil {
			t.Fatal(err)
		}
	})

	// Open the target tree containing the adopted inline value.
	adopted := openOkraRoot(t, ctx, store, copied, false)
	defer adopted.Discard()

	// Require the adopted inline value to retain its original bytes.
	got, found, err := adopted.Get(ctx, []byte("adopted"))
	if err != nil || !found || !bytes.Equal(got, values["small"]) {
		t.Fatalf("adopted inline cursor: %q, %v, %v", got, found, err)
	}
}

// TestInlineReplaceAllMatchesSet preserves deterministic packed roots across
// bulk construction and incremental insertion, including empty and large values.
func TestInlineReplaceAllMatchesSet(t *testing.T) {
	// Create a sorted stream of empty, small and Blob-sized values, on a
	// store that records the refs each write declares.
	ctx := t.Context()
	store := &refGraphStore{StoreOps: newOkraTestStore()}
	const count = 4096
	key := func(i int) []byte { return []byte(fmt.Sprintf("key-%05d", i)) }
	values := func(yield func([]byte, []byte) bool) {
		for i := range count {
			if !yield(key(i), bytes.Repeat([]byte{byte(i % 256)}, i%300)) {
				return
			}
		}
	}

	// Build the tree one page per block and several levels per block.
	var rootHash []byte
	blocks := make(map[uint32]int)
	for _, levels := range []uint32{0, 2, 3} {
		// Require the bulk and incremental builds to agree on every block.
		var want *block.BlockRef
		for _, bulk := range []bool{false, true} {
			ref := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
				tx.inlineValues = true
				tx.levelsPerBlock = levels
				if bulk {
					if err := tx.ReplaceAll(ctx, values); err != nil {
						t.Fatal(err)
					}
					return
				}
				for key, value := range values {
					if err := tx.Set(ctx, key, value); err != nil {
						t.Fatal(err)
					}
				}
			})
			if want == nil {
				want = ref
			} else if !want.EqualsRef(ref) {
				t.Fatalf("levels %d: bulk root differs from incremental root", levels)
			}
		}

		// Require the layout to keep the tree's hash and record its levels.
		read := openOkraRoot(t, ctx, store, want, false)
		if got := read.root.GetLevelsPerBlock(); got != levels {
			t.Fatalf("levels %d: root records %d", levels, got)
		}
		if rootHash == nil {
			rootHash = read.root.GetRootHash()
		} else if !bytes.Equal(rootHash, read.root.GetRootHash()) {
			t.Fatalf("levels %d: root hash depends on the layout", levels)
		}
		read.Discard()

		// Require both decoded forms of every page block to list the refs
		// its write recorded, embedded pages' refs included.
		store.assertPageRefs(t, ctx, want)

		// Copy only the blocks the recorded refs reach, as a collector keeps
		// them, and require every value to read back from the copy alone.
		copied, n := store.copyReachable(t, ctx, want)
		blocks[levels] = n
		read = openOkraRoot(t, ctx, copied, want, false)
		for key, value := range values {
			got, found, err := read.Get(ctx, key)
			if err != nil || !found || !bytes.Equal(got, value) {
				t.Fatalf("levels %d: %s readback from the reachable copy failed: %v", levels, key, err)
			}
		}
		read.Discard()

		// Delete every third key in place, collapsing and merging pages, and
		// require the result to match a rebuild of the remaining keys.
		btx, rootCursor := block.NewTransaction(store, nil, want, nil)
		edit, err := NewTxWithInlineValues(ctx, rootCursor, nil, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i += 3 {
			if err := edit.Delete(ctx, key(i)); err != nil {
				t.Fatal(err)
			}
		}
		if err := edit.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		edited, _, err := btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
			tx.inlineValues = true
			tx.levelsPerBlock = levels
			err := tx.ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
				i := 0
				for key, value := range values {
					i++
					if (i-1)%3 == 0 {
						continue
					}
					if !yield(key, value) {
						return
					}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
		})
		if !edited.EqualsRef(rebuilt) {
			t.Fatalf("levels %d: deleting in place differs from a rebuild", levels)
		}
		store.assertPageRefs(t, ctx, edited)
		copied, _ = store.copyReachable(t, ctx, edited)
		read = openOkraRoot(t, ctx, copied, edited, false)
		for i := range count {
			_, found, err := read.Get(ctx, key(i))
			if err != nil || found != (i%3 != 0) {
				t.Fatalf("levels %d: key %d after deletes: found %v, %v", levels, i, found, err)
			}
		}
		read.Discard()
	}

	// Require each added level per block to store the tree in fewer blocks.
	if blocks[2] >= blocks[0] || blocks[3] >= blocks[2] {
		t.Fatalf("blocks by levels per block = %v", blocks)
	}
}

// refGraphStore records the outgoing refs each write declares, the edges a
// collector follows to decide which blocks to keep.
type refGraphStore struct {
	block.StoreOps
	mu   sync.Mutex
	refs map[string][]*block.BlockRef
}

func (s *refGraphStore) record(ref *block.BlockRef, refs []*block.BlockRef, known bool) {
	if ref == nil || !known {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refs == nil {
		s.refs = make(map[string][]*block.BlockRef)
	}
	s.refs[ref.MarshalString()] = refs
}

func (s *refGraphStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, exists, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err == nil {
		s.record(ref, opts.GetRefs(), opts.GetRefsKnown())
	}
	return ref, exists, err
}

func (s *refGraphStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	exists, err := s.StoreOps.PutBlockBatch(ctx, entries)
	if err == nil {
		for _, entry := range entries {
			s.record(entry.Ref, entry.Refs, entry.RefsKnown)
		}
	}
	return exists, err
}

// copyReachable copies the blocks root reaches through the recorded refs into
// a new store, and returns it with the number of blocks copied.
func (s *refGraphStore) copyReachable(t *testing.T, ctx context.Context, root *block.BlockRef) (block.StoreOps, int) {
	t.Helper()
	out := newOkraTestStore()
	seen := make(map[string]bool)
	stack := []*block.BlockRef{root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		id := ref.MarshalString()
		if seen[id] {
			continue
		}
		seen[id] = true
		data, found, err := s.GetBlock(ctx, ref)
		if err != nil || !found {
			t.Fatalf("reachable block %s missing: %v", id, err)
		}
		if _, _, err := out.PutBlock(ctx, data, nil); err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		stack = append(stack, s.refs[id]...)
		s.mu.Unlock()
	}
	return out, len(seen)
}

// assertPageRefs walks the Okra pages under the root block at ref and
// requires the refs ExtractBlockRefs finds in each page block, decoded as a
// Page and as a flat page, to equal the refs its write recorded.
func (s *refGraphStore) assertPageRefs(t *testing.T, ctx context.Context, ref *block.BlockRef) {
	t.Helper()
	read := func(ref *block.BlockRef) []byte {
		data, found, err := s.GetBlock(ctx, ref)
		if err != nil || !found {
			t.Fatalf("block %s missing: %v", ref.MarshalString(), err)
		}
		return data
	}
	names := func(refs []*block.BlockRef) []string {
		out := make([]string, len(refs))
		for i, ref := range refs {
			out[i] = ref.MarshalString()
		}
		slices.Sort(out)
		return out
	}
	root := &Root{}
	if err := root.UnmarshalVT(read(ref)); err != nil {
		t.Fatal(err)
	}
	stack := []*block.BlockRef{root.GetRootPageRef()}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		data := read(ref)
		page := &Page{}
		if err := page.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		flat := &flatPage{}
		if err := flat.UnmarshalDecodedBlock(data); err != nil {
			t.Fatal(err)
		}
		pageRefs, err := block.ExtractBlockRefs(page)
		if err != nil {
			t.Fatal(err)
		}
		flatRefs, err := block.ExtractBlockRefs(flat)
		if err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		recorded := s.refs[ref.MarshalString()]
		s.mu.Unlock()
		want := names(recorded)
		if got := names(pageRefs); !slices.Equal(got, want) {
			t.Fatalf("page block %s: Page refs %d, recorded %d", ref.MarshalString(), len(got), len(want))
		}
		if got := names(flatRefs); !slices.Equal(got, want) {
			t.Fatalf("page block %s: flat page refs %d, recorded %d", ref.MarshalString(), len(got), len(want))
		}
		for pg := range page.BlockPages() {
			for r := range pg.Refs() {
				if r.Child {
					stack = append(stack, r.Ref)
				}
			}
		}
	}
}
