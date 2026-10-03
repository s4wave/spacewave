package plugin_space_runtime

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_mock "github.com/s4wave/spacewave/bldr/plugin/host/mock"
	plugin_host_static "github.com/s4wave/spacewave/bldr/plugin/host/static"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/testbed"
)

func TestStartControllerWithConfigSharesOneRuntime(t *testing.T) {
	// Start a testbed for two mounts of the same Space.
	tb := newTestbed(t)
	conf := newTestConfig(tb, "space-a")

	// Acquire the first Space runtime reference.
	first, firstRef, err := StartControllerWithConfig(t.Context(), tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}

	// Acquire another runtime reference with an equal configuration.
	second, secondRef, err := StartControllerWithConfig(t.Context(), tb.Bus, conf.CloneVT())
	if err != nil {
		t.Fatal(err)
	}

	// Verify both mounts share the same runtime and running generation.
	if first != second {
		t.Fatal("equal configs started two runtimes")
	}
	gen := waitGeneration(t, first, nil)

	// Acquire a different Space to verify runtime isolation.
	other, otherRef, err := StartControllerWithConfig(t.Context(), tb.Bus, newTestConfig(tb, "space-b"))
	if err != nil {
		t.Fatal(err)
	}
	otherRef.Release()

	// Verify different Spaces receive distinct runtime controllers.
	if other == first {
		t.Fatal("different Spaces shared one runtime")
	}

	// The runtime outlives every reference but the last.
	firstRef.Release()
	select {
	case <-gen.Done():
		t.Fatal("releasing one of two references stopped the runtime")
	case <-time.After(50 * time.Millisecond):
	}

	// Verify releasing the last mount stops the shared generation.
	secondRef.Release()
	select {
	case <-gen.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("releasing the last reference did not stop the runtime")
	}
}

