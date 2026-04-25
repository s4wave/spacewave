//go:build js

package opfs

import (
	"bytes"
	"io"
	"testing"
)

func TestSyncSupported(t *testing.T) {
	r1 := SyncSupported()
	r2 := SyncSupported()
	if r1 != r2 {
		t.Fatal("SyncSupported returned inconsistent results")
	}
	t.Logf("SyncSupported = %v", r1)
}

func TestFileOpsReadWrite(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-ops-rw", true)
	}()

	dir, err := root.GetDirectoryHandle("test-ops-rw", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}

	fh, err := dir.GetFileHandle("data.bin", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	defer ops.Close()

	want := []byte("hello opfs world")
	n, err := ops.Write(want)
	if err != nil {
		t.Fatal("Write:", err)
	}
	if n != len(want) {
		t.Fatalf("Write: wrote %d, want %d", n, len(want))
	}
	if err := ops.Flush(); err != nil {
		t.Fatal("Flush:", err)
	}

	got := make([]byte, len(want))
	n, err = ops.ReadAt(got, 0)
	if err != nil {
		t.Fatal("ReadAt:", err)
	}
	if n != len(want) {
		t.Fatalf("ReadAt: read %d, want %d", n, len(want))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadAt: got %q, want %q", got, want)
	}
}

func TestFileOpsSeekRead(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-ops-seek", true)
	}()

	dir, err := root.GetDirectoryHandle("test-ops-seek", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}

	fh, err := dir.GetFileHandle("data.bin", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	defer ops.Close()

	data := []byte("abcdefghijklmnop")
	_, err = ops.Write(data)
	if err != nil {
		t.Fatal("Write:", err)
	}
	if err := ops.Flush(); err != nil {
		t.Fatal("Flush:", err)
	}

	// Seek to offset 4, read 4 bytes.
	pos, err := ops.Seek(4, io.SeekStart)
	if err != nil {
		t.Fatal("Seek:", err)
	}
	if pos != 4 {
		t.Fatalf("Seek: got pos %d, want 4", pos)
	}

	buf := make([]byte, 4)
	n, err := ops.Read(buf)
	if err != nil {
		t.Fatal("Read:", err)
	}
	if n != 4 {
		t.Fatalf("Read: got %d, want 4", n)
	}
	if string(buf) != "efgh" {
		t.Fatalf("Read: got %q, want %q", buf, "efgh")
	}

	// Seek from end.
	pos, err = ops.Seek(-4, io.SeekEnd)
	if err != nil {
		t.Fatal("SeekEnd:", err)
	}
	if pos != 12 {
		t.Fatalf("SeekEnd: got pos %d, want 12", pos)
	}

	n, err = ops.Read(buf)
	if err != nil {
		t.Fatal("Read from end:", err)
	}
	if string(buf[:n]) != "mnop" {
		t.Fatalf("Read from end: got %q, want %q", buf[:n], "mnop")
	}
}

func TestDirectoryHandleCreateRemove(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-dir-cr", true)
	}()

	dir1, err := root.GetDirectoryHandle("test-dir-cr", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}
	dir2, err := dir1.GetDirectoryHandle("nested", true)
	if err != nil {
		t.Fatal("nested dir:", err)
	}

	fh, err := dir2.GetFileHandle("file.txt", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}
	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	_, err = ops.Write([]byte("test"))
	if err != nil {
		t.Fatal("Write:", err)
	}
	if err := ops.Close(); err != nil {
		t.Fatal("Close:", err)
	}

	if err := dir1.RemoveEntry("nested", true); err != nil {
		t.Fatal("RemoveEntry:", err)
	}

	_, err = dir1.GetDirectoryHandle("nested", false)
	if err == nil {
		t.Fatal("expected error after remove, got nil")
	}
}

func TestFileHandleRoundtrip(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-fh-rt", true)
	}()

	dir, err := root.GetDirectoryHandle("test-fh-rt", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}

	fh, err := dir.GetFileHandle("roundtrip.bin", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}

	want := []byte("roundtrip data 12345")
	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	_, err = ops.Write(want)
	if err != nil {
		t.Fatal("Write:", err)
	}
	if err := ops.Flush(); err != nil {
		t.Fatal("Flush:", err)
	}
	if err := ops.Close(); err != nil {
		t.Fatal("Close:", err)
	}

	// Read back via ReadFile (getFile + arrayBuffer).
	got, err := fh.ReadFile()
	if err != nil {
		t.Fatal("ReadFile:", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadFile: got %q, want %q", got, want)
	}
}

