package s4db

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// windowsShortPathLimit leaves room for an 8.3 name under the 260-byte limit.
const windowsShortPathLimit = 248

// openNativeFile permits failed-build removal while an owner wait is open.
// All engine descriptors share deletion so ownership covers cleanup too.
func openNativeFile(path string, flags int) (*os.File, error) {
	// Preserve os.OpenFile's long-path support while adding delete sharing.
	// Extended paths need an absolute normalized name, including UNC names.
	if path == "" {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	winPath := path
	prefix := strings.ReplaceAll(path[:min(len(path), 4)], "/", `\`)
	if prefix != `\\?\` && prefix != `\??\` {
		full, err := windows.FullPath(path)
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		if len(full) >= windowsShortPathLimit {
			winPath = full
			if prefix != `\\.\` {
				winPath = `\\?\` + full
				if strings.HasPrefix(full, `\\`) {
					winPath = `\\?\UNC\` + full[2:]
				}
			}
		}
	}

	// Native opens either reuse/create a file, open an existing reader, or build
	// a new exclusive file. Every handle needs writes for reader-slot byte locks.
	creation := uint32(windows.OPEN_EXISTING)
	if flags&os.O_CREATE != 0 {
		creation = windows.OPEN_ALWAYS
	}
	if flags&os.O_EXCL != 0 {
		creation = windows.CREATE_NEW
	}
	name, err := windows.UTF16PtrFromString(winPath)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, creation, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
