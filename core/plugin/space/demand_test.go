package plugin_space

import (
	"context"
	"slices"
	"strings"
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
	h.loads.waitStarts(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{first}), 1)
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
	h.loads.waitStarts(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{second}), 1)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, artifactKeys([]*bldr_manifest.ManifestRef{second}))
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	if starts := h.loads.getStarts("absent"); starts != 1 {
		t.Fatalf("absent loads started = %d, want 1", starts)
	}
}

// TestHostedSpaceLoadsOnlyAdmittedRoots checks that a hosted Space's host
// never starts a World root the device has not allowed: the plugin loads
// without it until the device's person allows the root, and loses it again
// when they decline.
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
	if starts := h.loads.getStartsOf(pluginReadinessPluginID, keys); starts != 0 {
		t.Fatalf("unapproved root started %d times", starts)
	}

	// Allowing the root sends a load that names it.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.waitStarts(ctx, t, pluginReadinessPluginID, keys, 1)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, keys)

	// Declining it withdraws that load.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_DECLINED)
	h.loads.waitManifests(ctx, t, pluginReadinessPluginID, nil)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	if starts := h.loads.getStartsOf(pluginReadinessPluginID, keys); starts != 1 {
		t.Fatalf("root started %d times across approval and decline, want 1", starts)
	}
}

// TestHostedSpaceReplayFallsBackUntilRootAllowed checks that a hosted Space,
// for a plugin only a waiting replay asks for, asks admission for the World
// root it selected before it sends the plain load it falls back to while the
// device declines that root, and never starts the declined root. Allowing the
// root starts it for the session peer with nothing left awaiting approval.
// Ending the watch releases the scheduler lookup.
func TestHostedSpaceReplayFallsBackUntilRootAllowed(t *testing.T) {
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

	// The device's person declines the root, then replay asks for the plugin.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_DECLINED)
	mtx.Lock()
	asked = replayIDs
	mtx.Unlock()
	h.space.NotifyChanged()

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

	// The plugin's World manifest is stored before replay asks, so the
	// Space's admission gate asks AdmitPluginRoot for its root the first time
	// a pass selects it. When that ask adds a new directive, the bus calls
	// the recorder's handler before AddDirective returns; when it joins an
	// equivalent directive already on the bus, the handler is not called
	// again, but it was called when that earlier directive was added. Either
	// way the recorded ask precedes the pass's load, so a root the Space never
	// selected, such as after a failed read of its hosted manifests, shows as
	// no ask before the plain load.
	h.loads.waitStarts(ctx, t, pluginReadinessPluginID, nil, 1)
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	askedAt, askedOk, startedAt, _ := h.loads.getOrder(pluginReadinessPluginID, manifestRoot(ref), nil)
	if !askedOk || askedAt > startedAt {
		t.Fatalf("plain load started at %d before the root was asked (asked %v at %d)", startedAt, askedOk, askedAt)
	}
	if starts := h.loads.getStartsOf(pluginReadinessPluginID, keys); starts != 0 {
		t.Fatalf("declined replay root started %d times", starts)
	}

	// Allowing the root starts it for the session peer and clears the
	// awaiting state.
	h.setApproval(ctx, t, ref, s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	h.loads.waitStarts(ctx, t, pluginReadinessPluginID, keys, 1)
	if peer := h.loads.getPeer(pluginReadinessPluginID); peer != h.tb.Volume.GetPeerID().String() {
		t.Fatalf("replay load peer = %q, want the session peer", peer)
	}
	h.loads.wait(ctx, t, []string{"absent", pluginReadinessPluginID})
	waitAwaiting(nil)

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
// Its host checks named roots against those approvals, as the scheduler does.
func startHostedSpace(ctx context.Context, t *testing.T) *hostedSpace {
	t.Helper()
	return startHostedSpaceWith(ctx, t, []string{"absent", pluginReadinessPluginID}, true)
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

// loadRecorder resolves every LoadPlugin directive and records which plugins
// have a live load. With admit set it also stands in for a host scheduler: a
// load naming roots waits for AdmitPluginRoot on admit before it goes live,
// and its status shows PluginSwapAwaitingApproval meanwhile, and it then
// runs only the admitted ones. It also records the order in which each
// AdmitPluginRoot is asked and each load starts.
type loadRecorder struct {
	// admit is the bus answering AdmitPluginRoot, or nil to admit every load.
	admit bus.Bus
	// status is the stand-in scheduler's plugin status.
	status *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot]
	// bcast guards seq, asked, firstStart, live, starts, startsOf,
	// manifests, peers, awaiting and lookups.
	bcast broadcast.Broadcast
	// seq numbers the asks and starts in the order the recorder sees them.
	seq int
	// asked is the seq of the first AdmitPluginRoot asked, by rootKey.
	asked map[string]int
	// firstStart is the seq of the first start, by loadKey.
	firstStart map[string]int
	// live counts the live loads by plugin ID.
	live map[string]int
	// starts counts the loads started by plugin ID.
	starts map[string]int
	// startsOf counts the loads started by loadKey of plugin ID and the
	// artifact keys the load runs.
	startsOf map[string]int
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
		asked:      make(map[string]int),
		firstStart: make(map[string]int),
		live:       make(map[string]int),
		starts:     make(map[string]int),
		startsOf:   make(map[string]int),
		manifests:  make(map[string][]string),
		peers:      make(map[string]string),
		awaiting:   make(map[string]string),
		status:     ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{}),
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

// getStarts returns how many loads of pluginID started.
func (r *loadRecorder) getStarts(pluginID string) int {
	var n int
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		n = r.starts[pluginID]
	})
	return n
}

