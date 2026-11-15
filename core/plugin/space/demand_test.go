package plugin_space

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/s4wave/spacewave/testbed"
)

// TestPluginDemandLoadsOnlyDemandedPlugins checks that the Space loads only
// the listed plugins its demand selects, including the account's, and that
// withdrawing one plugin stops exactly that plugin's load without restarting
// the others.
func TestPluginDemandLoadsOnlyDemandedPlugins(t *testing.T) {
	// Bound the test and build the testbed.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// List three plugins in the Space.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: []string{"bridge", "server", "viewer"}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	// Demand two of them and the account's plugin.
	var mtx sync.Mutex
	demanded := []string{"bridge", "inspector", "server"}
	isDemanded := func(pluginID string) bool {
		mtx.Lock()
		defer mtx.Unlock()
		return slices.Contains(demanded, pluginID)
	}
	accountIDs := []string{"inspector"}
	getAccountIDs := func() []string {
		mtx.Lock()
		defer mtx.Unlock()
		return accountIDs
	}

	// Record every LoadPlugin the Space adds.
	loads := newLoadRecorder()
	releaseLoads, err := tb.Bus.AddController(ctx, loads, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoads()

	// Start the Space controller with the demand.
	tb.StaticResolver.AddFactory(NewFactory(
		tb.Bus,
		WithPluginDemand(isDemanded),
		WithAccountPlugins(getAccountIDs),
	))
	space, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer spaceRef.Release()
	loads.wait(ctx, t, []string{"bridge", "inspector", "server"})

	// The account plugin's manifest is approved like a listed one.
	var approved []string
	space.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		approved = space.pluginIDs
	})
	if !slices.Contains(approved, "inspector") {
		t.Fatalf("approved plugin IDs = %v, want inspector", approved)
	}

	// The account drops its plugin.
	mtx.Lock()
	accountIDs = nil
	mtx.Unlock()
	space.NotifyChanged()
	loads.wait(ctx, t, []string{"bridge", "server"})

	// Withdraw the bridge.
	mtx.Lock()
	demanded = []string{"server"}
	mtx.Unlock()
	space.NotifyChanged()
	loads.wait(ctx, t, []string{"server"})

	// The server kept its first load.
	if starts := loads.getStarts("server"); starts != 1 {
		t.Fatalf("server loads started = %d, want 1", starts)
	}
}

// TestHostedSpaceLoadsWorldManifests checks that a Space hosted in a plugin,
// whose loads go to a host that cannot read its World, names the World's
// admitted manifest in each unpinned load, keeps it while a newer manifest
// waits for approval, replaces it once the newer one is allowed, and sends a
// plain load for a plugin with no manifest there.
func TestHostedSpaceLoadsWorldManifests(t *testing.T) {
	// Bound the test and start the hosted Space with the device's approvals.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	h := startHostedSpace(ctx, t)

	// Each plugin loads: the readiness plugin from the World's manifest.
	first := h.storeManifest(ctx, t, "test/manifests/first", 1)
	h.setApproval(ctx, t, first, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{first}))
	h.loads.waitManifests(ctx, t, "absent", nil)

	// A newer revision waiting for approval leaves the admitted load running.
	starts := h.loads.getStarts(pluginReadinessPluginID)
	second := h.storeManifest(ctx, t, "test/manifests/second", 2)
	<-time.After(200 * time.Millisecond)
	if got := h.loads.getStarts(pluginReadinessPluginID); got != starts {
		t.Fatalf("an unapproved revision started %d loads", got-starts)
	}
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{first}))

	// Allowing the newer revision replaces the load.
	h.setApproval(ctx, t, second, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{second}))
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	if starts := h.loads.getStarts("absent"); starts != 1 {
		t.Fatalf("absent loads started = %d, want 1", starts)
	}
}

