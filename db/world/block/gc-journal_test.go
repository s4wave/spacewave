package world_block

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

type gcJournalTestTree struct {
	values         map[string][]byte
	scanPrefixKeys int
	scanPrefix     int
	scanValues     int
}

func newGCJournalTestTree() *gcJournalTestTree {
	return &gcJournalTestTree{values: make(map[string][]byte)}
}

func (t *gcJournalTestTree) Size(context.Context) (uint64, error) {
	return uint64(len(t.values)), nil
}

func (t *gcJournalTestTree) Get(_ context.Context, key []byte) ([]byte, bool, error) {
	val, ok := t.values[string(key)]
	if !ok {
		return nil, false, nil
	}
	return bytes.Clone(val), true, nil
}

func (t *gcJournalTestTree) Exists(_ context.Context, key []byte) (bool, error) {
	_, ok := t.values[string(key)]
	return ok, nil
}

func (t *gcJournalTestTree) Set(_ context.Context, key, value []byte) error {
	t.values[string(key)] = bytes.Clone(value)
	return nil
}

func (t *gcJournalTestTree) Delete(_ context.Context, key []byte) error {
	delete(t.values, string(key))
	return nil
}

func (t *gcJournalTestTree) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	t.scanPrefix++
	keys := t.sortedKeys(prefix)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		t.scanValues++
		if err := cb([]byte(key), bytes.Clone(t.values[key])); err != nil {
			return err
		}
	}
	return nil
}

func (t *gcJournalTestTree) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	t.scanPrefixKeys++
	keys := t.sortedKeys(prefix)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := cb([]byte(key)); err != nil {
			return err
		}
	}
	return nil
}

func (t *gcJournalTestTree) Iterate(context.Context, []byte, bool, bool) kvtx.Iterator {
	return nil
}

func (t *gcJournalTestTree) GetCursor() *block.Cursor {
	return nil
}

func (t *gcJournalTestTree) GetCursorAtKey(context.Context, []byte) (*block.Cursor, error) {
	return nil, nil
}

func (t *gcJournalTestTree) SetCursorAtKey(context.Context, []byte, *block.Cursor, bool) error {
	return nil
}

func (t *gcJournalTestTree) DeleteCursorAtKey(context.Context, []byte) (*block.Cursor, error) {
	return nil, nil
}

func (t *gcJournalTestTree) BlockIterate(context.Context, []byte, bool, bool) kvtx.BlockIterator {
	return nil
}

func (t *gcJournalTestTree) Commit(context.Context) error {
	return nil
}

func (t *gcJournalTestTree) Discard() {}

