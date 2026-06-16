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
	ctx := t.Context()
	access := unixfs_access.NewRotatingAccess()

	firstBody := []byte("first generation")
	firstHandle, err := newTestRotatingAccessRoot(ctx, firstBody)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer firstHandle.Release()

	firstAccessCtx, firstCancel := context.WithTimeout(ctx, 5*time.Second)
	defer firstCancel()

	firstStarted := make(chan struct{})
	firstResult := make(chan rotatingAccessResult, 1)
	go accessRotatingHandleWithRebind(firstAccessCtx, access, firstStarted, firstResult)

	select {
	case <-firstStarted:
	case <-firstAccessCtx.Done():
		t.Fatalf("blocked access did not start: %v", firstAccessCtx.Err())
	}

	access.SetCurrent(unixfs_access.NewAccessUnixFSFunc(firstHandle))

	first := waitRotatingAccessResult(t, firstAccessCtx, firstResult)
	defer first.release()
	assertRotatingAccessBody(t, firstAccessCtx, first.handle, firstBody)

	secondBody := []byte("second generation")
	secondHandle, err := newTestRotatingAccessRoot(ctx, secondBody)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer secondHandle.Release()

	secondAccessCtx, secondCancel := context.WithTimeout(ctx, 5*time.Second)
	defer secondCancel()

	secondStarted := make(chan struct{})
	secondResult := make(chan rotatingAccessResult, 1)
	access.SetBlocked()
	go accessRotatingHandleWithRebind(secondAccessCtx, access, secondStarted, secondResult)

	select {
	case <-secondStarted:
	case <-secondAccessCtx.Done():
		t.Fatalf("blocked replacement access did not start: %v", secondAccessCtx.Err())
	}

	access.SetCurrent(unixfs_access.NewAccessUnixFSFunc(secondHandle))

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
	t.Helper()

	fileHandle, _, err := handle.LookupPath(ctx, "/asset.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()

	got, err := unixfs.ReadFile(ctx, fileHandle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected body: %q", string(got))
	}
}

func newTestRotatingAccessRoot(ctx context.Context, body []byte) (*unixfs.FSHandle, error) {
	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		return nil, err
	}
	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	if err := billy_util.WriteFile(rbfs, "/asset.txt", body, 0o644); err != nil {
		rootRef.Release()
		return nil, err
	}
	return rootRef, nil
}
