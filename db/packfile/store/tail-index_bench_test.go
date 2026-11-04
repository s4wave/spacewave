package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
)

// benchTails returns the index tails to measure: every pack in the directory
// named by PACKFILE_INDEX_PACK_DIR, or packs of 4096 blocks (the most a pack
// holds) and 300 blocks built with the writer the store uses.
func benchTails(b *testing.B) (names []string, tails [][]byte, sizes []int64, counts []uint64) {
	b.Helper()
	if dir := os.Getenv(indexPackDirEnv); dir != "" {
		files, err := filepath.Glob(filepath.Join(dir, "*"))
		if err != nil || len(files) == 0 {
			b.Fatalf("no packs in %s: %v", dir, err)
		}
		for _, name := range files {
			f, err := os.Open(name)
			if err != nil {
				b.Fatal(err)
			}
			fi, err := f.Stat()
			if err != nil {
				b.Fatal(err)
			}
			_, tail, err := kvfile.ReadIndexTail(f, uint64(fi.Size()))
			f.Close()
			if err != nil {
				b.Fatal(err)
			}
			rd, err := kvfile.BuildReaderWithIndexTail(tail, uint64(fi.Size()))
			if err != nil {
				b.Fatal(err)
			}
			names = append(names, filepath.Base(name))
			tails = append(tails, bytes.Clone(tail))
			sizes = append(sizes, fi.Size())
			counts = append(counts, rd.Size())
		}
		return
	}
	for _, n := range []int{300, 4096} {
		var items []packItem
		for i := range n {
			items = append(items, testPackItem(b, fmt.Sprintf("bench block %d %s", i, bytes.Repeat([]byte{'x'}, (i*7919)%4001))))
		}
		data, _ := packItems(b, items)
		names = append(names, fmt.Sprintf("blocks-%d", n))
		tails = append(tails, bytes.Clone(mustReadIndexTail(b, data)))
		sizes = append(sizes, int64(len(data)))
		counts = append(counts, uint64(n))
	}
	return
}

// BenchmarkIndexLoad measures building a pack's index from its tail, the work
// of one index load after the tail is read, in each layout.
func BenchmarkIndexLoad(b *testing.B) {
	names, tails, sizes, counts := benchTails(b)
	for i, name := range names {
		b.Run("parsed/"+name, func(b *testing.B) {
			b.ReportAllocs()
			r := NewPackReader("bench-pack", sizes[i], nil)
			defer r.Close()
			r.SetExpectedBlockCount(counts[i])
			for b.Loop() {
				entries, err := r.parseIndexTail(tails[i])
				if err != nil {
					b.Fatal(err)
				}
				_ = newParsedIndex(entries)
			}
		})
		b.Run("tail/"+name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := newTailIndex(bytes.Clone(tails[i]), uint64(sizes[i]), counts[i]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkIndexFind measures one block lookup by key in each layout.
func BenchmarkIndexFind(b *testing.B) {
	names, tails, sizes, counts := benchTails(b)
	for i, name := range names {
		r := NewPackReader("bench-pack", sizes[i], nil)
		r.SetExpectedBlockCount(counts[i])
		entries, err := r.parseIndexTail(tails[i])
		r.Close()
		if err != nil {
			b.Fatal(err)
		}
		p := newParsedIndex(entries)
		ti, err := newTailIndex(bytes.Clone(tails[i]), uint64(sizes[i]), counts[i])
		if err != nil {
			b.Fatal(err)
		}
		keys := make([][]byte, p.len())
		for j := range keys {
			keys[j], _ = p.atOffset(j)
		}
		for _, idx := range []struct {
			name string
			ix   blockIndex
		}{{"parsed", p}, {"tail", ti}} {
			b.Run(idx.name+"/"+name, func(b *testing.B) {
				b.ReportAllocs()
				var j int
				for b.Loop() {
					if _, ok := idx.ix.find(keys[j%len(keys)]); !ok {
						b.Fatal("key not found")
					}
					j++
				}
			})
		}
	}
}
