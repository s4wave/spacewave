//go:build js

package opfs

import (
	"context"
	"io"
	"syscall/js"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

const (
	remoteHandleIDField       = "__spacewaveOPFSRemoteHandleID"
	remoteHandleKindField     = "__spacewaveOPFSRemoteHandleKind"
	opfsBridgePortGlobal      = "__spacewaveOpfsBridgePort"
	opfsBridgeInstallerGlobal = "__spacewaveInstallOpfsRemoteDriver"

	remoteHandleKindDirectory = "directory"
	remoteHandleKindFile      = "file"
	remoteHandleKindStream    = "stream"
)

// RemoteDriver routes OPFS File System Access operations over a worker bridge.
type RemoteDriver struct {
	port js.Value

	// bcast is the single mutex guarding port, handles, and swapGen. It also
	// wakes WaitSwap waiters when the bridge port is swapped for a fresh OPFS
	// worker so the mounted volume can remount and rebuild its stale handles.
	bcast   broadcast.Broadcast
	handles map[int]remoteDriverHandle
	swapGen uint64

	installFunc      js.Func
	installFuncReady bool
}

type remoteDriverHandle struct {
	kind string
}

type remoteTransportError struct {
	op      string
	message string
}

func (e *remoteTransportError) Error() string {
	if e.op != "" && e.message != "" {
		return "opfs remote " + e.op + ": " + e.message
	}
	if e.op != "" {
		return "opfs remote " + e.op + ": transport failed"
	}
	return "opfs remote transport failed"
}

// NewRemoteDriver creates a RemoteDriver over a JavaScript OPFS bridge client.
func NewRemoteDriver(port js.Value) *RemoteDriver {
	d := &RemoteDriver{
		handles: make(map[int]remoteDriverHandle),
	}
	d.installPort(port)
	d.installGlobalSetter()
	return d
}

// InstallRemoteDriverFromGlobal selects a RemoteDriver when the worker runtime
// injected an OPFS bridge before Go startup.
func InstallRemoteDriverFromGlobal() bool {
	port := js.Global().Get(opfsBridgePortGlobal)
	if !remotePortUsable(port) {
		return false
	}
	if d, ok := DefaultDriver.(*RemoteDriver); ok {
		return d.installPort(port)
	}
	DefaultDriver = NewRemoteDriver(port)
	return true
}

func (d *RemoteDriver) installPort(port js.Value) bool {
	if !remotePortUsable(port) {
		return false
	}

	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// A swapped port is a fresh OPFS worker with its own handle id space, so
		// the cached directory/file/stream tokens are stale and must be dropped.
		// In-flight requests await the previous OpfsBridgeClient promise, which
		// that client rejects when it is closed during the swap.
		if remotePortUsable(d.port) && d.port.Equal(port) {
			return
		}

		// Replacing an existing usable port is a swap: bump the generation and
		// wake WaitSwap so the mounted volume remounts and rebuilds its handle
		// tree from a fresh GetRoot. Every cached handle from the root down
		// references the dead worker, and the volume cannot reopen them in place
		// because it does not own the directory chain. The first install (no
		// prior port) is not a swap and must not trigger a remount.
		swapped := remotePortUsable(d.port)
		d.port = port
		d.handles = make(map[int]remoteDriverHandle)
		if swapped {
			d.swapGen++
			broadcast()
		}
	})
	return true
}

// WaitSwap blocks until the bridge port is swapped for a fresh OPFS worker, or
// ctx is done. A swap invalidates every cached handle, so the mounted volume
// uses this to drive a remount from a fresh GetRoot.
func (d *RemoteDriver) WaitSwap(ctx context.Context) error {
	var startGen uint64
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		startGen = d.swapGen
	})

	// Capture the generation that was current when the wait began.
	for {
		// Wait for a generation change or context cancellation.
		var changed bool
		var waitCh <-chan struct{}

		// Compare the current generation and retain its wake channel atomically.
		d.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = d.swapGen != startGen
			waitCh = getWaitCh()
		})
		if changed {
			// Return after observing the bridge swap.
			return nil
		}

		// Wait for a swap notification or cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// WaitBridgeSwap blocks until the active driver's bridge port is swapped for a
