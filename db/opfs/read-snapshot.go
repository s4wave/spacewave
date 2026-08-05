//go:build js

package opfs

import (
	"io"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

type readSnapshotDriver interface {
	readSnapshotAt(snapshot *ReadSnapshot, p []byte, off int64) (int, error)
	closeReadSnapshot(snapshot *ReadSnapshot) error
}

// ReadSnapshot retains one immutable OPFS File and its resolved size.
type ReadSnapshot struct {
	// driver and runtime references select the direct, TinyGo, or remote path.
	driver   readSnapshotDriver
	name     string
	handle   js.Value
	tinyGoID int
	size     int64

	// mu serializes reads with exactly-once release.
	mu       sync.Mutex
	closed   bool
	closeErr error
}

// OpenReadSnapshot opens an immutable view of an existing file.
func OpenReadSnapshot(dir js.Value, name string) (*ReadSnapshot, error) {
	return DefaultDriver.OpenReadSnapshot(dir, name)
}

// OpenReadSnapshot resolves one immutable File and records its size.
func (d BrowserDriver) OpenReadSnapshot(dir js.Value, name string) (*ReadSnapshot, error) {
	// Use retained JavaScript references when TinyGo cannot hold File directly.
	if jsutil.UseTinyGoHelpers() {
		return openReadSnapshotWithTinyGoImport(dir, name)
	}

	// Resolve the mutable directory entry to one immutable File exactly once.
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		return nil, errors.Wrap(err, "getFileHandle")
	}
	file, err := AwaitPromise(jsutil.Call(fileHandle, "getFile"))
	if err != nil {
		return nil, errors.Wrap(err, "getFile")
	}
	return &ReadSnapshot{
		driver: d,
		name:   name,
		handle: file,
		size:   int64(file.Get("size").Int()),
	}, nil
}

// ReadAt reads immutable bytes starting at off.
func (s *ReadSnapshot) ReadAt(p []byte, off int64) (int, error) {
	// Serialize reads with release so the driver reference stays live.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, errors.New("opfs read snapshot is closed")
	}

	// Validate and clamp the request to the recorded immutable size.
	if off < 0 {
		return 0, errors.New("opfs read snapshot has negative offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= s.size {
		return 0, io.EOF
	}
	readLen := min(int64(len(p)), s.size-off)

	// Read only the in-bounds prefix through the selected runtime driver.
	n, err := s.driver.readSnapshotAt(s, p[:readLen], off)
	if n < 0 || n > int(readLen) {
		return 0, errors.Errorf("read snapshot %s returned invalid count %d for %d bytes", s.name, n, readLen)
	}
	if err != nil {
		return n, classifyReadSnapshotError(err)
	}
	if n != int(readLen) || readLen < int64(len(p)) {
		return n, io.EOF
	}
	return n, nil
}

func classifyReadSnapshotError(err error) error {
	// Treat a reclaimed immutable File as missing so manifest refresh can retry.
	var jsErr *JSError
	if errors.As(err, &jsErr) && jsErr.Name == "NotReadableError" {
		return &JSError{Name: "NotFoundError", Message: jsErr.Error()}
	}
	return err
}

func (BrowserDriver) readSnapshotAt(snapshot *ReadSnapshot, p []byte, off int64) (int, error) {
	// Route TinyGo reads through the retained JavaScript reference table.
	if jsutil.UseTinyGoHelpers() {
		return snapshot.readAtWithTinyGoImport(p, off)
	}

	// Slice the retained File without resolving its directory entry again.
	blob := jsutil.Call(snapshot.handle, "slice", off, off+int64(len(p)))
	buffer, err := AwaitPromise(jsutil.Call(blob, "arrayBuffer"))
	if err != nil {
		return 0, errors.Wrap(err, "arrayBuffer")
	}
	bytes := jsutil.NewUint8Array(buffer)
	n := bytes.Get("length").Int()
	js.CopyBytesToGo(p[:n], bytes)
	return n, nil
}

// Size returns the size recorded when the immutable snapshot was opened.
func (s *ReadSnapshot) Size() (int64, error) {
	return s.size, nil
}

// Close releases the retained File or runtime token exactly once.
func (s *ReadSnapshot) Close() error {
	// Mark release before calling the driver so failures cannot double-release.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	s.closeErr = s.driver.closeReadSnapshot(s)
	return s.closeErr
}

func (BrowserDriver) closeReadSnapshot(snapshot *ReadSnapshot) error {
	// Release TinyGo's explicit retained reference when present.
	if jsutil.UseTinyGoHelpers() {
		return snapshot.closeWithTinyGoImport()
	}

	// Drop the standard Go JavaScript reference to the immutable File.
	snapshot.handle = js.Undefined()
	return nil
}