// TestHostedSpaceLoadsOnlyAdmittedRoots checks that a hosted Space, whose
// host runs every root it is sent, never sends a World root the device has
// not allowed: the plugin loads without it until the device's person allows
// the root, and loses it again when they decline.
func TestHostedSpaceLoadsOnlyAdmittedRoots(t *testing.T) {
	// Bound the test and start the hosted Space with the device's approvals.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	h := startHostedSpace(ctx, t)
	ref := h.storeManifest(ctx, t, "test/manifests/first", 1)
	keys := artifactKeys([]*bldr_manifest.ManifestRef{ref})

	// With no approval, the load reaching the host names no World root.
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, nil)
	if starts := h.loads.getStarts(pluginReadinessPluginID); starts != 1 {
		t.Fatalf("unapproved: readiness loads started = %d, want 1", starts)
	}

	// Allowing the root sends a load that names it.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, keys)

	// Declining it withdraws that load.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_DECLINED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, nil)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
}

// TestHostedSpaceReplayLoadAwaitsAdmission checks that a plugin only a
// waiting replay asks for, whose World root the device has declined, reaches
// the host only as a load naming that root and the session peer, never as a
// plain load, even after the refusal: the host's admission holds it, the
// Space reads the host's awaiting-approval state, and allowing the root
// starts it exactly once from that root and clears that state. Ending the
// watch releases the scheduler lookup.
func TestHostedSpaceReplayLoadAwaitsAdmission(t *testing.T) {
	// Start a hosted Space whose replay asks for the readiness plugin, which
	// it does not list, on a host that admits roots through the approvals.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	replayIDs := []string{pluginReadinessPluginID}
	var mtx sync.Mutex
	var asked []string
	h := startHostedSpaceWith(ctx, t, []string{"absent"}, true, WithReplayPlugins(func() []string {
		mtx.Lock()
		defer mtx.Unlock()
		return asked
	}))
	ref := h.storeManifest(ctx, t, "test/manifests/first", 1)
	keys := artifactKeys([]*bldr_manifest.ManifestRef{ref})

	// The device declines the root, then replay asks for the plugin.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_DECLINED)
	ask := func(ids []string) {
		t.Helper()
		mtx.Lock()
		asked = ids
		mtx.Unlock()
		h.space.NotifyChanged()
		h.waitPluginIDs(ctx, t, append([]string{"absent"}, ids...))
	}
	ask(replayIDs)

	// Follow the Space's reading of the host's awaiting-approval state.
	var bcast broadcast.Broadcast
	var awaiting []string
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- h.space.WatchAwaitingApproval(watchCtx, replayIDs, func(ids []string) {
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				awaiting = ids
				broadcast()
			})
		})
	}()
	waitAwaiting := func(want []string) {
		t.Helper()
		for {
			var got []string
			var ch <-chan struct{}
			bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				got, ch = awaiting, getWaitCh()
			})
			if slices.Equal(got, want) {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("awaiting approval = %v, want %v", got, want)
			case <-ch:
			}
		}
	}

	// Declined, the load names the root and the session peer and awaits
	// admission.
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, keys)
	if peer := h.loads.getPeer(pluginReadinessPluginID); peer != h.tb.Volume.GetPeerID().String() {
		t.Fatalf("replay load peer = %q, want the session peer", peer)
	}
	waitAwaiting(replayIDs)

	// Once the refusal is observed, a full pass runs: the pass publishing a
	// marker ID starts after the refusal, and the pass dropping it starts
	// after that one ended. Loads are recorded as they are added, so no pass
	// up to here sent a plain load or started the plugin.
	h.waitRefused(ctx, t, ref)
	ask(append(slices.Clone(replayIDs), "replay-marker"))
	ask(replayIDs)
	if plain := h.loads.getPlain(pluginReadinessPluginID); plain != 0 {
		t.Fatalf("refused replay plugin got %d plain loads", plain)
	}
	if starts := h.loads.getStarts(pluginReadinessPluginID); starts != 0 {
		t.Fatalf("refused replay plugin started %d times", starts)
	}
	h.loads.wait(ctx, t, []string{"absent"})

	// Allowing the root starts it exactly once, from that root, and clears
	// the awaiting state.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	waitAwaiting(nil)
	if starts := h.loads.getStarts(pluginReadinessPluginID); starts != 1 {
		t.Fatalf("approved replay plugin started %d times, want 1", starts)
	}
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, keys)
	if plain := h.loads.getPlain(pluginReadinessPluginID); plain != 0 {
		t.Fatalf("approved replay plugin got %d plain loads", plain)
	}

	// Ending the watch returns and releases the scheduler lookup.
	stopWatch()
	select {
	case <-watchDone:
	case <-ctx.Done():
		t.Fatal("the awaiting-approval watch did not end with its context")
	}
	h.loads.waitLookups(ctx, t, 0)
}

