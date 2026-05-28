//go:build js && tinygo

package opfs

import (
	"io"
	"runtime"
	"syscall/js"
	"unsafe"

	"github.com/pkg/errors"
)

// Exported OPFS helper callbacks publish primitive completion only. JavaScript
// owns the later TinyGo scheduler resume, and direct //export callbacks avoid
// TinyGo asyncify's //go:wasmexport goroutine-stack allocation on every
// Promise completion.

//go:wasmimport gojs bldr.opfs.takeStoredBytes
func tinyGoOPFSTakeStoredBytes(bytesID uint32, ptr unsafe.Pointer, len uint32) uint32

//go:wasmimport gojs bldr.opfs.getRootRef
func tinyGoOPFSGetRootRef(opID uint32)

//go:wasmimport gojs bldr.opfs.getDirectoryRef
func tinyGoOPFSGetDirectoryRef(opID uint32, parentRef uint64, namePtr unsafe.Pointer, nameLen uint32, create uint32)

//go:wasmimport gojs bldr.opfs.openFileRef
func tinyGoOPFSOpenFileRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32, create uint32)

//go:wasmimport gojs bldr.opfs.fileExistsRef
func tinyGoOPFSFileExistsRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32)

//go:wasmimport gojs bldr.opfs.deleteEntryRef
func tinyGoOPFSDeleteEntryRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32, recursive uint32)

//go:wasmimport gojs bldr.opfs.yieldMicrotask
func tinyGoOPFSYieldMicrotask(opID uint32)

//go:wasmimport gojs bldr.opfs.sizeRef
func tinyGoOPFSSizeRef(opID uint32, handleRef uint64)

//go:wasmimport gojs bldr.opfs.truncateRef
func tinyGoOPFSTruncateRef(opID uint32, handleRef uint64, size int64)

//go:wasmimport gojs bldr.opfs.readFileRef
func tinyGoOPFSReadFileRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32)

//go:wasmimport gojs bldr.opfs.readAtRef
func tinyGoOPFSReadAtRef(opID uint32, handleRef uint64, dstPtr unsafe.Pointer, dstLen uint32, off int64)

//go:wasmimport gojs bldr.opfs.listDirectoryRef
func tinyGoOPFSListDirectoryRef(opID uint32, dirRef uint64)

//go:wasmimport gojs bldr.opfs.writeAtRef
func tinyGoOPFSWriteAtRef(opID uint32, handleRef uint64, dataPtr unsafe.Pointer, dataLen uint32, off int64, keepExisting uint32)

//go:wasmimport gojs bldr.opfs.writeFileRef
func tinyGoOPFSWriteFileRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32, dataPtr unsafe.Pointer, dataLen uint32)

//go:wasmimport gojs bldr.opfs.openWriteStreamRef
func tinyGoOPFSOpenWriteStreamRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32)

//go:wasmimport gojs bldr.opfs.writeStreamRef
func tinyGoOPFSWriteStreamRef(opID uint32, streamID uint32, dataPtr unsafe.Pointer, dataLen uint32)

//go:wasmimport gojs bldr.opfs.closeWriteStreamRef
func tinyGoOPFSCloseWriteStreamRef(opID uint32, streamID uint32)

//go:wasmimport gojs bldr.opfs.abortWriteStreamRef
func tinyGoOPFSAbortWriteStreamRef(opID uint32, streamID uint32)

type tinyGoJSValue struct {
	_     [0]func()
	ref   uint64
	gcPtr unsafe.Pointer
}

func tinyGoJSRef(value js.Value) uint64 {
	return (*tinyGoJSValue)(unsafe.Pointer(&value)).ref
}

func tinyGoJSValueFromRef(ref uint64) js.Value {
	value := tinyGoJSValue{ref: ref}
	return *(*js.Value)(unsafe.Pointer(&value))
}

func tinyGoBytesPtr(bytes []byte) unsafe.Pointer {
	if len(bytes) == 0 {
		return nil
	}
	return unsafe.Pointer(&bytes[0])
}

func tinyGoBoolUint32(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}

func tinyGoRefFromValues(values []int) (uint64, error) {
	if len(values) < 2 {
		return 0, errors.New("opfs helper returned incomplete js.Value ref")
	}
	return uint64(uint32(values[0]))<<32 | uint64(uint32(values[1])), nil
}

func tinyGoValueFromValues(values []int) (js.Value, error) {
	ref, err := tinyGoRefFromValues(values)
	if err != nil {
		return js.Undefined(), err
	}
	return tinyGoJSValueFromRef(ref), nil
}

