package writelog

import (
	"bufio"
	"bytes"
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
	// missed is set once a write may have escaped the log: one recorded
	// while no epoch was open, or a store reporting writes it did not show.
	// A set missed keeps the next start from resuming the closed epoch.
	missed atomic.Bool
	// claimed is the directory Claim took, whose resume marker a miss
	// retires.
	claimed atomic.Pointer[string]
	// unsynced is set when a retired segment failed to sync, which keeps
	// Close from leaving a resume marker.
	unsynced atomic.Bool
	// retiring counts the retired segments still syncing.
	retiring sync.WaitGroup
	// attached is set while the log is a store's observer, as the store's
	// Attached calls report it.
	attached atomic.Bool
	// marker orders a miss's retirement of the marker after markResumable's
	// last check of missed and its write: either the check sees the miss, or
	// the miss waits for the marker and retires it.
	marker sync.Mutex

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
	// closed is the close line of the claimed directory's last epoch, and
	// resumable is set while that epoch may resume at it: Claim found its
	// marker, or this log closed it cleanly, and no start used it since.
	closed    Position
	resumable bool
	// witness fingerprints the store's files at Close, for the marker; nil
	// writes a marker only a log claimed by Set resumes.
	witness Witness
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

// record appends one line when the log is on, and records a miss when it is
// off.
func (l *Log) record(kind Kind, h *hash.Hash) {
	// While off, record the miss, then check again: a resume turns the log on
	// before it reads missed, so a write either sees the log on here or its
	// miss stops the resume.
	if h == nil {
		return
	}
	if !l.on.Load() {
		l.miss()
		if !l.on.Load() {
			return
		}
	}

	// Append under the lock, unless the epoch ended since the check.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		l.miss()
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

// Missed records that the store may accept a write the log does not see, as
// when its observer is set after the store's first write or detached before
// its last. It ends an open epoch with no close line, so its readers see a
// gap, and keeps a closed one from resuming.
func (l *Log) Missed() {
	l.miss()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.breakEpoch(errors.New("the store took a write the log did not see"))
	}
}

// miss records a write the log did not see while no epoch was open. Only the
// first miss does any work: it retires a resume marker Close left, so no later
// start resumes past the write, and when the retirement cannot be made durable
// it poisons the epoch, as retireMarker describes. With no marker, which is
// the case until Close, that is one failed open. Every later call is one
// atomic load.
func (l *Log) miss() {
	// Skip once recorded.
	if l.missed.Load() || !l.missed.CompareAndSwap(false, true) {
		return
	}

	// Retire the marker, after any marker write in progress.
	if dir := l.claimed.Load(); dir != nil {
		l.marker.Lock()
		defer l.marker.Unlock()
		if _, durable := retireMarker(*dir); !durable {
			l.le.WithField("dir", *dir).Error(stuckMarker)
		}
	}
}

// Attached records whether the log is a store's observer. The store calls it
// as it attaches and detaches the log.
func (l *Log) Attached(on bool) {
	l.attached.Store(on)
}

// Claim binds the log to dir and takes the directory's resume marker, durably,
// so the epoch it names may resume only in this log, and only while it misses
// no write. Call it before the store takes its first write, whether or not the
// log will be on, with the log already the store's observer: a process that
// writes the store without claiming leaves the marker for a later start to
// resume past its writes. A log that is no store's observer cannot see the
// writes it would need, so Claim still takes the marker but records a miss,
// and returns an error: nothing resumes and Close leaves no marker. No error
// stops the store; the caller logs it. Set claims its directory when Claim
// did not.
//
// Opened is the witness of the store's files taken before the store opened,
// with closing the Witness Close takes once the store has closed: the epoch
// resumes only when the marker carries a witness equal to opened, so a write
// no log saw between the two processes, such as a tool that opened the store
// alone, starts a new epoch. An empty opened, a marker without a witness, and
// a witness that differs all start a new epoch; so does a closing that fails.
func (l *Log) Claim(dir string, opened []byte, closing Witness) error {
	// Take the marker, and keep its epoch only for the same witness.
	l.mu.Lock()
	defer l.mu.Unlock()
	l.witness = closing
	m, err := l.claim(dir)
	if err != nil {
		return err
	}
	if len(opened) == 0 || !bytes.Equal(m.witness, opened) {
		l.resumable = false
	}

	// Refuse to resume what no store showed this log.
	if !l.attached.Load() {
		l.miss()
		return errors.New("write log: claimed while no store's observer; resume disabled")
	}
	return nil
}

// claim binds the log to dir, taking its marker and returning it, unless it is
// bound to it already, when it returns the marker this log would write.
func (l *Log) claim(dir string) (marker, error) {
	// Keep a directory already claimed.
	if prev := l.claimed.Load(); prev != nil && *prev == dir {
		return marker{pos: l.closed}, nil
	}

	// Take the marker, then bind; a miss before the bind finds no marker it
	// could retire, and missed keeps the epoch from resuming.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return marker{}, err
	}
	m, resumable, retired := takeMarker(dir)
	l.closed, l.resumable = m.pos, resumable
	if !retired {
		l.le.WithField("dir", dir).Error(stuckMarker)
	}
	l.claimed.Store(&dir)
	return m, nil
}

