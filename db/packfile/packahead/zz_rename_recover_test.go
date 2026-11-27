package packahead

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenameRecoversCutIndex: a finalized pack whose index a failed-pack
// finalize retry truncated (recoverFile cuts the renamed file at its last good
// batch) makes Open fail; renaming <seq>-<id>.pack to <seq>.open makes Open
// recover every value and finalize it again under the same id.
func TestRenameRecoversCutIndex(t *testing.T) {
	cuts := []struct {
		name string
		keep func(end, size int64) int64
	}{
		{"cut at last batch", func(end, _ int64) int64 { return end }},
		{"half the index", func(end, size int64) int64 { return end + (size-end)/2 }},
		{"index less 8 bytes", func(_, size int64) int64 { return size - 8 }},
		{"index less 1 byte", func(_, size int64) int64 { return size - 1 }},
	}
	for _, c := range cuts {
		t.Run(c.name, func(t *testing.T) {
			ctx := t.Context()
			dir := t.TempDir()

			// Write batches, close, reopen: Open finalizes pack 1.
			l, err := Open(ctx, dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string][]byte{}
			var keys [][]byte
			for i := range 40 {
				batch := []Block{testBlock(t, i*2, 3000+i), testBlock(t, i*2+1, 900)}
				if err := l.Append(ctx, batch); err != nil {
					t.Fatal(err)
				}
				for _, b := range batch {
					want[string(b.Key)] = b.Value
					keys = append(keys, b.Key)
				}
			}
			l.Close()
			l, err = Open(ctx, dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			l.Close()
			matches, _ := filepath.Glob(filepath.Join(dir, "0000000000000001-*.pack"))
			if len(matches) != 1 {
				t.Fatalf("finalized packs %v", matches)
			}
			packPath := matches[0]
			id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(packPath), "0000000000000001-"), ".pack")

			// Cut the index.
			data, err := os.ReadFile(packPath)
			if err != nil {
				t.Fatal(err)
			}
			_, end := scanBatches(data)
			size := int64(len(data))
			if end >= size {
				t.Fatalf("scan end %d of %d", end, size)
			}
			if err := os.Truncate(packPath, c.keep(end, size)); err != nil {
				t.Fatal(err)
			}
			// Remove the empty pack 2 the second Open left, as a running store has one.
			if _, err := Open(ctx, dir, Options{}); err == nil {
				t.Fatal("Open accepted a cut index")
			} else {
				t.Logf("open before rename: %v", err)
			}

			// The recovery: rename to <seq>.open.
			if err := os.Rename(packPath, filepath.Join(dir, "0000000000000001.open")); err != nil {
				t.Fatal(err)
			}
			l, err = Open(ctx, dir, Options{})
			if err != nil {
				t.Fatalf("open after rename: %v", err)
			}
			defer l.Close()
			for _, k := range keys {
				got, ok, err := l.Get(k)
				if err != nil || !ok || !bytes.Equal(got, want[string(k)]) {
					t.Fatalf("get after recovery: ok=%v err=%v", ok, err)
				}
			}
			infos := waitFinalized(t, l, 1)
			var found bool
			for _, info := range infos {
				if info.Seq == 1 {
					found = true
					checkPack(t, info, want, Options{})
					if info.ID == id || info.Blocks != len(keys) {
						t.Fatalf("recovered id %s blocks %d, was %s %d (a new random id is expected)", info.ID, info.Blocks, id, len(keys))
					}
				}
			}
			if !found {
				t.Fatalf("pack 1 not finalized: %+v", infos)
			}
			ents, _ := os.ReadDir(dir)
			for _, e := range ents {
				t.Logf("after: %s", e.Name())
			}
		})
	}
}
