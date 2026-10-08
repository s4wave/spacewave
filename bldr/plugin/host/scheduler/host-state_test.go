package plugin_host_scheduler

import (
	"errors"
	"testing"

	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestHostChangesPreserveUnaffectedExecution proves a host snapshot change
// replaces the execution only when it changes the host the execution runs on.
func TestHostChangesPreserveUnaffectedExecution(t *testing.T) {
	// Build a scheduler and one installed artifact for the native platform.
	const native = "desktop/linux/amd64"
	c := NewController(logrus.NewEntry(logrus.New()), nil, &Config{})
	_, instance := c.newPluginInstance(pluginReference{pluginID: "core", instanceKey: "space"})
	ref := newTestManifestRef("core", native, 1, "core")
	digest, err := hash.Sum(hash.RecommendedHashType, []byte("core"))
	if err != nil {
		t.Fatal(err)
	}
	ref.ManifestRef.RootRef = block.NewBlockRef(digest)

	// publish delivers a host lookup result the way the instance watcher does.
	publish := func(resErr []error, hosts ...plugin_host.PluginHost) {
		// Publish the snapshot, then run the instance's selection on it.
		t.Helper()
		c.publishHosts(resErr, hosts)
		instance.pluginUpdateMtx.Lock()
		instance.selectInstalledManifestLocked(c.pluginHostsCtr.GetValue())
		instance.pluginUpdateMtx.Unlock()
	}

	// selected returns the arguments of the current execution.
	selected := func() *executePluginArgs {
		t.Helper()
		state := instance.executePluginRoutine.GetState()
		if state == nil {
			t.Fatal("installed artifact lost its selection")
		}
		return state
	}

	// Create the hosts the snapshots below draw from.
	first := &testPluginHost{id: native}
	second := &testPluginHost{id: native}
	web := &testPluginHost{id: "web/js/wasm"}
	other := &testPluginHost{id: "js"}

	// Start the execution on the first native host.
	publish(nil, first)
	release := instance.addManifestSelection(ref)
	defer release()
	running := selected()
	if running.pluginHost != first {
		t.Fatal("artifact did not select the native host")
	}

	// An unused host arriving or a second host of the platform arriving keeps it.
	publish(nil, first, web)
	if selected() != running {
		t.Fatal("unused host arrival replaced the execution")
	}
	publish(nil, web, first, second)
	if selected() != running {
		t.Fatal("same-platform arrival or snapshot reordering replaced the execution")
	}

	// The bus swaps the last host into a removed slot, so removing an unused
	// host lists the older native host after the newer one.
	publish(nil, second, first)
	if selected() != running {
		t.Fatal("snapshot reordering replaced the execution")
	}
	publish(nil, other, second, first)
	if selected() != running {
		t.Fatal("unused host arrival replaced the execution")
	}

	// Removing the selected host selects the remaining host of the platform.
	publish(nil, other, second)
	if selected().pluginHost != second {
		t.Fatal("selected host removal did not reach the remaining host")
	}

	// Replacing the host with another instance of the platform replaces it.
	replacement := &testPluginHost{id: native}
	publish(nil, other, replacement)
	if selected().pluginHost != replacement {
		t.Fatal("same-platform replacement kept the removed host")
	}

	// Removing every compatible host leaves the artifact selected without a host.
	publish(nil, other)
	if state := selected(); state.pluginHost != nil || state.serveAssets {
		t.Fatal("host removal kept a host the execution cannot run on")
	}
}

// TestHostStateReportsResolverErrors proves a resolver error accompanies the
// live hosts and does not withdraw them.
func TestHostStateReportsResolverErrors(t *testing.T) {
	// Publish a host together with a resolver error.
	c := NewController(logrus.NewEntry(logrus.New()), nil, &Config{})
	host := &testPluginHost{id: "js"}
	resolverErr := errors.New("resolver failed")
	c.publishHosts([]error{resolverErr}, []plugin_host.PluginHost{host})

	// The state carries both the host and the error.
	hosts, _, err := c.GetHostState()
	if len(hosts) != 1 || hosts[0] != host {
		t.Fatal("resolver error withdrew the live host")
	}
	if !errors.Is(err, resolverErr) {
		t.Fatalf("host state error is %v, expected the resolver error", err)
	}

	// The next error-free lookup clears the error.
	c.publishHosts(nil, []plugin_host.PluginHost{host})
	if _, _, err := c.GetHostState(); err != nil {
		t.Fatalf("recovered lookup kept the error: %v", err)
	}
}

// TestOrderHostsKeepsPreviousOrder proves the lookup's reordering of surviving
// hosts does not change the published order, and new hosts follow the old ones.
func TestOrderHostsKeepsPreviousOrder(t *testing.T) {
	// Publish three hosts, then list them reversed beside a new fourth host.
	a := &testPluginHost{id: "a"}
	b := &testPluginHost{id: "b"}
	c := &testPluginHost{id: "c"}
	d := &testPluginHost{id: "d"}
	prev := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{a, b, c}}

	// The old hosts keep their order and the new host follows them.
	got := orderHosts(prev, []plugin_host.PluginHost{d, c, b, a})
	want := []plugin_host.PluginHost{a, b, c, d}
	if len(got) != len(want) {
		t.Fatalf("ordered %d hosts, expected %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host %d is out of order", i)
		}
	}

	// Without a previous snapshot the lookup order stands.
	if got := orderHosts(nil, []plugin_host.PluginHost{c, a}); got[0] != c || got[1] != a {
		t.Fatal("first snapshot was reordered")
	}
}
