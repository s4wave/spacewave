//go:build js

package opfs

import (
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

const (
	jsErrorCodeUnknown = iota
	jsErrorCodeNotFound
	jsErrorCodeNoModificationAllowed
	jsErrorCodeQuotaExceeded

	browserDriverFileChunkSize = 1 << 20
)

type opfsHelperResult struct {
	values     []int
	valueCount int
	value0     int
	value1     int
	err        error
	rejected   bool
	errCode    int
}

var (
	opfsHelperMu     sync.Mutex
	opfsHelperNextID int
	opfsHelperOps    = map[int]chan opfsHelperResult{}
)

var (
	rootMu     sync.Mutex
	rootHandle js.Value
	rootCached bool
)

// Driver owns browser OPFS operations and the objects opened from them.
type Driver interface {
	asyncFileDriver
	writeStreamDriver

	ClassifyError(error) ErrorKind
	GetRoot() (js.Value, error)
	GetDirectory(parent js.Value, name string, create bool) (js.Value, error)
	GetDirectoryPath(parent js.Value, path []string, create bool) (js.Value, error)
	OpenAsyncFile(dir js.Value, name string) (*AsyncFile, error)
	OpenReadSnapshot(dir js.Value, name string) (*ReadSnapshot, error)
	CreateAsyncFile(dir js.Value, name string) (*AsyncFile, error)
	WriteFile(dir js.Value, name string, data []byte) error
	CreateWriteStream(dir js.Value, name string) (*WriteStream, error)
	ReadFile(dir js.Value, name string) ([]byte, error)
	DeleteEntry(dir js.Value, name string, recursive bool) error
	ListDirectory(dir js.Value) ([]string, error)
	FileExists(dir js.Value, name string) (bool, error)
	DirExists(dir js.Value, name string) (bool, error)
	SyncAvailable() bool
	PreferSyncAccessHandles() bool
	OpenSyncFile(dir js.Value, name string) (*SyncFile, error)
	CreateSyncFile(dir js.Value, name string) (*SyncFile, error)
	CreateSyncFileContext(ctx context.Context, dir js.Value, name string) (*SyncFile, error)
	NewBroadcastChannel(name string) (js.Value, error)
	SendBroadcastChannel(channel js.Value, msg BroadcastMessage) error
	CloseBroadcastChannel(channel js.Value) error
	AcquireWebLock(ctx context.Context, name string, exclusive bool) (*WebLockResult, error)
	AcquireWebLockIfAvailable(ctx context.Context, name string, exclusive bool) (*WebLockResult, error)
}

type asyncFileDriver interface {
	readAsyncFileAt(f *AsyncFile, p []byte, off int64) (int, error)
	writeAsyncFileAt(ctx context.Context, f *AsyncFile, p []byte, off int64) (int, error)
	sizeAsyncFile(f *AsyncFile) (int64, error)
	truncateAsyncFile(f *AsyncFile, size int64) error
	closeAsyncFile(f *AsyncFile) error
}

type writeStreamDriver interface {
	writeStream(w *WriteStream, p []byte) (int, error)
	closeWriteStream(w *WriteStream) error
	abortWriteStream(w *WriteStream) error
}

// BrowserDriver owns local browser OPFS operations and error classification.
type BrowserDriver struct{}

// DefaultDriver selects the process-local browser OPFS driver by default.
var DefaultDriver Driver = BrowserDriver{}

// ErrorKind classifies a browser OPFS failure by its DOMException name.
type ErrorKind int

const (
	ErrorKindUnknown ErrorKind = iota
	ErrorKindNotFound
	ErrorKindNoModificationAllowed
	// ErrorKindSecurity is a DOMException SecurityError: the browser denied
	// access to OPFS for this profile. On root acquisition this is a terminal
	// storage-capability denial, not a transient failure.
	ErrorKindSecurity
	// ErrorKindQuotaExceeded is a DOMException QuotaExceededError.
	ErrorKindQuotaExceeded
)

// JSError represents a JavaScript error or DOMException.
type JSError struct {
	// Name is the error name (e.g. "NotFoundError", "TypeError").
	Name string
	// Message is the error message.
	Message string
}

// Error implements the error interface.
func (e *JSError) Error() string {
	if e.Name != "" {
		return e.Name + ": " + e.Message
	}
	return e.Message
}

// IsNotFound checks if an error is a "NotFoundError" DOMException.
func IsNotFound(err error) bool {
	return DefaultDriver.ClassifyError(err) == ErrorKindNotFound
}

// IsSecurity checks if an error is a "SecurityError" DOMException.
func IsSecurity(err error) bool {
	return DefaultDriver.ClassifyError(err) == ErrorKindSecurity
}

// IsQuotaExceeded checks if an error is a "QuotaExceededError" DOMException.
func IsQuotaExceeded(err error) bool {
	return DefaultDriver.ClassifyError(err) == ErrorKindQuotaExceeded
}

// ClassifyError classifies an OPFS/browser error.
func ClassifyError(err error) ErrorKind {
	return DefaultDriver.ClassifyError(err)
}

// ClassifyError classifies an OPFS/browser error.
func (BrowserDriver) ClassifyError(err error) ErrorKind {
	var jsErr *JSError
	if !errors.As(err, &jsErr) {
		return ErrorKindUnknown
	}
	switch jsErr.Name {
	case "NotFoundError":
		return ErrorKindNotFound
	case "NoModificationAllowedError":
		return ErrorKindNoModificationAllowed
	case "SecurityError":
		return ErrorKindSecurity
	case "QuotaExceededError":
		return ErrorKindQuotaExceeded
	default:
		return ErrorKindUnknown
	}
}

// newJSError creates a JSError from a js.Value error object.
func newJSError(val js.Value) *JSError {
	if val.Type() == js.TypeNumber {
		return newJSErrorCode(val.Int())
	}

	name := val.Get("name")
	msg := val.Get("message")
	e := &JSError{}
	if !name.IsUndefined() && !name.IsNull() {
		e.Name = name.String()
	}
	if !msg.IsUndefined() && !msg.IsNull() {
		e.Message = msg.String()
	}
	if e.Name == "" && e.Message == "" {
		e.Message = jsutil.Call(val, "toString").String()
	}
	return e
}

func newJSErrorCode(code int) *JSError {
	switch code {
	case jsErrorCodeNotFound:
		return &JSError{Name: "NotFoundError", Message: "entry not found"}
	case jsErrorCodeNoModificationAllowed:
		return &JSError{Name: "NoModificationAllowedError", Message: "entry cannot be modified"}
	case jsErrorCodeQuotaExceeded:
		return &JSError{Name: "QuotaExceededError", Message: "storage quota exceeded"}
	default:
		return &JSError{Message: "promise rejected"}
	}
}

// AwaitPromise blocks the calling goroutine until a JS Promise resolves or rejects.
// Returns the resolved value or an error wrapping the rejection reason.
func AwaitPromise(promise js.Value) (js.Value, error) {
	// Allocate callback state for the promise result and rejection.
	ch := make(chan struct{})
	var closeOnce atomic.Bool
	var result js.Value
	var jsErr error

	// Register the promise fulfillment callback.
	thenCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			result = args[0]
		} else {
			result = js.Undefined()
		}
		if closeOnce.CompareAndSwap(false, true) {
			close(ch)
		}
		return nil
	})
	defer thenCb.Release()

	// Register the promise rejection callback.
	catchCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			jsErr = newJSError(args[0])
		} else {
			jsErr = errors.New("promise rejected")
		}
		if closeOnce.CompareAndSwap(false, true) {
			close(ch)
		}
		return nil
	})
	defer catchCb.Release()

	// Attach callbacks, wait for settlement, and return the captured result.
	jsutil.AwaitPromise(promise, thenCb, catchCb)
	<-ch

	return result, jsErr
}

