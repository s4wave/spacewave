//go:build js && wasm

package opfs

import (
	"github.com/hack-pad/safejs"
)

// DirectoryHandle wraps a FileSystemDirectoryHandle.
type DirectoryHandle struct {
	jsHandle safejs.Value
}

func wrapDirectoryHandle(v safejs.Value) *DirectoryHandle {
	return &DirectoryHandle{jsHandle: v}
}

// GetDirectoryHandle returns a DirectoryHandle for the named subdirectory.
// If create is true, the directory is created if it does not exist.
func (dh *DirectoryHandle) GetDirectoryHandle(name string, create bool) (*DirectoryHandle, error) {
	opts, err := safejs.ValueOf(map[string]any{"create": create})
	if err != nil {
		return nil, err
	}
	promise, err := dh.jsHandle.Call("getDirectoryHandle", name, opts)
	if err != nil {
		return nil, err
	}
	jsDir, err := awaitPromise(promise)
	if err != nil {
		return nil, err
	}
	return wrapDirectoryHandle(jsDir), nil
}

// GetFileHandle returns a FileHandle for the named file.
// If create is true, the file is created if it does not exist.
func (dh *DirectoryHandle) GetFileHandle(name string, create bool) (*FileHandle, error) {
	opts, err := safejs.ValueOf(map[string]any{"create": create})
	if err != nil {
		return nil, err
	}
	promise, err := dh.jsHandle.Call("getFileHandle", name, opts)
	if err != nil {
		return nil, err
	}
	jsFile, err := awaitPromise(promise)
	if err != nil {
		return nil, err
	}
	return wrapFileHandle(jsFile, name), nil
}

// RemoveEntry removes the named file or directory.
// If recursive is true, directories are removed recursively.
func (dh *DirectoryHandle) RemoveEntry(name string, recursive bool) error {
	opts, err := safejs.ValueOf(map[string]any{"recursive": recursive})
	if err != nil {
		return err
	}
	promise, err := dh.jsHandle.Call("removeEntry", name, opts)
	if err != nil {
		return err
	}
	_, err = awaitPromise(promise)
	return err
}

// Entries returns all entries in the directory as name/handle pairs.
// Handles are either *DirectoryHandle or *FileHandle based on kind.
func (dh *DirectoryHandle) Entries() ([]Entry, error) {
	jsIter, err := dh.jsHandle.Call("entries")
	if err != nil {
		return nil, err
	}
	return collectAsyncIterator(jsIter)
}

// Entry represents a directory entry (file or subdirectory).
type Entry struct {
	Name string
	Kind string // "file" or "directory"
	// Handle is the raw JS handle value. Use AsFileHandle or AsDirectoryHandle.
	Handle safejs.Value
}

// AsFileHandle wraps the entry's handle as a FileHandle.
func (e *Entry) AsFileHandle() *FileHandle {
	return wrapFileHandle(e.Handle, e.Name)
}

// AsDirectoryHandle wraps the entry's handle as a DirectoryHandle.
func (e *Entry) AsDirectoryHandle() *DirectoryHandle {
	return wrapDirectoryHandle(e.Handle)
}

// collectAsyncIterator collects all entries from a JS async iterator
// returned by entries()/keys()/values().
func collectAsyncIterator(jsIter safejs.Value) ([]Entry, error) {
	var entries []Entry
	for {
		promise, err := jsIter.Call("next")
		if err != nil {
			return nil, err
		}
		result, err := awaitPromise(promise)
		if err != nil {
			return nil, err
		}
		done, err := result.Get("done")
		if err != nil {
			return nil, err
		}
		isDone, err := done.Bool()
		if err != nil {
			return nil, err
		}
		if isDone {
			break
		}
		value, err := result.Get("value")
		if err != nil {
			return nil, err
		}
		// value is a [name, handle] array
		jsName, err := value.Index(0)
		if err != nil {
			return nil, err
		}
		name, err := jsName.String()
		if err != nil {
			return nil, err
		}
		jsHandle, err := value.Index(1)
		if err != nil {
			return nil, err
		}
		jsKind, err := jsHandle.Get("kind")
		if err != nil {
			return nil, err
		}
		kind, err := jsKind.String()
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{
			Name:   name,
			Kind:   kind,
			Handle: jsHandle,
		})
	}
	return entries, nil
}
