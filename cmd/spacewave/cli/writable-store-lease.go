//go:build !js && !wasip1

package spacewave_cli

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"

	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
)

// These offsets mirror bbolt lock-file version 1. Writer-capable openers
// increment the header count, and every live opener records its PID in the
// reader table.
const (
	bboltLockFileMagic     = 0xBB01D100
	bboltLockFileVersion   = 1
	bboltReaderTableOffset = 128
	bboltReaderSlotSize    = 64
	bboltMaxReadersOffset  = 8
	bboltWriterCountOffset = 16
)

func findWritableStoreLeaseHolder(statePath string) (int, string, error) {
	entries, err := os.ReadDir(statePath)
	if err != nil {
		return 0, "", err
	}
	for _, entry := range entries {
		if entry.IsDir() ||
			entry.Name() == statePathLeaseStorageID+storage_native.BoltDBExt ||
			!strings.HasSuffix(entry.Name(), storage_native.BoltDBExt) {
			continue
		}
		storePath := filepath.Join(statePath, entry.Name())
		pid, err := findWritableStoreLeaseHolderAt(storePath, os.Getpid())
		if err != nil {
			return 0, "", err
		}
		if pid > 0 {
			return pid, storePath, nil
		}
	}
	return 0, "", nil
}

func findWritableStoreLeaseHolderAt(storePath string, excludePID int) (int, error) {
	lockData, err := os.ReadFile(storePath + "-lock")
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if len(lockData) < bboltReaderTableOffset ||
		binary.LittleEndian.Uint32(lockData[0:4]) != bboltLockFileMagic ||
		binary.LittleEndian.Uint32(lockData[4:8]) != bboltLockFileVersion ||
		binary.LittleEndian.Uint32(lockData[bboltWriterCountOffset:bboltWriterCountOffset+4]) == 0 {
		return 0, nil
	}

	maxReaders := int(binary.LittleEndian.Uint32(lockData[bboltMaxReadersOffset : bboltMaxReadersOffset+4]))
	for slot := range maxReaders {
		offset := bboltReaderTableOffset + slot*bboltReaderSlotSize
		if offset+bboltReaderSlotSize > len(lockData) {
			break
		}
		pid := int(binary.LittleEndian.Uint32(lockData[offset : offset+4]))
		if pid != 0 && pid != excludePID && statePathLeaseProcessAlive(pid) {
			return pid, nil
		}
	}
	return 0, nil
}
