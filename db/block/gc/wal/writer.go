//go:build js

// Package block_gc_wal implements the GC write-ahead log for OPFS volumes.
package block_gc_wal

import (
	"context"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
)

// walExtension is the file extension for WAL entries.
const walExtension = ".wal"

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

	// Allocate durable sequence number and write the WAL entry under the same
	// exclusive order lock so concurrent appenders cannot observe the same
	// filename set before either entry has been persisted.
	releaseOrder, err := filelock.AcquireWebLock(w.orderLock, true)
	if err != nil {
		return errors.Wrap(err, "acquire order lock")
	}
	defer releaseOrder()

	seq, err := w.nextSequenceLocked()
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
	if err := filelock.WriteFile(w.dir, filename, w.lockPrefix, data); err != nil {
		return errors.Wrap(err, "write WAL file")
	}
	return nil
}

// nextSequenceLocked derives the next sequence from existing WAL filenames.
// The caller must hold orderLock until the corresponding WAL file is written.
func (w *Writer) nextSequenceLocked() (uint64, error) {
	names, err := opfs.ListDirectory(w.dir)
	if err != nil {
		return 0, errors.Wrap(err, "list WAL directory")
	}
	var seq uint64
	for _, name := range names {
		n, ok := parseFilenameSequence(name)
		if ok && n > seq {
			seq = n
		}
	}
	return seq + 1, nil
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

func parseFilenameSequence(name string) (uint64, bool) {
	if !strings.HasSuffix(name, walExtension) || len(name) <= seqDigits || name[seqDigits] != '-' {
		return 0, false
	}
	seq, err := strconv.ParseUint(name[:seqDigits], 10, 64)
	return seq, err == nil
}