// fresh OPFS worker, or ctx is done. The local browser driver has no bridge to
// swap, so it blocks until ctx is done.
func WaitBridgeSwap(ctx context.Context) error {
	if d, ok := DefaultDriver.(*RemoteDriver); ok {
		return d.WaitSwap(ctx)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (d *RemoteDriver) installGlobalSetter() {
	if d.installFuncReady {
		return
	}
	d.installFunc = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return false
		}
		return d.installPort(args[0])
	})
	d.installFuncReady = true
	js.Global().Set(opfsBridgeInstallerGlobal, d.installFunc)
}

// ClassifyError classifies a browser OPFS or remote DOMException error.
func (d *RemoteDriver) ClassifyError(err error) ErrorKind {
	return BrowserDriver{}.ClassifyError(err)
}

// GetRoot returns the remote OPFS root directory handle token.
func (d *RemoteDriver) GetRoot() (js.Value, error) {
	result, err := d.call("getRoot", jsutil.NewObject())
	if err != nil {
		return js.Undefined(), err
	}
	return d.newHandle(result, remoteHandleKindDirectory)
}

// GetDirectory returns a remote subdirectory handle token within parent.
func (d *RemoteDriver) GetDirectory(parent js.Value, name string, create bool) (js.Value, error) {
	parentID, err := d.handleID(parent, remoteHandleKindDirectory)
	if err != nil {
		return js.Undefined(), err
	}
	result, err := d.call("getDirectory", remoteArgs(
		"dir", parentID,
		"name", name,
		"create", create,
	))
	if err != nil {
		return js.Undefined(), err
	}
	return d.newHandle(result, remoteHandleKindDirectory)
}

// GetDirectoryPath navigates a sequence of remote directory names from parent.
func (d *RemoteDriver) GetDirectoryPath(parent js.Value, path []string, create bool) (js.Value, error) {
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

// OpenAsyncFile opens an existing remote file.
func (d *RemoteDriver) OpenAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return nil, err
	}
	result, err := d.call("openFile", remoteArgs(
		"dir", dirID,
		"name", name,
	))
	if err != nil {
		return nil, err
	}
	handle, err := d.newHandle(result, remoteHandleKindFile)
	if err != nil {
		return nil, err
	}
	return &AsyncFile{driver: d, name: name, handle: handle}, nil
}

// CreateAsyncFile opens or creates a remote file.
func (d *RemoteDriver) CreateAsyncFile(dir js.Value, name string) (*AsyncFile, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return nil, err
	}
	result, err := d.call("createFile", remoteArgs(
		"dir", dirID,
		"name", name,
	))
	if err != nil {
		return nil, err
	}
	handle, err := d.newHandle(result, remoteHandleKindFile)
	if err != nil {
		return nil, err
	}
	return &AsyncFile{driver: d, name: name, handle: handle}, nil
}

// WriteFile creates or overwrites a remote file.
func (d *RemoteDriver) WriteFile(dir js.Value, name string, data []byte) error {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return err
	}
	buffer := remoteBufferFromBytes(data)
	_, err = d.callTransfer("writeFile", remoteArgs(
		"dir", dirID,
		"name", name,
		"data", buffer,
	), buffer)
	return err
}

// CreateWriteStream creates or replaces a remote file and opens one stream token.
func (d *RemoteDriver) CreateWriteStream(dir js.Value, name string) (*WriteStream, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return nil, err
	}
	result, err := d.call("createWriteStream", remoteArgs(
		"dir", dirID,
		"name", name,
	))
	if err != nil {
		return nil, err
	}
	handle, err := d.newHandle(result, remoteHandleKindStream)
	if err != nil {
		return nil, err
	}
	return &WriteStream{driver: d, name: name, writable: handle}, nil
}

// ReadFile reads a remote file into memory.
func (d *RemoteDriver) ReadFile(dir js.Value, name string) ([]byte, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return nil, err
	}
	result, err := d.call("readFile", remoteArgs(
		"dir", dirID,
		"name", name,
	))
	if err != nil {
		return nil, err
	}
	return remoteBytes(result)
}