// Set switches the log: nil turns it off, closing the open epoch with its
// close line; options turn it on in their directory, claiming it. Options
// equal to the current ones change nothing, so a broken epoch stays ended
// until the log is turned off and on again. Turning on resumes the
// directory's last epoch when it closed cleanly, in this log or in the one
// whose marker Claim took, and the log missed no write since; otherwise it
// starts a new epoch and removes the segments of every other epoch there,
// which no cycle can use once the epoch changed. Turning off leaves no marker:
// only Close does.
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

	// Close any epoch, claim the directory, then start an epoch in it.
	if err := l.closeEpoch(); err != nil {
		l.le.WithError(err).Warn("write log: closing the epoch")
	}
	l.opts = next
	if _, err := l.claim(next.Dir); err != nil {
		l.breakEpoch(err)
		return err
	}
	if err := l.startEpoch(); err != nil {
		l.breakEpoch(err)
		return err
	}
	return nil
}

// Close closes the open epoch with its close line, turns the log off, and
// leaves the resume marker that lets the next process's Claim resume the
// epoch, unless a write escaped it. Call it once the store takes no more
// writes: a later write is a miss that retires the marker again, and a process
// killed between such a write and its retirement leaves the marker behind. It
// waits on the last segment's sync, a sync of the marker and one of the
// directory. After a miss it retries the marker's retirement instead, so a
// marker a miss could neither remove nor poison does not outlive a directory
// that has become writable again.
func (l *Log) Close() error {
	// Close the epoch and turn the log off.
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.closeEpoch()
	l.opts = Options{}

	// Leave the marker once the close line is written.
	if err != nil {
		return err
	}
	return l.markResumable()
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

// startEpoch opens an epoch in the configured directory: the last one
// resumed, or a new one, removing the segments of the other epochs there.
func (l *Log) startEpoch() error {
	// Retire a marker this log's Close left, durably, before turning on: a
	// resumed epoch's lines then never follow a marker a crash could bring
	// back, and one that cannot be retired resumes nothing.
	resumable := l.resumable
	l.resumable = false
	if _, durable := retireMarker(l.opts.Dir); !durable {
		resumable = false
	}

	// Turn on before reading missed, so a write the log does not take is a
	// miss this start sees; writes wait on the lock until the segment opens.
	l.on.Store(true)
	resume := resumable && !l.missed.Load() && closedAt(l.opts.Dir, l.closed)
	if resume {
		l.epoch, l.seq, l.broken = l.closed.Epoch, l.closed.Seq, ""
	} else {
		var id [8]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		l.epoch, l.seq, l.broken = hex.EncodeToString(id[:]), 0, ""
		l.missed.Store(false)
	}

	// Remove the other epochs' segments, and a resumed epoch's segment that
	// holds no line but its close, which the next segment replaces under its
	// name; count the kept ones, then open the next segment.
	segs, err := listSegments(l.opts.Dir)
	if err != nil {
		return err
	}
	l.bytes = 0
	for _, seg := range segs {
		if seg.epoch == l.epoch && seg.first != l.seq+1 {
			l.bytes += seg.size
			continue
		}
		if err := os.Remove(seg.path(l.opts.Dir)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return l.openSegment()
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

// rotate retires the open segment and opens the next.
func (l *Log) rotate() error {
	// Flush the full segment and retire it.
	if err := l.w.Flush(); err != nil {
		return err
	}
	l.retire(l.file)
	l.file = nil

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

	// Write the close line, then retire the file.
	line := appendMarker(l.w.AvailableBuffer(), l.seq, kindClose, l.epoch)
	_, err := l.w.Write(line)
	if err == nil {
		err = l.w.Flush()
	}
	l.bytes += int64(len(line))
	l.retire(l.file)
	l.file = nil
	if err != nil {
		l.broken = err.Error()
		return err
	}

	// Remember the close line, which a start in this log may resume at.
	l.closed = Position{Epoch: l.epoch, Seq: l.seq}
	l.resumable = true
	return nil
}

// retire syncs and closes a finished segment off the writer's path. Every
// segment but the open one is retired, so a close waits on one sync.
func (l *Log) retire(file io.WriteCloser) {
	l.retiring.Add(1)
	go func() {
		defer l.retiring.Done()
		synced := false
		if f, ok := file.(interface{ Sync() error }); ok {
			synced = f.Sync() == nil
		}
		if err := file.Close(); err != nil || !synced {
			l.unsynced.Store(true)
		}
	}()
}

// markResumable writes the marker naming the closed epoch's close line, once
// its segments are durable, so the next process's Claim can resume it. The
// last check of missed and the write hold marker, so a miss either stops the
// write or retires the marker after it.
func (l *Log) markResumable() error {
	// Skip when the epoch cannot resume; after a miss, retry the retirement.
	dir := l.claimed.Load()
	if dir == nil {
		return nil
	}
	if l.missed.Load() {
		if _, durable := retireMarker(*dir); !durable {
			l.le.WithField("dir", *dir).Error(stuckMarker)
		}
		return nil
	}
	if !l.resumable {
		return nil
	}

	// Wait for the retired segments' syncs, so the marker implies the lines.
	l.retiring.Wait()
	if l.unsynced.Load() || !closedAt(*dir, l.closed) {
		return nil
	}

	// Fingerprint the closed store; one that cannot be read leaves no marker.
	var witness []byte
	if l.witness != nil {
		var err error
		if witness, err = l.witness(); err != nil || len(witness) == 0 {
			return err
		}
	}

	// Write the marker beside them unless a miss came first.
	l.marker.Lock()
	defer l.marker.Unlock()
	if l.missed.Load() {
		return nil
	}
	if err := writeMarker(*dir, l.closed, witness); err != nil {
		_, _ = retireMarker(*dir)
		return err
	}
	return nil
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
