package s4dbv1

import (
	"os"

	"golang.org/x/sys/windows"
)

// openSource permits renaming the source while its writer lock remains held.
// The conversion retains this handle until the verified replacement is installed.
func openSource(path string) (*os.File, error) {
	// Open a handle that permits replacement of the file's directory entry.
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
