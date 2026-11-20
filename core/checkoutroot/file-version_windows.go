package checkoutroot

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openFileVersion encodes the open file's identity, size and timestamps.
func openFileVersion(file nativeFile) ([]byte, error) {
	// Only regular files have a version that can authorize replacing the entry.
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}

	// Read the volume serial, 128-bit file ID and basic timestamps from the same handle.
	conn, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var identity [3]uint64
	var basic [5]uint64
	if err := conn.Control(func(handle uintptr) {
		h := windows.Handle(handle)
		queryErr := windows.GetFileInformationByHandleEx(h, windows.FileIdInfo, (*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity)))
		if queryErr != nil {
			err = queryErr
			return
		}
		err = windows.GetFileInformationByHandleEx(h, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic)))
	}); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	// Hash the identity, file size, last-write time and change time.
	// Windows reports both times in 100-nanosecond ticks.
	var fields [48]byte
	for i, field := range identity {
		binary.LittleEndian.PutUint64(fields[i*8:], field)
	}
	binary.LittleEndian.PutUint64(fields[24:32], uint64(info.Size())) //nolint:gosec // Encode the signed field by its bit pattern.
	binary.LittleEndian.PutUint64(fields[32:40], basic[2])
	binary.LittleEndian.PutUint64(fields[40:48], basic[3])
	version := sha256.Sum256(fields[:])
	return version[:], nil
}

// destinationVersion reads the version of the regular file at the destination name.
func destinationVersion(dir *os.Root, name string) ([]byte, error) {
	// Inspect the entry itself so a symlink cannot authorize replacement.
	info, err := dir.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}

	// Open through the pinned directory to get a handle for the version query.
	file, err := dir.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return openFileVersion(file)
}