// TestControllerHostsStayLive observes the scheduler while preserving both mounts.
func TestControllerHostsStayLive(t *testing.T) {
	// Prepare a Space with two application plugin declarations.
	tb := newTestbed(t)
	conf := newTestConfig(tb, "space-a")
	conf.AppPluginIds = []string{"app", "web"}

	// Acquire the Space runtime for the original plugin declarations.
	rt, ref, err := StartControllerWithConfig(t.Context(), tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)

	// Acquire the runtime through reordered and duplicate plugin declarations.
	alias := conf.CloneVT()
	alias.AppPluginIds = []string{"web", "app", "app"}
	second, secondRef, err := StartControllerWithConfig(t.Context(), tb.Bus, alias)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondRef.Release)

	// Verify canonical plugin declarations share a runtime.
	if second != rt {
		t.Fatal("canonical declarations did not share runtime")
	}

	// Acquire a Space runtime with different application plugins.
	different := conf.CloneVT()
	different.AppPluginIds = []string{"other-app"}
	other, otherRef, err := StartControllerWithConfig(t.Context(), tb.Bus, different)
	if err != nil {
		t.Fatal(err)
	}
	otherRef.Release()

	// Verify different plugin declarations produce a distinct runtime.
	if other == rt {
		t.Fatal("different application declarations shared runtime")
	}

	// Wait for the generation whose scheduler must survive host changes.
	gen := waitGeneration(t, rt, nil)

	// Observe scheduler changes and require the Space generation to stay live.
	await := func(match func([]plugin_host.PluginHost, error) bool) {
		// Bound the scheduler observation by the test context.
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		// Wait for the scheduler to publish the requested host state.
		for {
			// Check the host snapshot before waiting on its change notification.
			hosts, wait, err := gen.GetScheduler().GetHostState()
			if match(hosts, err) {
				break
			}

			// Await a scheduler change when its host state does not yet match.
			if err := wait(ctx); err != nil {
				t.Fatal(err)
			}
		}

		// Verify the scheduler change preserved the running Space generation.
		if current, _, err := rt.GetGeneration(); current != gen || err != nil {
			t.Fatal("host change replaced the Space runtime")
		}
	}

	// Publish the first host to the runtime scheduler.
	firstHost := plugin_host_mock.NewHost("test/platform")
	releaseHost, err := tb.Bus.AddController(t.Context(), plugin_host_static.NewController([]plugin_host.PluginHost{firstHost}), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the scheduler observes the first host.
	await(func(hosts []plugin_host.PluginHost, _ error) bool {
		return slices.Contains(hosts, plugin_host.PluginHost(firstHost))
	})

	// Replace the first host with another host on the same platform.
	replacement := plugin_host_mock.NewHost("test/platform")
	releaseHost()
	releaseReplacement, err := tb.Bus.AddController(t.Context(), plugin_host_static.NewController([]plugin_host.PluginHost{replacement}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseReplacement)

	// Verify the scheduler retains only the replacement host.
	await(func(hosts []plugin_host.PluginHost, _ error) bool {
		return slices.Contains(hosts, plugin_host.PluginHost(replacement)) && !slices.Contains(hosts, plugin_host.PluginHost(firstHost))
	})

	// Publish a host resolver error without removing the replacement host.
	injected := errors.New("host resolver failed")
	releaseError, err := tb.Bus.AddController(t.Context(), plugin_host_mock.NewLookupErrorController(injected), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the scheduler reports the resolver error with its live host.
	await(func(hosts []plugin_host.PluginHost, err error) bool {
		return errors.Is(err, injected) && slices.Contains(hosts, plugin_host.PluginHost(replacement))
	})

	// Verify withdrawing the failed resolver clears the scheduler error.
	releaseError()
	await(func(_ []plugin_host.PluginHost, err error) bool { return err == nil })

	// Verify the second mount keeps the generation live after the first releases.
	ref.Release()
	await(func(_ []plugin_host.PluginHost, _ error) bool { return true })
}

func TestReserveServicePrefix(t *testing.T) {
	// Prepare a runtime with an empty set of RPC service prefixes.
	c := &Controller{prefixes: make(map[string]struct{})}

	// Reserve the attached service prefix for one mount.
	release, err := c.ReserveServicePrefix("attached/")
	if err != nil {
		t.Fatal(err)
	}

	// Verify an active reservation prevents a second mount using the prefix.
	if _, err := c.ReserveServicePrefix("attached/"); err == nil {
		t.Fatal("a bound prefix was reserved twice")
	}

	// Verify another service prefix remains available.
	if _, err := c.ReserveServicePrefix("other/"); err != nil {
		t.Fatalf("an unbound prefix was refused: %v", err)
	}

	// Verify releasing a reservation makes the service prefix available again.
	release()
	if _, err := c.ReserveServicePrefix("attached/"); err != nil {
		t.Fatalf("a released prefix was refused: %v", err)
	}
}

// newTestbed starts a testbed that stops when the test ends.
func newTestbed(t *testing.T) *testbed.Testbed {
	// Start an in-memory testbed with the plugin volume alias.
	t.Helper()
	tb, err := testbed.WithTestbedOptions(t.Context(), []db_testbed.Option{
		db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
			VolumeConfig: &volume_controller.Config{VolumeIdAlias: []string{bldr_plugin.PluginVolumeID}},
		}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	return tb
}

// newTestConfig returns a runtime config for spaceID on the testbed engine.
func newTestConfig(tb *testbed.Testbed, spaceID string) *Config {
	return &Config{Space: &plugin_space.Config{
		SpaceId:       spaceID,
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}}
}

// waitGeneration waits for rt to run a generation other than prev.
func waitGeneration(t *testing.T, rt *Controller, prev *Generation) *Generation {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		gen, waitCh, _ := rt.GetGeneration()
		if gen != nil && gen != prev {
			return gen
		}
		select {
		case <-waitCh:
		case <-timeout:
			t.Fatal("timed out waiting for a runtime generation")
		}
	}
}
