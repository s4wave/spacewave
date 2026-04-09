//go:build js

package opfs

import (
	"io"
	"io/fs"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
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
	var jsErr *JSError
	return errors.As(err, &jsErr) && jsErr.Name == "NotFoundError"
}

// newJSError creates a JSError from a js.Value error object.
func newJSError(val js.Value) *JSError {
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
		e.Message = val.Call("toString").String()
	}
	return e
}

// AwaitPromise blocks the calling goroutine until a JS Promise resolves or rejects.
// Returns the resolved value or an error wrapping the rejection reason.
func AwaitPromise(promise js.Value) (js.Value, error) {
	ch := make(chan struct{})
	var result js.Value
	var jsErr error

	thenCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			result = args[0]
		} else {
			result = js.Undefined()
		}
		close(ch)
		return nil
	})
	defer thenCb.Release()

	catchCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			jsErr = newJSError(args[0])
		} else {
			jsErr = errors.New("promise rejected")
		}
		close(ch)
		return nil
	})
	defer catchCb.Release()

	promise.Call("then", thenCb).Call("catch", catchCb)
	<-ch

	return result, jsErr
}

// GetRoot returns the OPFS root FileSystemDirectoryHandle.
func GetRoot() (js.Value, error) {
	storage := js.Global().Get("navigator").Get("storage")
	promise := storage.Call("getDirectory")
	return AwaitPromise(promise)
}

// GetDirectory returns a subdirectory handle within parent.
// If create is true, the directory is created if it does not exist.
func GetDirectory(parent js.Value, name string, create bool) (js.Value, error) {
	opts := js.Global().Get("Object").New()
	opts.Set("create", create)
	promise := parent.Call("getDirectoryHandle", name, opts)
	return AwaitPromise(promise)
}

// GetDirectoryPath navigates a sequence of directory names from parent.
// Each element is a single directory name (no slashes).
// If create is true, intermediate directories are created.
func GetDirectoryPath(parent js.Value, path []string, create bool) (js.Value, error) {
	dir := parent
	for _, name := range path {
		next, err := GetDirectory(dir, name, create)
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
	fileHandle, err := AwaitPromise(dir.Call("getFileHandle", name))
	if err != nil {
		return nil, err
	}
	return &AsyncFile{name: name, handle: fileHandle}, nil
}

// CreateAsyncFile opens or creates a file with async OPFS APIs.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
func CreateAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	opts := js.Global().Get("Object").New()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(dir.Call("getFileHandle", name, opts))
	if err != nil {
		return nil, errors.Wrap(err, "getFileHandle")
	}
	return &AsyncFile{name: name, handle: fileHandle}, nil
}

// AsyncFile wraps an async FileSystemFileHandle as an fs.File.
// Uses getFile()/slice() for reads and createWritable() for writes.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
type AsyncFile struct {
	name   string
	handle js.Value // FileSystemFileHandle
	pos    int64
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
	file, err := AwaitPromise(f.handle.Call("getFile"))
	if err != nil {
		return 0, errors.Wrap(err, "getFile")
	}

	size := file.Get("size").Int()
	if off >= int64(size) {
		return 0, io.EOF
	}

	end := off + int64(len(p))
	if end > int64(size) {
		end = int64(size)
	}

	blob := file.Call("slice", off, end)
	ab, err := AwaitPromise(blob.Call("arrayBuffer"))
	if err != nil {
		return 0, errors.Wrap(err, "arrayBuffer")
	}

	arr := js.Global().Get("Uint8Array").New(ab)
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
	n, err := f.WriteAt(p, f.pos)
	f.pos += int64(n)
	return n, err
}

// WriteAt writes len(p) bytes to the file starting at byte offset off.
func (f *AsyncFile) WriteAt(p []byte, off int64) (int, error) {
	writable, err := AwaitPromise(f.handle.Call("createWritable"))
	if err != nil {
		return 0, errors.Wrap(err, "createWritable")
	}

	if off > 0 {
		if _, err := AwaitPromise(writable.Call("seek", off)); err != nil {
			AwaitPromise(writable.Call("close")) //nolint
			return 0, errors.Wrap(err, "seek")
		}
	}

	arr := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(arr, p)

	if _, err := AwaitPromise(writable.Call("write", arr)); err != nil {
		AwaitPromise(writable.Call("close")) //nolint
		return 0, errors.Wrap(err, "write")
	}
	if _, err := AwaitPromise(writable.Call("close")); err != nil {
		return len(p), errors.Wrap(err, "close writable")
	}
	return len(p), nil
}

// Seek sets the offset for the next Read or Write.
func (f *AsyncFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = offset
	case io.SeekCurrent:
		f.pos += offset
	case io.SeekEnd:
		file, err := AwaitPromise(f.handle.Call("getFile"))
		if err != nil {
			return f.pos, errors.Wrap(err, "getFile")
		}
		f.pos = int64(file.Get("size").Int()) + offset
	}
	return f.pos, nil
}

// Size returns the file size in bytes.
func (f *AsyncFile) Size() (int64, error) {
	file, err := AwaitPromise(f.handle.Call("getFile"))
	if err != nil {
		return 0, errors.Wrap(err, "getFile")
	}
	return int64(file.Get("size").Int()), nil
}

// Truncate sets the file size via a writable stream.
func (f *AsyncFile) Truncate(size int64) error {
	writable, err := AwaitPromise(f.handle.Call("createWritable"))
	if err != nil {
		return errors.Wrap(err, "createWritable")
	}
	if _, err := AwaitPromise(writable.Call("truncate", size)); err != nil {
		AwaitPromise(writable.Call("close")) //nolint
		return errors.Wrap(err, "truncate")
	}
	if _, err := AwaitPromise(writable.Call("close")); err != nil {
		return errors.Wrap(err, "close writable")
	}
	return nil
}

