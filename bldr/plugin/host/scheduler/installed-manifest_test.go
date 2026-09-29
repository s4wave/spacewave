package plugin_host_scheduler

import (
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestInstalledManifestSelection proves explicit installation beats catalog
// order and keeps independent bindings when one installation changes.
func TestInstalledManifestSelection(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}}
	c := NewController(le, nil, &Config{})
	c.pluginHostsCtr = ccontainer.NewCContainer(hosts)
	_, first := c.newPluginInstance(pluginReference{pluginID: "colors", instanceKey: "first"})
	_, second := c.newPluginInstance(pluginReference{pluginID: "colors", instanceKey: "second"})
	old := newTestManifestRef("colors", "js", 1, "old")
	newer := newTestManifestRef("colors", "js", 99, "newer")
	for _, ref := range []*manifest.ManifestRef{old, newer} {
		digest, err := hash.Sum(hash.RecommendedHashType, []byte(ref.GetManifestRef().GetBucketId()))
		if err != nil {
			t.Fatal(err)
		}
		ref.ManifestRef.RootRef = block.NewBlockRef(digest)
	}
	firstRelease := first.addManifestSelection(old)
	defer firstRelease()
	secondRelease := second.addManifestSelection(newer)
	defer secondRelease()
	assertSelection := func(instance *pluginInstance, expected *manifest.ManifestRef) {
		t.Helper()
		state := instance.executePluginRoutine.GetState()
		if state == nil || !state.manifestSnapshot.GetManifestRef().EqualVT(expected.GetManifestRef()) {
			t.Fatal("installation selected a different artifact")
		}
	}
	assertSelection(first, old)
	assertSelection(second, newer)

	// A later catalog result cannot advance the selected installation.
	if first.setExecutePluginState(&executePluginArgs{
		pluginHost: hosts.pluginHosts[0], manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: newer.GetManifestRef()},
	}) || first.setExecutePluginState(nil) {
		t.Fatal("catalog update replaced an explicit installation")
	}
	assertSelection(first, old)

	// A new retained demand may deliberately select an older numeric revision.
	releaseReplacement := second.addManifestSelection(old)
	assertSelection(second, old)
	assertSelection(first, old)
	if second.acceptsManifest(&executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: newer.GetManifestRef()}}) {
		t.Fatal("superseded preparation remained admissible")
	}
	releaseReplacement()
	releaseReplacement()
	assertSelection(second, newer)
}

// TestInstalledManifestSkipsForeignPlatforms proves one installation spanning
// platforms gives each peer the newest artifact its own host can run, and that
// only the artifact of the same platform is a fallback.
func TestInstalledManifestSkipsForeignPlatforms(t *testing.T) {
	// Build the installation: a newer linux artifact, its predecessor, and darwin.
	linuxNew := newTestManifestRef("core", "desktop/linux/amd64", 3, "linux-new")
	darwin := newTestManifestRef("core", "desktop/darwin/arm64", 2, "darwin")
	linuxOld := newTestManifestRef("core", "desktop/linux/amd64", 1, "linux-old")
	installation := []*manifest.ManifestRef{linuxNew, darwin, linuxOld}
	for _, ref := range installation {
		digest, err := hash.Sum(hash.RecommendedHashType, []byte(ref.GetManifestRef().GetBucketId()))
		if err != nil {
			t.Fatal(err)
		}
		ref.ManifestRef.RootRef = block.NewBlockRef(digest)
	}

	// Each peer selects the newest artifact for its own platform.
	for _, test := range []struct {
		platform  string
		selected  *manifest.ManifestRef
		fallbacks []*manifest.ManifestRef
	}{
		{"desktop/linux/amd64", linuxNew, []*manifest.ManifestRef{linuxOld}},
		{"desktop/darwin/arm64", darwin, nil},
	} {
		// Run a scheduler whose only host is on the peer's platform.
		hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: test.platform}}}
		c := NewController(logrus.NewEntry(logrus.New()), nil, &Config{})
		c.pluginHostsCtr = ccontainer.NewCContainer(hosts)
		_, instance := c.newPluginInstance(pluginReference{pluginID: "core", instanceKey: "space"})
		release := instance.addManifestSelection(installation...)

		// Check the selected artifact and its fallbacks.
		state := instance.executePluginRoutine.GetState()
		if state == nil || !state.manifestSnapshot.GetManifestRef().EqualVT(test.selected.GetManifestRef()) {
			t.Fatalf("%s peer selected an artifact it cannot run", test.platform)
		}
		if len(state.fallbacks) != len(test.fallbacks) {
			t.Fatalf("%s peer has %d fallbacks, expected %d", test.platform, len(state.fallbacks), len(test.fallbacks))
		}
		for i, fallback := range state.fallbacks {
			if !fallback.manifestSnapshot.GetManifestRef().EqualVT(test.fallbacks[i].GetManifestRef()) {
				t.Fatalf("%s peer fell back to the wrong artifact", test.platform)
			}
		}
		release()
	}
}