// DeleteEntry removes a remote file or directory entry.
func (d *RemoteDriver) DeleteEntry(dir js.Value, name string, recursive bool) error {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return err
	}
	_, err = d.call("deleteEntry", remoteArgs(
		"dir", dirID,
		"name", name,
		"recursive", recursive,
	))
	return err
}

// ListDirectory returns remote directory entry names.
func (d *RemoteDriver) ListDirectory(dir js.Value) ([]string, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return nil, err
	}
	result, err := d.call("listDirectory", remoteArgs("dir", dirID))
	if err != nil {
		return nil, err
	}
	return remoteStringList(result)
}

// FileExists checks whether a remote file exists.
func (d *RemoteDriver) FileExists(dir js.Value, name string) (bool, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return false, err
	}
	result, err := d.call("fileExists", remoteArgs(
		"dir", dirID,
		"name", name,
	))
	if err != nil {
		return false, err
	}
	return remoteBool(result, "exists")
}

// DirExists checks whether a remote subdirectory exists.
func (d *RemoteDriver) DirExists(dir js.Value, name string) (bool, error) {
	dirID, err := d.handleID(dir, remoteHandleKindDirectory)
	if err != nil {
		return false, err
	}
	result, err := d.call("dirExists", remoteArgs(
		"dir", dirID,
		"name", name,
	))
	if err != nil {
		return false, err
	}
	return remoteBool(result, "exists")
}

// SyncAvailable reports false for remote OPFS bridge drivers.
func (d *RemoteDriver) SyncAvailable() bool {
	return false
}

// PreferSyncAccessHandles reports false for remote OPFS bridge drivers.
func (d *RemoteDriver) PreferSyncAccessHandles() bool {
	return false
}

// OpenSyncFile returns an error because sync access handles stay local-only.
func (d *RemoteDriver) OpenSyncFile(dir js.Value, name string) (*SyncFile, error) {
	return nil, errors.New("sync access handles unavailable for remote OPFS driver")
}

// CreateSyncFile returns an error because sync access handles stay local-only.
func (d *RemoteDriver) CreateSyncFile(dir js.Value, name string) (*SyncFile, error) {
	return nil, errors.New("sync access handles unavailable for remote OPFS driver")
}

// CreateSyncFileContext returns an error because sync access handles stay local-only.
func (d *RemoteDriver) CreateSyncFileContext(ctx context.Context, dir js.Value, name string) (*SyncFile, error) {
	return nil, errors.New("sync access handles unavailable for remote OPFS driver")
}

// NewBroadcastChannel creates a local browser BroadcastChannel.
func (d *RemoteDriver) NewBroadcastChannel(name string) (js.Value, error) {
	return BrowserDriver{}.NewBroadcastChannel(name)
}

// SendBroadcastChannel posts a local browser BroadcastChannel message.
func (d *RemoteDriver) SendBroadcastChannel(channel js.Value, msg BroadcastMessage) error {
	return BrowserDriver{}.SendBroadcastChannel(channel, msg)
}

// CloseBroadcastChannel closes a local browser BroadcastChannel.
func (d *RemoteDriver) CloseBroadcastChannel(channel js.Value) error {
	return BrowserDriver{}.CloseBroadcastChannel(channel)
}

// AcquireWebLock requests a SharedWorker-local Web Lock.
func (d *RemoteDriver) AcquireWebLock(ctx context.Context, name string, exclusive bool) (*WebLockResult, error) {
	return BrowserDriver{}.AcquireWebLock(ctx, name, exclusive)
}

// AcquireWebLockIfAvailable requests a SharedWorker-local Web Lock without waiting.
func (d *RemoteDriver) AcquireWebLockIfAvailable(ctx context.Context, name string, exclusive bool) (*WebLockResult, error) {
	return BrowserDriver{}.AcquireWebLockIfAvailable(ctx, name, exclusive)
}