func yieldMicrotask() error {
	if jsutil.UseTinyGoHelpers() {
		return yieldMicrotaskWithTinyGoImport()
	}

	var cb js.Func
	exec := js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve := args[0]
		cb = js.FuncOf(func(this js.Value, args []js.Value) any {
			if resolve.IsUndefined() || resolve.IsNull() || resolve.Type() != js.TypeFunction {
				panic("queueMicrotask resolve callback unavailable")
			}
			defer func() {
				if e := recover(); e != nil {
					panic("queueMicrotask resolve invoke failed")
				}
			}()
			resolve.Invoke(js.Undefined())
			cb.Release()
			return nil
		})
		jsutil.Call(js.Global(), "queueMicrotask", cb)
		return nil
	})
	defer exec.Release()
	_, err := AwaitPromise(jsutil.NewPromise(exec))
	return err
}

// GetRoot returns the OPFS root FileSystemDirectoryHandle.
func GetRoot() (js.Value, error) {
	return DefaultDriver.GetRoot()
}

// GetRoot returns the OPFS root FileSystemDirectoryHandle.
// The root is a stable per-origin singleton, so it is resolved once per worker
// and cached: repeated mounts reuse the cached handle instead of issuing fresh
// navigator.storage.getDirectory() calls. The cache stores only after a
// successful acquisition, so a first-call denial (e.g. a SecurityError from a
// wedged OPFS bucket) is never cached and propagates to the caller, which
// classifies it as a terminal storage-capability error.
func (d BrowserDriver) GetRoot() (js.Value, error) {
	rootMu.Lock()
	defer rootMu.Unlock()
	if rootCached {
		return rootHandle, nil
	}
	handle, err := d.getRootUncached()
	if err != nil {
		return handle, err
	}
	rootHandle = handle
	rootCached = true
	return rootHandle, nil
}

func (BrowserDriver) getRootUncached() (js.Value, error) {
	if jsutil.UseTinyGoHelpers() {
		return getRootWithTinyGoImport()
	}
	storage := js.Global().Get("navigator").Get("storage")
	promise := jsutil.Call(storage, "getDirectory")
	return AwaitPromise(promise)
}

// GetDirectory returns a subdirectory handle within parent.
// If create is true, the directory is created if it does not exist.
func GetDirectory(parent js.Value, name string, create bool) (js.Value, error) {
	return DefaultDriver.GetDirectory(parent, name, create)
}

// GetDirectory returns a subdirectory handle within parent.
// If create is true, the directory is created if it does not exist.
func (d BrowserDriver) GetDirectory(parent js.Value, name string, create bool) (js.Value, error) {
	if jsutil.UseTinyGoHelpers() {
		return getDirectoryWithTinyGoImport(parent, name, create)
	}

	opts := jsutil.NewObject()
	opts.Set("create", create)
	promise := jsutil.Call(parent, "getDirectoryHandle", name, opts)
	return AwaitPromise(promise)
}

// GetDirectoryPath navigates a sequence of directory names from parent.
// Each element is a single directory name (no slashes).
// If create is true, intermediate directories are created.
func GetDirectoryPath(parent js.Value, path []string, create bool) (js.Value, error) {
	return DefaultDriver.GetDirectoryPath(parent, path, create)
}

