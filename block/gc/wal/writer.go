//go:build js

// Package block_gc_wal implements the GC write-ahead log for OPFS volumes.
package block_gc_wal

import (
	"context"
	"strconv"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
)

// walExtension is the file extension for WAL entries.
const walExtension = ".wal"

// seqCounterFile is the filename for the persisted sequence counter.
const seqCounterFile = "seq"

// seqDigits is the zero-padded width of the sequence prefix in filenames.
const seqDigits = 20

// Writer appends WAL entries to an OPFS directory.
type Writer struct {
	dir        js.Value
	lockPrefix string
	orderLock  string
	stwLock    string
}

// NewWriter creates a WAL writer for the given OPFS directory.
// lockPrefix is used for per-file WebLock names.
// orderLock is the WebLock name for sequence allocation (e.g. "<vol>|gc-wal-order").
// stwLock is the STW WebLock name (e.g. "<vol>|gc-stw"). Acquired in shared
// mode during append so the sweep executor can block writers by taking it
// exclusively.
func NewWriter(dir js.Value, lockPrefix, orderLock, stwLock string) *Writer {
	return &Writer{
		dir:        dir,
		lockPrefix: lockPrefix,
		orderLock:  orderLock,
		stwLock:    stwLock,
	}
}

// Dir returns the OPFS directory handle for the WAL files.
func (w *Writer) Dir() js.Value {
	return w.dir
}

// Append serializes the given edges into a WALEntry, allocates a durable
// sequence number, and writes the entry as a single OPFS file.
// Acquires the STW lock in shared mode for the duration of the append.
func (w *Writer) Append(ctx context.Context, adds, removes []*RefEdge) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}

	// Acquire STW lock in shared mode. Multiple writers can proceed
	// concurrently. The sweep executor takes this lock exclusively to
	// block new appends during reconciliation.
	stwRelease, err := filelock.AcquireWebLock(w.stwLock, false)
	if err != nil {
		return errors.Wrap(err, "acquire STW shared lock")
	}
	defer stwRelease()

	// Allocate durable sequence number under exclusive ordering lock.
	seq, err := w.allocSequence()
	if err != nil {
		return errors.Wrap(err, "allocate WAL sequence")
	}

	entry := &WALEntry{
		Sequence:  seq,
		Timestamp: time.Now().UnixNano(),
		Adds:      adds,
		Removes:   removes,
	}

	data, err := entry.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal WAL entry")
	}

	filename := formatFilename(seq)
	f, release, err := filelock.AcquireFile(w.dir, filename, w.lockPrefix, true)
	if err != nil {
		return errors.Wrap(err, "acquire WAL file")
	}
	defer release()

	if err := f.Truncate(0); err != nil {
		return errors.Wrap(err, "truncate WAL file")
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		return errors.Wrap(err, "write WAL file")
	}
	return f.Flush()
}

// allocSequence acquires the ordering lock, reads the current counter,
// increments it, persists, and returns the new value. The ordering lock
// provides mutual exclusion so the counter file needs no per-file lock.
func (w *Writer) allocSequence() (uint64, error) {
	release, err := filelock.AcquireWebLock(w.orderLock, true)
	if err != nil {
		return 0, errors.Wrap(err, "acquire order lock")
	}
	defer release()

	if !opfs.SyncAvailable() {
		var seq uint64
		data, err := opfs.ReadFile(w.dir, seqCounterFile)
		if err != nil && !opfs.IsNotFound(err) {
			return 0, errors.Wrap(err, "read counter")
		}
		if len(data) != 0 {
			seq, _ = strconv.ParseUint(string(data), 10, 64)
		}
		seq++
		data = []byte(strconv.FormatUint(seq, 10))
		if err := opfs.WriteFile(w.dir, seqCounterFile, data); err != nil {
			return 0, errors.Wrap(err, "write counter")
		}
		return seq, nil
	}

	// Open counter file once for both read and write.
	f, err := opfs.CreateSyncFile(w.dir, seqCounterFile)
	if err != nil {
		return 0, errors.Wrap(err, "open counter file")
	}
	defer f.Close()

	var seq uint64
	if size := f.Size(); size > 0 {
		buf := make([]byte, size)
		n, err := f.ReadAt(buf, 0)
		if err == nil {
			seq, _ = strconv.ParseUint(string(buf[:n]), 10, 64)
		}
	}

	seq++
	data := []byte(strconv.FormatUint(seq, 10))
	f.Truncate(0)
	if _, err := f.WriteAt(data, 0); err != nil {
		return 0, errors.Wrap(err, "write counter")
	}
	f.Flush()
	return seq, nil
}

// formatFilename produces a WAL filename: <zero-padded seq>-<ulid>.wal
func formatFilename(seq uint64) string {
	s := strconv.FormatUint(seq, 10)
	pad := max(seqDigits-len(s), 0)
	var buf []byte
	for range pad {
		buf = append(buf, '0')
	}
	buf = append(buf, s...)
	buf = append(buf, '-')
	buf = append(buf, ulid.NewULID()...)
	buf = append(buf, walExtension...)
	return string(buf)
}