func (d *RemoteDriver) readAsyncFileAt(f *AsyncFile, p []byte, off int64) (int, error) {
	fileID, err := d.handleID(f.handle, remoteHandleKindFile)
	if err != nil {
		return 0, err
	}
	result, err := d.call("readAt", remoteArgs(
		"file", fileID,
		"offset", off,
		"length", len(p),
	))
	if err != nil {
		return 0, err
	}
	return remoteCopyBytes(p, result, "readAt")
}

func (d *RemoteDriver) writeAsyncFileAt(ctx context.Context, f *AsyncFile, p []byte, off int64) (int, error) {
	fileID, err := d.handleID(f.handle, remoteHandleKindFile)
	if err != nil {
		return 0, err
	}
	buffer := remoteBufferFromBytes(p)
	result, err := d.callTransfer("writeAt", remoteArgs(
		"file", fileID,
		"offset", off,
		"data", buffer,
	), buffer)
	if err != nil {
		return 0, err
	}
	if remoteValueAvailable(result) {
		n, err := remoteInt(result, "n", "written", "bytesWritten")
		if err == nil {
			return int(n), nil
		}
	}
	return len(p), nil
}

func (d *RemoteDriver) sizeAsyncFile(f *AsyncFile) (int64, error) {
	fileID, err := d.handleID(f.handle, remoteHandleKindFile)
	if err != nil {
		return 0, err
	}
	result, err := d.call("size", remoteArgs("file", fileID))
	if err != nil {
		return 0, err
	}
	return remoteInt(result, "size")
}

func (d *RemoteDriver) truncateAsyncFile(f *AsyncFile, size int64) error {
	fileID, err := d.handleID(f.handle, remoteHandleKindFile)
	if err != nil {
		return err
	}
	_, err = d.call("truncate", remoteArgs(
		"file", fileID,
		"size", size,
	))
	return err
}

func (d *RemoteDriver) closeAsyncFile(f *AsyncFile) error {
	fileID, err := d.handleID(f.handle, remoteHandleKindFile)
	if err != nil {
		return err
	}
	_, err = d.call("closeFile", remoteArgs("file", fileID))
	if err == nil {
		d.removeHandle(fileID)
	}
	return err
}

func (d *RemoteDriver) writeStream(w *WriteStream, p []byte) (int, error) {
	streamID, err := d.handleID(w.writable, remoteHandleKindStream)
	if err != nil {
		return 0, err
	}
	buffer := remoteBufferFromBytes(p)
	result, err := d.callTransfer("streamWrite", remoteArgs(
		"stream", streamID,
		"data", buffer,
	), buffer)
	if err != nil {
		return 0, err
	}
	if remoteValueAvailable(result) {
		n, err := remoteInt(result, "n", "written", "bytesWritten")
		if err == nil {
			w.pos += n
			return int(n), nil
		}
	}
	w.pos += int64(len(p))
	return len(p), nil
}

func (d *RemoteDriver) closeWriteStream(w *WriteStream) error {
	streamID, err := d.handleID(w.writable, remoteHandleKindStream)
	if err != nil {
		return err
	}
	_, err = d.call("streamClose", remoteArgs("stream", streamID))
	if err == nil {
		d.removeHandle(streamID)
	}
	return err
}

func (d *RemoteDriver) abortWriteStream(w *WriteStream) error {
	streamID, err := d.handleID(w.writable, remoteHandleKindStream)
	if err != nil {
		return err
	}
	_, err = d.call("streamAbort", remoteArgs("stream", streamID))
	if err == nil {
		d.removeHandle(streamID)
	}
	return err
}

func (d *RemoteDriver) call(op string, args js.Value) (js.Value, error) {
	return d.callTransfer(op, args)
}

func (d *RemoteDriver) callTransfer(op string, args js.Value, transfer ...js.Value) (js.Value, error) {
	if port := js.Global().Get(opfsBridgePortGlobal); remotePortUsable(port) {
		d.installPort(port)
	}
	var port js.Value
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		port = d.port
	})
	if !remotePortUsable(port) {
		return js.Undefined(), &remoteTransportError{op: op, message: "bridge port unavailable"}
	}
	return d.request(port, op, args, transfer...)
}

