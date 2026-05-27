//go:build js && tinygo

package opfs

import (
	"io"
	"runtime"
	"syscall/js"
	"unsafe"

	"github.com/pkg/errors"
)

// Exported OPFS helper callbacks publish completion only. JavaScript owns the
// later TinyGo scheduler resume so resumed goroutines do not enter syscall/js
// while a js.FuncOf callback frame is still active.

//go:wasmimport gojs bldr.opfs.takeStoredBytes
func tinyGoOPFSTakeStoredBytes(bytesID uint32, ptr unsafe.Pointer, len uint32) uint32

//go:wasmimport gojs bldr.opfs.readFileRef
func tinyGoOPFSReadFileRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32)

//go:wasmimport gojs bldr.opfs.readAtRef
func tinyGoOPFSReadAtRef(opID uint32, handleRef uint64, dstPtr unsafe.Pointer, dstLen uint32, off int64)

//go:wasmimport gojs bldr.opfs.listDirectoryRef
func tinyGoOPFSListDirectoryRef(opID uint32, dirRef uint64)

//go:wasmimport gojs bldr.opfs.writeAtRef
func tinyGoOPFSWriteAtRef(opID uint32, handleRef uint64, dataPtr unsafe.Pointer, dataLen uint32, off int64, keepExisting uint32)

//go:wasmimport gojs bldr.opfs.writeFileBeginRef
func tinyGoOPFSWriteFileBeginRef(opID uint32, dirRef uint64, namePtr unsafe.Pointer, nameLen uint32)

//go:wasmimport gojs bldr.opfs.writeFileChunkRef
func tinyGoOPFSWriteFileChunkRef(opID uint32, sessionID uint32, dataPtr unsafe.Pointer, dataLen uint32)

//go:wasmimport gojs bldr.opfs.writeFileCloseRef
func tinyGoOPFSWriteFileCloseRef(opID uint32, sessionID uint32)

//go:wasmimport gojs bldr.opfs.writeFileAbortRef
func tinyGoOPFSWriteFileAbortRef(sessionID uint32) uint32

type tinyGoJSValue struct {
	_     [0]func()
	ref   uint64
	gcPtr unsafe.Pointer
}

func tinyGoJSRef(value js.Value) uint64 {
	return (*tinyGoJSValue)(unsafe.Pointer(&value)).ref
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
	sessionID, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSWriteFileBeginRef(uint32(opID), tinyGoJSRef(dir), tinyGoBytesPtr(nameBytes), uint32(len(nameBytes)))
		runtime.KeepAlive(nameBytes)
	})
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			tinyGoOPFSWriteFileAbortRef(uint32(sessionID))
		}
	}()

	written := 0
	for off := 0; off < len(data); off += tinyGoOPFSWriteFileChunkSize {
		end := off + tinyGoOPFSWriteFileChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[off:end]
		n, err := invokeOPFSIntHelper(func(opID int) {
			tinyGoOPFSWriteFileChunkRef(uint32(opID), uint32(sessionID), tinyGoBytesPtr(chunk), uint32(len(chunk)))
		})
		if err != nil {
			return err
		}
		if n != len(chunk) {
			return errors.Errorf("short write file %s: wrote chunk %d of %d", name, n, len(chunk))
		}
		written += n
	}

	closedWritten, err := invokeOPFSIntHelper(func(opID int) {
		tinyGoOPFSWriteFileCloseRef(uint32(opID), uint32(sessionID))
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

//go:wasmexport BLDR_OPFS_HELPER_RESOLVE
func tinygoOPFSHelperResolve(opID uint32, count uint32, value0 uint32, value1 uint32) {
	values := make([]int, 0, count)
	if count > 0 {
		values = append(values, int(value0))
	}
	if count > 1 {
		values = append(values, int(value1))
	}
	completeOPFSHelperOp(int(opID), opfsHelperResult{values: values})
}

//go:wasmexport BLDR_OPFS_HELPER_REJECT
func tinygoOPFSHelperReject(opID uint32, code uint32) {
	completeOPFSHelperOp(int(opID), opfsHelperResult{err: newJSErrorCode(int(code))})
}
