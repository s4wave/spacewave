package unixfs

import (
	"context"
	"sync"
	"time"

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
	// mtx guards the fields below and is held across a flush.
	// Lock order: mtx before fsInode.rmtx.
	mtx sync.Mutex
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
	// err is the error of an idle flush, returned by the next operation.
	err error
}

// fsTreeWrites tracks the inodes of one tree that hold buffered writes.
type fsTreeWrites struct {
	// mtx guards dirty. Never held while taking another lock.
	mtx sync.Mutex
	// dirty is the set of inodes with buffered writes.
	dirty map[*fsInode]struct{}
}

// writeAt buffers a write to the file at i, committing the buffer first when
// the write does not continue it. A write of at least the optimal write size,
// or to a file without one, is committed directly.
// caller must NOT hold rmtx
func (i *fsInode) writeAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Hold the buffer for the whole write.
	w := &i.w
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Report an idle flush failure before accepting more data.
	if err := w.takeErr(); err != nil {
		return err
	}

	// Commit the buffered extent when this write does not continue it.
	if len(w.buf) != 0 && offset != w.off+int64(len(w.buf)) {
		if err := i.flushLocked(ctx); err != nil {
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
		return i.flushLocked(ctx)
	}

	// Commit the extent if the writer pauses.
	if w.timer == nil {
		w.timer = time.AfterFunc(fsInodeIdleFlush, i.idleFlush)
	} else {
		w.timer.Reset(fsInodeIdleFlush)
	}
	return nil
}

// flushLocked commits the buffered extent. The extent is kept when ctx ends
// first, and dropped after any other result.
// caller must hold w.mtx and must NOT hold rmtx
func (i *fsInode) flushLocked(ctx context.Context) error {
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

	// Clear the extent, written or failed.
	w.buf = nil
	i.tree().mark(i, false)
	return err
}

// idleFlush commits the buffered extent after the writer paused, keeping any
// error for the next operation.
func (i *fsInode) idleFlush() {
	// Commit under the buffer lock and keep the first failure.
	w := &i.w
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if err := i.flushLocked(context.Background()); err != nil && w.err == nil {
		w.err = err
	}
}

// sync commits the buffered writes at i and returns any idle flush error.
// caller must NOT hold rmtx
func (i *fsInode) sync(ctx context.Context) error {
	// Commit the extent, then report an earlier idle flush failure.
	w := &i.w
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if err := i.flushLocked(ctx); err != nil {
		return err
	}
	return w.takeErr()
}

// buffered returns the end offset and timestamp of the buffered writes, or a
// zero end when nothing is buffered, and any idle flush error.
func (i *fsInode) buffered() (int64, time.Time, error) {
	// Report an earlier idle flush failure first.
	w := &i.w
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if err := w.takeErr(); err != nil {
		return 0, time.Time{}, err
	}

	// Report the extent's end and timestamp.
	if len(w.buf) == 0 {
		return 0, time.Time{}, nil
	}
	return w.off + int64(len(w.buf)), w.ts, nil
}

// takeErr returns and clears the idle flush error.
// caller must hold mtx
func (w *fsInodeWrites) takeErr() error {
	err := w.err
	w.err = nil
	return err
}

// syncTree commits the buffered writes of every inode in the tree holding i.
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

// mark adds or removes an inode from the dirty set.
func (t *fsTreeWrites) mark(i *fsInode, dirty bool) {
	// Remove a committed inode.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if !dirty {
		delete(t.dirty, i)
		return
	}

	// Add an inode that buffered a write.
	if t.dirty == nil {
		t.dirty = make(map[*fsInode]struct{})
	}
	t.dirty[i] = struct{}{}
}

// snapshot returns the inodes with buffered writes.
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
