//go:build js

package opfs

import (
	"io"
	"testing"
)

func TestAsyncFileReadWrite(t *testing.T) {
	root, err := GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteEntry(root, "test-async", true) //nolint

	dir, err := GetDirectory(root, "test-async", true)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello async opfs")
	f, err := CreateAsyncFile(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	n, err := f.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("wrote %d, expected %d", n, len(data))
	}
	f.Close()

	// Read back.
	f, err = OpenAsyncFile(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("read %q, expected %q", got, data)
	}

	// ReadAt partial.
	buf := make([]byte, 5)
	n, err = f.ReadAt(buf, 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "async" {
		t.Fatalf("ReadAt got %q, expected %q", buf[:n], "async")
	}

	// Stat.
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(data)) {
		t.Fatalf("size %d, expected %d", info.Size(), len(data))
	}
	f.Close()
}

func TestSyncFile(t *testing.T) {
	if !SyncAvailable() {
		t.Skip("sync access handles not available (SharedWorker context)")
	}

	root, err := GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteEntry(root, "test-sync", true) //nolint

	dir, err := GetDirectory(root, "test-sync", true)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello sync opfs")
	f, err := CreateSyncFile(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	n, err := f.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("wrote %d, expected %d", n, len(data))
	}
	f.Flush()
	f.Close()

	// Read back.
	f, err = OpenSyncFile(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("read %q, expected %q", got, data)
	}
	f.Close()

	// ReadAt partial.
	f, err = OpenSyncFile(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	n, err = f.ReadAt(buf, 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "sync" {
		t.Fatalf("ReadAt got %q, expected %q", buf[:n], "sync")
	}

	// Size.
	if f.Size() != int64(len(data)) {
		t.Fatalf("size %d, expected %d", f.Size(), len(data))
	}
	f.Close()
}

func TestSyncFileDeleteAfterClose(t *testing.T) {
	if !SyncAvailable() {
		t.Skip("sync access handles not available (SharedWorker context)")
	}

	root, err := GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteEntry(root, "test-sync-delete", true) //nolint

	dir, err := GetDirectory(root, "test-sync-delete", true)
	if err != nil {
		t.Fatal(err)
	}

	f, err := CreateSyncFile(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("delete me")); err != nil {
		t.Fatal(err)
	}
	f.Flush()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := DeleteFile(dir, "test.bin"); err != nil {
		t.Fatal(err)
	}
	exists, err := FileExists(dir, "test.bin")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected file to be deleted")
	}
}

func TestGetDirectoryPath(t *testing.T) {
	root, err := GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteEntry(root, "test-path", true) //nolint

	// Create nested path.
	dir, err := GetDirectoryPath(root, []string{"test-path", "a", "b"}, true)
	if err != nil {
		t.Fatal(err)
	}

	// Write a file in the leaf.
	if err := WriteFile(dir, "marker", []byte("ok")); err != nil {
		t.Fatal(err)
	}

	// Navigate again and read back.
	dir2, err := GetDirectoryPath(root, []string{"test-path", "a", "b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ReadFile(dir2, "marker")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("got %q, expected %q", data, "ok")
	}
}