// GetDirectoryPath navigates a sequence of directory names from parent.
// Each element is a single directory name (no slashes).
// If create is true, intermediate directories are created.
func (d BrowserDriver) GetDirectoryPath(parent js.Value, path []string, create bool) (js.Value, error) {
	dir := parent
	for _, name := range path {
		next, err := d.GetDirectory(dir, name, create)
		if err != nil {
			return js.Undefined(), errors.Wrap(err, name)
		}
		dir = next
	}
	return dir, nil
}

// OpenAsyncFile opens an existing file with async OPFS APIs.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
func OpenAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	return DefaultDriver.OpenAsyncFile(dir, name)
}

// OpenAsyncFile opens an existing file with async OPFS APIs.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
func (d BrowserDriver) OpenAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	if jsutil.UseTinyGoHelpers() {
		return openAsyncFileWithTinyGoImport(dir, name, false)
	}

	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		return nil, err
	}
	return &AsyncFile{driver: d, name: name, handle: fileHandle}, nil
}

// CreateAsyncFile opens or creates a file with async OPFS APIs.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
func CreateAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	return DefaultDriver.CreateAsyncFile(dir, name)
}

// CreateAsyncFile opens or creates a file with async OPFS APIs.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
func (d BrowserDriver) CreateAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	if jsutil.UseTinyGoHelpers() {
		return openAsyncFileWithTinyGoImport(dir, name, true)
	}

	opts := jsutil.NewObject()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name, opts))
	if err != nil {
		return nil, errors.Wrap(err, "getFileHandle")
	}
	return &AsyncFile{driver: d, name: name, handle: fileHandle}, nil
}

// AsyncFile wraps a driver-owned async OPFS file as an fs.File.
// BrowserDriver uses FileSystemFileHandle; RemoteDriver uses handle tokens.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
type AsyncFile struct {
	driver asyncFileDriver
	name   string
	handle js.Value // FileSystemFileHandle or RemoteDriver handle token
	pos    int64
}

// WriteStream owns one overwrite writable session for streaming file output.
type WriteStream struct {
	driver   writeStreamDriver
	name     string
	writable js.Value
	tinyGoID int
	pos      int64
}

func (f *AsyncFile) owner() asyncFileDriver {
	if f.driver != nil {
		return f.driver
	}
	return BrowserDriver{}
}

func (w *WriteStream) owner() writeStreamDriver {
	if w.driver != nil {
		return w.driver
	}
	return BrowserDriver{}
}

// Read reads up to len(p) bytes from the current position.
func (f *AsyncFile) Read(p []byte) (int, error) {
	n, err := f.ReadAt(p, f.pos)
	f.pos += int64(n)
	return n, err
}

// ReadAt reads len(p) bytes from the file starting at byte offset off.
// Uses File.slice() for range reads without loading the entire file.
func (f *AsyncFile) ReadAt(p []byte, off int64) (int, error) {
	return f.owner().readAsyncFileAt(f, p, off)
}

func (BrowserDriver) readAsyncFileAt(f *AsyncFile, p []byte, off int64) (int, error) {
	if jsutil.UseTinyGoHelpers() {
		return f.readAtWithTinyGoImport(p, off)
	}

	file, err := AwaitPromise(jsutil.Call(f.handle, "getFile"))
	if err != nil {
		return 0, errors.Wrap(err, "getFile")
	}

	size := file.Get("size").Int()
	if off >= int64(size) {
		return 0, io.EOF
	}

	end := min(off+int64(len(p)), int64(size))

	blob := jsutil.Call(file, "slice", off, end)
	ab, err := AwaitPromise(jsutil.Call(blob, "arrayBuffer"))
	if err != nil {
		return 0, errors.Wrap(err, "arrayBuffer")
	}

	arr := jsutil.NewUint8Array(ab)
	n := arr.Get("length").Int()
	js.CopyBytesToGo(p[:n], arr)
	if n == 0 && len(p) > 0 {
		return 0, io.EOF
	}
	return n, nil
}

// Write writes len(p) bytes at the current position.
// Opens a writable stream, seeks, writes, and closes per call.
func (f *AsyncFile) Write(p []byte) (int, error) {
	n, err := f.WriteAtContext(context.Background(), p, f.pos)
	f.pos += int64(n)
	return n, err
}

// WriteAt writes len(p) bytes to the file starting at byte offset off.
func (f *AsyncFile) WriteAt(p []byte, off int64) (int, error) {
	return f.WriteAtContext(context.Background(), p, off)
}

// WriteAtContext writes len(p) bytes to the file starting at byte offset off.
//
// The writable is opened with keepExistingData=true so partial writes preserve
// bytes outside [off, off+len(p)). Without this, createWritable's draft starts
// empty and close() truncates the source file to the highest-written offset,
// destroying any other content.
func (f *AsyncFile) WriteAtContext(ctx context.Context, p []byte, off int64) (int, error) {
	return f.owner().writeAsyncFileAt(ctx, f, p, off)
}

