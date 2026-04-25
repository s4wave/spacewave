//go:build js && wasm

package opfs

import (
	"github.com/hack-pad/safejs"
)

// FileHandle wraps a FileSystemFileHandle.
type FileHandle struct {
	jsHandle safejs.Value
	name     string
}

func wrapFileHandle(v safejs.Value, name string) *FileHandle {
	return &FileHandle{jsHandle: v, name: name}
}

// CreateSyncAccessHandle returns a SyncAccessHandle for synchronous
// read/write access. Only available in dedicated workers.
func (fh *FileHandle) CreateSyncAccessHandle() (*SyncAccessHandle, error) {
	promise, err := fh.jsHandle.Call("createSyncAccessHandle")
	if err != nil {
		return nil, err
	}
	jsSync, err := awaitPromise(promise)
	if err != nil {
		return nil, err
	}
	return wrapSyncAccessHandle(jsSync, fh.name), nil
}

// ReadFile reads the entire file contents via getFile() + arrayBuffer().
func (fh *FileHandle) ReadFile() ([]byte, error) {
	promise, err := fh.jsHandle.Call("getFile")
	if err != nil {
		return nil, err
	}
	jsFile, err := awaitPromise(promise)
	if err != nil {
		return nil, err
	}
	abPromise, err := jsFile.Call("arrayBuffer")
	if err != nil {
		return nil, err
	}
	jsArrayBuf, err := awaitPromise(abPromise)
	if err != nil {
		return nil, err
	}
	// Wrap in Uint8Array for CopyBytesToGo.
	jsUint8Array := safejs.MustGetGlobal("Uint8Array")
	jsBytes, err := jsUint8Array.New(jsArrayBuf)
	if err != nil {
		return nil, err
	}
	jsLen, err := jsBytes.Get("length")
	if err != nil {
		return nil, err
	}
	length, err := jsLen.Int()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, length)
	if length > 0 {
		_, err = safejs.CopyBytesToGo(buf, jsBytes)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}