// loadKey identifies a load by plugin ID and the artifact keys it runs.
func loadKey(pluginID string, keys []string) string {
	return pluginID + "\x00" + strings.Join(keys, "\x00")
}

// rootKey identifies an admission ask by plugin ID and manifest root.
func rootKey(pluginID, root string) string {
	return pluginID + "\x00" + root
}

// getOrder returns the seq of the first AdmitPluginRoot asked for root of
// pluginID and of the first start of the load of pluginID running keys, each
// with whether it happened.
func (r *loadRecorder) getOrder(pluginID, root string, keys []string) (asked int, askedOk bool, started int, startedOk bool) {
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		asked, askedOk = r.asked[rootKey(pluginID, root)]
		started, startedOk = r.firstStart[loadKey(pluginID, keys)]
	})
	return asked, askedOk, started, startedOk
}

// getStartsOf returns how many loads of pluginID running exactly keys started.
func (r *loadRecorder) getStartsOf(pluginID string, keys []string) int {
	var n int
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		n = r.startsOf[loadKey(pluginID, keys)]
	})
	return n
}

// waitStarts waits until at least want loads of pluginID running exactly keys
// started.
func (r *loadRecorder) waitStarts(ctx context.Context, t *testing.T, pluginID string, keys []string, want int) {
	t.Helper()
	for {
		var got int
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			got, ch = r.startsOf[loadKey(pluginID, keys)], getWaitCh()
		})
		if got >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s loads of %v started = %d, want %d", pluginID, keys, got, want)
		case <-ch:
		}
	}
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
	// Record each admission ask. The bus calls every handler before
	// AddDirective returns when it adds a new directive, so an ask is recorded
	// before its asker goes on. An ask equivalent to a directive already on
	// the bus only references it and calls no handler; its first ask was
	// recorded when that directive was added.
	if admit, ok := inst.GetDirective().(bldr_plugin.AdmitPluginRoot); ok {
		key := rootKey(admit.AdmitPluginRootPluginID(), admit.AdmitPluginRootManifestRoot())
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := r.asked[key]; !ok {
				r.seq++
				r.asked[key] = r.seq
				broadcast()
			}
		})
		return nil, nil
	}
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	pluginID := load.LoadPluginID()
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		// Hold a load naming roots until one is admitted for its peer, then
		// run only the admitted ones. The resolver of a directive the Space
		// already withdrew can still begin, with its context ended, and still
		// read an admission another asker holds; it records nothing, since
		// that load never reached the host, so each record checks ctx under
		// the lock the withdrawal is ordered before.
		refs, peerID := load.LoadPluginManifests(), load.LoadPluginPeerID()
		if r.admit != nil && len(refs) != 0 {
			var withdrawn bool
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if withdrawn = ctx.Err() != nil; withdrawn {
					return
				}
				r.manifests[pluginID] = artifactKeys(refs)
				r.peers[pluginID] = peerID
				broadcast()
			})
			if withdrawn {
				return nil
			}
			r.setAwaiting(pluginID, load.LoadPluginInstanceKey(), peerID, true)
			admitted, err := r.awaitAdmitted(ctx, peerID, pluginID, refs)
			r.setAwaiting(pluginID, load.LoadPluginInstanceKey(), peerID, false)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			refs = admitted
		}
		keys := artifactKeys(refs)

		// Mark the load live until the directive is released.
		var withdrawn bool
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if withdrawn = ctx.Err() != nil; withdrawn {
				return
			}
			key := loadKey(pluginID, keys)
			if _, ok := r.firstStart[key]; !ok {
				r.seq++
				r.firstStart[key] = r.seq
			}
			r.live[pluginID]++
			r.starts[pluginID]++
			r.startsOf[key]++
			r.manifests[pluginID] = keys
			r.peers[pluginID] = peerID
			broadcast()
		})
		if withdrawn {
			return nil
		}
		<-ctx.Done()
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.live[pluginID]--
			broadcast()
		})
		return nil
	}), nil)
}

// awaitAdmitted returns the refs whose roots AdmitPluginRoot on r.admit admits
// for peerID, waiting until at least one is.
func (r *loadRecorder) awaitAdmitted(ctx context.Context, peerID, pluginID string, refs []*bldr_manifest.ManifestRef) ([]*bldr_manifest.ManifestRef, error) {
	for {
		// Take an admission value immediately, or observe an idle refusal,
		// as the real scheduler does when checking a root.
		var admitted []*bldr_manifest.ManifestRef
		for _, ref := range refs {
			_, _, valsRef, err := bus.ExecWaitValue[bldr_plugin.AdmitPluginRootValue](
				ctx, r.admit, bldr_plugin.NewAdmitPluginRoot(peerID, pluginID, manifestRoot(ref)), bus.ReturnIfIdle(true), nil, nil,
			)
			if err != nil {
				return nil, err
			}
			if valsRef != nil {
				valsRef.Release()
				admitted = append(admitted, ref)
			}
		}
		if len(admitted) != 0 {
			return admitted, nil
		}

		// Wait until any root is admitted, then read them all again.
		waitCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{}, len(refs))
		for _, ref := range refs {
			go func() {
				if _, err := bldr_plugin.WaitPluginRootAdmitted(waitCtx, r.admit, peerID, pluginID, manifestRoot(ref)); err == nil {
					done <- struct{}{}
				}
			}()
		}
		select {
		case <-ctx.Done():
			cancel()
			return nil, context.Cause(ctx)
		case <-done:
		}
		cancel()
	}
}

// _ is a type assertion
var _ controller.Controller = (*loadRecorder)(nil)