func (BrowserDriver) writeAsyncFileAt(ctx context.Context, f *AsyncFile, p []byte, off int64) (int, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs/async-file/write-at")
	defer task.End()

	if jsutil.UseTinyGoHelpers() {
		return f.writeAtWithTinyGoImport(p, off, true)
	}

	writeCtx, writeTask := trace.NewTask(ctx, "hydra/opfs/async-file/write-at/create-writable")
	writable, err := openWritable(f.handle, true)
	writeTask.End()
	if err != nil {
		return 0, err
	}

	if off > 0 {
		_, seekTask := trace.NewTask(writeCtx, "hydra/opfs/async-file/write-at/seek")
		_, err := AwaitPromise(jsutil.Call(writable, "seek", off))
		seekTask.End()
		if err != nil {
			if _, abortErr := AwaitPromise(jsutil.Call(writable, "abort")); abortErr != nil {
				return 0, errors.Wrapf(err, "abort after seek failure: %v", abortErr)
			}
			return 0, errors.Wrap(err, "seek")
		}
	}

	arr := jsutil.NewUint8Array(len(p))
	js.CopyBytesToJS(arr, p)

	writeDataCtx, writeDataTask := trace.NewTask(writeCtx, "hydra/opfs/async-file/write-at/write")
	_, err = AwaitPromise(jsutil.Call(writable, "write", arr))
	writeDataTask.End()
	if err != nil {
		if _, abortErr := AwaitPromise(jsutil.Call(writable, "abort")); abortErr != nil {
			return 0, errors.Wrapf(err, "abort failed write: %v", abortErr)
		}
		return 0, errors.Wrap(err, "write")
	}

	_, closeTask := trace.NewTask(writeDataCtx, "hydra/opfs/async-file/write-at/close-writable")
	_, err = AwaitPromise(jsutil.Call(writable, "close"))
	closeTask.End()
	if err != nil {
		if _, abortErr := AwaitPromise(jsutil.Call(writable, "abort")); abortErr != nil {
			return len(p), errors.Wrapf(err, "abort failed close: %v", abortErr)
		}
		return len(p), errors.Wrap(err, "close writable")
	}
	return len(p), nil
}

func invokeOPFSIntHelper(call func(opID int)) (int, error) {
	values, err := invokeOPFSHelper(call)
	if err != nil || len(values) == 0 {
		return 0, err
	}
	return values[0], nil
}

func invokeOPFSHelper(call func(opID int)) ([]int, error) {
	opID, ch := registerOPFSHelperOp()
	defer unregisterOPFSHelperOp(opID)

	call(opID)
	result := <-ch
	if result.err != nil {
		return nil, result.err
	}
	if result.rejected {
		return nil, newJSErrorCode(result.errCode)
	}
	if result.values != nil {
		return result.values, nil
	}
	switch result.valueCount {
	case 0:
		return nil, nil
	case 1:
		return []int{result.value0}, nil
	default:
		return []int{result.value0, result.value1}, nil
	}
}

func registerOPFSHelperOp() (int, chan opfsHelperResult) {
	ch := make(chan opfsHelperResult, 1)
	opfsHelperMu.Lock()
	opfsHelperNextID++
	if opfsHelperNextID == 0 {
		opfsHelperNextID++
	}
	opID := opfsHelperNextID
	opfsHelperOps[opID] = ch
	opfsHelperMu.Unlock()
	return opID, ch
}

func unregisterOPFSHelperOp(opID int) {
	opfsHelperMu.Lock()
	delete(opfsHelperOps, opID)
	opfsHelperMu.Unlock()
}

func completeOPFSHelperOp(opID int, result opfsHelperResult) {
	opfsHelperMu.Lock()
	ch := opfsHelperOps[opID]
	delete(opfsHelperOps, opID)
	opfsHelperMu.Unlock()
	if ch != nil {
		ch <- result
	}
}

// Seek sets the offset for the next Read or Write.
func (f *AsyncFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = offset
	case io.SeekCurrent:
		f.pos += offset
	case io.SeekEnd:
		size, err := f.Size()
		if err != nil {
			return f.pos, err
		}
		f.pos = size + offset
	}
	return f.pos, nil
}

// Size returns the file size in bytes.
func (f *AsyncFile) Size() (int64, error) {
	return f.owner().sizeAsyncFile(f)
}

func (BrowserDriver) sizeAsyncFile(f *AsyncFile) (int64, error) {
	if jsutil.UseTinyGoHelpers() {
		return f.sizeWithTinyGoImport()
	}

	file, err := AwaitPromise(jsutil.Call(f.handle, "getFile"))
	if err != nil {
		return 0, errors.Wrap(err, "getFile")
	}
	return int64(file.Get("size").Int()), nil
}

// Truncate sets the file size via a writable stream.
//
// Opens the writable with keepExistingData=true so bytes in [0, size) are
// preserved when growing or shrinking; otherwise the draft would start empty
// and close() would replace the source file with a sparse zero-filled blob.
func (f *AsyncFile) Truncate(size int64) error {
	return f.owner().truncateAsyncFile(f, size)
}

func (BrowserDriver) truncateAsyncFile(f *AsyncFile, size int64) error {
	if jsutil.UseTinyGoHelpers() {
		return f.truncateWithTinyGoImport(size)
	}

	writable, err := openWritable(f.handle, true)
	if err != nil {
		return err
	}
	if _, err := AwaitPromise(jsutil.Call(writable, "truncate", size)); err != nil {
		AwaitPromise(jsutil.Call(writable, "close")) //nolint
		return errors.Wrap(err, "truncate")
	}
	if _, err := AwaitPromise(jsutil.Call(writable, "close")); err != nil {
		return errors.Wrap(err, "close writable")
	}
	return nil
}

// Stat returns file info.
//
// OPFS does not expose a modification time, so ModTime() on the returned
// fs.FileInfo is always the zero Time. Do not rely on it for ordering.
func (f *AsyncFile) Stat() (fs.FileInfo, error) {
	size, err := f.Size()
	if err != nil {
		return nil, err
	}
	return &syncFileInfo{name: f.name, size: size}, nil
}