func TestRootDirectory(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-root-dir", true)
	}()

	dir, err := root.GetDirectoryHandle("test-root-dir", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}

	fh, err := dir.GetFileHandle("hello.txt", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	want := []byte("hello from root")
	_, err = ops.Write(want)
	if err != nil {
		t.Fatal("Write:", err)
	}
	if err := ops.Close(); err != nil {
		t.Fatal("Close:", err)
	}

	got, err := fh.ReadFile()
	if err != nil {
		t.Fatal("ReadFile:", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFileOpsTruncate(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-truncate", true)
	}()

	dir, err := root.GetDirectoryHandle("test-truncate", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}
	fh, err := dir.GetFileHandle("trunc.bin", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}
	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	defer ops.Close()

	data := []byte("0123456789abcdef")
	_, err = ops.Write(data)
	if err != nil {
		t.Fatal("Write:", err)
	}

	if err := ops.Truncate(8); err != nil {
		t.Fatal("Truncate:", err)
	}

	fi, err := ops.Stat()
	if err != nil {
		t.Fatal("Stat:", err)
	}
	if fi.Size() != 8 {
		t.Fatalf("Stat.Size after truncate: got %d, want 8", fi.Size())
	}
}

func TestFileOpsFlush(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-flush", true)
	}()

	dir, err := root.GetDirectoryHandle("test-flush", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}
	fh, err := dir.GetFileHandle("flush.bin", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	want := []byte("flush test data")
	_, err = ops.Write(want)
	if err != nil {
		t.Fatal("Write:", err)
	}
	if err := ops.Flush(); err != nil {
		t.Fatal("Flush:", err)
	}
	if err := ops.Close(); err != nil {
		t.Fatal("Close:", err)
	}

	// Reopen and read back.
	ops2, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps (reopen):", err)
	}
	defer ops2.Close()

	got := make([]byte, len(want))
	n, err := ops2.Read(got)
	if err != nil {
		t.Fatal("Read:", err)
	}
	if n != len(want) {
		t.Fatalf("Read: got %d bytes, want %d", n, len(want))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Read after flush: got %q, want %q", got, want)
	}
}

func TestDirectoryIteration(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-iter", true)
	}()

	dir, err := root.GetDirectoryHandle("test-iter", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}

	names := []string{"alpha.txt", "beta.txt", "gamma.txt"}
	for _, name := range names {
		fh, err := dir.GetFileHandle(name, true)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		ops, err := fh.OpenFileOps()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		_, err = ops.Write([]byte(name))
		if err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := ops.Close(); err != nil {
			t.Fatalf("close %s: %v", name, err)
		}
	}

	entries, err := dir.Entries()
	if err != nil {
		t.Fatal("Entries:", err)
	}

	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Name] = true
		if e.Kind != "file" {
			t.Errorf("entry %s: got kind %q, want 'file'", e.Name, e.Kind)
		}
	}
	for _, name := range names {
		if !found[name] {
			t.Errorf("missing entry: %s", name)
		}
	}
}

func TestFileOpsStat(t *testing.T) {
	root, err := GetRootDirectory()
	if err != nil {
		t.Fatal("GetRootDirectory:", err)
	}
	defer func() {
		_ = root.RemoveEntry("test-stat", true)
	}()

	dir, err := root.GetDirectoryHandle("test-stat", true)
	if err != nil {
		t.Fatal("GetDirectoryHandle:", err)
	}
	fh, err := dir.GetFileHandle("stat.bin", true)
	if err != nil {
		t.Fatal("GetFileHandle:", err)
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		t.Fatal("OpenFileOps:", err)
	}
	defer ops.Close()

	fi, err := ops.Stat()
	if err != nil {
		t.Fatal("Stat:", err)
	}
	if fi.Size() != 0 {
		t.Fatalf("empty file size: got %d, want 0", fi.Size())
	}
	if fi.Name() != "stat.bin" {
		t.Fatalf("Name: got %q, want %q", fi.Name(), "stat.bin")
	}
	if fi.IsDir() {
		t.Fatal("IsDir should be false")
	}

	_, err = ops.Write([]byte("hello"))
	if err != nil {
		t.Fatal("Write:", err)
	}

	fi, err = ops.Stat()
	if err != nil {
		t.Fatal("Stat after write:", err)
	}
	if fi.Size() != 5 {
		t.Fatalf("size after write: got %d, want 5", fi.Size())
	}
}