// hostedSpace is a Space hosted on the testbed bus, recording the loads it
// sends there.
type hostedSpace struct {
	// tb is the testbed whose bus hosts the Space.
	tb *testbed.Testbed
	// space is the hosted Space controller.
	space *Controller
	// approvals is the device's approval store answering admission.
	approvals *plugin_approval.Approvals
	// loads records the loads the Space sends to the host.
	loads *loadRecorder
}

// startHostedSpace starts a Space hosted on a testbed bus that holds the
// device's approvals, listing the readiness plugin and one with no manifest.
func startHostedSpace(ctx context.Context, t *testing.T) *hostedSpace {
	t.Helper()
	return startHostedSpaceWith(ctx, t, []string{"absent", pluginReadinessPluginID}, false)
}

// startHostedSpaceWith starts a Space listing listed, hosted on a testbed bus
// that holds the device's approvals. With admit, the recorded host holds each
// load that names roots until AdmitPluginRoot admits one, as a host scheduler
// does.
func startHostedSpaceWith(ctx context.Context, t *testing.T, listed []string, admit bool, opts ...FactoryOption) *hostedSpace {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// List the plugins and create the manifest store.
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		"",
		space_world_ops.DefaultSpaceSettingsObjectKey,
		&space_world.SpaceSettings{PluginIds: listed},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, hostedManifestStore); err != nil {
		t.Fatal(err)
	}

	// Answer admission from the device's approvals and record the loads.
	approvals := plugin_approval.NewApprovals(tb.Bus, tb.EngineVolumeID, "test-plugin-approvals")
	releaseApprovals, err := tb.Bus.AddController(ctx, plugin_approval.NewController(tb.Logger, approvals), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseApprovals)
	loads := newLoadRecorder()
	if admit {
		loads.admit = tb.Bus
	}
	releaseLoads, err := tb.Bus.AddController(ctx, loads, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseLoads)

	// Start the Space hosted on the testbed bus.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus, append([]FactoryOption{WithLoadTarget(tb.Bus)}, opts...)...))
	space, _, spaceRef, err := StartControllerWithConfig(ctx, tb.Bus, &Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(spaceRef.Release)
	return &hostedSpace{tb: tb, space: space, approvals: approvals, loads: loads}
}

// hostedManifestStore is the manifest store the hosted Space lists.
const hostedManifestStore = "test/plugin-manifests"

// storeManifest stores a readiness manifest revision in the World.
func (h *hostedSpace) storeManifest(ctx context.Context, t *testing.T, objKey string, rev uint64) *bldr_manifest.ManifestRef {
	t.Helper()
	ref := createReadinessManifestRev(t, ctx, h.tb, rev)
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx, h.tb.WorldState, h.tb.Volume.GetPeerID(), objKey, []string{hostedManifestStore}, ref,
	); err != nil {
		t.Fatal(err)
	}
	return ref
}

// setApproval records the device's decision on ref's root.
func (h *hostedSpace) setApproval(ctx context.Context, t *testing.T, ref *bldr_manifest.ManifestRef, state s4wave_plugin_approval.PluginApprovalState) {
	t.Helper()
	if err := h.approvals.Set(ctx, h.tb.Volume.GetPeerID().String(), &s4wave_plugin_approval.PluginApproval{
		PluginId:     pluginReadinessPluginID,
		ManifestRoot: manifestRoot(ref),
		State:        state,
	}); err != nil {
		t.Fatal(err)
	}
}

// waitPluginIDs waits until the Space has published want as its plugin IDs,
// which it does at the start of each reconcile pass.
func (h *hostedSpace) waitPluginIDs(ctx context.Context, t *testing.T, want []string) {
	t.Helper()
	for {
		var got []string
		var ch <-chan struct{}
		h.space.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			got, ch = h.space.pluginIDs, getWaitCh()
		})
		if slices.Equal(got, want) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("plugin IDs = %v, want %v", got, want)
		case <-ch:
		}
	}
}