// Close releases driver-owned async file state.
func (f *AsyncFile) Close() error {
	return f.owner().closeAsyncFile(f)
}

func (BrowserDriver) closeAsyncFile(f *AsyncFile) error {
	return nil
}

// WriteFile creates or overwrites a file in the given directory.
//
// Small non-TinyGo writes perform the truncate, write, and close in a single
// createWritable session with keepExistingData=false: the draft starts empty,
// the write at offset 0 produces the new contents, and close() commits a file
// of exactly len(data) bytes (any prior file content is replaced). Large
// non-TinyGo writes use chunked commits to avoid holding a large Blob.
//
// TinyGo uses one JS helper promise for the full overwrite. The helper copies
// Go bytes before async work starts and owns the FileSystemWritableFileStream
// through close, so Go does not carry a writable session across scheduler
// resumptions while callers may be holding blockshard or GC Web Locks.
func WriteFile(dir js.Value, name string, data []byte) error {
	return DefaultDriver.WriteFile(dir, name, data)
}

// WriteFile creates or overwrites a file in the given directory.
func (d BrowserDriver) WriteFile(dir js.Value, name string, data []byte) error {
	if jsutil.UseTinyGoHelpers() {
		return writeFileWithTinyGoImport(dir, name, data)
	}
	if len(data) > browserDriverFileChunkSize {
		return d.writeFileChunked(dir, name, data)
	}

	opts := jsutil.NewObject()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name, opts))
	if err != nil {
		return errors.Wrap(err, "getFileHandle")
	}
	writable, err := openWritable(fileHandle, false)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		arr := jsutil.NewUint8Array(len(data))
		js.CopyBytesToJS(arr, data)
		if _, err := AwaitPromise(jsutil.Call(writable, "write", arr)); err != nil {
			if _, abortErr := AwaitPromise(jsutil.Call(writable, "abort")); abortErr != nil {
				return errors.Wrapf(err, "abort failed write: %v", abortErr)
			}
			return errors.Wrap(err, "write")
		}
	}
	if _, err := AwaitPromise(jsutil.Call(writable, "close")); err != nil {
		if _, abortErr := AwaitPromise(jsutil.Call(writable, "abort")); abortErr != nil {
			return errors.Wrapf(err, "abort failed close: %v", abortErr)
		}
		return errors.Wrap(err, "close writable")
	}
	return nil
}

// CreateWriteStream creates or replaces a file and opens one streaming writer.
func CreateWriteStream(dir js.Value, name string) (*WriteStream, error) {
	return DefaultDriver.CreateWriteStream(dir, name)
}

// CreateWriteStream creates or replaces a file and opens one streaming writer.
func (d BrowserDriver) CreateWriteStream(dir js.Value, name string) (*WriteStream, error) {
	if jsutil.UseTinyGoHelpers() {
		return createWriteStreamWithTinyGoImport(dir, name)
	}

	opts := jsutil.NewObject()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name, opts))
	if err != nil {
		return nil, errors.Wrap(err, "getFileHandle")
	}
	writable, err := openWritable(fileHandle, false)
	if err != nil {
		return nil, err
	}
	return &WriteStream{driver: d, name: name, writable: writable}, nil
}

// Write appends p to the stream's current offset.
func (w *WriteStream) Write(p []byte) (int, error) {
	return w.owner().writeStream(w, p)
}

func (BrowserDriver) writeStream(w *WriteStream, p []byte) (int, error) {
	if jsutil.UseTinyGoHelpers() {
		return w.writeWithTinyGoImport(p)
	}

	arr := jsutil.NewUint8Array(len(p))
	js.CopyBytesToJS(arr, p)
	if _, err := AwaitPromise(jsutil.Call(w.writable, "write", arr)); err != nil {
		return 0, errors.Wrap(err, "write")
	}
	w.pos += int64(len(p))
	return len(p), nil
}

// Close commits the writable session.
func (w *WriteStream) Close() error {
	return w.owner().closeWriteStream(w)
}

func (BrowserDriver) closeWriteStream(w *WriteStream) error {
	if jsutil.UseTinyGoHelpers() {
		return w.closeWithTinyGoImport()
	}
	if _, err := AwaitPromise(jsutil.Call(w.writable, "close")); err != nil {
		return errors.Wrap(err, "close writable")
	}
	return nil
}

// Abort discards the writable session.
func (w *WriteStream) Abort() error {
	return w.owner().abortWriteStream(w)
}

func (BrowserDriver) abortWriteStream(w *WriteStream) error {
	if jsutil.UseTinyGoHelpers() {
		return w.abortWithTinyGoImport()
	}
	_, err := AwaitPromise(jsutil.Call(w.writable, "abort"))
	return err
}

// ReadFile reads the contents of a file in the given directory.
func ReadFile(dir js.Value, name string) ([]byte, error) {
	return DefaultDriver.ReadFile(dir, name)
}