// Stat returns file info.
func (f *AsyncFile) Stat() (fs.FileInfo, error) {
	size, err := f.Size()
	if err != nil {
		return nil, err
	}
	return &syncFileInfo{name: f.name, size: size}, nil
}

// Close is a no-op for async files (no persistent handle to release).
func (f *AsyncFile) Close() error {
	return nil
}

// WriteFile creates or overwrites a file in the given directory.
func WriteFile(dir js.Value, name string, data []byte) error {
	f, err := CreateAsyncFile(dir, name)
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// ReadFile reads the contents of a file in the given directory.
func ReadFile(dir js.Value, name string) ([]byte, error) {
	f, err := OpenAsyncFile(dir, name)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

// DeleteEntry removes a file or directory entry from the parent directory.
// Returns a "not found" JSError if the entry does not exist.
func DeleteEntry(dir js.Value, name string, recursive bool) error {
	opts := js.Global().Get("Object").New()
	opts.Set("recursive", recursive)
	_, err := AwaitPromise(dir.Call("removeEntry", name, opts))
	return err
}

// DeleteFile removes a file from the given directory.
// Returns a "not found" JSError if the file does not exist.
func DeleteFile(dir js.Value, name string) error {
	return DeleteEntry(dir, name, false)
}

// ListDirectory returns sorted entry names in the given directory.
func ListDirectory(dir js.Value) ([]string, error) {
	// OPFS directories expose an async iterator via values().
	// We iterate it by calling .next() repeatedly.
	iter := dir.Call("entries")
	var names []string
	for {
		nextResult, err := AwaitPromise(iter.Call("next"))
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

// FileExists checks if a file exists in the given directory without reading it.
func FileExists(dir js.Value, name string) (bool, error) {
	_, err := AwaitPromise(dir.Call("getFileHandle", name))
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SyncAvailable returns true if sync access handles are available.
// Sync access handles are only available in DedicatedWorker contexts.
func SyncAvailable() bool {
	fileHandleCtor := js.Global().Get("FileSystemFileHandle")
	if fileHandleCtor.IsUndefined() || fileHandleCtor.IsNull() {
		return false
	}
	proto := fileHandleCtor.Get("prototype")
	if proto.IsUndefined() || proto.IsNull() {
		return false
	}
	method := proto.Get("createSyncAccessHandle")
	return !method.IsUndefined() && !method.IsNull() && method.Type() == js.TypeFunction
}

// OpenSyncFile opens an existing file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func OpenSyncFile(dir js.Value, name string) (*SyncFile, error) {
	fileHandle, err := AwaitPromise(dir.Call("getFileHandle", name))
	if err != nil {
		return nil, err
	}
	return newSyncFile(name, fileHandle)
}

// CreateSyncFile opens or creates a file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func CreateSyncFile(dir js.Value, name string) (*SyncFile, error) {
	opts := js.Global().Get("Object").New()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(dir.Call("getFileHandle", name, opts))
	if err != nil {
		return nil, errors.Wrap(err, "getFileHandle")
	}
	return newSyncFile(name, fileHandle)
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
	var jsErr *JSError
	return errors.As(err, &jsErr) && jsErr.Name == "NoModificationAllowedError"
}

// syncAccessHandleRetries is the number of times to retry createSyncAccessHandle
// when it fails with NoModificationAllowedError (stale handle closing).
const syncAccessHandleRetries = 3

func newSyncFile(name string, fileHandle js.Value) (*SyncFile, error) {
	method := fileHandle.Get("createSyncAccessHandle")
	if method.IsUndefined() || method.IsNull() || method.Type() != js.TypeFunction {
		return nil, errors.New("createSyncAccessHandle unavailable")
	}
	var lastErr error
	for range syncAccessHandleRetries {
		ah, err := AwaitPromise(fileHandle.Call("createSyncAccessHandle"))
		if err == nil {
			return &SyncFile{name: name, ah: ah}, nil
		}
		if !IsNoModificationAllowed(err) {
			return nil, errors.Wrap(err, "createSyncAccessHandle")
		}
		lastErr = err
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
	arr := js.Global().Get("Uint8Array").New(len(p))
	opts := js.Global().Get("Object").New()
	opts.Set("at", off)
	n := f.ah.Call("read", arr, opts).Int()
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
	arr := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(arr, p)
	opts := js.Global().Get("Object").New()
	opts.Set("at", off)
	n := f.ah.Call("write", arr, opts).Int()
	return n, nil
}

// Seek sets the offset for the next Read or Write.
func (f *SyncFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = offset
	case io.SeekCurrent:
		f.pos += offset
	case io.SeekEnd:
		f.pos = int64(f.ah.Call("getSize").Int()) + offset
	}
	return f.pos, nil
}

// Size returns the file size in bytes.
func (f *SyncFile) Size() int64 {
	return int64(f.ah.Call("getSize").Int())
}

// Truncate sets the file size. Pads with zero bytes if growing.
func (f *SyncFile) Truncate(size int64) {
	f.ah.Call("truncate", size)
}

// Flush flushes buffered writes to stable storage.
func (f *SyncFile) Flush() {
	f.ah.Call("flush")
}

// Stat returns file info.
func (f *SyncFile) Stat() (fs.FileInfo, error) {
	return &syncFileInfo{name: f.name, size: f.Size()}, nil
}

// Close releases the sync access handle.
func (f *SyncFile) Close() error {
	f.ah.Call("close")
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
	_, err := GetDirectory(dir, name, false)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
