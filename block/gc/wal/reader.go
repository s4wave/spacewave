//go:build js

package block_gc_wal

import (
	"strings"
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	"github.com/pkg/errors"
)

// ReadWAL lists the WAL directory in lexicographic order, reads and
// deserializes each .wal file, and returns entries in durable sequence
// order. Non-.wal files (e.g. the seq counter) are skipped.
func ReadWAL(dir js.Value, lockPrefix string) ([]*WALEntry, []string, error) {
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return nil, nil, errors.Wrap(err, "list WAL directory")
	}

	// Filter and sort .wal files. ListDirectory returns sorted names;
	// sequence-prefixed filenames sort lexicographically into replay order.
	var walFiles []string
	for _, name := range names {
		if strings.HasSuffix(name, walExtension) {
			walFiles = append(walFiles, name)
		}
	}

	entries := make([]*WALEntry, 0, len(walFiles))
	for _, name := range walFiles {
		entry, err := readWALFile(dir, name, lockPrefix)
		if err != nil {
			return nil, nil, errors.Wrap(err, name)
		}
		entries = append(entries, entry)
	}
	return entries, walFiles, nil
}

// DeleteWALEntry removes a single WAL file from the directory.
func DeleteWALEntry(dir js.Value, filename string) error {
	return opfs.DeleteFile(dir, filename)
}

// readWALFile reads and deserializes a single WAL entry file.
func readWALFile(dir js.Value, name, lockPrefix string) (*WALEntry, error) {
	f, release, err := filelock.AcquireFile(dir, name, lockPrefix, false)
	if err != nil {
		return nil, errors.Wrap(err, "acquire file")
	}
	defer release()

	size, err := f.Size()
	if err != nil {
		return nil, errors.Wrap(err, "file size")
	}
	if size == 0 {
		return &WALEntry{}, nil
	}

	buf := make([]byte, size)
	n, err := f.ReadAt(buf, 0)
	if err != nil {
		return nil, errors.Wrap(err, "read file")
	}

	entry := &WALEntry{}
	if err := entry.UnmarshalVT(buf[:n]); err != nil {
		return nil, errors.Wrap(err, "unmarshal WAL entry")
	}
	return entry, nil
}