// ReadFile reads the contents of a file in the given directory.
func (d BrowserDriver) ReadFile(dir js.Value, name string) ([]byte, error) {
	f, err := d.OpenAsyncFile(dir, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	size, err := f.Size()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	if int64(int(size)) != size {
		return nil, errors.Errorf("file %s too large to read into memory: %d bytes", name, size)
	}
	buf := make([]byte, int(size))
	for off := int64(0); off < size; {
		end := min(off+browserDriverFileChunkSize, size)
		n, err := f.ReadAt(buf[off:end], off)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n == 0 {
			return nil, errors.Errorf("short read file %s at offset %d", name, off)
		}
		off += int64(n)
	}
	return buf, nil
}

func (d BrowserDriver) writeFileChunked(dir js.Value, name string, data []byte) error {
	f, err := d.CreateAsyncFile(dir, name)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	for off := 0; off < len(data); off += browserDriverFileChunkSize {
		end := min(off+browserDriverFileChunkSize, len(data))
		if _, err := f.WriteAt(data[off:end], int64(off)); err != nil {
			return err
		}
	}
	return nil
}

// DeleteEntry removes a file or directory entry from the parent directory.
// Returns a "not found" JSError if the entry does not exist.
func DeleteEntry(dir js.Value, name string, recursive bool) error {
	return DefaultDriver.DeleteEntry(dir, name, recursive)
}

// DeleteEntry removes a file or directory entry from the parent directory.
// Returns a "not found" JSError if the entry does not exist.
func (d BrowserDriver) DeleteEntry(dir js.Value, name string, recursive bool) error {
	if jsutil.UseTinyGoHelpers() {
		var lastErr error
		for range syncAccessHandleRetries {
			err := deleteEntryWithTinyGoImport(dir, name, recursive)
			if err == nil {
				return nil
			}
			if d.ClassifyError(err) != ErrorKindNoModificationAllowed {
				return err
			}
			lastErr = err
			if err := yieldMicrotask(); err != nil {
				return err
			}
		}
		return lastErr
	}

	opts := jsutil.NewObject()
	opts.Set("recursive", recursive)
	var lastErr error
	for range syncAccessHandleRetries {
		_, err := AwaitPromise(jsutil.Call(dir, "removeEntry", name, opts))
		if err == nil {
			return nil
		}
		if d.ClassifyError(err) != ErrorKindNoModificationAllowed {
			return err
		}
		lastErr = err
		if err := yieldMicrotask(); err != nil {
			return err
		}
	}
	return lastErr
}

// DeleteFile removes a file from the given directory.
// Returns a "not found" JSError if the file does not exist.
func DeleteFile(dir js.Value, name string) error {
	if err := DeleteEntry(dir, name, false); err != nil {
		return err
	}
	for range deleteVisibilityRetries {
		exists, err := FileExists(dir, name)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if err := yieldMicrotask(); err != nil {
			return err
		}
	}
	exists, err := FileExists(dir, name)
	if err != nil {
		return err
	}
	if exists {
		return errors.Errorf("delete file %s: still exists after delete", name)
	}
	return nil
}

// ListDirectory returns sorted entry names in the given directory.
func ListDirectory(dir js.Value) ([]string, error) {
	return DefaultDriver.ListDirectory(dir)
}

// ListDirectory returns sorted entry names in the given directory.
func (BrowserDriver) ListDirectory(dir js.Value) ([]string, error) {
	if jsutil.UseTinyGoHelpers() {
		return listDirectoryWithTinyGoImport(dir)
	}

	// OPFS directories expose an async iterator via values().
	// We iterate it by calling .next() repeatedly.
	iter := jsutil.Call(dir, "entries")
	var names []string
	for {
		nextResult, err := AwaitPromise(jsutil.Call(iter, "next"))
		if err != nil {
			return nil, errors.Wrap(err, "iterator next")
		}
		done := nextResult.Get("done").Bool()
		if done {
			break
		}

		// entry is [name, handle]
		entry := nextResult.Get("value")
		name := entry.Index(0).String()
		names = append(names, name)
	}
	return names, nil
}

func decodeHelperNameList(buf []byte) ([]string, error) {
	if len(buf) < 4 {
		return nil, errors.New("opfs list directory helper returned truncated name count")
	}
	count := int(binary.BigEndian.Uint32(buf[:4]))
	names := make([]string, 0, count)
	buf = buf[4:]
	for range count {
		if len(buf) < 4 {
			return nil, errors.New("opfs list directory helper returned truncated name length")
		}
		size := int(binary.BigEndian.Uint32(buf[:4]))
		buf = buf[4:]
		if size > len(buf) {
			return nil, errors.New("opfs list directory helper returned truncated name bytes")
		}
		names = append(names, string(buf[:size]))
		buf = buf[size:]
	}
	if len(buf) != 0 {
		return nil, errors.New("opfs list directory helper returned trailing bytes")
	}
	return names, nil
}

// FileExists checks if a file exists in the given directory without reading it.
func FileExists(dir js.Value, name string) (bool, error) {
	return DefaultDriver.FileExists(dir, name)
}

// FileExists checks if a file exists in the given directory without reading it.
func (d BrowserDriver) FileExists(dir js.Value, name string) (bool, error) {
	if jsutil.UseTinyGoHelpers() {
		return fileExistsWithTinyGoImport(dir, name)
	}

	_, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		if d.ClassifyError(err) == ErrorKindNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DirExists checks if a subdirectory exists in the given directory.
func (d BrowserDriver) DirExists(dir js.Value, name string) (bool, error) {
	_, err := d.GetDirectory(dir, name, false)
	if err != nil {
		if d.ClassifyError(err) == ErrorKindNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SyncAvailable returns true if sync access handles are available.
// Sync access handles are only available in DedicatedWorker contexts.
func SyncAvailable() bool {
	return DefaultDriver.SyncAvailable()
}

// SyncAvailable returns true if sync access handles are available.
// createSyncAccessHandle is [Exposed=DedicatedWorker], so sync handles are only
// usable in a DedicatedWorker global scope. Probing the method alone is not
// enough: a non-conforming SharedWorker or main-thread context that exposes the
// method as a throwing stub would otherwise select the sync path and crash. Gate
// on the actual DedicatedWorker global scope so SharedWorker and main-thread
// contexts always fall back to async OPFS.
func (BrowserDriver) SyncAvailable() bool {
	global := js.Global()
	ctorName := global.Get("constructor").Get("name")
	if ctorName.Type() != js.TypeString || ctorName.String() != "DedicatedWorkerGlobalScope" {
		return false
	}
	fileHandleCtor := global.Get("FileSystemFileHandle")
	if fileHandleCtor.IsUndefined() || fileHandleCtor.IsNull() {
		return false
	}
	proto := fileHandleCtor.Get("prototype")
	if proto.IsUndefined() || proto.IsNull() {
		return false
	}
	method := proto.Get("createSyncAccessHandle")
	return method.Type() == js.TypeFunction
}

// PreferSyncAccessHandles reports whether writes should take the sync access
// handle path, which needs both SyncAvailable and a build that does not use
// the TinyGo helpers. A TinyGo DedicatedWorker exposing createSyncAccessHandle
// reports false, and its writes go through the async path.
func PreferSyncAccessHandles() bool {
	return DefaultDriver.PreferSyncAccessHandles()
}

// PreferSyncAccessHandles reports whether writes should take the sync access
// handle path, which needs both SyncAvailable and a build that does not use
// the TinyGo helpers. A TinyGo DedicatedWorker exposing createSyncAccessHandle
// reports false, and its writes go through the async path.
func (d BrowserDriver) PreferSyncAccessHandles() bool {
	return d.SyncAvailable() && !jsutil.UseTinyGoHelpers()
}

// OpenSyncFile opens an existing file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func OpenSyncFile(dir js.Value, name string) (*SyncFile, error) {
	return DefaultDriver.OpenSyncFile(dir, name)
}

// OpenSyncFile opens an existing file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func (BrowserDriver) OpenSyncFile(dir js.Value, name string) (*SyncFile, error) {
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		return nil, err
	}
	return newSyncFile(name, fileHandle)
}

// CreateSyncFile opens or creates a file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func CreateSyncFile(dir js.Value, name string) (*SyncFile, error) {
	return DefaultDriver.CreateSyncFile(dir, name)
}

