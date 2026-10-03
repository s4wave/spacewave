package unixfs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// fakeOps is a stub ops object whose released state is controlled by the
// test. Only the members Rename touches are implemented; everything else
// stays nil and is never reached before the cross-location error.
type fakeOps struct {
	FSCursorOps
	released atomic.Bool
}

// CheckReleased reports the injected released state.
func (f *fakeOps) CheckReleased() bool { return f.released.Load() }

// MoveTo reports that no optimized move was performed.
func (f *fakeOps) MoveTo(ctx context.Context, dest FSCursorOps, destName string, ts time.Time) (bool, error) {
	return false, nil
}

// MoveFrom reports that no optimized move was performed.
func (f *fakeOps) MoveFrom(ctx context.Context, destName string, src FSCursorOps, ts time.Time) (bool, error) {
	return false, nil
}

// fakeFileOps is an in-memory file that records each committed write.
type fakeFileOps struct {
	fakeOps
	mtx    sync.Mutex
	data   []byte
	writes int
	err    error
}

// GetIsFile reports a file.
func (f *fakeFileOps) GetIsFile() bool { return true }

// GetOptimalWriteSize returns a small extent so tests can fill it.
func (f *fakeFileOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 16, nil
}

// GetSize returns the committed size.
func (f *fakeFileOps) GetSize(ctx context.Context) (uint64, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return uint64(len(f.data)), nil
}

// ReadAt reads committed bytes.
func (f *fakeFileOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return int64(copy(data, f.data[offset:])), nil
}

// WriteAt commits a write, or returns the injected error.
func (f *fakeFileOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Fail with the injected error.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.err != nil {
		return f.err
	}

	// Grow the file to cover the write and copy it in.
	if end := int(offset) + len(data); end > len(f.data) {
		f.data = append(f.data, make([]byte, end-len(f.data))...)
	}
	copy(f.data[offset:], data)
	f.writes++
	return nil
}

// committed returns the committed bytes and write count.
func (f *fakeFileOps) committed() (string, int) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return string(f.data), f.writes
}

// fakeCursor serves one shared ops object.
type fakeCursor struct {
	mtx      sync.Mutex
	ops      FSCursorOps
	released atomic.Bool
}

// CheckReleased reports the cursor as live.
func (c *fakeCursor) CheckReleased() bool { return false }

// GetProxyCursor reports no redirection.
func (c *fakeCursor) GetProxyCursor(ctx context.Context) (FSCursor, error) {
	return nil, nil
}

// AddChangeCb ignores change callbacks.
func (c *fakeCursor) AddChangeCb(cb FSCursorChangeCb) {}

// GetCursorOps returns the cursor's ops object.
func (c *fakeCursor) GetCursorOps(ctx context.Context) (FSCursorOps, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.ops, nil
}

// setOps swaps the served ops object.
func (c *fakeCursor) setOps(ops FSCursorOps) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.ops = ops
}

// Release records that the cursor was released.
func (c *fakeCursor) Release() { c.released.Store(true) }

