package checkoutroot

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// exclusiveRenameInfo is the Windows FileRenameInformation layout.
type exclusiveRenameInfo struct {
	// replace is zero to refuse any existing destination.
	replace uint32
	// directory anchors the relative destination.
	directory windows.Handle
	// nameBytes is the UTF-16 byte length without a terminator.
	nameBytes uint32
	// name contains a single filename.
	name [syscall.MAX_PATH]uint16
}

// renameExclusive moves the stage to an absent name without resolving the root again.
func renameExclusive(dir *os.Root, temp, name string) error {
	// Pin both names to the same open directory.
	fd, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer fd.Close()
	dirHandle := windows.Handle(fd.Fd())

	// Encode both components in the native filesystem's string format.
	src, err := windows.NewNTUnicodeString(temp)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}

	// Build a relative rename record with replacement disabled.
	info := exclusiveRenameInfo{directory: dirHandle}
	if len(dst) > len(info.name) {
		return syscall.ENAMETOOLONG
	}
	copy(info.name[:], dst)
	info.nameBytes = uint32(2 * (len(dst) - 1))

	// Open the stage with rename permission and without following reparse points.
	attrs := windows.OBJECT_ATTRIBUTES{
		Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: dirHandle,
		ObjectName: src, Attributes: windows.OBJ_CASE_INSENSITIVE,
	}
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE, &attrs, &windows.IO_STATUS_BLOCK{}, nil, 0,
		windows.FILE_SHARE_DELETE|windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)

	// A no-replace rename works on Windows filesystems without hard links.
	err = windows.NtSetInformationFile(handle, &windows.IO_STATUS_BLOCK{}, (*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)), windows.FileRenameInformation)
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