// CreateSyncFile opens or creates a file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func (d BrowserDriver) CreateSyncFile(dir js.Value, name string) (*SyncFile, error) {
	return d.CreateSyncFileContext(context.Background(), dir, name)
}

// CreateSyncFileContext opens or creates a file with a sync access handle and
// attributes the handle lookup and access-handle creation work to ctx.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func CreateSyncFileContext(ctx context.Context, dir js.Value, name string) (*SyncFile, error) {
	return DefaultDriver.CreateSyncFileContext(ctx, dir, name)
}

// CreateSyncFileContext opens or creates a file with a sync access handle and
// attributes the handle lookup and access-handle creation work to ctx.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func (BrowserDriver) CreateSyncFileContext(ctx context.Context, dir js.Value, name string) (*SyncFile, error) {
	// Split lookup from sync-handle creation so traces show which OPFS call is expensive.
	_, subtask := trace.NewTask(ctx, "hydra/opfs/create-sync-file/get-file-handle")
	opts := jsutil.NewObject()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name, opts))
	subtask.End()

	if err != nil {
		return nil, errors.Wrap(err, "getFileHandle")
	}
	return newSyncFileContext(ctx, name, fileHandle)
}

// SyncFile wraps a FileSystemSyncAccessHandle as an fs.File.
// Supports Read, ReadAt, Write, WriteAt, Seek, Truncate, Flush, Close.
// Only available in DedicatedWorker contexts.
type SyncFile struct {
	name string
	ah   js.Value
	pos  int64
}

// IsNoModificationAllowed checks if an error is a "NoModificationAllowedError"
// DOMException. This occurs when createSyncAccessHandle is called while another
// access handle or writable stream is open on the same file.
func IsNoModificationAllowed(err error) bool {
	return DefaultDriver.ClassifyError(err) == ErrorKindNoModificationAllowed
}

// syncAccessHandleRetries is the number of times to retry createSyncAccessHandle
// when it fails with NoModificationAllowedError (stale handle closing).
const syncAccessHandleRetries = 3

// deleteVisibilityRetries is the number of event-loop turns to wait for
// removeEntry() visibility before treating the delete as failed.
const deleteVisibilityRetries = 16

func newSyncFile(name string, fileHandle js.Value) (*SyncFile, error) {
	return newSyncFileContext(context.Background(), name, fileHandle)
}

func newSyncFileContext(ctx context.Context, name string, fileHandle js.Value) (*SyncFile, error) {
	method := fileHandle.Get("createSyncAccessHandle")
	if method.IsUndefined() || method.IsNull() || method.Type() != js.TypeFunction {
		return nil, errors.New("createSyncAccessHandle unavailable")
	}

	var lastErr error
	for range syncAccessHandleRetries {
		// Trace each open attempt separately so contention retries stay visible.
		_, subtask := trace.NewTask(ctx, "hydra/opfs/create-sync-file/create-sync-access-handle")
		ah, err := AwaitPromise(jsutil.Call(fileHandle, "createSyncAccessHandle"))
		subtask.End()

		if err == nil {
			return &SyncFile{name: name, ah: ah}, nil
		}
		if !IsNoModificationAllowed(err) {
			return nil, errors.Wrap(err, "createSyncAccessHandle")
		}

		_, subtask = trace.NewTask(ctx, "hydra/opfs/create-sync-file/no-modification-allowed")
		subtask.End()
		lastErr = err

		if err := yieldMicrotask(); err != nil {
			return nil, err
		}
	}
	return nil, errors.Wrap(lastErr, "createSyncAccessHandle")
}

