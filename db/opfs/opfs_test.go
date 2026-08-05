//go:build js

package opfs

import (
	"errors"
	"io"
	"slices"
	"syscall/js"
	"testing"
)

// TestSyncAvailableDedicatedWorkerGate verifies SyncAvailable gates on the
// DedicatedWorker global scope, not merely on the presence of the
// createSyncAccessHandle method. A non-conforming SharedWorker or main-thread
// context that exposes the method as a stub must still report sync as
// unavailable so the storage owner falls back to async OPFS.
func TestSyncAvailableDedicatedWorkerGate(t *testing.T) {
	global := js.Global()

	origCtor := global.Get("constructor")
	origFileHandle := global.Get("FileSystemFileHandle")
	t.Cleanup(func() {
		global.Set("constructor", origCtor)
		global.Set("FileSystemFileHandle", origFileHandle)
	})

	// Install a FileSystemFileHandle whose prototype exposes the method, so the
	// only remaining gate under test is the global-scope constructor name.
	method := js.FuncOf(func(this js.Value, args []js.Value) any { return nil })
	t.Cleanup(method.Release)
	proto := js.Global().Get("Object").Call("create", js.Null())
	proto.Set("createSyncAccessHandle", method)
	fileHandleCtor := js.Global().Get("Object").Call("create", js.Null())
	fileHandleCtor.Set("prototype", proto)
	global.Set("FileSystemFileHandle", fileHandleCtor)

	// Non-DedicatedWorker scope: method present but the gate must reject it.
	global.Set("constructor", map[string]any{"name": "SharedWorkerGlobalScope"})
	if DefaultDriver.SyncAvailable() {
		t.Fatal("SyncAvailable() = true in SharedWorker scope, want false")
	}

	// DedicatedWorker scope with the method present: the gate must accept it.
	global.Set("constructor", map[string]any{"name": "DedicatedWorkerGlobalScope"})
	if !DefaultDriver.SyncAvailable() {
		t.Fatal("SyncAvailable() = false in DedicatedWorker scope with method, want true")
	}

	// DedicatedWorker scope but no method: must reject.
	global.Set("FileSystemFileHandle", js.Undefined())
	if DefaultDriver.SyncAvailable() {
		t.Fatal("SyncAvailable() = true with no FileSystemFileHandle, want false")
	}
}

func TestBrowserDriverReadWriteListDeleteClassify(t *testing.T) {
	root, err := DefaultDriver.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer DefaultDriver.DeleteEntry(root, "test-driver", true) //nolint

	dir, err := DefaultDriver.GetDirectory(root, "test-driver", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := DefaultDriver.WriteFile(dir, "alpha", []byte("driver-owned")); err != nil {
		t.Fatal(err)
	}
	got, err := DefaultDriver.ReadFile(dir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "driver-owned" {
		t.Fatalf("ReadFile = %q, want driver-owned", string(got))
	}
	names, err := DefaultDriver.ListDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "alpha") {
		t.Fatalf("ListDirectory = %v, want alpha", names)
	}
	if err := DefaultDriver.DeleteEntry(dir, "alpha", false); err != nil {
		t.Fatal(err)
	}
	exists, err := DefaultDriver.FileExists(dir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected alpha to be deleted")
	}
	err = DefaultDriver.DeleteEntry(dir, "alpha", false)
	if DefaultDriver.ClassifyError(err) != ErrorKindNotFound {
		t.Fatalf("ClassifyError(%v) = %v, want ErrorKindNotFound", err, DefaultDriver.ClassifyError(err))
	}
}
func TestQuotaExceededErrorClassification(t *testing.T) {
	err := &JSError{
		Name:    "QuotaExceededError",
		Message: "the operation exceeded its storage quota",
	}
	if !IsQuotaExceeded(err) {
		t.Fatalf("IsQuotaExceeded(%v) = false, want true", err)
	}
	if kind := ClassifyError(err); kind != ErrorKindQuotaExceeded {
		t.Fatalf("ClassifyError(%v) = %v, want ErrorKindQuotaExceeded", err, kind)
	}
	wrapped := WithQuotaEstimate(err, 1024)
	var quotaErr *QuotaExceededError
	if !errors.As(wrapped, &quotaErr) {
		t.Fatalf("WithQuotaEstimate(%v) = %T, want *QuotaExceededError", err, wrapped)
	}
	if quotaErr.Required != 1024 || quotaErr.Quota == 0 {
		t.Fatalf("quota error = %+v, want required bytes and browser quota", quotaErr)
	}
}

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

func TestReadSnapshotRetainsImmutableFile(t *testing.T) {
	// Create one immutable source file and resolve its snapshot.
	root, err := GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteEntry(root, "test-read-snapshot", true) //nolint
	dir, err := GetDirectory(root, "test-read-snapshot", true)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("hello immutable snapshot")
	if err := WriteFile(dir, "segment.sst", original); err != nil {
		t.Fatal(err)
	}
	snapshot, err := OpenReadSnapshot(dir, "segment.sst")
	if err != nil {
		t.Fatal(err)
	}

	// Record size once and preserve repeated range and EOF behavior.
	size, err := snapshot.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(original)) {
		t.Fatalf("snapshot size = %d, want %d", size, len(original))
	}
	buf := make([]byte, 9)
	n, err := snapshot.ReadAt(buf, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "immutable" {
		t.Fatalf("snapshot range = %q, want immutable", got)
	}
	buf = make([]byte, 5)
	n, err = snapshot.ReadAt(buf, int64(len(original)-3))
	if n != 3 || err != io.EOF {
		t.Fatalf("snapshot final read = (%d, %v), want (3, EOF)", n, err)
	}
	if got := string(buf[:n]); got != "hot" {
		t.Fatalf("snapshot final bytes = %q, want hot", got)
	}

	// Classify browser reclamation as missing for manifest refresh and retry.
	if err := DeleteEntry(dir, "segment.sst", false); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.ReadAt(buf, 0); !IsNotFound(err) {
		t.Fatalf("snapshot read after reclamation = %v, want not found", err)
	}

	// Release the retained reference once and reject later reads.
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := snapshot.ReadAt(buf, 0); err == nil {
		t.Fatal("read after close succeeded")
	}
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