// TestRenameResolvesReleasedDestOps tests that Rename re-resolves the
// destination operations when they report released, instead of proceeding
// with the released object.
func TestRenameResolvesReleasedDestOps(t *testing.T) {
	// Bound the rename test and release its context afterward.
	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()

	// Provide independent source and destination cursor operations.
	srcCursor := &fakeCursor{ops: &fakeOps{}}
	destCursor := &fakeCursor{ops: &fakeOps{}}

	// Open the source and destination handles for the rename.
	srcHandle, err := NewFSHandle(srcCursor)
	if err != nil {
		t.Fatal(err)
	}
	defer srcHandle.Release()
	destHandle, err := NewFSHandle(destCursor)
	if err != nil {
		t.Fatal(err)
	}
	defer destHandle.Release()

	// Replace the destination inode's ops with a released stub.
	destInode := destHandle.i()
	rel, err := destInode.rmtx.Lock(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	destInode.fsOps = &fakeOps{released: atomic.Bool{}}
	destInode.fsOps.(*fakeOps).released.Store(true)
	rel()

	// Mark the stub healthy again only after Rename has re-resolved, by
	// serving healthy ops from the cursor: the resolver replaces the
	// stub through GetCursorOps, so flipping the cursor's ops to healthy
	// lets the retry succeed.
	go func() {
		time.Sleep(50 * time.Millisecond)
		destCursor.setOps(&fakeOps{})
	}()

	// Verify the rename reports the unsupported cross-location move.
	err = srcHandle.Rename(ctx, destHandle, "moved.txt", time.Now())
	if err == nil {
		t.Fatal("expected cross-location rename to be unsupported")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, unixfs_errors.ErrReleased) {
		t.Fatalf("rename did not re-resolve the released destination ops: %v", err)
	}
	if !errors.Is(err, unixfs_errors.ErrCrossFsRename) {
		t.Fatalf("unexpected rename error: %v", err)
	}
}

// TestReleaseAfterChildRelease tests that released descendant inodes do not
// keep their ancestors alive: releasing a deep child releases the
// intermediate inodes, and releasing the root handle then releases the root
// cursor.
func TestReleaseAfterChildRelease(t *testing.T) {
	// Open the root handle whose descendants will be released.
	cursor := &fakeCursor{ops: &fakeOps{}}
	rootHandle, err := NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}

	// Build a child inode chain beneath the live root handle.
	root := rootHandle.i()
	mid := newFsInode(root, "a", nil)
	root.children = []*fsInode{mid}
	leaf := newFsInode(mid, "b", nil)
	mid.children = []*fsInode{leaf}
	leafHandle, _ := leaf.addReferenceLocked(false)

	// Release the leaf and verify the intermediate inode follows it.
	leafHandle.Release()
	if !mid.checkReleased() {
		t.Fatal("intermediate inode survived its last child")
	}
	if root.checkReleased() {
		t.Fatal("root inode released while its handle is live")
	}

	// Release the root handle and verify its inode and cursor close.
	rootHandle.Release()
	if !root.checkReleased() {
		t.Fatal("root inode survived its last handle")
	}
	if !cursor.released.Load() {
		t.Fatal("root cursor was not released")
	}
}

// TestWriteBehindSharedByHandles tests that small contiguous writes stay
// buffered at the inode until an extent fills or Sync, and that every handle
// at the inode sees the buffered bytes.
func TestWriteBehindSharedByHandles(t *testing.T) {
	// Open two handles at one file.
	ctx := context.Background()
	ops := &fakeFileOps{}
	h, err := NewFSHandle(&fakeCursor{ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	other, err := h.Clone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()

	// Buffer two small writes without committing them.
	if err := h.WriteAt(ctx, 0, []byte("abc"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAt(ctx, 3, []byte("def"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, writes := ops.committed(); writes != 0 {
		t.Fatalf("committed %d writes before sync", writes)
	}

	// The second handle sees the buffered size without a commit.
	size, err := other.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 6 {
		t.Fatalf("size %d, want 6", size)
	}

	// Filling the extent commits it as one write.
	if err := h.WriteAt(ctx, 6, []byte("0123456789"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if data, writes := ops.committed(); writes != 1 || data != "abcdef0123456789" {
		t.Fatalf("committed %q in %d writes", data, writes)
	}

	// A read through the second handle commits and returns the next extent.
	if err := h.WriteAt(ctx, 16, []byte("xyz"), time.Now()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	if _, err := other.ReadAt(ctx, 16, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "xyz" {
		t.Fatalf("read %q, want xyz", buf)
	}
	if _, writes := ops.committed(); writes != 2 {
		t.Fatalf("committed %d writes, want 2", writes)
	}
}

// TestWriteBehindIdleFlushError tests that a buffered extent commits after
// the writer pauses, and that a failed commit is returned by the next
// operation and not again after.
func TestWriteBehindIdleFlushError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Open a file whose commits fail.
		ctx := t.Context()
		errCommit := errors.New("commit failed")
		ops := &fakeFileOps{err: errCommit}
		h, err := NewFSHandle(&fakeCursor{ops: ops})
		if err != nil {
			t.Fatal(err)
		}
		defer h.Release()

		// Buffer a write and let the writer pause.
		if err := h.WriteAt(ctx, 0, []byte("abc"), time.Now()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(fsInodeIdleFlush)
		synctest.Wait()

		// The next operation reports the failed commit once.
		if _, err := h.GetSize(ctx); !errors.Is(err, errCommit) {
			t.Fatalf("GetSize error %v, want %v", err, errCommit)
		}
		if err := h.Sync(ctx); err != nil {
			t.Fatalf("Sync reported the error again: %v", err)
		}
	})
}
