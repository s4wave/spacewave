package volume_s4db

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/windows"
)

// replaceConverted atomically replaces the active name while its old inode's
// writer lock remains held. The caller has verified the new file and retained
// the old inode under its backup name. MoveFileEx cannot replace an open target;
// FileRenameInfoEx with POSIX semantics keeps those existing handles valid.
func replaceConverted(from, to string) error {
	// Open only the staged file for renaming; never release the source lock.
	oldName, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	h, err := windows.CreateFile(oldName, windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	defer windows.CloseHandle(h)

	// Supply an absolute target with no root directory handle in the native ABI.
	absolute, err := filepath.Abs(to)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	name, err := windows.UTF16FromString(absolute)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}

	// FILE_RENAME_INFO starts with flags, an aligned HANDLE, and a DWORD name
	// length, followed by UTF-16 characters. The zero root handle and padding
	// remain untouched; the length excludes the terminator but the buffer keeps it.
	const pointerBytes = strconv.IntSize / 8
	const lengthOffset = 2 * pointerBytes
	const nameOffset = lengthOffset + 4
	const headerSize = lengthOffset + 8
	buf := make([]byte, headerSize+2*len(name))
	binary.LittleEndian.PutUint32(buf, windows.FILE_RENAME_REPLACE_IF_EXISTS|windows.FILE_RENAME_POSIX_SEMANTICS)
	binary.LittleEndian.PutUint32(buf[lengthOffset:], uint32(2*(len(name)-1))) // #nosec G115 -- an existing Windows path fits in 32 bits.
	for i, character := range name {
		binary.LittleEndian.PutUint16(buf[nameOffset+2*i:], character)
	}

	// The operation replaces the name without closing or invalidating old handles.
	if err := windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, &buf[0], uint32(len(buf))); err != nil { // #nosec G115 -- a Windows path buffer fits in 32 bits.
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
