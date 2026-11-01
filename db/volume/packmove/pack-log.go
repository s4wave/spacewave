package packmove

import (
	"bytes"
	"io"
	"os"

	"github.com/pkg/errors"
)

// logState is what replaying a pack log recovers.
type logState struct {
	// lastKey is the last key the last committed batch examined, or nil.
	lastKey []byte
	// packs counts the committed batches that hold blocks.
	packs uint64
	// blocks counts the blocks in them.
	blocks uint64
	// bytes is the size of their data.
	bytes uint64
	// bad counts the committed blocks whose data failed their hash.
	bad uint64
	// pending is the trailing batch that opened but never committed, or nil.
	pending *frame
	// validLen is the length of the log up to its last whole frame.
	validLen int64
}

// packLog is the append-only record of a pack run: every batch's keys before
// the batch uploads, and a commit once it is in the bucket. It is both the
// progress a killed run resumes from and the record that Switch reads to find
// the blocks the bucket holds.
type packLog struct {
	// f is the log, open for appending.
	f *os.File
	// buf is the encoding scratch.
	buf []byte
}

// openPackLog opens or creates the log at path, cuts a torn tail left by a
// killed write, and returns the log with the state its frames record.
func openPackLog(path string) (*packLog, logState, error) {
	// Open the file for reading and appending.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- the operator chooses the state directory.
	if err != nil {
		return nil, logState{}, err
	}

	// Replay the frames the log holds.
	state, err := replayLog(f)
	if err != nil {
		_ = f.Close()
		return nil, logState{}, err
	}

	// Cut a torn tail, then append after the last whole frame.
	if err := f.Truncate(state.validLen); err != nil {
		_ = f.Close()
		return nil, logState{}, err
	}
	if _, err := f.Seek(state.validLen, 0); err != nil {
		_ = f.Close()
		return nil, logState{}, err
	}
	return &packLog{f: f}, state, nil
}

// replayLog reads the frames of f and returns the state they record.
func replayLog(f *os.File) (logState, error) {
	var state logState
	if _, err := f.Seek(0, 0); err != nil {
		return state, err
	}
	fr := newFrameReader(f)
	for {
		next, err := fr.next()
		if err != nil {
			if errors.Is(err, errTornFrame) || errors.Is(err, io.EOF) {
				return state, nil
			}
			return state, err
		}
		state.validLen += next.size
		if next.kind == framePending {
			state.pending = &next
			continue
		}
		if state.pending == nil {
			return state, errors.New("pack log commits a batch it never opened")
		}
		state.commit(state.pending)
		state.pending = nil
	}
}

// commit adds a committed batch to the state.
func (s *logState) commit(batch *frame) {
	// Count the batch's packed and rejected blocks.
	packed := batch.packed()
	if packed != 0 {
		s.packs++
	}
	s.blocks += packed
	s.bad += uint64(len(batch.entries)) - packed
	s.bytes += batch.bytes
	if len(batch.through) != 0 {
		s.lastKey = bytes.Clone(batch.through)
	}
}

// open appends the pending frame of a batch, before the batch uploads.
func (l *packLog) open(batch frame) error {
	batch.kind = framePending
	l.buf = appendFrame(l.buf[:0], batch)
	_, err := l.f.Write(l.buf)
	return err
}

// commit appends the commit frame of the pending batch and makes the log
// durable, which also makes the pending frame durable.
func (l *packLog) commit() error {
	l.buf = appendFrame(l.buf[:0], frame{kind: frameCommit})
	if _, err := l.f.Write(l.buf); err != nil {
		return err
	}
	return l.f.Sync()
}

// close closes the log file.
func (l *packLog) close() error {
	return l.f.Close()
}
