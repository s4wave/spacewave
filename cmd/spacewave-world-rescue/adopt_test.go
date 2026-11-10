//go:build !js && !wasip1 && !windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// checkAdoptDir applies the checks of hyperline-backend world-adopt-packs
// (backend/world-adopt-packs.go, adoptWorldPack) to every pack in dir, and
// checks dir holds only complete pack and entry pairs: no subdirectory and
// no pack under its final name without its entry. It returns the packs and
// their total key count.
func checkAdoptDir(t *testing.T, dir string) (packs, keys int) {
	t.Helper()
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		if n.IsDir() {
			t.Fatalf("subdirectory %s in the adopt layout", n.Name())
		}
		have[n.Name()] = true
	}
	for _, n := range names {
		name := n.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, adoptPartSuffix) || name == "rescue.ledger" || name == "refs.txt" {
			continue
		}

		// A pack has its entry, and an entry its pack.
		id, isEntry := strings.CutSuffix(name, adoptEntrySuffix)
		if !isEntry {
			if !have[name+adoptEntrySuffix] {
				t.Fatalf("pack %s has no entry", name)
			}
			continue
		}
		if !have[id] {
			t.Fatalf("entry %s has no pack", name)
		}

		// The entry names this file at its size.
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		entry := &packfile.PackfileEntry{}
		if err := entry.UnmarshalVT(raw); err != nil {
			t.Fatalf("decode entry %s: %v", name, err)
		}
		f, err := os.Open(filepath.Join(dir, id))
		if err != nil {
			t.Fatal(err)
		}
		st, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if entry.GetId() != id || entry.GetSizeBytes() != uint64(st.Size()) {
			t.Fatalf("entry names %s of %d bytes, the file is %s of %d", entry.GetId(), entry.GetSizeBytes(), id, st.Size())
		}

		// The pack's own index holds as many keys as the entry counts.
		rdr, err := kvfile.BuildReader(f, uint64(st.Size()))
		if err != nil {
			t.Fatalf("read pack index %s: %v", id, err)
		}
		var n int
		if err := rdr.ScanPrefixEntries(nil, func(*kvfile.IndexEntry, int) error {
			n++
			return nil
		}); err != nil {
			t.Fatalf("scan pack index %s: %v", id, err)
		}
		_ = f.Close()
		if uint64(n) != entry.GetBlockCount() {
			t.Fatalf("pack %s holds %d keys, its entry %d", id, n, entry.GetBlockCount())
		}
		if !strings.HasPrefix(id, "pfv1_") {
			t.Fatalf("pack id %s", id)
		}
		packs++
		keys += n
	}
	return packs, keys
}

// TestRescueAdoptLayout runs in the adopt layout to a byte cap hit mid-run,
// then resumes to the end: after each run the directory holds only complete
// pairs that pass the adopt checks, and together they hold every wanted
// block.
func TestRescueAdoptLayout(t *testing.T) {
	// Want every block of three packs, with a cap of two and a half.
	src := newSource(t)
	var refs []*block.BlockRef
	for range 3 {
		refs = append(refs, src.pack(4)...)
	}
	var size int64
	err := dirObjects{}.ListObjects(context.Background(), filepath.Join(src.dir, testRoot), testPrefix+"packs/", func(_ string, n int64) error {
		size = max(size, n)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, refs), out)
	o.Layout = layoutAdopt
	o.Fetch.CapBytes = uint64(2*size + size/2)

	// The partial run stops at the cap with complete pairs only.
	res, err := run(context.Background(), o, &fakeServer{dir: src.dir})
	if err != nil || res.Stop != stopByteCap || res.Done != 8 {
		t.Fatalf("partial run: %v, %+v", err, res)
	}
	if packs, keys := checkAdoptDir(t, out); packs == 0 || keys != 8 {
		t.Fatalf("partial run: %d packs, %d keys", packs, keys)
	}

	// The resumed run finishes with complete pairs only, holding every block.
	o.Fetch.CapBytes = 1 << 40
	res, err = run(context.Background(), o, &fakeServer{dir: src.dir})
	if err != nil || res.Stop != "" || res.Done != 12 {
		t.Fatalf("resumed run: %v, %+v", err, res)
	}
	if packs, keys := checkAdoptDir(t, out); packs < 2 || keys != 12 {
		t.Fatalf("resumed run: %d packs, %d keys", packs, keys)
	}
}

// TestRecoverAdopt finishes a pair a crash left between its renames and
// leaves a pack staged without its entry invisible.
func TestRecoverAdopt(t *testing.T) {
	// Stage a pair as a crash after the pack's rename leaves it, and a pack
	// with no entry staged.
	dir := t.TempDir()
	for name, data := range map[string]string{
		"pfv1_a":            "pack a",
		"pfv1_a.entry.part": "entry a",
		"pfv1_b.part":       "pack b",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Recovery publishes a's entry and leaves b staged.
	if err := recoverAdopt(dir); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		"pfv1_a": true, "pfv1_a.entry": true, "pfv1_a.entry.part": false,
		"pfv1_b": false, "pfv1_b.entry": false, "pfv1_b.part": true,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != want {
			t.Fatalf("%s exists %v, want %v", name, err == nil, want)
		}
	}

	// A staged entry with no pack is refused rather than published.
	if err := (adoptObjects{}).PutObject(context.Background(), dir, "entries/pfv1_c", []byte("e"), ""); err == nil {
		t.Fatal("an entry with no staged pack was published")
	}
	if _, err := os.Stat(filepath.Join(dir, "pfv1_c.entry")); err == nil {
		t.Fatal("entry c published")
	}
	if _, err := (adoptObjects{}).GetObject(context.Background(), dir, "other/x"); err == nil {
		t.Fatal("a key outside packs/ and entries/ was read")
	}
}
