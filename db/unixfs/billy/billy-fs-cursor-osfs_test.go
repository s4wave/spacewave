//go:build !js

package unixfs_billy_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

// TestBillyFSCursorOSFSConcurrentReads checks that two listings and a file
// range read on one osfs root can remain open together.
func TestBillyFSCursorOSFSConcurrentReads(t *testing.T) {
	// Open the directory and a file on the same cursor tree.
	const entries = 100_000
	ctx := t.Context()
	root := openOSFSDirectory(t, entries)
	file, err := root.Lookup(ctx, "0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(file.Release)

	// Count each listing independently and read back the file contents.
	list := func(firstEntry func()) {
		count := 0
		err := root.ReaddirAll(ctx, 0, func(unixfs.FSCursorDirent) error {
			if count == 0 && firstEntry != nil {
				firstEntry()
			}
			count++
			return nil
		})
		if err != nil || count != entries {
			t.Errorf("listed %d of %d entries: %v", count, entries, err)
		}
	}
	read := func() {
		var data [1]byte
		n, err := file.ReadAt(ctx, 0, data[:])
		if n != 1 || data[0] != 'x' || err != nil {
			t.Errorf("read %d bytes %q: %v", n, data, err)
		}
	}

	// Hold both listings at their first entry until the file read completes.
	firstEntries := make(chan bool, 2)
	readDone := make(chan struct{})
	listDone := make(chan struct{}, 2)
	for range cap(listDone) {
		go func() {
			// Hold the listing open once it starts delivering entries.
			started := false
			list(func() {
				started = true
				firstEntries <- true
				<-readDone
			})

			// Release the reader even if the listing ended before its first entry.
			if !started {
				firstEntries <- false
			}
			listDone <- struct{}{}
		}()
	}

	// Read while both callbacks are held, then let the listings finish.
	first, second := <-firstEntries, <-firstEntries
	if !first || !second {
		t.Error("listing ended before its first entry")
	}
	read()
	close(readDone)
	for range cap(listDone) {
		<-listDone
	}
}

// TestBillyFSCursorOSFSListingPage checks that a fixed-size page reads work
// proportional to the page and its offset.
func TestBillyFSCursorOSFSListingPage(t *testing.T) {
	// Measure the same page in two directories with tenfold different sizes.
	const skip, limit = 32, 64
	ctx := t.Context()
	measure := func(entries int) testing.BenchmarkResult {
		root := openOSFSDirectory(t, entries)
		result := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				count := 0
				err := root.ReaddirAll(ctx, skip, func(unixfs.FSCursorDirent) error {
					count++
					if count == limit {
						return io.EOF
					}
					return nil
				})
				if !errors.Is(err, io.EOF) || count != limit {
					b.Fatalf("listed %d of %d requested entries: %v", count, limit, err)
				}
			}
		})
		t.Logf("%d entries: %s %s", entries, result, result.MemString())
		return result
	}
	small := measure(10_000)
	large := measure(100_000)

	// Allow storage and scheduling variance while rejecting a full-directory scan.
	if large.NsPerOp() > small.NsPerOp()*2 {
		t.Errorf("page time grew from %d to %d ns/op", small.NsPerOp(), large.NsPerOp())
	}
	if large.AllocsPerOp() != small.AllocsPerOp() || large.AllocedBytesPerOp() > small.AllocedBytesPerOp()*2 {
		t.Errorf("page allocations grew from %s to %s", small.MemString(), large.MemString())
	}
}

// openOSFSDirectory opens a UnixFS handle over an osfs directory with entries
// files, including file "0" with one byte of content.
func openOSFSDirectory(t *testing.T, entries int) *unixfs.FSHandle {
	// Populate the directory through the host filesystem outside the timing loop.
	t.Helper()
	dir := t.TempDir()
	for n := range entries {
		if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(n)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Keep the osfs cursor alive until the test has closed its handles.
	cursor := unixfs_billy.NewBillyFSCursor(osfs.New(dir), "")
	t.Cleanup(cursor.Release)
	root, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Release)
	return root
}
