package unixfs_access_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

func TestRotatingAccessRebindsBlockedAccess(t *testing.T) {
	// Create a rotating UnixFS provider that initially blocks access.
	ctx := t.Context()
	access := unixfs_access.NewRotatingAccess()

	// Create the first UnixFS root with its expected file body.
	firstBody := []byte("first generation")
	firstHandle, err := newTestRotatingAccessRoot(ctx, firstBody)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer firstHandle.Release()

	// Bound the first UnixFS access attempt and release its context afterward.
	firstAccessCtx, firstCancel := context.WithTimeout(ctx, 5*time.Second)
	defer firstCancel()

	// Start the first access attempt while the UnixFS provider is blocked.
	firstStarted := make(chan struct{})
	firstResult := make(chan rotatingAccessResult, 1)
	go accessRotatingHandleWithRebind(firstAccessCtx, access, firstStarted, firstResult)

	// Wait until the first attempt reaches the blocked UnixFS provider.
	select {
	case <-firstStarted:
	case <-firstAccessCtx.Done():
		t.Fatalf("blocked access did not start: %v", firstAccessCtx.Err())
	}

	// Publish the first UnixFS provider to release the blocked attempt.
	access.SetCurrent(unixfs_access.NewAccessUnixFSFunc(firstHandle))

	// Verify the first access attempt reads the published UnixFS root.
	first := waitRotatingAccessResult(t, firstAccessCtx, firstResult)
	defer first.release()
	assertRotatingAccessBody(t, firstAccessCtx, first.handle, firstBody)

	// Create a replacement UnixFS root with a distinct file body.
	secondBody := []byte("second generation")
	secondHandle, err := newTestRotatingAccessRoot(ctx, secondBody)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer secondHandle.Release()

	// Bound the replacement UnixFS access attempt.
	secondAccessCtx, secondCancel := context.WithTimeout(ctx, 5*time.Second)
	defer secondCancel()

	// Block the UnixFS provider again and start the replacement attempt.
	secondStarted := make(chan struct{})
	secondResult := make(chan rotatingAccessResult, 1)
	access.SetBlocked()
	go accessRotatingHandleWithRebind(secondAccessCtx, access, secondStarted, secondResult)

	// Wait until the replacement attempt reaches the blocked provider.
	select {
	case <-secondStarted:
	case <-secondAccessCtx.Done():
		t.Fatalf("blocked replacement access did not start: %v", secondAccessCtx.Err())
	}

	// Publish the replacement UnixFS provider.
	access.SetCurrent(unixfs_access.NewAccessUnixFSFunc(secondHandle))

	// Verify the replacement attempt reads the new UnixFS root.
	second := waitRotatingAccessResult(t, secondAccessCtx, secondResult)
	defer second.release()
	assertRotatingAccessBody(t, secondAccessCtx, second.handle, secondBody)
}

type rotatingAccessResult struct {
	handle  *unixfs.FSHandle
	release func()
	err     error
}

type signalContext struct {
	context.Context

	started func()
}

func (s *signalContext) Done() <-chan struct{} {
	s.started()
	return s.Context.Done()
}

func accessRotatingHandleWithRebind(
	ctx context.Context,
	access *unixfs_access.RotatingAccess,
	started chan<- struct{},
	result chan<- rotatingAccessResult,
) {
	var startedOnce sync.Once
	closeStarted := func() {
		startedOnce.Do(func() {
			close(started)
		})
	}

	for {
		resolveCtx, resolveCancel := context.WithCancel(ctx)
		accessCtx := resolveCtx
		if started != nil {
			accessCtx = &signalContext{
				Context: resolveCtx,
				started: closeStarted,
			}
		}

		var releasedOnce sync.Once
		releasedCh := make(chan struct{})
		released := func() {
			releasedOnce.Do(func() {
				resolveCancel()
				close(releasedCh)
			})
		}

		handle, release, err := access.AccessUnixFS(accessCtx, released)
		resolveCancel()
		closeStarted()
		if err != nil {
			select {
			case <-releasedCh:
				started = nil
				continue
			default:
			}
		}
		result <- rotatingAccessResult{
			handle:  handle,
			release: release,
			err:     err,
		}
		return
	}
}

func waitRotatingAccessResult(
	t *testing.T,
	ctx context.Context,
	result <-chan rotatingAccessResult,
) rotatingAccessResult {
	t.Helper()

	select {
	case res := <-result:
		if res.err != nil {
			t.Fatal(res.err.Error())
		}
		if res.handle == nil {
			t.Fatal("expected handle")
		}
		if res.release == nil {
			t.Fatal("expected release function")
		}
		return res
	case <-ctx.Done():
		t.Fatalf("access did not complete after provider rotation: %v", ctx.Err())
	}
	return rotatingAccessResult{}
}

func assertRotatingAccessBody(
	t *testing.T,
	ctx context.Context,
	handle *unixfs.FSHandle,
	want []byte,
) {
	// Open the asset file from the resolved UnixFS handle for the body assertion.
	t.Helper()
	fileHandle, _, err := handle.LookupPath(ctx, "/asset.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()

	// Read the resolved asset file body.
	got, err := unixfs.ReadFile(ctx, fileHandle)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the asset file body matches the expected provider.
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected body: %q", string(got))
	}
}

func newTestRotatingAccessRoot(ctx context.Context, body []byte) (*unixfs.FSHandle, error) {
	// Create an in-memory UnixFS root for the provider.
	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		return nil, err
	}

	// Write the expected asset body into the UnixFS root.
	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	if err := billy_util.WriteFile(rbfs, "/asset.txt", body, 0o644); err != nil {
		rootRef.Release()
		return nil, err
	}
	return rootRef, nil
}

func TestFSCursorRetriesRotatedAccess(t *testing.T) {
	// Create a rotating provider and signal when the cursor blocks on it.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	rotating := unixfs_access.NewRotatingAccess()
	started := make(chan struct{})
	var startedOnce sync.Once
	cursor := unixfs_access.NewFSCursor(func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		startedOnce.Do(func() { close(started) })
		return rotating.AccessUnixFS(ctx, released)
	})

	// Resolve the cursor while the provider is blocked.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()
	errCh := make(chan error, 1)
	go func() {
		fileHandle, _, err := handle.LookupPath(ctx, "/asset.txt")
		if err == nil {
			fileHandle.Release()
		}
		errCh <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("access did not start: %v", ctx.Err())
	}

	// Rotate to a second blocked provider, then publish the UnixFS root.
	rotating.SetBlocked()
	root, err := newTestRotatingAccessRoot(ctx, []byte("rotated generation"))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer root.Release()
	rotating.SetCurrent(unixfs_access.NewAccessUnixFSFunc(root))

	// Verify the pending lookup resolves against the published root.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-ctx.Done():
		t.Fatalf("lookup did not complete after provider rotation: %v", ctx.Err())
	}
}
