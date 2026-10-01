package plugin_space

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/testbed"
)

// TestPluginDemandLoadsOnlyDemandedPlugins checks that the Space loads only
// the listed plugins its demand selects, and that withdrawing one plugin stops
// exactly that plugin's load without restarting the others.
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

	// Demand two of them.
	var mtx sync.Mutex
	demanded := []string{"bridge", "server"}
	isDemanded := func(pluginID string) bool {
		mtx.Lock()
		defer mtx.Unlock()
		return slices.Contains(demanded, pluginID)
	}

	// Record every LoadPlugin the Space adds.
	loads := newLoadRecorder()
	releaseLoads, err := tb.Bus.AddController(ctx, loads, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoads()

	// Start the Space controller with the demand.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus, WithPluginDemand(isDemanded)))
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

// loadRecorder resolves every LoadPlugin directive and records which plugins
// have a live load.
type loadRecorder struct {
	// bcast guards live and starts.
	bcast broadcast.Broadcast
	// live counts the live loads by plugin ID.
	live map[string]int
	// starts counts the loads started by plugin ID.
	starts map[string]int
}

// newLoadRecorder constructs a loadRecorder.
func newLoadRecorder() *loadRecorder {
	return &loadRecorder{live: make(map[string]int), starts: make(map[string]int)}
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
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	pluginID := load.LoadPluginID()
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		// Mark the load live until the directive is released.
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.live[pluginID]++
			r.starts[pluginID]++
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
