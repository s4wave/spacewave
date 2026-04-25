//go:build js && wasm

// Package opfs provides Go wrappers for the Origin Private File System browser API.
//
// OPFS provides synchronous file access via FileSystemSyncAccessHandle on
// dedicated workers, bypassing IndexedDB's async request/event model.
// Browser minimum: Chrome 102+, Firefox 111+, Safari 17+.
package opfs

import (
	"github.com/hack-pad/safejs"
)

// jsNavigator is the cached navigator global.
var jsNavigator = safejs.MustGetGlobal("navigator")

// GetRootDirectory returns the root FileSystemDirectoryHandle via
// navigator.storage.getDirectory().
func GetRootDirectory() (*DirectoryHandle, error) {
	storage, err := jsNavigator.Get("storage")
	if err != nil {
		return nil, err
	}
	promise, err := storage.Call("getDirectory")
	if err != nil {
		return nil, err
	}
	jsDir, err := awaitPromise(promise)
	if err != nil {
		return nil, err
	}
	return wrapDirectoryHandle(jsDir), nil
}
