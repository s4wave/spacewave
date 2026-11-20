//go:build linux || darwin

package checkoutroot

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// openFileVersion takes the version from an open file's stat record.
func openFileVersion(file nativeFile) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return statFileVersion(info), nil
}

// destinationVersion takes the version of the regular file at the destination name.
func destinationVersion(dir *os.Root, name string) ([]byte, error) {
	info, err := dir.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return statFileVersion(info), nil
}

// statFileVersion encodes the identity and content metadata of a regular file.
func statFileVersion(info fs.FileInfo) []byte {
	// Only regular files have a version that can authorize replacing the entry.
	if !info.Mode().IsRegular() {
		return nil
	}

	// Encode the identity and content metadata in fixed-width fields.
	stat := info.Sys().(*syscall.Stat_t)
	var fields [40]byte
	binary.LittleEndian.PutUint64(fields[0:8], fileDevice(stat))
	binary.LittleEndian.PutUint64(fields[8:16], stat.Ino)
	binary.LittleEndian.PutUint64(fields[16:24], uint64(info.Size()))               //nolint:gosec // Encode the signed field by its bit pattern.
	binary.LittleEndian.PutUint64(fields[24:32], uint64(info.ModTime().UnixNano())) //nolint:gosec // Encode the signed time by its bit pattern.
	binary.LittleEndian.PutUint64(fields[32:40], uint64(changeTime(stat)))          //nolint:gosec // Encode the signed time by its bit pattern.

	// Hash the encoded fields into an opaque token.
	version := sha256.Sum256(fields[:])
	return version[:]
}