func (d *RemoteDriver) request(port js.Value, op string, args js.Value, transfer ...js.Value) (result js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			message := "bridge request failed"
			if e, ok := r.(error); ok {
				message = e.Error()
			} else if s, ok := r.(string); ok {
				message = s
			}
			result = js.Undefined()
			err = &remoteTransportError{op: op, message: message}
		}
	}()
	if len(transfer) == 0 {
		promise := jsutil.Call(port, "request", op, args)
		if promise.Type() != js.TypeObject || !jsutil.Available(promise.Get("then")) {
			return promise, nil
		}
		result, err = AwaitPromise(promise)
		if err != nil {
			return js.Undefined(), err
		}
		return result, nil
	}
	promise := jsutil.Call(port, "request", op, args, remoteTransferList(transfer...))
	if promise.Type() != js.TypeObject || !jsutil.Available(promise.Get("then")) {
		return promise, nil
	}
	result, err = AwaitPromise(promise)
	if err != nil {
		return js.Undefined(), err
	}
	return result, nil
}

func remoteTransferList(transfer ...js.Value) js.Value {
	transferList := js.Global().Get("Array").New()
	for _, value := range transfer {
		jsutil.Call(transferList, "push", value)
	}
	return transferList
}

func (d *RemoteDriver) newHandle(result js.Value, kind string) (js.Value, error) {
	id, err := remoteHandleID(result)
	if err != nil {
		return js.Undefined(), err
	}
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if d.handles == nil {
			d.handles = make(map[int]remoteDriverHandle)
		}
		d.handles[id] = remoteDriverHandle{kind: kind}
	})

	handle := jsutil.NewObject()
	handle.Set(remoteHandleIDField, id)
	handle.Set(remoteHandleKindField, kind)
	return handle, nil
}

func (d *RemoteDriver) handleID(handle js.Value, kind string) (int, error) {
	if !remoteValueAvailable(handle) || handle.Type() != js.TypeObject {
		return 0, &remoteTransportError{message: "remote OPFS handle unavailable"}
	}
	idValue := handle.Get(remoteHandleIDField)
	if idValue.Type() != js.TypeNumber {
		idValue = handle.Get("id")
	}
	if idValue.Type() != js.TypeNumber {
		return 0, &remoteTransportError{message: "remote OPFS handle missing id"}
	}
	id := idValue.Int()
	var h remoteDriverHandle
	var ok bool
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		h, ok = d.handles[id]
	})
	if !ok {
		return 0, &remoteTransportError{message: "remote OPFS handle is stale"}
	}
	if kind != "" && h.kind != kind {
		return 0, &remoteTransportError{message: "remote OPFS handle kind mismatch"}
	}
	return id, nil
}

func (d *RemoteDriver) removeHandle(id int) {
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(d.handles, id)
	})
}

func remoteArgs(fields ...any) js.Value {
	args := jsutil.NewObject()
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		args.Set(key, fields[i+1])
	}
	return args
}

func remoteError(value js.Value) error {
	if !remoteValueAvailable(value) || value.Type() != js.TypeObject {
		return &remoteTransportError{message: "remote OPFS error missing details"}
	}
	err := &JSError{}
	name := value.Get("name")
	if name.Type() == js.TypeString {
		err.Name = name.String()
	}
	message := value.Get("message")
	if message.Type() == js.TypeString {
		err.Message = message.String()
	}
	if err.Name == "" && err.Message == "" {
		err.Message = jsutil.Call(value, "toString").String()
	}
	return err
}

func remoteHandleID(value js.Value) (int, error) {
	if value.Type() == js.TypeNumber {
		return value.Int(), nil
	}
	if remoteValueAvailable(value) && value.Type() == js.TypeObject {
		id := value.Get("id")
		if id.Type() == js.TypeNumber {
			return id.Int(), nil
		}
		handleID := value.Get("handleID")
		if handleID.Type() == js.TypeNumber {
			return handleID.Int(), nil
		}
	}
	return 0, &remoteTransportError{message: "remote OPFS handle result missing id"}
}

