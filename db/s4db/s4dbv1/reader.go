package s4dbv1

import (
	"bytes"
	"context"
	"os"

	"github.com/s4wave/spacewave/db/s4db/filelock"
)

// Reader is an open previous-format database file. It holds the engine's
// writer lock and one reader slot from Open to Close, so no process of the
// previous engine writes the file while it is read, and Open refuses a file
// another process already has open.
type Reader struct {
	// f is the file.
	f *os.File
	// slot is the reader slot held, or -1.
	slot int
	// writer reports that the writer lock is held.
	writer bool
	// sb is the checkpoint the stream starts from.
	sb superblock
	// live estimates the bytes a copy of the file's contents takes.
	live int64
}

// Open opens the previous-format database at path for one streaming read.
// It fails with ErrInUse when another process has the file open, with
// ErrUnsupported when the file is not in the previous engine's format, and
// with ErrCorrupt when no superblock reads.
func Open(path string) (*Reader, error) {
	// Open the file for writing, which the byte locks need, and load it.
	f, err := os.OpenFile(path, os.O_RDWR, 0) // #nosec G703 -- the caller chooses the path.
	if err != nil {
		return nil, err
	}
	r := &Reader{f: f, slot: -1}
	if err := r.load(); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

// load excludes other processes, then reads the header, the superblock and
// the live size.
func (r *Reader) load() error {
	// Take the writer lock without waiting, so a process mid-transaction
	// refuses the open, then check every slot and take one.
	ok, err := filelock.Lock(r.f, lockWriter, false)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInUse
	}
	r.writer = true
	for i := range slots {
		held, err := filelock.Held(r.f, lockSlot+int64(i))
		if err != nil {
			return err
		}
		if held {
			return ErrInUse
		}
	}
	if _, err := filelock.Lock(r.f, lockSlot, false); err != nil {
		return err
	}
	r.slot = 0

	// Check the header and read the checkpoint.
	page := make([]byte, pageSize)
	if _, err := r.f.ReadAt(page, pageOff(headerPage)); err != nil {
		return err
	}
	if err := checkHeader(page); err != nil {
		return err
	}
	if r.sb, err = readSuper(r.f); err != nil {
		return err
	}

	// Estimate the live size: the tree pages and the live value bytes.
	live, err := liveValueBytes(r.f, r.sb)
	if err != nil {
		return err
	}
	r.live = live + int64(r.sb.treePages)*pageSize // #nosec G115 -- a page count.
	return nil
}

// LiveBytes estimates the bytes the file's contents take: the tree pages at
// the checkpoint and the value bytes its references point at. The log after
// the checkpoint is not counted.
func (r *Reader) LiveBytes() int64 {
	return r.live
}

// Stream calls fn with every key and its value in ascending key order,
// merging the checkpointed tree with the log after it and reading each
// referenced extent. The key and value last only through the call. Stream
// stops at the first error fn returns, or when ctx ends.
func (r *Reader) Stream(ctx context.Context, fn func(key, value []byte) error) error {
	// Replay the log, then walk the tree beside it.
	ov, err := replayLog(r.f, r.sb)
	if err != nil {
		return err
	}
	oit := ov.Iter()
	ook := oit.First()
	tw := newTreeWalk(r.f, r.sb.root)
	tk, tv, tok := tw.next()

	// Emit the smaller key of the two streams; at a tie the log wins.
	for n := 0; tok || ook; n++ {
		if n%1024 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		var key []byte
		var v value
		c := 1
		if tok && ook {
			c = bytes.Compare(tk, oit.Item().key)
		} else if tok {
			c = -1
		}
		switch {
		case c < 0:
			key, v = tk, tv
			tk, tv, tok = tw.next()
		default:
			key, v = oit.Item().key, oit.Item().val
			ook = oit.Next()
			if c == 0 {
				tk, tv, tok = tw.next()
			}
		}
		if err := r.emit(key, v, fn); err != nil {
			return err
		}
	}
	return tw.err
}

// emit passes a live pair to fn, reading its extent.
func (r *Reader) emit(key []byte, v value, fn func(key, value []byte) error) error {
	if v.del {
		return nil
	}
	data, err := v.read(r.f)
	if err != nil {
		return err
	}
	return fn(key, data)
}

// Close releases the locks and the file.
func (r *Reader) Close() error {
	// Release the slot, then the writer lock, then the file.
	var err error
	if r.slot >= 0 {
		err = filelock.Unlock(r.f, lockSlot+int64(r.slot))
		r.slot = -1
	}
	if r.writer {
		if uerr := filelock.Unlock(r.f, lockWriter); err == nil {
			err = uerr
		}
		r.writer = false
	}
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	return err
}