func (t *gcJournalTestTree) sortedKeys(prefix []byte) []string {
	keys := make([]string, 0, len(t.values))
	for key := range t.values {
		if prefix == nil || bytes.HasPrefix([]byte(key), prefix) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func TestGCJournalReadsSequenceMetadataWithoutScan(t *testing.T) {
	ctx := context.Background()
	tree := newGCJournalTestTree()
	storeGCJournalSeqForTest(t, tree, 42)
	storeGCJournalCountForTest(t, tree, 42)

	journal, err := newGCJournal(ctx, tree, false)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Entries() != 42 {
		t.Fatalf("entries = %d, want 42", journal.Entries())
	}
	if tree.scanPrefixKeys != 0 {
		t.Fatalf("metadata load scanned journal keys %d times", tree.scanPrefixKeys)
	}
}

func TestGCJournalAppendStoresSequenceMetadata(t *testing.T) {
	ctx := context.Background()
	tree := newGCJournalTestTree()
	journal, err := newGCJournal(ctx, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if tree.scanPrefixKeys != 1 {
		t.Fatalf("missing metadata should scan once, scanned %d times", tree.scanPrefixKeys)
	}

	err = journal.Append(ctx, []block_gc.RefEdge{{Subject: "a", Object: "b"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	seqData, found, err := tree.Get(ctx, gcJournalSeqKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected sequence metadata after append")
	}
	if got := binary.BigEndian.Uint64(seqData); got != 1 {
		t.Fatalf("metadata sequence = %d, want 1", got)
	}

	tree.scanPrefixKeys = 0
	reloaded, err := newGCJournal(ctx, tree, false)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Entries() != 1 {
		t.Fatalf("reloaded entries = %d, want 1", reloaded.Entries())
	}
	if tree.scanPrefixKeys != 0 {
		t.Fatalf("metadata reload scanned journal keys %d times", tree.scanPrefixKeys)
	}
}

func TestGCJournalIterateAndClearIgnoreMetadata(t *testing.T) {
	ctx := context.Background()
	tree := newGCJournalTestTree()
	journal, err := newGCJournal(ctx, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(ctx, []block_gc.RefEdge{{Subject: "a", Object: "b"}}, nil); err != nil {
		t.Fatal(err)
	}

	var count int
	err = journal.Iterate(ctx, func(adds, removes []block_gc.RefEdge) error {
		count++
		if len(adds) != 1 || adds[0].Subject != "a" || adds[0].Object != "b" {
			t.Fatalf("unexpected adds: %#v", adds)
		}
		if len(removes) != 0 {
			t.Fatalf("unexpected removes: %#v", removes)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("iterated %d entries, want 1", count)
	}

	if err := journal.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if journal.Entries() != 0 {
		t.Fatalf("entries after clear = %d, want 0", journal.Entries())
	}
	if _, found, err := tree.Get(ctx, gcJournalSeqKey); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("expected clear to remove sequence metadata")
	}
}

func TestGCJournalTakeDeleteAppliedPreservesPendingEntries(t *testing.T) {
	ctx := context.Background()
	tree := newGCJournalTestTree()
	journal, err := newGCJournal(ctx, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		err := journal.Append(ctx, []block_gc.RefEdge{{
			Subject: "subject-" + string(rune('a'+i)),
			Object:  "object-" + string(rune('a'+i)),
		}}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	entries, err := journal.Take(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("take entries = %d, want 2", len(entries))
	}
	if tree.scanValues != 3 {
		t.Fatalf("scanned values = %d, want 3", tree.scanValues)
	}
	if err := journal.DeleteApplied(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if journal.Entries() != 1 {
		t.Fatalf("pending entries = %d, want 1", journal.Entries())
	}

	reloaded, err := newGCJournal(ctx, tree, false)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Entries() != 1 {
		t.Fatalf("reloaded pending entries = %d, want 1", reloaded.Entries())
	}
	remaining, err := reloaded.Take(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining entries = %d, want 1", len(remaining))
	}
	if got := remaining[0].adds[0].Subject; got != "subject-c" {
		t.Fatalf("remaining subject = %q, want subject-c", got)
	}
}

func TestGCJournalTakeSplitsOversizedFirstEntry(t *testing.T) {
	ctx := context.Background()
	tree := newGCJournalTestTree()
	journal, err := newGCJournal(ctx, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(ctx, []block_gc.RefEdge{
		{Subject: "subject-a", Object: "object-a"},
		{Subject: "subject-b", Object: "object-b"},
		{Subject: "subject-c", Object: "object-c"},
	}, []block_gc.RefEdge{
		{Subject: "subject-d", Object: "object-d"},
		{Subject: "subject-e", Object: "object-e"},
	}); err != nil {
		t.Fatal(err)
	}

	entries, err := journal.Take(ctx, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("take entries = %d, want 1", len(entries))
	}
	if got := len(entries[0].adds) + len(entries[0].removes); got != 2 {
		t.Fatalf("taken edges = %d, want 2", got)
	}
	if err := journal.DeleteApplied(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if journal.Entries() != 1 {
		t.Fatalf("pending entries = %d, want oversized entry retained", journal.Entries())
	}

	remaining, err := journal.Take(ctx, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining entries = %d, want 1", len(remaining))
	}
	if got := len(remaining[0].adds) + len(remaining[0].removes); got != 3 {
		t.Fatalf("remaining edges = %d, want 3", got)
	}
	if remaining[0].adds[0].Subject != "subject-c" {
		t.Fatalf("first remaining add = %q, want subject-c", remaining[0].adds[0].Subject)
	}
}

func TestGCJournalAppendSplitsAtDefaultEdgeLimit(t *testing.T) {
	ctx := context.Background()
	tree := newGCJournalTestTree()
	journal, err := newGCJournal(ctx, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	adds := make([]block_gc.RefEdge, defaultGCJournalReconcileEdgeLimit+1)
	for i := range adds {
		adds[i] = block_gc.RefEdge{
			Subject: "subject-" + strconv.Itoa(i),
			Object:  "object-" + strconv.Itoa(i),
		}
	}
	if err := journal.Append(ctx, adds, nil); err != nil {
		t.Fatal(err)
	}
	if journal.Entries() != 2 {
		t.Fatalf("pending entries = %d, want 2", journal.Entries())
	}

	entries, err := journal.Take(ctx, 64, defaultGCJournalReconcileEdgeLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("taken entries = %d, want 1", len(entries))
	}
	if got := len(entries[0].adds) + len(entries[0].removes); got != int(defaultGCJournalReconcileEdgeLimit) {
		t.Fatalf("taken edges = %d, want %d", got, defaultGCJournalReconcileEdgeLimit)
	}
}

func storeGCJournalSeqForTest(t *testing.T, tree *gcJournalTestTree, seq uint64) {
	t.Helper()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], seq)
	if err := tree.Set(context.Background(), gcJournalSeqKey, buf[:]); err != nil {
		t.Fatal(err)
	}
}

func storeGCJournalCountForTest(t *testing.T, tree *gcJournalTestTree, count uint64) {
	t.Helper()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], count)
	if err := tree.Set(context.Background(), gcJournalCountKey, buf[:]); err != nil {
		t.Fatal(err)
	}
}

// _ is a type assertion.
var _ kvtx.BlockTx = (*gcJournalTestTree)(nil)
