package writelog

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// DefaultCap is the bytes of segments a log directory holds when the options
// set none: about 18 minutes of writes at 940 puts a second.
const DefaultCap = 64 << 20

// DefaultSegmentBytes is the size a segment rotates at when the options set
// none: about 70 seconds of writes at 940 puts a second.
const DefaultSegmentBytes = 4 << 20

// bufferBytes is the line buffer, about a second of writes at 940 puts a
// second; a full buffer is one write to the file.
const bufferBytes = 64 << 10

// errCap ends an epoch whose directory reached its cap.
var errCap = errors.New("write log directory reached its cap")

// Options configures a log that is on.
type Options struct {
	// Dir is the directory of the segments.
	Dir string
	// Cap bounds the bytes of the directory's segments; zero uses DefaultCap.
	// Past it, the epoch ends.
	Cap int64
	// SegmentBytes is the size a segment rotates at; zero uses
	// DefaultSegmentBytes.
	SegmentBytes int64
	// create opens a new segment file; nil creates it with os.OpenFile.
	create func(path string) (io.WriteCloser, error)
}

// Status is the state of a log.
type Status struct {
	// On is set while an epoch is open.
	On bool
	// Dir is the configured directory, empty while the log is switched off.
	Dir string
	// Epoch is the open epoch, or the last one.
	Epoch string
	// Seq is the epoch's last line.
	Seq uint64
	// Bytes is the bytes of the directory's segments, with the buffered lines.
	Bytes int64
	// Broken is why the last epoch ended without a close line, empty when it
	// did not.
	Broken string
}

// Log is a write log. Its zero value is not usable; use New. A Log is off
// until Set turns it on.
type Log struct {
	// le logs the end of a broken epoch.
	le *logrus.Entry
	// on is set while an epoch is open, read without mu by every record.
	on atomic.Bool

	// mu guards the fields below.
	mu sync.Mutex
	// opts is the configuration, Dir empty while switched off.
	opts Options
	// epoch is the open epoch's id, or the last one.
	epoch string
	// seq is the epoch's last line.
	seq uint64
	// file is the open segment, nil while no epoch is open.
	file io.WriteCloser
	// w buffers the lines of file.
	w *bufio.Writer
	// segment is the bytes of the open segment, with the buffered lines.
	segment int64
	// bytes is the bytes of the directory's segments, with the buffered lines.
	bytes int64
	// broken is why the last epoch ended without a close line.
	broken string
	// scratch holds the encoded hash of the line being written.
	scratch [hashScratchBytes]byte
}

// New builds a log that is off.
func New(le *logrus.Entry) *Log {
	return &Log{le: le}
}

// Put records a block the store accepted.
func (l *Log) Put(h *hash.Hash) { l.record(KindPut, h) }

// Root records a block a named root was set to.
func (l *Log) Root(h *hash.Hash) { l.record(KindRoot, h) }

// Fill records a block read back from the remote.
func (l *Log) Fill(h *hash.Hash) { l.record(KindFill, h) }

// record appends one line when the log is on.
func (l *Log) record(kind Kind, h *hash.Hash) {
	// Skip at once while off.
	if !l.on.Load() || h == nil {
		return
	}

	// Append under the lock, unless the epoch ended since the check.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	if err := l.appendLine(kind, h); err != nil {
		l.breakEpoch(err)
	}
}

// appendLine appends one record to the open epoch, rotating the segment or
// flushing the buffer first when needed.
func (l *Log) appendLine(kind Kind, h *hash.Hash) error {
	// End the epoch past the cap, and rotate a full segment.
	if l.bytes >= l.opts.Cap {
		return errCap
	}
	if l.segment >= l.opts.SegmentBytes {
		if err := l.rotate(); err != nil {
			return err
		}
	}

	// Make room for a whole line in the buffer.
	if l.w.Available() < maxLineBytes {
		if err := l.w.Flush(); err != nil {
			return err
		}
	}

	// Write the line into the buffer's free space.
	l.seq++
	line := appendRecord(l.w.AvailableBuffer(), l.seq, kind, h, &l.scratch)
	if _, err := l.w.Write(line); err != nil {
		return err
	}
	l.segment += int64(len(line))
	l.bytes += int64(len(line))
	return nil
}

