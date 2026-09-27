package electron

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/sirupsen/logrus"
)

// openRuntime supplies the desktop Resource contract to a controller test.
type openRuntime struct {
	web_runtime.WebRuntime
	// service serves the test Electron main Resource tree.
	service bldr_resource.SRPCResourceServiceClient
}

// waitingRuntime never becomes ready before its shell exits.
type waitingRuntime struct {
	*openRuntime
}

// WaitReady waits for the shell lifetime to end.
func (r *waitingRuntime) WaitReady(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// WaitReady reports that the test runtime is ready.
func (r *openRuntime) WaitReady(context.Context) error {
	return nil
}

// ConnectDesktopRuntimeResourceClient opens the test desktop Resource tree.
func (r *openRuntime) ConnectDesktopRuntimeResourceClient(ctx context.Context) (*resource_client.Client, error) {
	return resource_client.NewClient(ctx, r.service)
}

// openService records the existing Electron main acknowledgement call.
type openService struct {
	// calls counts acknowledged main-window requests.
	calls atomic.Int32
}

// WatchDesktopState is unused by the open request.
func (s *openService) WatchDesktopState(*desktop_runtime.WatchDesktopStateRequest, desktop_runtime.SRPCDesktopRuntimeResourceService_WatchDesktopStateStream) error {
	return nil
}

// SetDesktopState is unused by the open request.
func (s *openService) SetDesktopState(context.Context, *desktop_runtime.SetDesktopStateRequest) (*desktop_runtime.SetDesktopStateResponse, error) {
	return &desktop_runtime.SetDesktopStateResponse{}, nil
}

// OpenOrFocusMainWindow acknowledges one request.
func (s *openService) OpenOrFocusMainWindow(context.Context, *desktop_runtime.OpenOrFocusMainWindowRequest) (*desktop_runtime.OpenOrFocusMainWindowResponse, error) {
	s.calls.Add(1)
	return &desktop_runtime.OpenOrFocusMainWindowResponse{}, nil
}

// QuitDesktopRuntime is unused by the open request.
func (s *openService) QuitDesktopRuntime(context.Context, *desktop_runtime.QuitDesktopRuntimeRequest) (*desktop_runtime.QuitDesktopRuntimeResponse, error) {
	return &desktop_runtime.QuitDesktopRuntimeResponse{}, nil
}

// newOpenRuntime connects the test runtime through the real Resource server.
func newOpenRuntime(t *testing.T, desktop *openService) *openRuntime {
	t.Helper()
	rootMux := srpc.NewMux()
	if err := desktop_runtime.SRPCRegisterDesktopRuntimeResourceService(rootMux, desktop); err != nil {
		t.Fatal(err)
	}
	resourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(resourceMux); err != nil {
		t.Fatal(err)
	}
	service := bldr_resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(resourceMux))))
	return &openRuntime{service: service}
}

