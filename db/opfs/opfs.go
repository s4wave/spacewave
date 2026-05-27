//go:build js

package opfs

import (
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"sync"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/util/jsbuf"
)

const (
	jsErrorCodeUnknown = iota
	jsErrorCodeNotFound
	jsErrorCodeNoModificationAllowed

	tinyGoOPFSReadFile  = "BLDR_OPFS_READ_FILE"
	tinyGoOPFSReadAt    = "BLDR_OPFS_READ_AT"
	tinyGoOPFSListDir   = "BLDR_OPFS_LIST_DIRECTORY"
	tinyGoOPFSWriteAt   = "BLDR_OPFS_WRITE_AT"
	tinyGoOPFSWriteFile = "BLDR_OPFS_WRITE_FILE"

	tinyGoOPFSWriteFileBegin = "BLDR_OPFS_WRITE_FILE_BEGIN"
	tinyGoOPFSWriteFileChunk = "BLDR_OPFS_WRITE_FILE_CHUNK"
	tinyGoOPFSWriteFileClose = "BLDR_OPFS_WRITE_FILE_CLOSE"
	tinyGoOPFSWriteFileAbort = "BLDR_OPFS_WRITE_FILE_ABORT"

	tinyGoOPFSWriteFileChunkSize = 1 << 20
)

type opfsHelperResult struct {
	values []int
	err    error
}

var (
	opfsHelperMu     sync.Mutex
	opfsHelperNextID int
	opfsHelperOps    = map[int]chan opfsHelperResult{}
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
	default:
		return &JSError{Message: "promise rejected"}
	}
}

// AwaitPromise blocks the calling goroutine until a JS Promise resolves or rejects.
// Returns the resolved value or an error wrapping the rejection reason.
func AwaitPromise(promise js.Value) (js.Value, error) {
	ch := make(chan struct{})
	var closeOnce sync.Once
	var result js.Value
	var jsErr error

	thenCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			result = args[0]
		} else {
			result = js.Undefined()
		}
		closeOnce.Do(func() { close(ch) })
		return nil
	})
	defer thenCb.Release()

	catchCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			jsErr = newJSError(args[0])
		} else {
			jsErr = errors.New("promise rejected")
		}
		closeOnce.Do(func() { close(ch) })
		return nil
	})
	defer catchCb.Release()

	jsutil.AwaitPromise(promise, thenCb, catchCb)
	<-ch

	return result, jsErr
}

func yieldMicrotask() error {
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
	storage := js.Global().Get("navigator").Get("storage")
	promise := jsutil.Call(storage, "getDirectory")
	return AwaitPromise(promise)
}

// GetDirectory returns a subdirectory handle within parent.
// If create is true, the directory is created if it does not exist.
func GetDirectory(parent js.Value, name string, create bool) (js.Value, error) {
	opts := jsutil.NewObject()
	opts.Set("create", create)
	promise := jsutil.Call(parent, "getDirectoryHandle", name, opts)
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
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		return nil, err
	}
	return &AsyncFile{name: name, handle: fileHandle}, nil
}

// CreateAsyncFile opens or creates a file with async OPFS APIs.
// Works in any context (SharedWorker, DedicatedWorker, main thread).
func CreateAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	opts := jsutil.NewObject()
	opts.Set("create", true)
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name, opts))
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
	if jsutil.UseTinyGoHelpers() {
		readAt := js.Global().Get(tinyGoOPFSReadAt)
		if jsutil.Available(readAt) {
			return f.readAtWithHelper(readAt, p, off)
		}
	}

	file, err := AwaitPromise(jsutil.Call(f.handle, "getFile"))
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

func (f *AsyncFile) readAtWithHelper(readAt js.Value, p []byte, off int64) (int, error) {
	dst, err := jsbuf.NewBytes(len(p))
	if err != nil {
		return 0, err
	}

	n, err := invokeOPFSIntHelper(func(opID int) {
		readAt.Invoke(f.handle, dst, off, opID)
	})
	if err != nil {
		return 0, err
	}
	if n > len(p) {
		return 0, errors.Errorf("read exceeded buffer for file %s: read %d into %d", f.name, n, len(p))
	}
	if n > 0 {
		js.CopyBytesToGo(p[:n], dst)
	}
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
	ctx, task := trace.NewTask(ctx, "hydra/opfs/async-file/write-at")
	defer task.End()

	if jsutil.UseTinyGoHelpers() {
		writeAt := js.Global().Get(tinyGoOPFSWriteAt)
		if jsutil.Available(writeAt) {
			return f.writeAtWithHelper(writeAt, p, off, true)
		}
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
			AwaitPromise(jsutil.Call(writable, "close")) //nolint
			return 0, errors.Wrap(err, "seek")
		}
	}

	arr := jsutil.NewUint8Array(len(p))
	js.CopyBytesToJS(arr, p)

	writeDataCtx, writeDataTask := trace.NewTask(writeCtx, "hydra/opfs/async-file/write-at/write")
	_, err = AwaitPromise(jsutil.Call(writable, "write", arr))
	writeDataTask.End()
	if err != nil {
		AwaitPromise(jsutil.Call(writable, "close")) //nolint
		return 0, errors.Wrap(err, "write")
	}

	_, closeTask := trace.NewTask(writeDataCtx, "hydra/opfs/async-file/write-at/close-writable")
	_, err = AwaitPromise(jsutil.Call(writable, "close"))
	closeTask.End()
	if err != nil {
		return len(p), errors.Wrap(err, "close writable")
	}
	return len(p), nil
}