// waitRefused waits until the device's approvals answer that ref's root is
// not admitted for the session peer.
func (h *hostedSpace) waitRefused(ctx context.Context, t *testing.T, ref *bldr_manifest.ManifestRef) {
	t.Helper()
	di, dirRef, err := h.tb.Bus.AddDirective(ctx, bldr_plugin.NewAdmitPluginRoot(
		h.tb.Volume.GetPeerID().String(), pluginReadinessPluginID, manifestRoot(ref),
	), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dirRef.Release()
	refused := make(chan struct{})
	var once sync.Once
	releaseState := di.AddStateCallback(func(idle bool, _ []error, vals []directive.AttachedValue) {
		if idle && len(vals) == 0 {
			once.Do(func() { close(refused) })
		}
	})
	defer releaseState()
	select {
	case <-ctx.Done():
		t.Fatal("the declined root was never refused")
	case <-refused:
	}
}

// loadRecorder resolves every LoadPlugin directive and records which plugins
// have a live load. With admit set it also stands in for a host scheduler: a
// load naming roots waits for AdmitPluginRoot on admit before it goes live,
// and its status shows PluginSwapAwaitingApproval meanwhile.
type loadRecorder struct {
	// admit is the bus answering AdmitPluginRoot, or nil to admit every load.
	admit bus.Bus
	// status is the stand-in scheduler's plugin status.
	status *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot]
	// bcast guards live, starts, plain, manifests, peers, awaiting and
	// lookups.
	bcast broadcast.Broadcast
	// live counts the live loads by plugin ID.
	live map[string]int
	// starts counts the loads started by plugin ID.
	starts map[string]int
	// plain counts the loads naming no manifest by plugin ID, as they are
	// added.
	plain map[string]int
	// manifests is the artifact keys the last started load of each plugin ID
	// names.
	manifests map[string][]string
	// peers is the admission peer the last load of each plugin ID names.
	peers map[string]string
	// awaiting is the instance key of each plugin awaiting admission.
	awaiting map[string]string
	// lookups counts the live LookupPluginScheduler resolvers.
	lookups int
}

// newLoadRecorder constructs a loadRecorder.
func newLoadRecorder() *loadRecorder {
	return &loadRecorder{
		live:      make(map[string]int),
		starts:    make(map[string]int),
		plain:     make(map[string]int),
		manifests: make(map[string][]string),
		peers:     make(map[string]string),
		awaiting:  make(map[string]string),
		status:    ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{}),
	}
}

// getPeer returns the admission peer the last load of pluginID names.
func (r *loadRecorder) getPeer(pluginID string) string {
	var peer string
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		peer = r.peers[pluginID]
	})
	return peer
}

// waitLookups waits until exactly want LookupPluginScheduler resolvers live.
func (r *loadRecorder) waitLookups(ctx context.Context, t *testing.T, want int) {
	t.Helper()
	for {
		var got int
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			got, ch = r.lookups, getWaitCh()
		})
		if got == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("live scheduler lookups = %d, want %d", got, want)
		case <-ch:
		}
	}
}

// setAwaiting records whether pluginID's load awaits admission and publishes
// the stand-in scheduler's status.
func (r *loadRecorder) setAwaiting(pluginID, instanceKey, peerID string, awaiting bool) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if awaiting {
			r.awaiting[pluginID] = instanceKey
		} else {
			delete(r.awaiting, pluginID)
		}
		snapshot := &bldr_plugin.PluginStatusSnapshot{}
		for id, key := range r.awaiting {
			snapshot.Executions = append(snapshot.Executions, &bldr_plugin.PluginExecutionStatus{
				PluginId:    id,
				InstanceKey: key,
				Swap: &bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_AwaitingApproval{
					AwaitingApproval: &bldr_plugin.PluginSwapAwaitingApproval{PeerId: peerID},
				}},
			})
		}
		r.status.SetValue(snapshot)
		broadcast()
	})
}