// Set switches the log: nil turns it off, closing the open epoch with its
// close line; options turn it on, starting an epoch in their directory. Options
// equal to the current ones change nothing, so a broken epoch stays ended until
// the log is turned off and on again. A new directory, or a log turned on,
// starts a new epoch and removes the segments of every other epoch there, which
// no cycle can use once the epoch changed.
func (l *Log) Set(opts *Options) error {
	// Hold the log for the switch.
	l.mu.Lock()
	defer l.mu.Unlock()

	// Turn off, closing the epoch.
	if opts == nil || opts.Dir == "" {
		err := l.closeEpoch()
		l.opts = Options{}
		return err
	}

	// Keep the epoch of the same directory, taking the new bounds.
	next := *opts
	if next.Cap <= 0 {
		next.Cap = DefaultCap
	}
	if next.SegmentBytes <= 0 {
		next.SegmentBytes = DefaultSegmentBytes
	}
	if next.Dir == l.opts.Dir {
		l.opts.Cap, l.opts.SegmentBytes = next.Cap, next.SegmentBytes
		return nil
	}

	// Close any epoch, then start one in the directory.
	if err := l.closeEpoch(); err != nil {
		l.le.WithError(err).Warn("write log: closing the epoch")
	}
	l.opts = next
	if err := l.startEpoch(); err != nil {
		l.breakEpoch(err)
		return err
	}
	return nil
}

// Close closes the open epoch with its close line and turns the log off.
func (l *Log) Close() error {
	return l.Set(nil)
}

// Status returns the log's state.
func (l *Log) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Status{
		On:     l.file != nil,
		Dir:    l.opts.Dir,
		Epoch:  l.epoch,
		Seq:    l.seq,
		Bytes:  l.bytes,
		Broken: l.broken,
	}
}

// Position flushes the buffered lines and returns the open epoch's last line,
// or false when no epoch is open. A caller reads it before the roots, so every
// write the roots do not reflect is logged after it.
func (l *Log) Position() (Position, bool) {
	// Flush the open epoch's lines, then name its last.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return Position{}, false
	}
	if err := l.w.Flush(); err != nil {
		l.breakEpoch(err)
		return Position{}, false
	}
	return Position{Epoch: l.epoch, Seq: l.seq}, true
}

// startEpoch opens a new epoch in the configured directory, removing the
// segments of the other epochs there.
func (l *Log) startEpoch() error {
	// Name the epoch.
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	l.epoch, l.seq, l.broken = hex.EncodeToString(id[:]), 0, ""

	// Remove the other epochs' segments, then open the first segment.
	if err := os.MkdirAll(l.opts.Dir, 0o755); err != nil {
		return err
	}
	segs, err := listSegments(l.opts.Dir)
	if err != nil {
		return err
	}
	for _, seg := range segs {
		if err := os.Remove(seg.path(l.opts.Dir)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	l.bytes = 0
	if err := l.openSegment(); err != nil {
		return err
	}
	l.on.Store(true)
	return nil
}

// openSegment opens the segment holding the next line and writes its epoch
// line.
func (l *Log) openSegment() error {
	// Create the file.
	seg := segment{epoch: l.epoch, first: l.seq + 1}
	create := l.opts.create
	if create == nil {
		create = createFile
	}
	file, err := create(seg.path(l.opts.Dir))
	if err != nil {
		return err
	}

	// Buffer it, and write the epoch line.
	l.file = file
	if l.w == nil {
		l.w = bufio.NewWriterSize(file, bufferBytes)
	} else {
		l.w.Reset(file)
	}
	line := appendMarker(l.w.AvailableBuffer(), seg.first, kindEpoch, l.epoch)
	if _, err := l.w.Write(line); err != nil {
		return err
	}
	l.segment = int64(len(line))
	l.bytes += int64(len(line))
	return nil
}

// rotate closes the open segment and opens the next.
func (l *Log) rotate() error {
	// Close the full segment.
	if err := l.w.Flush(); err != nil {
		return err
	}
	file := l.file
	l.file = nil
	if err := file.Close(); err != nil {
		return err
	}

	// Recount the directory, which a reader trimming it directly shrinks,
	// and open the next segment.
	segs, err := listSegments(l.opts.Dir)
	if err != nil {
		return err
	}
	l.bytes = 0
	for _, s := range segs {
		l.bytes += s.size
	}
	return l.openSegment()
}

// closeEpoch writes the close line of the open epoch and closes its segment.
func (l *Log) closeEpoch() error {
	// Skip when no epoch is open.
	if l.file == nil {
		return nil
	}
	l.on.Store(false)

	// Write the close line and close the file.
	line := appendMarker(l.w.AvailableBuffer(), l.seq, kindClose, l.epoch)
	_, err := l.w.Write(line)
	if err == nil {
		err = l.w.Flush()
	}
	l.bytes += int64(len(line))
	if cerr := l.file.Close(); err == nil {
		err = cerr
	}
	l.file = nil
	if err != nil {
		l.broken = err.Error()
	}
	return err
}

// breakEpoch ends the open epoch with no close line, so every reader sees a
// gap, and turns the log off until Set turns it on again.
func (l *Log) breakEpoch(err error) {
	// Stop the records and drop the buffer.
	l.on.Store(false)
	l.broken = err.Error()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	if l.w != nil {
		l.w.Reset(io.Discard)
	}

	// Warn once.
	l.le.WithError(err).WithField("epoch", l.epoch).Warn("write log epoch ended without a close line")
}

// createFile creates a segment file, failing if it exists.
func createFile(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}