func tinyGoCopyStoredBytes(id, size int) ([]byte, error) {
	if size == 0 {
		if tinyGoOPFSTakeStoredBytes(uint32(id), nil, 0) == 0 {
			return nil, errors.New("opfs stored byte helper unavailable")
		}
		return nil, nil
	}
	buf := make([]byte, size)
	if tinyGoOPFSTakeStoredBytes(uint32(id), unsafe.Pointer(&buf[0]), uint32(size)) == 0 {
		return nil, errors.New("opfs stored byte helper unavailable")
	}
	return buf, nil
}

func getRootWithTinyGoImport() (js.Value, error) {
	values, err := invokeOPFSHelper(func(opID int) {
		tinyGoOPFSGetRootRef(uint32(opID))
	})
	if err != nil {
		return js.Undefined(), err
	}
	return tinyGoValueFromValues(values)
}

func getDirectoryWithTinyGoImport(parent js.Value, name string, create bool) (js.Value, error) {
	nameBytes := []byte(name)
	values, err := invokeOPFSHelper(func(opID int) {
		tinyGoOPFSGetDirectoryRef(uint32(opID), tinyGoJSRef(parent), tinyGoBytesPtr(nameBytes), uint32(len(nameBytes)), tinyGoBoolUint32(create))
		runtime.KeepAlive(nameBytes)
	})
	if err != nil {
		return js.Undefined(), err
	}
	return tinyGoValueFromValues(values)
}

func openAsyncFileWithTinyGoImport(dir js.Value, name string, create bool) (*AsyncFile, error) {
	nameBytes := []byte(name)
	values, err := invokeOPFSHelper(func(opID int) {
		tinyGoOPFSOpenFileRef(uint32(opID), tinyGoJSRef(dir), tinyGoBytesPtr(nameBytes), uint32(len(nameBytes)), tinyGoBoolUint32(create))
		runtime.KeepAlive(nameBytes)
	})
	if err != nil {
		return nil, err
	}
	handle, err := tinyGoValueFromValues(values)
	if err != nil {
		return nil, err
	}
	return &AsyncFile{name: name, handle: handle}, nil
}

func fileExistsWithTinyGoImport(dir js.Value, name string) (bool, error) {
	nameBytes := []byte(name)
	exists, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSFileExistsRef(uint32(opID), tinyGoJSRef(dir), tinyGoBytesPtr(nameBytes), uint32(len(nameBytes)))
		runtime.KeepAlive(nameBytes)
	})
	if err != nil {
		return false, err
	}
	return exists != 0, nil
}

func deleteEntryWithTinyGoImport(dir js.Value, name string, recursive bool) error {
	nameBytes := []byte(name)
	_, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSDeleteEntryRef(uint32(opID), tinyGoJSRef(dir), tinyGoBytesPtr(nameBytes), uint32(len(nameBytes)), tinyGoBoolUint32(recursive))
		runtime.KeepAlive(nameBytes)
	})
	return err
}

func yieldMicrotaskWithTinyGoImport() error {
	_, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSYieldMicrotask(uint32(opID))
	})
	return err
}

func (f *AsyncFile) sizeWithTinyGoImport() (int64, error) {
	size, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSSizeRef(uint32(opID), tinyGoJSRef(f.handle))
	})
	return int64(size), err
}

func (f *AsyncFile) truncateWithTinyGoImport(size int64) error {
	_, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSTruncateRef(uint32(opID), tinyGoJSRef(f.handle), size)
	})
	return err
}

func (f *AsyncFile) readAtWithTinyGoImport(p []byte, off int64) (int, error) {
	n, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSReadAtRef(
			uint32(opID),
			tinyGoJSRef(f.handle),
			tinyGoBytesPtr(p),
			uint32(len(p)),
			off,
		)
	})
	if err != nil {
		return 0, err
	}
	if n > len(p) {
		return 0, errors.Errorf("read exceeded buffer for file %s: read %d into %d", f.name, n, len(p))
	}
	if n == 0 && len(p) > 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *AsyncFile) writeAtWithTinyGoImport(p []byte, off int64, keepExisting bool) (int, error) {
	written, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSWriteAtRef(
			uint32(opID),
			tinyGoJSRef(f.handle),
			tinyGoBytesPtr(p),
			uint32(len(p)),
			off,
			tinyGoBoolUint32(keepExisting),
		)
	})
	if err != nil {
		return 0, err
	}
	if written != len(p) {
		return written, errors.Errorf("short write file %s: wrote %d of %d", f.name, written, len(p))
	}
	return written, nil
}

