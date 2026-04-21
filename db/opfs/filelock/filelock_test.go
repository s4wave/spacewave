//go:build js

package filelock

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/opfs"
)

func TestAcquireFileBasic(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-filelock-basic", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-filelock-basic", true) //nolint

	// Acquire with create=true on a new file.
	file, release, err := AcquireFile(dir, "data", "test-filelock-basic", true)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("hello filelock")
	file.WriteAt(data, 0)
	file.Flush()
	release()

	// Acquire again with create=false.
	file, release, err = AcquireFile(dir, "data", "test-filelock-basic", false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	buf := make([]byte, len(data))
	n, err := file.ReadAt(buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("read %q, want %q", buf[:n], data)
	}
}

func TestAcquireFileConcurrent(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-filelock-conc", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-filelock-conc", true) //nolint

	// Create initial file with counter = 0.
	f, err := opfs.CreateSyncFile(dir, "counter")
	if err != nil {
		t.Fatal(err)
	}
	var zero [8]byte
	f.WriteAt(zero[:], 0)
	f.Flush()
	f.Close()

	// Launch concurrent goroutines that each increment the counter.
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			file, release, err := AcquireFile(dir, "counter", "test-filelock-conc", false)
			if err != nil {
				t.Error(err)
				return
			}
			defer release()

			var b [8]byte
			file.ReadAt(b[:], 0)
			val := binary.LittleEndian.Uint64(b[:])
			val++
			binary.LittleEndian.PutUint64(b[:], val)
			file.WriteAt(b[:], 0)
			file.Flush()
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}

	// Verify final count.
	f2, err := opfs.OpenSyncFile(dir, "counter")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	var result [8]byte
	f2.ReadAt(result[:], 0)
	got := binary.LittleEndian.Uint64(result[:])
	if got != n {
		t.Errorf("counter = %d, want %d", got, n)
	}
}