func remoteBool(value js.Value, field string) (bool, error) {
	if value.Type() == js.TypeBoolean {
		return value.Bool(), nil
	}
	if remoteValueAvailable(value) && value.Type() == js.TypeObject {
		fieldValue := value.Get(field)
		if fieldValue.Type() == js.TypeBoolean {
			return fieldValue.Bool(), nil
		}
	}
	return false, &remoteTransportError{message: "remote OPFS bool result missing value"}
}

func remoteInt(value js.Value, fields ...string) (int64, error) {
	if value.Type() == js.TypeNumber {
		return int64(value.Int()), nil
	}
	if remoteValueAvailable(value) && value.Type() == js.TypeObject {
		for _, field := range fields {
			fieldValue := value.Get(field)
			if fieldValue.Type() == js.TypeNumber {
				return int64(fieldValue.Int()), nil
			}
		}
	}
	return 0, &remoteTransportError{message: "remote OPFS integer result missing value"}
}

func remoteStringList(value js.Value) ([]string, error) {
	list := value
	if remoteValueAvailable(value) && value.Type() == js.TypeObject {
		names := value.Get("names")
		if remoteValueAvailable(names) {
			list = names
		}
	}
	if !remoteValueAvailable(list) || list.Type() != js.TypeObject {
		return nil, &remoteTransportError{message: "remote OPFS list result missing names"}
	}
	length := list.Get("length")
	if length.Type() != js.TypeNumber {
		return nil, &remoteTransportError{message: "remote OPFS list result missing length"}
	}
	names := make([]string, 0, length.Int())
	for i := 0; i < length.Int(); i++ {
		item := list.Index(i)
		if item.Type() != js.TypeString {
			return nil, &remoteTransportError{message: "remote OPFS list result contains non-string entry"}
		}
		names = append(names, item.String())
	}
	return names, nil
}

func remoteBytes(value js.Value) ([]byte, error) {
	buffer, err := remoteBufferResult(value)
	if err != nil {
		return nil, err
	}
	arr := jsutil.NewUint8Array(buffer)
	n := arr.Get("length").Int()
	data := make([]byte, n)
	js.CopyBytesToGo(data, arr)
	return data, nil
}

func remoteCopyBytes(dst []byte, value js.Value, op string) (int, error) {
	buffer, err := remoteBufferResult(value)
	if err != nil {
		return 0, err
	}
	arr := jsutil.NewUint8Array(buffer)
	n := arr.Get("length").Int()
	if n > len(dst) {
		return 0, &remoteTransportError{op: op, message: "remote OPFS read returned too many bytes"}
	}
	js.CopyBytesToGo(dst[:n], arr)
	if n == 0 && len(dst) > 0 {
		return 0, io.EOF
	}
	return n, nil
}

func remoteBufferResult(value js.Value) (js.Value, error) {
	if !remoteValueAvailable(value) {
		return js.Undefined(), &remoteTransportError{message: "remote OPFS byte result missing data"}
	}
	if value.Type() == js.TypeObject {
		byteLength := value.Get("byteLength")
		if byteLength.Type() == js.TypeNumber {
			return value, nil
		}
		for _, field := range []string{"bytes", "data", "buffer"} {
			fieldValue := value.Get(field)
			if remoteValueAvailable(fieldValue) && fieldValue.Type() == js.TypeObject {
				if fieldValue.Get("byteLength").Type() == js.TypeNumber {
					return fieldValue, nil
				}
			}
		}
	}
	return js.Undefined(), &remoteTransportError{message: "remote OPFS byte result is not an ArrayBuffer"}
}

func remoteBufferFromBytes(data []byte) js.Value {
	arr := jsutil.NewUint8Array(len(data))
	js.CopyBytesToJS(arr, data)
	return arr.Get("buffer")
}

func remotePortUsable(port js.Value) bool {
	if !remoteValueAvailable(port) || port.Type() != js.TypeObject {
		return false
	}
	return jsutil.Available(port.Get("request"))
}

func remoteValueAvailable(value js.Value) bool {
	return !value.IsUndefined() && !value.IsNull()
}

// _ is a type assertion.
var _ Driver = (*RemoteDriver)(nil)