func writeFileWithTinyGoImport(dir js.Value, name string, data []byte) error {
	nameBytes := []byte(name)
	written, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSWriteFileRef(
			uint32(opID),
			tinyGoJSRef(dir),
			tinyGoBytesPtr(nameBytes),
			uint32(len(nameBytes)),
			tinyGoBytesPtr(data),
			uint32(len(data)),
		)
		runtime.KeepAlive(nameBytes)
		runtime.KeepAlive(data)
	})
	if err != nil {
		return err
	}
	if written != len(data) {
		return errors.Errorf("short write file %s: wrote %d of %d", name, written, len(data))
	}
	return nil
}

func createWriteStreamWithTinyGoImport(dir js.Value, name string) (*WriteStream, error) {
	nameBytes := []byte(name)
	streamID, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSOpenWriteStreamRef(
			uint32(opID),
			tinyGoJSRef(dir),
			tinyGoBytesPtr(nameBytes),
			uint32(len(nameBytes)),
		)
		runtime.KeepAlive(nameBytes)
	})
	if err != nil {
		return nil, err
	}
	return &WriteStream{name: name, tinyGoID: streamID}, nil
}

func (w *WriteStream) writeWithTinyGoImport(p []byte) (int, error) {
	written, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSWriteStreamRef(
			uint32(opID),
			uint32(w.tinyGoID),
			tinyGoBytesPtr(p),
			uint32(len(p)),
		)
		runtime.KeepAlive(p)
	})
	if err != nil {
		return 0, err
	}
	if written != len(p) {
		return written, errors.Errorf("short write stream %s: wrote %d of %d", w.name, written, len(p))
	}
	w.pos += int64(written)
	return written, nil
}

func (w *WriteStream) closeWithTinyGoImport() error {
	closed, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSCloseWriteStreamRef(uint32(opID), uint32(w.tinyGoID))
	})
	if err != nil {
		return err
	}
	if closed != 1 {
		return errors.Errorf("close write stream %s: stream unavailable", w.name)
	}
	w.tinyGoID = 0
	return nil
}

func (w *WriteStream) abortWithTinyGoImport() error {
	if w.tinyGoID == 0 {
		return nil
	}
	aborted, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSAbortWriteStreamRef(uint32(opID), uint32(w.tinyGoID))
	})
	if err != nil {
		return err
	}
	if aborted != 1 {
		return errors.Errorf("abort write stream %s: stream unavailable", w.name)
	}
	w.tinyGoID = 0
	return nil
}

func readFileWithTinyGoImport(dir js.Value, name string) ([]byte, error) {
	nameBytes := []byte(name)
	values, err := invokeOPFSHelper(func(opID int) {
		tinyGoOPFSReadFileRef(uint32(opID), tinyGoJSRef(dir), tinyGoBytesPtr(nameBytes), uint32(len(nameBytes)))
		runtime.KeepAlive(nameBytes)
	})
	if err != nil {
		return nil, err
	}
	if len(values) < 2 {
		return nil, errors.New("opfs read file helper returned incomplete result")
	}
	id, size := values[0], values[1]
	return tinyGoCopyStoredBytes(id, size)
}

func listDirectoryWithTinyGoImport(dir js.Value) ([]string, error) {
	values, err := invokeOPFSHelper(func(opID int) {
		tinyGoOPFSListDirectoryRef(uint32(opID), tinyGoJSRef(dir))
	})
	if err != nil {
		return nil, err
	}
	if len(values) < 2 {
		return nil, errors.New("opfs list directory helper returned incomplete result")
	}
	id, size := values[0], values[1]
	buf, err := tinyGoCopyStoredBytes(id, size)
	if err != nil {
		return nil, err
	}
	names, err := decodeHelperNameList(buf)
	if err != nil {
		return nil, errors.Wrap(err, "decode opfs list directory names")
	}
	return names, nil
}

//export BLDR_OPFS_HELPER_RESOLVE
func tinygoOPFSHelperResolve(opID uint32, count uint32, value0 uint32, value1 uint32) {
	completeOPFSHelperOp(int(opID), opfsHelperResult{
		valueCount: int(count),
		value0:     int(value0),
		value1:     int(value1),
	})
}

//export BLDR_OPFS_HELPER_REJECT
func tinygoOPFSHelperReject(opID uint32, code uint32) {
	completeOPFSHelperOp(int(opID), opfsHelperResult{
		rejected: true,
		errCode:  int(code),
	})
}
