//go:build js

package block_gc_wal

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/aperturerobotics/hydra/opfs"
)

func TestWALWriteRead(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-wal-rw", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-wal-rw", true) //nolint

	w := NewWriter(dir, "test-wal-rw", "test-wal-rw|order", "test-wal-rw|stw")

	// Write two entries.
	err = w.Append(context.Background(), []*RefEdge{
		{Subject: "a", Object: "b"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = w.Append(context.Background(), nil, []*RefEdge{
		{Subject: "c", Object: "d"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read back.
	entries, files, err := ReadWAL(dir, "test-wal-rw")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}

	// Verify sequence order.
	if entries[0].GetSequence() >= entries[1].GetSequence() {
		t.Errorf("entries not in sequence order: %d >= %d",
			entries[0].GetSequence(), entries[1].GetSequence())
	}

	// Verify content.
	if len(entries[0].GetAdds()) != 1 || entries[0].GetAdds()[0].GetSubject() != "a" {
		t.Errorf("entry 0 adds mismatch")
	}
	if len(entries[1].GetRemoves()) != 1 || entries[1].GetRemoves()[0].GetSubject() != "c" {
		t.Errorf("entry 1 removes mismatch")
	}

	// Delete first entry.
	if err := DeleteWALEntry(dir, files[0]); err != nil {
		t.Fatal(err)
	}

	// Read again, should have 1 entry.
	entries2, files2, err := ReadWAL(dir, "test-wal-rw")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries2) != 1 {
		t.Fatalf("got %d entries after delete, want 1", len(entries2))
	}
	if entries2[0].GetSequence() != entries[1].GetSequence() {
		t.Errorf("remaining entry has wrong sequence")
	}
	_ = files2
}

func TestWALConcurrentAppend(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-wal-conc", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-wal-conc", true) //nolint

	w := NewWriter(dir, "test-wal-conc", "test-wal-conc|order", "test-wal-conc|stw")

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(idx int) {
			defer wg.Done()
			err := w.Append(context.Background(), []*RefEdge{
				{Subject: "node", Object: "child-" + string(rune('a'+idx))},
			}, nil)
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if t.Failed() {
		return
	}

	entries, _, err := ReadWAL(dir, "test-wal-conc")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != n {
		t.Fatalf("got %d entries, want %d", len(entries), n)
	}

	// Verify sequences are unique and monotonically increasing.
	seqs := make([]uint64, len(entries))
	for i, e := range entries {
		seqs[i] = e.GetSequence()
	}
	if !sort.SliceIsSorted(seqs, func(i, j int) bool { return seqs[i] < seqs[j] }) {
		t.Errorf("sequences not sorted: %v", seqs)
	}
	seen := make(map[uint64]bool)
	for _, s := range seqs {
		if seen[s] {
			t.Errorf("duplicate sequence %d", s)
		}
		seen[s] = true
	}
}

func TestWALEmptyAppend(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-wal-empty", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-wal-empty", true) //nolint

	w := NewWriter(dir, "test-wal-empty", "test-wal-empty|order", "test-wal-empty|stw")

	// Empty append should be a no-op.
	if err := w.Append(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}

	entries, _, err := ReadWAL(dir, "test-wal-empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries for empty append, want 0", len(entries))
	}
}
