package s4dbv1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"github.com/s4wave/spacewave/db/s4db/filelock"
	"github.com/tidwall/btree"
)

// Reader is an open previous-format database file. It reads the file only
// through an io.ReaderAt, so nothing it does can write, truncate or sync the
// file. A Reader is not safe for concurrent use.
type Reader struct {
	// r reads the file.
	r io.ReaderAt
	// release drops the locks and closes the file, nil once closed.
	release func() error
	// sb is the checkpoint reads start from.
	sb superblock
	// live estimates the bytes a copy of the file's contents takes.
	live int64
	// ov holds the log after the checkpoint once replayed, nil before.
	ov *btree.BTreeG[oentry]
}

// Open opens the previous-format database at path read-only: the file is
// opened only for reading and holds a shared lock on the engine's writer byte
// until Close. Readers share the file; a process of the previous engine
// cannot commit to it and an in-place conversion cannot replace it meanwhile.
// It fails with ErrInUse when a writer or a conversion holds the file or
// this process has it open, with ErrUnsupported when the file is not in the
// previous engine's format, and with ErrCorrupt when no superblock reads.
func Open(path string) (*Reader, error) {
	return open(path, openSourceReadOnly, lockShared)
}

// OpenExclusive opens the previous-format database at path for the in-place
// conversion, which replaces the file. It holds the engine's writer lock and
// one reader slot until Close, so it fails with ErrInUse when any other
// handle has the file open, readers included. Its descriptor is writable only
// because a write lock needs one; the Reader still only reads.
func OpenExclusive(path string) (*Reader, error) {
	return open(path, openSource, lockExclusive)
}

// open opens path with opener, takes lock, and reads the header, the
// superblock and the live size, releasing everything on failure.
func open(path string, opener func(string) (*os.File, error), lock func(*os.File) (func() error, error)) (*Reader, error) {
	// Share descriptor ownership with the new engine: a rejected probe must
	// never close another descriptor of the locked source on Darwin.
	f, closeFile, err := openRegisteredSource(path, opener)
	if err != nil {
		return nil, err
	}
	unlock, err := lock(f)
	if err != nil {
		return nil, errors.Join(err, closeFile())
	}
	r := &Reader{r: f, release: func() error { return errors.Join(unlock(), closeFile()) }}

	// Load under the locks, releasing them on failure.
	if err := r.load(); err != nil {
		return nil, errors.Join(err, r.Close())
	}
	return r, nil
}

// lockShared takes a read lock on the writer byte, which excludes the
// previous engine's commits and an exclusive open.
func lockShared(f *os.File) (func() error, error) {
	ok, err := filelock.LockShared(f, lockWriter)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInUse
	}
	return func() error { return filelock.Unlock(f, lockWriter) }, nil
}

// lockExclusive takes the writer lock without waiting, so a process
// mid-transaction refuses the open, then checks every slot and takes one.
func lockExclusive(f *os.File) (func() error, error) {
	// Take the writer lock.
	ok, err := filelock.Lock(f, lockWriter, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInUse
	}
	unlockWriter := func() error { return filelock.Unlock(f, lockWriter) }

	// Refuse a file any reader slot holds, then take slot 0.
	for i := range slots {
		held, err := filelock.Held(f, lockSlot+int64(i))
		if err == nil && held {
			err = ErrInUse
		}
		if err != nil {
			return nil, errors.Join(err, unlockWriter())
		}
	}
	if _, err := filelock.Lock(f, lockSlot, false); err != nil {
		return nil, errors.Join(err, unlockWriter())
	}
	return func() error { return errors.Join(filelock.Unlock(f, lockSlot), unlockWriter()) }, nil
}

// load reads the header, the superblock and the live size.
func (r *Reader) load() error {
	// Check the header and read the checkpoint.
	page := make([]byte, pageSize)
	if _, err := r.r.ReadAt(page, pageOff(headerPage)); err != nil {
		return err
	}
	if err := checkHeader(page); err != nil {
		return err
	}
	var err error
	if r.sb, err = readSuper(r.r); err != nil {
		return err
	}

	// Estimate the live size: the tree pages and the live value bytes.
	live, err := liveValueBytes(r.r, r.sb)
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

// Checkpoint returns the checkpoint the reader reads from.
func (r *Reader) Checkpoint() Checkpoint {
	return Checkpoint{
		Seq:         r.sb.seq,
		Keys:        r.sb.count,
		TreePages:   r.sb.treePages,
		LogRunBytes: r.sb.logEnd - r.sb.logPos,
	}
}

// overlay returns the log after the checkpoint, replaying it on first use.
func (r *Reader) overlay() (*btree.BTreeG[oentry], error) {
	if r.ov == nil {
		ov, err := replayLog(r.r, r.sb)
		if err != nil {
			return nil, err
		}
		r.ov = ov
	}
	return r.ov, nil
}

// Get returns the value of key and whether the file holds it: the log after
// the checkpoint decides when it changed the key, else the checkpointed
// tree. The first Get or Stream replays that log; after it each Get reads
// one page per tree level and the value's extent.
func (r *Reader) Get(key []byte) ([]byte, bool, error) {
	// Look in the log, then descend the tree.
	ov, err := r.overlay()
	if err != nil {
		return nil, false, err
	}
	e, ok := ov.Get(oentry{key: key})
	v := e.val
	if !ok {
		if v, ok, err = findInTree(r.r, r.sb.root, key); err != nil || !ok {
			return nil, false, err
		}
	}
	if v.del {
		return nil, false, nil
	}

	// Read the value.
	data, err := v.read(r.r)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Stream calls fn with every key and its value in ascending key order,
// merging the checkpointed tree with the log after it and reading each
// referenced extent. The key and value last only through the call. Stream
// stops at the first error fn returns, or when ctx ends.
func (r *Reader) Stream(ctx context.Context, fn func(key, value []byte) error) error {
	// Replay the log, then walk the tree beside it.
	ov, err := r.overlay()
	if err != nil {
		return err
	}
	oit := ov.Iter()
	ook := oit.First()
	tw := newTreeWalk(r.r, r.sb.root)
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
	data, err := v.read(r.r)
	if err != nil {
		return err
	}
	return fn(key, data)
}

// Close releases the locks and the file. Closing twice does nothing.
func (r *Reader) Close() error {
	if r.release == nil {
		return nil
	}
	err := r.release()
	r.release, r.r, r.ov = nil, nil, nil
	return err
}
