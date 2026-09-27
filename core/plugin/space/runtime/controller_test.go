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
	tb := newTestbed(t)
	conf := newTestConfig(tb, "space-a")
	first, firstRef, err := StartControllerWithConfig(t.Context(), tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}
	second, secondRef, err := StartControllerWithConfig(t.Context(), tb.Bus, conf.CloneVT())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equal configs started two runtimes")
	}
	gen := waitGeneration(t, first, nil)

	other, otherRef, err := StartControllerWithConfig(t.Context(), tb.Bus, newTestConfig(tb, "space-b"))
	if err != nil {
		t.Fatal(err)
	}
	otherRef.Release()
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
	secondRef.Release()
	select {
	case <-gen.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("releasing the last reference did not stop the runtime")
	}
}

// TestControllerHostsStayLive observes the scheduler while preserving both mounts.
func TestControllerHostsStayLive(t *testing.T) {
	tb := newTestbed(t)
	conf := newTestConfig(tb, "space-a")
	conf.AppPluginIds = []string{"app", "web"}
	rt, ref, err := StartControllerWithConfig(t.Context(), tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	alias := conf.CloneVT()
	alias.AppPluginIds = []string{"web", "app", "app"}
	second, secondRef, err := StartControllerWithConfig(t.Context(), tb.Bus, alias)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondRef.Release)
	if second != rt {
		t.Fatal("canonical declarations did not share runtime")
	}
	different := conf.CloneVT()
	different.AppPluginIds = []string{"other-app"}
	other, otherRef, err := StartControllerWithConfig(t.Context(), tb.Bus, different)
	if err != nil {
		t.Fatal(err)
	}
	otherRef.Release()
	if other == rt {
		t.Fatal("different application declarations shared runtime")
	}
	gen := waitGeneration(t, rt, nil)
	await := func(match func([]plugin_host.PluginHost, error) bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		for {
			hosts, wait, err := gen.GetScheduler().GetHostState()
			if match(hosts, err) {
				break
			}
			if err := wait(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if current, _, err := rt.GetGeneration(); current != gen || err != nil {
			t.Fatal("host change replaced the Space runtime")
		}
	}
	firstHost := plugin_host_mock.NewHost("test/platform")
	releaseHost, err := tb.Bus.AddController(t.Context(), plugin_host_static.NewController([]plugin_host.PluginHost{firstHost}), nil)
	if err != nil {
		t.Fatal(err)
	}
	await(func(hosts []plugin_host.PluginHost, _ error) bool {
		return slices.Contains(hosts, plugin_host.PluginHost(firstHost))
	})
	replacement := plugin_host_mock.NewHost("test/platform")
	releaseHost()
	releaseReplacement, err := tb.Bus.AddController(t.Context(), plugin_host_static.NewController([]plugin_host.PluginHost{replacement}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseReplacement)
	await(func(hosts []plugin_host.PluginHost, _ error) bool {
		return slices.Contains(hosts, plugin_host.PluginHost(replacement)) && !slices.Contains(hosts, plugin_host.PluginHost(firstHost))
	})
	injected := errors.New("host resolver failed")
	releaseError, err := tb.Bus.AddController(t.Context(), plugin_host_mock.NewLookupErrorController(injected), nil)
	if err != nil {
		t.Fatal(err)
	}
	await(func(hosts []plugin_host.PluginHost, err error) bool {
		return errors.Is(err, injected) && slices.Contains(hosts, plugin_host.PluginHost(replacement))
	})
	releaseError()
	await(func(_ []plugin_host.PluginHost, err error) bool { return err == nil })
	ref.Release()
	await(func(_ []plugin_host.PluginHost, _ error) bool { return true })
}

func TestReserveServicePrefix(t *testing.T) {
	c := &Controller{prefixes: make(map[string]struct{})}
	release, err := c.ReserveServicePrefix("attached/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReserveServicePrefix("attached/"); err == nil {
		t.Fatal("a bound prefix was reserved twice")
	}
	if _, err := c.ReserveServicePrefix("other/"); err != nil {
		t.Fatalf("an unbound prefix was refused: %v", err)
	}

	release()
	if _, err := c.ReserveServicePrefix("attached/"); err != nil {
		t.Fatalf("a released prefix was refused: %v", err)
	}
}

// newTestbed starts a testbed that stops when the test ends.
func newTestbed(t *testing.T) *testbed.Testbed {
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
