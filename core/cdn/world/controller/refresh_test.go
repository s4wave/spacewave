package cdn_world_controller

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/sirupsen/logrus"
)

// TestRefreshRPCRefetchesMountedWorld proves invalidation reaches the mounted
// CDN owner without replacing its engine or fetching unrelated Spaces.
func TestRefreshRPCRefetchesMountedWorld(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Build a root pointer for a world with an empty head.
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId:    "release-space",
		Checkpoint: testHeadCheckpoint(t, "release-space"),
	})

	// Serve the pointer and signal each fetch after the first.
	var requests atomic.Int32
	refetched := make(chan struct{}, 1)
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) > 1 {
			select {
			case refetched <- struct{}{}:
			default:
			}
		}
		_, _ = w.Write(pointer)
	}))

	// Mount the world.
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("release", "release-space", cdnURL))
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()
	engine, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// A refresh for another Space is refused without a fetch.
	client := NewSRPCWorldRefreshClientWithServiceID(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(ctrl))), WorldRefreshServiceID("release"))
	response, err := client.Refresh(ctx, &RefreshRequest{SpaceId: "unrelated"})
	if err != nil || response.GetAccepted() || requests.Load() != 1 {
		t.Fatalf("unrelated refresh: %v %v requests=%d", response, err, requests.Load())
	}

	// A refresh for the mounted Space refetches the pointer.
	response, err = client.Refresh(ctx, &RefreshRequest{SpaceId: "release-space"})
	if err != nil || !response.GetAccepted() {
		t.Fatalf("refresh: %v %v", response, err)
	}
	select {
	case <-refetched:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// The mounted engine is kept.
	current, err := ctrl.GetWorldEngine(ctx)
	if err != nil || current != engine {
		t.Fatal("refresh replaced the mounted engine")
	}
}

// TestMountRetriesOneFailedPointerFetch proves a single failed pointer fetch
// is retried before the Space is reported unreachable, so one dropped request
// does not start the cached release.
func TestMountRetriesOneFailedPointerFetch(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Fail the first pointer fetch, then serve a world with an empty head.
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId:    "release-space",
		Checkpoint: testHeadCheckpoint(t, "release-space"),
	})
	var requests atomic.Int32
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "dropped", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(pointer)
	}))

	// Mount the world.
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("release", "release-space", cdnURL))
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()

	// The mount publishes the engine without first reporting the Space
	// unreachable.
	errUnreachable := errors.New("space reported unreachable")
	_, err := ctrl.ctr.WaitValueWithValidator(ctx, func(state *mountState) (bool, error) {
		if state != nil && state.err != nil {
			return false, errUnreachable
		}
		return state != nil && state.engine != nil, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := requests.Load(); n != 2 {
		t.Fatalf("pointer fetched %d times, want 2", n)
	}
}

// TestMissingHeadMountsOnRefreshRPC proves a mount whose Space has no published
// head waits on the store's root pointer instead of a timer: a Refresh RPC
// accepted while the head is missing fetches the published root and the mount
// builds its engine without waiting the retry delay.
func TestMissingHeadMountsOnRefreshRPC(t *testing.T) {
	// Bound the checks well below the old one-second retry delay.
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer waitCancel()

	// Bound the whole test against a stuck Execute.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Keep the first two pointer reads headless; the explicit refresh publishes a head.
	var requests atomic.Int32
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId:    "release-space",
		Checkpoint: testHeadCheckpoint(t, "release-space"),
	})
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The mount's first fetch and the engine build's own fetch confirm no
		// head is published before the RPC's fetch publishes one.
		if requests.Add(1) < 3 {
			http.Error(w, "no published head", http.StatusNotFound)
			return
		}
		_, _ = w.Write(pointer)
	}))

	// Mount the world.
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("release", "release-space", cdnURL))
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()

	// Wait for the mount to report the missing head.
	if _, err := ctrl.ctr.WaitValueWithValidator(waitCtx, func(state *mountState) (bool, error) {
		return state != nil && isMissingPublishedHead(state.err), nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	// A refresh RPC on the headless mount is accepted.
	client := NewSRPCWorldRefreshClientWithServiceID(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(ctrl))), WorldRefreshServiceID("release"))
	response, err := client.Refresh(waitCtx, &RefreshRequest{SpaceId: "release-space"})
	if err != nil || !response.GetAccepted() {
		t.Fatalf("refresh on missing head: %v %v", response, err)
	}

	// The new root pointer mounts the engine on the test's deadline.
	readyCtx, readyCancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer readyCancel()
	if engine, err := ctrl.GetWorldEngine(readyCtx); err != nil || engine == nil {
		t.Fatalf("engine did not mount on refresh: %v %v", engine, err)
	}
	if n := requests.Load(); n != 3 {
		t.Fatalf("pointer fetched %d times, want the refresh fetch as the third", n)
	}
}

// TestMissingHeadMountCancels proves the headless mount stops waiting on
// cancellation instead of being left running: Execute returns nil quickly
// once its context is canceled.
func TestMissingHeadMountCancels(t *testing.T) {
	// Serve no root pointer so the mount stays headless.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no published head", http.StatusNotFound)
	}))

	// Mount the world.
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("release", "release-space", cdnURL))
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()

	// Wait for the mount to report the missing head.
	if _, err := ctrl.ctr.WaitValueWithValidator(ctx, func(state *mountState) (bool, error) {
		return state != nil && isMissingPublishedHead(state.err), nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	// The mount exits on cancel.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("headless mount did not stop on cancel")
	}
}