func (f *AsyncFile) writeAtWithHelper(writeAt js.Value, p []byte, off int64, keepExisting bool) (int, error) {
	data, err := jsbuf.CopyBytesToJS(p)
	if err != nil {
		return 0, err
	}
	written, err := invokeOPFSIntHelper(func(opID int) {
		writeAt.Invoke(f.handle, data, off, keepExisting, opID)
	})
	if err != nil {
		return 0, err
	}
	if written != len(p) {
		return written, errors.Errorf("short write file %s: wrote %d of %d", f.name, written, len(p))
	}
	return written, nil
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
	return result.values, result.err
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
		file, err := AwaitPromise(jsutil.Call(f.handle, "getFile"))
		if err != nil {
			return f.pos, errors.Wrap(err, "getFile")
		}
		f.pos = int64(file.Get("size").Int()) + offset
	}
	return f.pos, nil
}

// Size returns the file size in bytes.
func (f *AsyncFile) Size() (int64, error) {
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

// Close is a no-op for async files (no persistent handle to release).
func (f *AsyncFile) Close() error {
	return nil
}

// WriteFile creates or overwrites a file in the given directory.
//
// Performs the truncate, write, and close in a single createWritable session
// with keepExistingData=false: the draft starts empty, the write at offset 0
// produces the new contents, and close() commits a file of exactly len(data)
// bytes (any prior file content is replaced). One Promise round-trip per
// stage instead of two (vs separate Truncate then Write calls).
func WriteFile(dir js.Value, name string, data []byte) error {
	if jsutil.UseTinyGoHelpers() {
		begin := js.Global().Get(tinyGoOPFSWriteFileBegin)
		chunk := js.Global().Get(tinyGoOPFSWriteFileChunk)
		closeFile := js.Global().Get(tinyGoOPFSWriteFileClose)
		if jsutil.Available(begin) && jsutil.Available(chunk) && jsutil.Available(closeFile) {
			return writeFileWithChunkedHelper(begin, chunk, closeFile, js.Global().Get(tinyGoOPFSWriteFileAbort), dir, name, data)
		}

		writeFile := js.Global().Get(tinyGoOPFSWriteFile)
		if jsutil.Available(writeFile) {
			return writeFileWithHelper(writeFile, dir, name, data)
		}
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
			AwaitPromise(jsutil.Call(writable, "close")) //nolint
			return errors.Wrap(err, "write")
		}
	}
	if _, err := AwaitPromise(jsutil.Call(writable, "close")); err != nil {
		return errors.Wrap(err, "close writable")
	}
	return nil
}

func writeFileWithChunkedHelper(begin, chunk, closeFile, abort js.Value, dir js.Value, name string, data []byte) error {
	sessionID, err := invokeOPFSIntHelper(func(opID int) {
		begin.Invoke(dir, name, opID)
	})
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed && jsutil.Available(abort) {
			abort.Invoke(sessionID)
		}
	}()

	written := 0
	for off := 0; off < len(data); off += tinyGoOPFSWriteFileChunkSize {
		end := off + tinyGoOPFSWriteFileChunkSize
		if end > len(data) {
			end = len(data)
		}
		bytes, err := jsbuf.CopyBytesToJS(data[off:end])
		if err != nil {
			return err
		}
		n, err := invokeOPFSIntHelper(func(opID int) {
			chunk.Invoke(sessionID, bytes, opID)
		})
		if err != nil {
			return err
		}
		if n != end-off {
			return errors.Errorf("short write file %s: wrote chunk %d of %d", name, n, end-off)
		}
		written += n
	}

	closedWritten, err := invokeOPFSIntHelper(func(opID int) {
		closeFile.Invoke(sessionID, opID)
	})
	if err != nil {
		return err
	}
	closed = true
	if closedWritten != written || written != len(data) {
		return errors.Errorf("short write file %s: wrote %d of %d", name, closedWritten, len(data))
	}
	return nil
}