// Read reads up to len(p) bytes from the current position.
func (f *SyncFile) Read(p []byte) (int, error) {
	n, err := f.ReadAt(p, f.pos)
	f.pos += int64(n)
	return n, err
}

// ReadAt reads len(p) bytes from the file starting at byte offset off.
func (f *SyncFile) ReadAt(p []byte, off int64) (int, error) {
	arr := jsutil.NewUint8Array(len(p))
	opts := jsutil.NewObject()
	opts.Set("at", off)
	n := jsutil.Call(f.ah, "read", arr, opts).Int()
	js.CopyBytesToGo(p[:n], arr)
	if n == 0 && len(p) > 0 {
		return 0, io.EOF
	}
	return n, nil
}

// Write writes len(p) bytes at the current position.
func (f *SyncFile) Write(p []byte) (int, error) {
	n, err := f.WriteAt(p, f.pos)
	f.pos += int64(n)
	return n, err
}

// WriteAt writes len(p) bytes to the file starting at byte offset off.
func (f *SyncFile) WriteAt(p []byte, off int64) (int, error) {
	arr := jsutil.NewUint8Array(len(p))
	js.CopyBytesToJS(arr, p)
	opts := jsutil.NewObject()
	opts.Set("at", off)
	n := jsutil.Call(f.ah, "write", arr, opts).Int()
	return n, nil
}

// openWritable creates a FileSystemWritableFileStream on fileHandle.
//
// keepExisting controls the createWritable() keepExistingData option:
//   - true: the writable's draft starts as a copy of the source file. Required
//     for partial writes (WriteAt with off>0) and for Truncate to a non-zero
//     size; without it, close() replaces the source with a draft that only
//     contains the bytes explicitly written, destroying every other byte.
//   - false: the draft starts empty. Use only when the entire file content is
//     about to be (re)written, since close() will commit exactly the bytes
//     written and discard the prior file.
func openWritable(fileHandle js.Value, keepExisting bool) (js.Value, error) {
	var opts js.Value
	if keepExisting {
		opts = jsutil.NewObject()
		opts.Set("keepExistingData", true)
	}
	var lastErr error
	for range syncAccessHandleRetries {
		var writable js.Value
		var err error
		if keepExisting {
			writable, err = AwaitPromise(jsutil.Call(fileHandle, "createWritable", opts))
		} else {
			writable, err = AwaitPromise(jsutil.Call(fileHandle, "createWritable"))
		}
		if err == nil {
			return writable, nil
		}
		if !IsNoModificationAllowed(err) {
			return js.Null(), errors.Wrap(err, "createWritable")
		}
		lastErr = err
		if err := yieldMicrotask(); err != nil {
			return js.Null(), err
		}
	}
	return js.Null(), errors.Wrap(lastErr, "createWritable")
}

// Seek sets the offset for the next Read or Write.
func (f *SyncFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = offset
	case io.SeekCurrent:
		f.pos += offset
	case io.SeekEnd:
		f.pos = int64(jsutil.Call(f.ah, "getSize").Int()) + offset
	}
	return f.pos, nil
}

// Size returns the file size in bytes.
func (f *SyncFile) Size() int64 {
	return int64(jsutil.Call(f.ah, "getSize").Int())
}

// Truncate sets the file size. Pads with zero bytes if growing.
func (f *SyncFile) Truncate(size int64) {
	jsutil.Call(f.ah, "truncate", size)
}

// Flush flushes buffered writes to stable storage.
func (f *SyncFile) Flush() {
	jsutil.Call(f.ah, "flush")
}

// Stat returns file info.
//
// OPFS does not expose a modification time, so ModTime() on the returned
// fs.FileInfo is always the zero Time. Do not rely on it for ordering.
func (f *SyncFile) Stat() (fs.FileInfo, error) {
	return &syncFileInfo{name: f.name, size: f.Size()}, nil
}

// Close releases the sync access handle.
func (f *SyncFile) Close() error {
	jsutil.Call(f.ah, "close")
	return nil
}

// syncFileInfo implements fs.FileInfo for SyncFile.
type syncFileInfo struct {
	name string
	size int64
}

func (fi *syncFileInfo) Name() string       { return fi.name }
func (fi *syncFileInfo) Size() int64        { return fi.size }
func (fi *syncFileInfo) Mode() fs.FileMode  { return 0o644 }
func (fi *syncFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *syncFileInfo) IsDir() bool        { return false }
func (fi *syncFileInfo) Sys() any           { return nil }

// _ is a type assertion.
var (
	_ fs.File     = (*SyncFile)(nil)
	_ io.ReaderAt = (*SyncFile)(nil)
	_ io.WriterAt = (*SyncFile)(nil)
	_ io.Seeker   = (*SyncFile)(nil)

	_ fs.File     = (*AsyncFile)(nil)
	_ io.ReaderAt = (*AsyncFile)(nil)
	_ io.WriterAt = (*AsyncFile)(nil)
	_ io.Seeker   = (*AsyncFile)(nil)

	_ fs.FileInfo = (*syncFileInfo)(nil)
)

// DirExists checks if a subdirectory exists in the given directory.
func DirExists(dir js.Value, name string) (bool, error) {
	return DefaultDriver.DirExists(dir, name)
}
