//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	plugin_space_runtime "github.com/s4wave/spacewave/core/plugin/space/runtime"
	provider "github.com/s4wave/spacewave/core/provider"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
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

// TestRetainApprovedProcessRuntimeReleasesOnDisableAndSessionEnd checks
// that the keeper's reference follows approved, disabled, and deleted bindings
// through the Session lifetime.
func TestRetainApprovedProcessRuntimeReleasesOnDisableAndSessionEnd(t *testing.T) {
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
		retainApprovedProcessRuntime(ctx, logrus.NewEntry(logrus.New()), "space/test", space, func() (func(), error) {
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
			retainApprovedProcessRuntime(watchCtx, logrus.NewEntry(logrus.New()), spaceID,
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
// persisted local completion across session snapshots and keeper shutdown:
// the restore retains the session once and projects the pending Device as
// session-ready before the release drops the session.
func TestReconcileDeviceEnrollmentRestoresAndReleasesLocalSession(t *testing.T) {
	// Seed a pending local enrollment and stub the local mount.
	statePath := t.TempDir()
	record := &deviceSetupRecord{
		SetupState: deviceSetupStateImported, Completion: deviceLocalCompletionPrefix + "stub",
		SessionIndex: 3, FailureReason: "Device object projection pending: block not found",
	}
	if err := writeDeviceSetupRecord(statePath, record); err != nil {
		t.Fatal(err)
	}
	oldMount := deviceMountLocalSession
	deviceMountLocalSession = func(_ context.Context, _ *sdkClient, _ string, current *deviceSetupRecord) (*deviceSetupRecord, error) {
		return current, nil
	}
	t.Cleanup(func() { deviceMountLocalSession = oldMount })

	// Stub the projection, which must wait for the World.
	var projected string
	waits := false
	withDeviceObjectUpsertStub(t, func(_ context.Context, _ *sdkClient, current *deviceSetupRecord, onBlocked func(error)) (string, error) {
		projected, waits = current.SetupState, onBlocked != nil
		return "devices/key", nil
	})

	// Mount the configured session index and count the calls.
	mount := &testLocalSessionMount{}
	mountCalls := 0
	mountFunc := func(index uint32) (localSessionMount, error) {
		mountCalls++
		if index != 3 {
			t.Fatalf("mount index = %d, want 3", index)
		}
		return mount, nil
	}

	// Reconcile across two snapshots and confirm one retained mount.
	le := logrus.NewEntry(logrus.New())
	var cleanup func()
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, mountFunc, &cleanup)
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, mountFunc, &cleanup)
	if cleanup == nil || mountCalls != 1 || mount.released {
		t.Fatalf("retained enrollment: cleanup=%v mounts=%d released=%v", cleanup != nil, mountCalls, mount.released)
	}

	// Release, which waits for the projection, and check the ready record.
	cleanup()
	if !mount.released {
		t.Fatal("daemon shutdown did not release Device session")
	}
	if projected != deviceSetupStateSessionReady || !waits {
		t.Fatalf("projection state=%q waits=%v", projected, waits)
	}
	ready, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if ready.SetupState != deviceSetupStateSessionReady || ready.DeviceObjectKey != "devices/key" || ready.FailureReason != "" {
		t.Fatalf("projected Device record = %+v", ready)
	}
}

// keeperTestSpaceBody presents the test World under one stable Space identity.
type keeperTestSpaceBody struct {
	// engine is the mounted Space World.
	engine world.Engine
	// engineID names that World to the Space runtime.
	engineID string
	// ref identifies the mounted Space.
	ref *sobject.SharedObjectRef
}

// GetWorldEngine returns the mounted Space World.
func (b *keeperTestSpaceBody) GetWorldEngine() world.Engine { return b.engine }

// GetWorldEngineID returns the mounted World address.
func (b *keeperTestSpaceBody) GetWorldEngineID() string { return b.engineID }

// GetWorldEngineBucketID returns no forwarded bucket for this local World.
func (b *keeperTestSpaceBody) GetWorldEngineBucketID() string { return "" }

// GetSharedObjectRef returns the Space identity.
func (b *keeperTestSpaceBody) GetSharedObjectRef() *sobject.SharedObjectRef { return b.ref }

// GetSharedObject returns no remote SharedObject for this local fixture.
func (b *keeperTestSpaceBody) GetSharedObject() sobject.SharedObject { return nil }

// TestRetainApprovedProcessRuntimeKeepsFlowgraphRunAfterCommandRelease
// approves a Flowgraph run through a command's contents mount and releases the
// mount. The keeper follows the Space's real binding watch and keeps the shared
// Space runtime running the approved run until the keeper ends.
func TestRetainApprovedProcessRuntimeKeepsFlowgraphRunAfterCommandRelease(t *testing.T) {
	// Mount the test volume under the plugin volume ID holding process decisions.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb, err := testbed.WithTestbedOptions(ctx, []db_testbed.Option{
		db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
			VolumeConfig: &volume_controller.Config{VolumeIdAlias: []string{bldr_plugin.PluginVolumeID}},
		}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Serve the Space resource whose binding watch the keeper follows.
	ref := &sobject.SharedObjectRef{ProviderResourceRef: &provider.ProviderResourceRef{
		ProviderId: "local", ProviderAccountId: "test", Id: "keeper-flowgraph",
	}}
	spaceID := space.SpaceEngineId(ref)
	registry := process_binding.NewBindingRegistry()
	spaceResource := resource_space.NewSpaceResourceWithSessionPeerID(
		tb.Logger, tb.Bus, &keeperTestSpaceBody{engine: tb.Engine, engineID: tb.EngineID, ref: ref}, tb.Volume.GetPeerID().String(),
	)
	spaceResource.SetBindingRegistry(registry)
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(spaceResource.GetMux()))),
	)

	// Mount the shared Space runtime as a contents mount does.
	conf := &plugin_space_runtime.Config{Space: &plugin_space.Config{
		SpaceId: spaceID, VolumeId: tb.EngineVolumeID,
		ObjectStoreId: process_binding.DefaultObjectStoreID,
		EngineId:      tb.EngineID, SessionPeerId: tb.Volume.GetPeerID().String(),
	}}
	mount := func() (*plugin_space_runtime.Controller, *resource_space.SpaceContentsResource, error) {
		// Reference the shared runtime and serve contents on its binding registry.
		runtime, runtimeRef, err := plugin_space_runtime.StartControllerWithConfig(ctx, tb.Bus, conf)
		if err != nil {
			return nil, nil, err
		}
		runtime.SetBindingRegistry(registry)
		contents := resource_space.NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, spaceID, tb.EngineID, runtime, runtimeRef)
		return runtime, contents, nil
	}

	// Run the keeper, reporting when it retains the runtime.
	retained := make(chan struct{})
	keeperCtx, cancelKeeper := context.WithCancel(ctx)
	keeperDone := make(chan struct{})
	go func() {
		defer close(keeperDone)
		retainApprovedProcessRuntime(keeperCtx, tb.Logger, spaceID, client, func() (func(), error) {
			_, contents, err := mount()
			if err != nil {
				return nil, err
			}
			close(retained)
			return contents.Release, nil
		})
	}()

	// Create the Flowgraph run and mount its runtime for a command.
	obj, err := tb.WorldState.CreateObject(ctx, "flowgraph-run/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	runtime, command, err := mount()
	if err != nil {
		t.Fatal(err)
	}
	gen, waitGen, err := runtime.GetGeneration()
	for gen == nil && err == nil {
		<-waitGen
		gen, waitGen, err = runtime.GetGeneration()
	}
	if err != nil {
		t.Fatal(err)
	}

	// Approve the Flowgraph run through the command's contents mount.
	if _, err := command.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "flowgraph-run/test", TypeId: s4wave_flowgraph.FlowgraphRunTypeID, Approved: true,
	}); err != nil {
		t.Fatal(err)
	}

	// The keeper retains the runtime, so releasing the command keeps it running.
	select {
	case <-retained:
	case <-ctx.Done():
		t.Fatal("keeper did not retain the approved Flowgraph run's runtime")
	}
	command.Release()
	select {
	case <-gen.Done():
		t.Fatal("command mount release stopped the retained runtime")
	case <-time.After(100 * time.Millisecond):
	}

	// Ending the keeper releases the last runtime reference.
	cancelKeeper()
	<-keeperDone
	select {
	case <-gen.Done():
	case <-ctx.Done():
		t.Fatal("keeper exit retained the runtime")
	}
}