func writeFileWithHelper(writeFile js.Value, dir js.Value, name string, data []byte) error {
	bytes, err := jsbuf.CopyBytesToJS(data)
	if err != nil {
		return err
	}
	written, err := invokeOPFSIntHelper(func(opID int) {
		writeFile.Invoke(dir, name, bytes, opID)
	})
	if err != nil {
		return err
	}
	if written != len(data) {
		return errors.Errorf("short write file %s: wrote %d of %d", name, written, len(data))
	}
	return nil
}

// ReadFile reads the contents of a file in the given directory.
func ReadFile(dir js.Value, name string) ([]byte, error) {
	if jsutil.UseTinyGoHelpers() {
		readFile := js.Global().Get(tinyGoOPFSReadFile)
		if jsutil.Available(readFile) {
			return readFileWithHelper(readFile, dir, name)
		}
	}

	f, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		return nil, err
	}
	file, err := AwaitPromise(jsutil.Call(f, "getFile"))
	if err != nil {
		return nil, errors.Wrap(err, "getFile")
	}
	ab, err := AwaitPromise(jsutil.Call(file, "arrayBuffer"))
	if err != nil {
		return nil, errors.Wrap(err, "arrayBuffer")
	}
	arr := jsutil.NewUint8Array(ab)
	buf := make([]byte, arr.Get("length").Int())
	js.CopyBytesToGo(buf, arr)
	return buf, nil
}

func readFileWithHelper(readFile js.Value, dir js.Value, name string) ([]byte, error) {
	values, err := invokeOPFSHelper(func(opID int) {
		readFile.Invoke(dir, name, opID)
	})
	if err != nil {
		return nil, err
	}
	if len(values) < 2 {
		return nil, errors.New("opfs read file helper returned incomplete result")
	}
	id, size := values[0], values[1]
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	n, ok := jsutil.CopyStoredBytes(id, buf)
	if !ok {
		return nil, errors.New("opfs read file byte copy helper unavailable")
	}
	if n != size {
		return nil, errors.Errorf("opfs read file copied %d bytes, expected %d", n, size)
	}
	return buf, nil
}

// DeleteEntry removes a file or directory entry from the parent directory.
// Returns a "not found" JSError if the entry does not exist.
func DeleteEntry(dir js.Value, name string, recursive bool) error {
	opts := jsutil.NewObject()
	opts.Set("recursive", recursive)
	var lastErr error
	for range syncAccessHandleRetries {
		_, err := AwaitPromise(jsutil.Call(dir, "removeEntry", name, opts))
		if err == nil {
			return nil
		}
		if !IsNoModificationAllowed(err) {
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
	if jsutil.UseTinyGoHelpers() {
		listDirectory := js.Global().Get(tinyGoOPFSListDir)
		if jsutil.Available(listDirectory) {
			return listDirectoryWithHelper(listDirectory, dir)
		}
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

func listDirectoryWithHelper(listDirectory js.Value, dir js.Value) ([]string, error) {
	values, err := invokeOPFSHelper(func(opID int) {
		listDirectory.Invoke(dir, opID)
	})
	if err != nil {
		return nil, err
	}
	if len(values) < 2 {
		return nil, errors.New("opfs list directory helper returned incomplete result")
	}
	id, size := values[0], values[1]
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	n, ok := jsutil.CopyStoredBytes(id, buf)
	if !ok {
		return nil, errors.New("opfs list directory byte copy helper unavailable")
	}
	if n != size {
		return nil, errors.Errorf("opfs list directory copied %d bytes, expected %d", n, size)
	}
	names, err := decodeHelperNameList(buf)
	if err != nil {
		return nil, errors.Wrap(err, "decode opfs list directory names")
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
	for i := 0; i < count; i++ {
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
	_, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
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

// PreferSyncAccessHandles reports whether OPFS owners should use sync access
// handles for writes in the current runtime.
func PreferSyncAccessHandles() bool {
	return SyncAvailable() && !jsutil.UseTinyGoHelpers()
}

// OpenSyncFile opens an existing file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func OpenSyncFile(dir js.Value, name string) (*SyncFile, error) {
	fileHandle, err := AwaitPromise(jsutil.Call(dir, "getFileHandle", name))
	if err != nil {
		return nil, err
	}
	return newSyncFile(name, fileHandle)
}

// CreateSyncFile opens or creates a file with a sync access handle.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func CreateSyncFile(dir js.Value, name string) (*SyncFile, error) {
	return CreateSyncFileContext(context.Background(), dir, name)
}

// CreateSyncFileContext opens or creates a file with a sync access handle and
// attributes the handle lookup and access-handle creation work to ctx.
// Only available in DedicatedWorker contexts (check SyncAvailable first).
func CreateSyncFileContext(ctx context.Context, dir js.Value, name string) (*SyncFile, error) {
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
	var jsErr *JSError
	return errors.As(err, &jsErr) && jsErr.Name == "NoModificationAllowedError"
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
	_, err := GetDirectory(dir, name, false)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