// TestControllerOpenDeduplicatesAndWarmReopens proves concurrent opens share one shell and a clean exit permits a new generation.
func TestControllerOpenDeduplicatesAndWarmReopens(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	desktop := &openService{}
	rt := newOpenRuntime(t, desktop)
	r, err := NewController(logrus.NewEntry(logrus.New()), nil, "", "", "", "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	exits := make(chan chan struct{}, 2)
	r.run = func(ctx context.Context) error {
		launches.Add(1)
		exit := make(chan struct{})
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.runtime = rt
			broadcast()
		})
		exits <- exit
		select {
		case <-exit:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	done := make(chan error, 1)
	go func() { done <- r.Execute(ctx) }()

	// Concurrent requests share the first process and both receive main's acknowledgement.
	type openResult struct {
		generation uint64
		err        error
	}
	results := make(chan openResult, 2)
	for range 2 {
		go func() {
			generation, err := r.OpenOrFocusMainWindow(ctx, "")
			results <- openResult{generation: generation, err: err}
		}()
	}
	var firstGeneration uint64
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if firstGeneration == 0 {
			firstGeneration = result.generation
		}
		if result.generation != firstGeneration {
			t.Fatalf("concurrent generation = %d, want %d", result.generation, firstGeneration)
		}
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("launches = %d, want 1", got)
	}
	if got := desktop.calls.Load(); got != 2 {
		t.Fatalf("open acknowledgements = %d, want 2", got)
	}

	// A clean shell exit completes the first generation's event.
	firstPresence := r.DesktopPresence(firstGeneration)
	if firstPresence == nil {
		t.Fatal("first shell is not active")
	}
	close(<-exits)
	ended, err := firstPresence.WaitValueWithValidator(ctx, func(state *bldr_web_plugin.WatchDesktopPresenceResponse) (bool, error) {
		return state.GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ended.GetError() != "" {
		t.Fatalf("clean shell exit = %q", ended.GetError())
	}

	// A warm reopen advances generation without reviving the first one.
	secondGeneration, err := r.OpenOrFocusMainWindow(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if secondGeneration != firstGeneration+1 {
		t.Fatalf("warm generation = %d, want %d", secondGeneration, firstGeneration+1)
	}
	if r.DesktopPresence(firstGeneration) != nil {
		t.Fatal("ended generation appears active after warm reopen")
	}
	if got := launches.Load(); got != 2 {
		t.Fatalf("warm launches = %d, want 2", got)
	}
	close(<-exits)
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("controller exit = %v, want cancellation", err)
	}
}

// TestControllerLaunchFailureReleasesDemand proves a failed launch completes its request and permits an explicit retry.
func TestControllerLaunchFailureReleasesDemand(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r, err := NewController(logrus.NewEntry(logrus.New()), nil, "", "", "", "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("shell could not start")
	r.run = func(context.Context) error { return want }
	done := make(chan error, 1)
	go func() { done <- r.Execute(ctx) }()
	if _, err := r.OpenOrFocusMainWindow(ctx, ""); !errors.Is(err, want) {
		t.Fatalf("first open = %v, want launch failure", err)
	}
	if _, err := r.OpenOrFocusMainWindow(ctx, ""); !errors.Is(err, want) {
		t.Fatalf("second open = %v, want fresh launch failure", err)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("controller exit = %v, want cancellation", err)
	}
}

// TestControllerCleanExitBeforeReadyFailsOpen rejects a shell that exits before acknowledging readiness.
func TestControllerCleanExitBeforeReadyFailsOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r, err := NewController(logrus.NewEntry(logrus.New()), nil, "", "", "", "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.run = func(context.Context) error { return nil }
	done := make(chan error, 1)
	go func() { done <- r.Execute(ctx) }()
	if _, err := r.OpenOrFocusMainWindow(ctx, ""); !errors.Is(err, errDesktopClosed) {
		t.Fatalf("open = %v, want closed before acknowledgement", err)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("controller exit = %v, want cancellation", err)
	}
}

// TestControllerExitCancelsOpenWaitingForReadiness unblocks readiness when Electron ends.
func TestControllerExitCancelsOpenWaitingForReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r, err := NewController(logrus.NewEntry(logrus.New()), nil, "", "", "", "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := &waitingRuntime{openRuntime: newOpenRuntime(t, &openService{})}
	exit := make(chan struct{})
	started := make(chan struct{})
	r.run = func(context.Context) error {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.runtime = rt
			broadcast()
		})
		close(started)
		<-exit
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- r.Execute(ctx) }()
	opened := make(chan error, 1)
	go func() {
		_, err := r.OpenOrFocusMainWindow(ctx, "")
		opened <- err
	}()
	<-started
	close(exit)
	if err := <-opened; !errors.Is(err, errDesktopClosed) {
		t.Fatalf("open = %v, want shell closed", err)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("controller exit = %v, want cancellation", err)
	}
}

// TestControllerProcessExitDetachesPrivateRuntime joins the process and detaches its private runtime.
func TestControllerProcessExitDetachesPrivateRuntime(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	baseline := len(b.GetControllers())
	path := filepath.Join(t.TempDir(), "exit-shell")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := NewController(le, b, path, t.TempDir(), "unused", "test-exit", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Process completion detaches the runtime even when its pipe never became ready.
	if err := r.runElectron(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(b.GetControllers()); got != baseline {
		t.Fatalf("controllers after shell exit = %d, want %d", got, baseline)
	}
}

// _ is a type assertion.
var _ desktop_runtime.SRPCDesktopRuntimeResourceServiceServer = (*openService)(nil)
