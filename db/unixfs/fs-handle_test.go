package unixfs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
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

// fakeCursor serves one shared ops object.
type fakeCursor struct {
	mtx sync.Mutex
	ops *fakeOps
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
func (c *fakeCursor) setOps(ops *fakeOps) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.ops = ops
}

// Release is a no-op.
func (c *fakeCursor) Release() {}

// TestRenameResolvesReleasedDestOps tests that Rename re-resolves the
// destination operations when they report released, instead of proceeding
// with the released object.
func TestRenameResolvesReleasedDestOps(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()

	srcCursor := &fakeCursor{ops: &fakeOps{}}
	destCursor := &fakeCursor{ops: &fakeOps{}}

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
