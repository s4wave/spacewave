//go:build js

package opfs

import (
	"io"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

// readSnapshotDriver owns reads and release for one runtime snapshot.
type readSnapshotDriver interface {
	readSnapshotAt(snapshot *ReadSnapshot, p []byte, off int64) (int, error)
	closeReadSnapshot(snapshot *ReadSnapshot) error
}

// ReadSnapshot retains one immutable OPFS File and its resolved size.
type ReadSnapshot struct {
	// driver selects the direct, TinyGo, or remote operations.
	driver readSnapshotDriver
	// name identifies the source file in read errors.
	name string
	// handle retains the immutable File in the direct browser path.
	handle js.Value
	// tinyGoID identifies the retained File in the TinyGo reference table.
	tinyGoID int
	// size is the immutable byte length recorded at open.
	size int64

	// mtx serializes reads with exactly-once release.
	mtx sync.Mutex
	// closed prevents reads after release and is guarded by mtx.
	closed bool
	// closeErr retains the release result and is guarded by mtx.
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
	s.mtx.Lock()
	defer s.mtx.Unlock()
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

// classifyReadSnapshotError maps a reclaimed snapshot to a retryable missing file.
func classifyReadSnapshotError(err error) error {
	// Treat a reclaimed immutable File as missing so manifest refresh can retry.
	var jsErr *JSError
	if errors.As(err, &jsErr) && jsErr.Name == "NotReadableError" {
		return &JSError{Name: "NotFoundError", Message: jsErr.Error()}
	}
	return err
}

// readSnapshotAt copies an in-bounds immutable range into p.
func (BrowserDriver) readSnapshotAt(snapshot *ReadSnapshot, p []byte, off int64) (int, error) {
	// Route TinyGo reads through the retained JavaScript reference table.
	if jsutil.UseTinyGoHelpers() {
		return snapshot.readAtWithTinyGoImport(p, off)
	}

	// Reuse the retained File for whole reads, but slice only a partial range.
	blob := snapshot.handle
	if off != 0 || int64(len(p)) != snapshot.size {
		blob = jsutil.Call(blob, "slice", off, off+int64(len(p)))
	}

	// Copy the selected immutable bytes into the caller's buffer.
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
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	s.closeErr = s.driver.closeReadSnapshot(s)
	return s.closeErr
}

// closeReadSnapshot releases the retained runtime reference.
func (BrowserDriver) closeReadSnapshot(snapshot *ReadSnapshot) error {
	// Release TinyGo's explicit retained reference when present.
	if jsutil.UseTinyGoHelpers() {
		return snapshot.closeWithTinyGoImport()
	}

	// Drop the standard Go JavaScript reference to the immutable File.
	snapshot.handle = js.Undefined()
	return nil
}
