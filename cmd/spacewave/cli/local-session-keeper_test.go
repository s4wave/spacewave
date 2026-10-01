//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	provider "github.com/s4wave/spacewave/core/provider"
	session "github.com/s4wave/spacewave/core/session"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// testBindingSpace exposes the binding stream to the keeper through the
// production Space Resource client contract.
type testBindingSpace struct {
	s4wave_space.SRPCSpaceResourceServiceClient
	stream *testBindingStream
}

// WatchProcessBindings returns the test binding stream.
func (s *testBindingSpace) WatchProcessBindings(context.Context, *s4wave_space.WatchProcessBindingsRequest) (s4wave_space.SRPCSpaceResourceService_WatchProcessBindingsClient, error) {
	return s.stream, nil
}

// testBindingStream sends binding snapshots without a plugin runtime mount.
type testBindingStream struct {
	srpc.Stream
	ctx    context.Context
	states chan *s4wave_space.WatchProcessBindingsResponse
}

// Recv waits for the next binding snapshot or stream cancellation.
func (s *testBindingStream) Recv() (*s4wave_space.WatchProcessBindingsResponse, error) {
	select {
	case state := <-s.states:
		return state, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// RecvTo copies the next snapshot into the caller's response.
func (s *testBindingStream) RecvTo(resp *s4wave_space.WatchProcessBindingsResponse) error {
	next, err := s.Recv()
	if err == nil {
		*resp = *next
	}
	return err
}

// Close ends the test binding stream.
func (s *testBindingStream) Close() error { return nil }

// TestRetainApprovedForgeWorkerRuntimeReleasesOnDisableAndSessionEnd checks
// that the keeper's reference follows approved, disabled, and deleted bindings
// through the Session lifetime.
func TestRetainApprovedForgeWorkerRuntimeReleasesOnDisableAndSessionEnd(t *testing.T) {
	// Create the cancelable context, binding stream, and Space stub.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream := &testBindingStream{
		ctx: ctx, states: make(chan *s4wave_space.WatchProcessBindingsResponse),
	}
	space := &testBindingSpace{stream: stream}

	// Create the event channel and run the keeper in the background.
	events := make(chan string, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		retainApprovedForgeWorkerRuntime(ctx, logrus.NewEntry(logrus.New()), "space/test", space, func() (func(), error) {
			events <- "mounted"
			return func() { events <- "released" }, nil
		})
	}()

	// Send one binding state and expect the given event sequence.
	set := func(approved bool) {
		t.Helper()
		stream.states <- &s4wave_space.WatchProcessBindingsResponse{
			ProcessBindings: []*s4wave_space.ProcessBindingInfo{{
				TypeId: forge_worker.WorkerTypeID, Approved: approved,
			}},
		}
	}

	// Expect the next keeper event within the timeout.
	want := func(expected string) {
		t.Helper()
		select {
		case got := <-events:
			if got != expected {
				t.Fatalf("keeper event = %q, want %q", got, expected)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("keeper did not report %q", expected)
		}
	}

	// Approve the binding, disable it, then approve it again.
	set(true)
	want("mounted")
	set(false)
	want("released")
	set(true)
	want("mounted")

	// Delete the binding and confirm the contents mount is released.
	stream.states <- &s4wave_space.WatchProcessBindingsResponse{}
	want("released")
	set(true)
	want("mounted")
	cancel()
	want("released")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("keeper did not stop with its Session")
	}
}

// TestReconcileLocalSpaceWatchesRetriesStoppedMountOnRevision checks that a
// failed initial Space mount does not hide an approved binding after readiness
// produces another Session resource-list snapshot.
func TestReconcileLocalSpaceWatchesRetriesStoppedMountOnRevision(t *testing.T) {
	// Reconcile the Space watches across snapshots.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	spaces := make(map[string]*keeperSpaceWatch)
	starts := 0

	// Track the started Space watches and their retries.
	start := func(spaceID string) *keeperSpaceWatch {
		// Create the watch context and done channel for this attempt.
		if spaceID != "space/test" {
			t.Fatalf("started Space %q", spaceID)
		}
		starts++

		// Create the watch context and done channel for this attempt.
		watchCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		if starts == 1 {
			// The first mount failed before a binding watch could start.
			close(done)
		} else {
			go func() {
				<-watchCtx.Done()
				close(done)
			}()
		}
		return &keeperSpaceWatch{cancel: stop, done: done}
	}
	reconcileLocalSpaceWatches(spaces, []string{"space/test"}, start)
	if starts != 1 {
		t.Fatalf("initial Space mount attempts = %d", starts)
	}
	reconcileLocalSpaceWatches(spaces, []string{"space/test"}, start)
	if starts != 2 {
		t.Fatalf("mount attempts after resource revision = %d, want 2", starts)
	}
	reconcileLocalSpaceWatches(spaces, nil, start)
	if len(spaces) != 0 {
		t.Fatalf("removed Spaces retained %d watches", len(spaces))
	}
}

// TestReconcileLocalSpaceWatchesRetriesFailedContentsMount checks that a failed
// approved runtime mount ends its binding watch and a Session revision retries it.
func TestReconcileLocalSpaceWatchesRetriesFailedContentsMount(t *testing.T) {
	// Start a binding watch whose first approved contents mount fails.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	spaces := make(map[string]*keeperSpaceWatch)
	streams := make(chan *testBindingStream, 2)
	events := make(chan string, 2)
	starts := 0

	// Track the started Space watches and their retries.
	start := func(spaceID string) *keeperSpaceWatch {
		// Fail the first attempt so the retry path can be exercised.
		if spaceID != "space/test" {
			t.Fatalf("started Space %q", spaceID)
		}
		starts++
		fail := starts == 1

		// Create the watch context and stream for this attempt.
		watchCtx, stop := context.WithCancel(ctx)
		stream := &testBindingStream{
			ctx: watchCtx, states: make(chan *s4wave_space.WatchProcessBindingsResponse),
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			retainApprovedForgeWorkerRuntime(watchCtx, logrus.NewEntry(logrus.New()), spaceID,
				&testBindingSpace{stream: stream}, func() (func(), error) {
					if fail {
						return nil, errors.New("contents unavailable")
					}
					events <- "mounted"
					return func() { events <- "released" }, nil
				})
		}()
		streams <- stream
		return &keeperSpaceWatch{cancel: stop, done: done}
	}
	approved := &s4wave_space.WatchProcessBindingsResponse{
		ProcessBindings: []*s4wave_space.ProcessBindingInfo{{
			TypeId: forge_worker.WorkerTypeID, Approved: true,
		}},
	}

	// A failed contents mount must end the watch instead of leaving it idle.
	reconcileLocalSpaceWatches(spaces, []string{"space/test"}, start)
	(<-streams).states <- approved
	select {
	case <-spaces["space/test"].done:
	case <-time.After(5 * time.Second):
		t.Fatal("failed contents mount left an idle binding watcher")
	}

	// The next Session resource revision retries the approved binding.
	reconcileLocalSpaceWatches(spaces, []string{"space/test"}, start)
	if starts != 2 {
		t.Fatalf("mount attempts after Session revision = %d, want 2", starts)
	}
	(<-streams).states <- approved
	select {
	case got := <-events:
		if got != "mounted" {
			t.Fatalf("keeper event = %q, want mounted", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Session revision did not retain the approved runtime")
	}

	// Removing the Space releases the retained runtime.
	reconcileLocalSpaceWatches(spaces, nil, start)
	select {
	case got := <-events:
		if got != "released" {
			t.Fatalf("keeper event = %q, want released", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Session removal did not release the runtime")
	}
}

// testLocalSessionMount records release of a retained session resource.
type testLocalSessionMount struct {
	released bool
}

// Release records the keeper's session cleanup.
func (m *testLocalSessionMount) Release() {
	m.released = true
}

// TestReconcileLocalSessionMountsRetainsConfiguredSession checks mount reuse
// across snapshots and release after removal.
func TestReconcileLocalSessionMountsRetainsConfiguredSession(t *testing.T) {
	// Build the session entry, mount stub, and mount counter.
	entry := &session.SessionListEntry{
		SessionIndex: 1,
		SessionRef: &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId: "local",
		}},
	}
	mount := &testLocalSessionMount{}
	mounted := make(map[uint32]localSessionMount)
	mountCalls := 0
	mountFunc := func(index uint32) (localSessionMount, error) {
		mountCalls++
		if index != 1 {
			t.Fatalf("mount index = %d, want 1", index)
		}
		return mount, nil
	}
	le := logrus.NewEntry(logrus.New())

	// Reconcile the configured session and confirm the mount is reused.
	reconcileLocalSessionMounts(le, []*session.SessionListEntry{entry}, mounted, mountFunc)
	reconcileLocalSessionMounts(le, []*session.SessionListEntry{entry}, mounted, mountFunc)
	if mountCalls != 1 || mount.released {
		t.Fatalf("configured session calls=%d released=%v", mountCalls, mount.released)
	}

	// Reconcile an empty session list and confirm the mount is released.
	reconcileLocalSessionMounts(le, nil, mounted, mountFunc)
	if !mount.released || len(mounted) != 0 {
		t.Fatalf("removed session released=%v mounted=%d", mount.released, len(mounted))
	}
}

// TestReconcileDeviceEnrollmentRestoresAndReleasesLocalSession covers the
// persisted local completion across session snapshots and keeper shutdown.
func TestReconcileDeviceEnrollmentRestoresAndReleasesLocalSession(t *testing.T) {
	// Seed the device setup record and stub the local mount.
	statePath := t.TempDir()
	record := &deviceSetupRecord{
		SetupState: deviceSetupStateSessionReady, Completion: deviceLocalCompletionPrefix + "stub",
		SessionIndex: 3, DeviceObjectKey: "devices/key",
	}
	if err := writeDeviceSetupRecord(statePath, record); err != nil {
		t.Fatal(err)
	}
	oldMount := deviceMountLocalSession
	deviceMountLocalSession = func(_ context.Context, _ *sdkClient, _ string, current *deviceSetupRecord) (*deviceSetupRecord, error) {
		return current, nil
	}
	t.Cleanup(func() { deviceMountLocalSession = oldMount })

	// Create the local session mount stub and mount counter.
	mount := &testLocalSessionMount{}
	mountCalls := 0

	// Mount the configured session index and count the calls.
	mountFunc := func(index uint32) (localSessionMount, error) {
		mountCalls++
		if index != 3 {
			t.Fatalf("mount index = %d, want 3", index)
		}
		return mount, nil
	}

	// Reconcile the configured session across snapshots.
	entries := []*session.SessionListEntry{{SessionIndex: 3}}
	le := logrus.NewEntry(logrus.New())
	var cleanup func()
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, entries, mountFunc, &cleanup)
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, entries, mountFunc, &cleanup)
	if cleanup == nil || mountCalls != 1 || mount.released {
		t.Fatalf("retained enrollment: cleanup=%v mounts=%d released=%v", cleanup != nil, mountCalls, mount.released)
	}
	cleanup()
	if !mount.released {
		t.Fatal("daemon shutdown did not release Device session")
	}
}

// TestReconcileDeviceEnrollmentRetriesPendingProjection checks that a failed
// World write remains pending until a later eligible session-list revision.
func TestReconcileDeviceEnrollmentRetriesPendingProjection(t *testing.T) {
	// Seed the pending device setup record and stub the projection.
	statePath := t.TempDir()
	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{
		SetupState: deviceSetupStateImported, SessionIndex: 3,
	}); err != nil {
		t.Fatal(err)
	}
	attempts := 0

	// Stub the Device object upsert to fail on the first attempt.
	withDeviceObjectUpsertStub(t, func(context.Context, *sdkClient, string, *deviceSetupRecord) (string, error) {
		attempts++
		if attempts == 1 {
			return "", errors.New("base World root is stale")
		}
		return "devices/key", nil
	})
	mount := func(uint32) (localSessionMount, error) {
		t.Fatal("linked Device should not restore local enrollment")
		return nil, nil
	}
	le := logrus.NewEntry(logrus.New())
	var cleanup func()
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, []*session.SessionListEntry{{SessionIndex: 4}}, mount, &cleanup)
	if attempts != 0 {
		t.Fatalf("projection before eligible revision: %d attempts", attempts)
	}

	// Seed the pending device setup record and stub the projection.
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, []*session.SessionListEntry{{SessionIndex: 3}}, mount, &cleanup)
	pending, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || pending.SetupState != deviceSetupStateImported || pending.DeviceObjectKey != "" || pending.FailureReason == "" {
		t.Fatalf("pending projection: attempts=%d record=%+v", attempts, pending)
	}

	// Stub the Device object upsert to fail on the first attempt.
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, []*session.SessionListEntry{{SessionIndex: 3}, {SessionIndex: 4}}, mount, &cleanup)
	ready, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || ready.SetupState != deviceSetupStateSessionReady || ready.DeviceObjectKey != "devices/key" || ready.FailureReason != "" {
		t.Fatalf("projected Device: attempts=%d record=%+v", attempts, ready)
	}
}
