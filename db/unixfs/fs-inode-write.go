package unixfs

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/util/csync"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// fsInodeIdleFlush is how long buffered writes wait for a contiguous write
// before the inode commits them.
const fsInodeIdleFlush = time.Second

// fsInodeWrites holds contiguous writes to a file inode until they reach the
// file's optimal write size, the writer pauses, or an operation needs them
// committed. Every FSHandle at the inode shares it, so a read through any
// handle sees the buffered bytes.
type fsInodeWrites struct {
	// mtx guards the fields below and is held across a flush, so a waiter
	// returns when its context ends.
	// Lock order: mtx before fsInode.rmtx.
	mtx csync.Mutex
	// off is the file offset of buf.
	off int64
	// buf is the extent not yet written. Empty when nothing is buffered.
	buf []byte
	// ts is the timestamp of the last buffered write.
	ts time.Time
	// limit is the extent size that commits buf.
	limit int64
	// timer commits buf after the writer pauses.
	timer *time.Timer
	// err is the first unreported flush error, returned by the next operation.
	err error
}

// fsTreeWrites tracks buffered writes and unreported flush failures in one tree.
// An inode stays pending from buffering through an asynchronous failure until
// an operation reports that failure. Update membership under w.mtx, retaining
// the entry until err is stored so a concurrent Sync snapshot cannot miss it.
// Release cannot report errors and therefore preserves them for other handles.
type fsTreeWrites struct {
	// mtx guards dirty. Never held while taking another lock.
	mtx sync.Mutex
	// dirty is the set of inodes with buffered writes or unreported failures.
	dirty map[*fsInode]struct{}
}

// writeAt buffers a write to the file at i, committing the buffer first when
// the write does not continue it. A write of at least the optimal write size,
// or to a file without one, is committed directly.
// caller must NOT hold rmtx
func (i *fsInode) writeAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Hold the buffer for the whole write.
	w := &i.w
	release, err := w.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Report an unreported flush failure before accepting more data.
	if err := i.takeErr(); err != nil {
		return err
	}

	// Commit the buffered extent when this write does not continue it.
	if len(w.buf) != 0 && offset != w.off+int64(len(w.buf)) {
		if err := i.flushLocked(ctx, false); err != nil {
			return err
		}
	}

	// Start a new extent sized to the file's optimal write size.
	if len(w.buf) == 0 {
		var limit int64
		err := i.accessInode(ctx, func(_ FSCursor, ops FSCursorOps) error {
			if !ops.GetIsFile() {
				return unixfs_errors.ErrNotFile
			}
			var err error
			limit, err = ops.GetOptimalWriteSize(ctx)
			return err
		})
		if err != nil {
			return err
		}
		if limit <= 0 || int64(len(data)) >= limit {
			return i.accessInode(ctx, func(_ FSCursor, ops FSCursorOps) error {
				return ops.WriteAt(ctx, offset, data, ts)
			})
		}
		w.off, w.limit = offset, limit
		w.buf = make([]byte, 0, limit)
		i.tree().mark(i, true)
	}

	// Append the write and commit a full extent.
	w.buf = append(w.buf, data...)
	w.ts = ts
	if int64(len(w.buf)) >= w.limit {
		return i.flushLocked(ctx, false)
	}

	// Commit the extent if the writer pauses.
	if w.timer == nil {
		w.timer = time.AfterFunc(fsInodeIdleFlush, i.flushUnreported)
	} else {
		w.timer.Reset(fsInodeIdleFlush)
	}
	return nil
}

// flushLocked commits the buffered extent. The extent is kept when ctx ends
// first, and dropped after any other result. If keepErr is set, the caller
// cannot report a failure, so retain the first one for the next operation.
// caller must hold w.mtx and must NOT hold rmtx
func (i *fsInode) flushLocked(ctx context.Context, keepErr bool) error {
	// Cancel the idle commit, which this flush replaces.
	w := &i.w
	if w.timer != nil {
		w.timer.Stop()
	}
	if len(w.buf) == 0 {
		return nil
	}

	// Write the extent through the current ops.
	err := i.accessInode(ctx, func(_ FSCursor, ops FSCursorOps) error {
		return ops.WriteAt(ctx, w.off, w.buf, w.ts)
	})
	if err != nil && ctx.Err() != nil {
		return err
	}

	// Keep an unreported failure pending before clearing the extent's entry.
	w.buf = nil
	if keepErr && err != nil && w.err == nil {
		w.err = err
	}
	i.tree().mark(i, w.err != nil)
	return err
}

// flushUnreported commits on the idle timer or Release, keeping any error for
// the next operation because these callers cannot report it.
func (i *fsInode) flushUnreported() {
	// Serialize the flush with operations that can report its failure.
	w := &i.w
	release, err := w.mtx.Lock(context.Background())
	if err != nil {
		return
	}
	defer release()
	_ = i.flushLocked(context.Background(), true)
}

// sync commits the buffered writes at i and returns any unreported flush error.
// caller must NOT hold rmtx
func (i *fsInode) sync(ctx context.Context) error {
	// Commit the extent, then report an earlier unreported failure.
	w := &i.w
	release, err := w.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := i.flushLocked(ctx, false); err != nil {
		return err
	}
	return i.takeErr()
}

// buffered returns the end offset and timestamp of the buffered writes, or a
// zero end when nothing is buffered, and any unreported flush error.
func (i *fsInode) buffered(ctx context.Context) (int64, time.Time, error) {
	// Report an earlier unreported flush failure first.
	w := &i.w
	release, err := w.mtx.Lock(ctx)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer release()
	if err := i.takeErr(); err != nil {
		return 0, time.Time{}, err
	}

	// Report the extent's end and timestamp.
	if len(w.buf) == 0 {
		return 0, time.Time{}, nil
	}
	return w.off + int64(len(w.buf)), w.ts, nil
}

// takeErr returns and clears the unreported flush error and its pending entry.
// caller must hold w.mtx
func (i *fsInode) takeErr() error {
	// Remove only a failure that this operation reports, keeping buffered data.
	w := &i.w
	err := w.err
	if err != nil {
		w.err = nil
		i.tree().mark(i, len(w.buf) != 0)
	}
	return err
}

// syncTree commits buffered writes and reports pending failures in i's tree.
// caller must NOT hold rmtx on any inode
func (i *fsInode) syncTree(ctx context.Context) error {
	var firstErr error
	for _, node := range i.tree().snapshot() {
		if err := node.sync(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// tree returns the write tracking of the tree holding i, kept on the root.
func (i *fsInode) tree() *fsTreeWrites {
	root := i
	for root.parent != nil {
		root = root.parent
	}
	return &root.treeWrites
}

// mark adds or removes an inode from the pending set.
func (t *fsTreeWrites) mark(i *fsInode, dirty bool) {
	// Remove an inode whose writes and errors have been handled.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if !dirty {
		delete(t.dirty, i)
		return
	}

	// Add an inode with buffered writes or an unreported failure.
	if t.dirty == nil {
		t.dirty = make(map[*fsInode]struct{})
	}
	t.dirty[i] = struct{}{}
}

// snapshot returns the inodes with buffered writes or unreported failures.
func (t *fsTreeWrites) snapshot() []*fsInode {
	// Copy the set so callers sync without holding mtx.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	nodes := make([]*fsInode, 0, len(t.dirty))
	for node := range t.dirty {
		nodes = append(nodes, node)
	}
	return nodes
}
