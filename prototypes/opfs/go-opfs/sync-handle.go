//go:build js && wasm

package opfs

import (
	"io"
	"io/fs"

	"github.com/hack-pad/safejs"
)

// SyncAccessHandle wraps a FileSystemSyncAccessHandle providing synchronous
// byte-level read/write to an OPFS file. Only available in dedicated workers.
type SyncAccessHandle struct {
	jsHandle safejs.Value
	name     string
	cursor   int64
}

func wrapSyncAccessHandle(v safejs.Value, name string) *SyncAccessHandle {
	return &SyncAccessHandle{jsHandle: v, name: name}
}

// ReadAt reads bytes from the file into buf at the given offset.
func (h *SyncAccessHandle) ReadAt(buf []byte, off int64) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	jsBuf, err := newUint8Array(len(buf))
	if err != nil {
		return 0, err
	}
	opts, err := safejs.ValueOf(map[string]any{"at": int(off)})
	if err != nil {
		return 0, err
	}
	result, err := h.jsHandle.Call("read", jsBuf, opts)
	if err != nil {
		return 0, err
	}
	n, err := result.Int()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.EOF
	}
	if n > 0 {
		_, err = safejs.CopyBytesToGo(buf[:n], jsBuf)
		if err != nil {
			return 0, err
		}
	}
	if n < len(buf) {
		return n, io.EOF
	}
	return n, nil
}

// Read reads bytes from the file at the current cursor position.
func (h *SyncAccessHandle) Read(buf []byte) (int, error) {
	n, err := h.ReadAt(buf, h.cursor)
	h.cursor += int64(n)
	return n, err
}

// WriteAt writes bytes from buf to the file at the given offset.
func (h *SyncAccessHandle) WriteAt(buf []byte, off int64) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	jsBuf, err := newUint8Array(len(buf))
	if err != nil {
		return 0, err
	}
	_, err = safejs.CopyBytesToJS(jsBuf, buf)
	if err != nil {
		return 0, err
	}
	opts, err := safejs.ValueOf(map[string]any{"at": int(off)})
	if err != nil {
		return 0, err
	}
	result, err := h.jsHandle.Call("write", jsBuf, opts)
	if err != nil {
		return 0, err
	}
	n, err := result.Int()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Write writes bytes to the file at the current cursor position.
func (h *SyncAccessHandle) Write(buf []byte) (int, error) {
	n, err := h.WriteAt(buf, h.cursor)
	h.cursor += int64(n)
	return n, err
}

// Seek sets the cursor position.
func (h *SyncAccessHandle) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		h.cursor = offset
	case io.SeekCurrent:
		h.cursor += offset
	case io.SeekEnd:
		size, err := h.getSize()
		if err != nil {
			return 0, err
		}
		h.cursor = size + offset
	}
	if h.cursor < 0 {
		h.cursor = 0
	}
	return h.cursor, nil
}

// Stat returns file info.
func (h *SyncAccessHandle) Stat() (fs.FileInfo, error) {
	size, err := h.getSize()
	if err != nil {
		return nil, err
	}
	return &fileInfo{name: h.name, size: size}, nil
}

// Truncate resizes the file to the given size in bytes.
func (h *SyncAccessHandle) Truncate(size int64) error {
	_, err := h.jsHandle.Call("truncate", int(size))
	return err
}

// getSize returns the file size in bytes.
func (h *SyncAccessHandle) getSize() (int64, error) {
	result, err := h.jsHandle.Call("getSize")
	if err != nil {
		return 0, err
	}
	n, err := result.Int()
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

// Flush persists any changes made via Write to disk.
func (h *SyncAccessHandle) Flush() error {
	_, err := h.jsHandle.Call("flush")
	return err
}

// Close releases the lock on the file handle.
func (h *SyncAccessHandle) Close() error {
	_, err := h.jsHandle.Call("close")
	return err
}

// newUint8Array creates a new JavaScript Uint8Array of the given length.
func newUint8Array(length int) (safejs.Value, error) {
	uint8Array := safejs.MustGetGlobal("Uint8Array")
	return uint8Array.New(length)
}