// GetInstanceKey returns the root host's instance key.
func (r *loadRecorder) GetInstanceKey() string {
	return ""
}

// GetPluginStatusCtr returns the stand-in scheduler's plugin status.
func (r *loadRecorder) GetPluginStatusCtr() ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot] {
	return r.status
}

// wait waits until exactly want have a live load.
func (r *loadRecorder) wait(ctx context.Context, t *testing.T, want []string) {
	t.Helper()
	for {
		// Snapshot the live plugins.
		var got []string
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			for id, n := range r.live {
				if n != 0 {
					got = append(got, id)
				}
			}
			ch = getWaitCh()
		})
		slices.Sort(got)
		if slices.Equal(got, want) {
			return
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			t.Fatalf("live loads = %v, want %v", got, want)
		case <-ch:
		}
	}
}

// getPlain returns how many loads of pluginID named no manifest.
func (r *loadRecorder) getPlain(pluginID string) int {
	var n int
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		n = r.plain[pluginID]
	})
	return n
}

// getStarts returns how many loads of pluginID started.
func (r *loadRecorder) getStarts(pluginID string) int {
	var n int
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		n = r.starts[pluginID]
	})
	return n
}

// waitManifests waits until the last load of pluginID names exactly want.
func (r *loadRecorder) waitManifests(ctx context.Context, t *testing.T, pluginID string, want []string) {
	t.Helper()
	for {
		// Snapshot the last load's manifests.
		var got []string
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			got = r.manifests[pluginID]
			ch = getWaitCh()
		})
		if slices.Equal(got, want) {
			return
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			t.Fatalf("%s load manifests = %v, want %v", pluginID, got, want)
		case <-ch:
		}
	}
}

// GetControllerInfo returns information about the controller.
func (r *loadRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/plugin-load-recorder",
		controller.MustParseVersion("0.0.1"),
		"records plugin loads",
	)
}

// Execute executes the controller.
func (r *loadRecorder) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close releases any resources used by the controller.
func (r *loadRecorder) Close() error {
	return nil
}

// HandleDirective marks a plugin live while its LoadPlugin resolver runs.
func (r *loadRecorder) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	// Answer a scheduler lookup while standing in for a scheduler, counting
	// the live lookups.
	if _, ok := inst.GetDirective().(bldr_plugin.LookupPluginScheduler); ok && r.admit != nil {
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, h directive.ResolverHandler) error {
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.lookups++
				broadcast()
			})
			defer r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.lookups--
				broadcast()
			})
			// Mark idle once published, as the host's resolver does, so
			// a watch that waits for idle sees the scheduler.
			h.AddValue(bldr_plugin.LookupPluginSchedulerValue(r))
			h.MarkIdle(true)
			<-ctx.Done()
			return nil
		}), nil)
	}
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	pluginID := load.LoadPluginID()
	refs := load.LoadPluginManifests()
	var keys []string
	for _, ref := range refs {
		keys = append(keys, bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef()))
	}
	if len(refs) == 0 {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.plain[pluginID]++
			broadcast()
		})
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		// Hold a load naming roots until one is admitted for its peer.
		if r.admit != nil && len(refs) != 0 {
			peerID := load.LoadPluginPeerID()
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.manifests[pluginID] = keys
				r.peers[pluginID] = peerID
				broadcast()
			})
			r.setAwaiting(pluginID, load.LoadPluginInstanceKey(), peerID, true)
			_, err := bldr_plugin.WaitPluginRootAdmitted(ctx, r.admit, peerID, pluginID, manifestRoot(refs[0]))
			r.setAwaiting(pluginID, load.LoadPluginInstanceKey(), peerID, false)
			if err != nil {
				return nil
			}
		}

		// Mark the load live until the directive is released.
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.live[pluginID]++
			r.starts[pluginID]++
			r.manifests[pluginID] = keys
			r.peers[pluginID] = load.LoadPluginPeerID()
			broadcast()
		})
		<-ctx.Done()
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.live[pluginID]--
			broadcast()
		})
		return nil
	}), nil)
}

// _ is a type assertion
var _ controller.Controller = (*loadRecorder)(nil)
