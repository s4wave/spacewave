package plugin_host_scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// pinnedFixture is a World manifest store and a plugin host that runs "js".
type pinnedFixture struct {
	ctx   context.Context
	le    *logrus.Entry
	ws    world.WorldState
	hosts *pluginHostSet
}

// pinnedHostKey is the fixture's plugin-host manifest store.
const pinnedHostKey = "plugin-host"

// newPinnedFixture opens a World testbed with an empty manifest store.
func newPinnedFixture(t *testing.T) *pinnedFixture {
	// Bind a bounded context and logger for the test.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())

	// Open a World testbed and an empty cursor over it.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)

	// Build a mock World state with the plugin-host manifest store.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest_world.CreateManifestStore(ctx, ws, pinnedHostKey); err != nil {
		t.Fatal(err)
	}
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}, settled: true}
	return &pinnedFixture{ctx: ctx, le: le, ws: ws, hosts: hosts}
}

// store links ref into the host's manifest store under key.
func (f *pinnedFixture) store(t *testing.T, key string, ref *manifest.ManifestRef) {
	t.Helper()
	if err := manifest_world.ExStoreManifestOp(f.ctx, f.ws, "", key, []string{pinnedHostKey}, ref); err != nil {
		t.Fatal(err)
	}
}

// pinned builds the "colors" plugin instance pinned to root.
func (f *pinnedFixture) pinned(root string) *pluginInstance {
	return &pluginInstance{
		c: &Controller{conf: &Config{WatchFetchManifest: true, DisableStoreManifest: true}, objKey: pinnedHostKey}, le: f.le, pluginID: "colors",
		manifestRoot:            root,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](f.le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, f.le),
		pluginLoadStateCtr:      ccontainer.NewCContainer[bldr_plugin.PluginLoadState](bldr_plugin.NewPluginLoadState(nil, bldr_plugin.InitialCapabilityRegistrationPending)),
		runningPluginCtr:        ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}
}

// selectManifest runs instance's manifest selection over the stored host object.
func (f *pinnedFixture) selectManifest(t *testing.T, instance *pluginInstance) {
	// Load the host object, releasing it before returning.
	t.Helper()
	obj, found, err := f.ws.GetObject(f.ctx, pinnedHostKey)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("host object: %t, %v", found, err)
	}

	// Run selection over the loaded host object.
	if _, _, err := instance.processManifestWorldState(f.ctx, f.le, f.hosts, f.ws, obj); err != nil {
		t.Fatal(err)
	}
}

// TestPinnedManifestRetainsExactRevision exercises immutable selection through stored World manifests.
func TestPinnedManifestRetainsExactRevision(t *testing.T) {
	// Store two executable revisions under the same plugin host.
	f := newPinnedFixture(t)
	old, _ := storeTestWorldManifest(t, f.ctx, f.ws, "colors", "js", 1)
	newer, _ := storeTestWorldManifest(t, f.ctx, f.ws, "colors", "js", 2)
	oldKey := manifest.NewManifestKey(pinnedHostKey, old.GetMeta())
	for _, ref := range []*manifest.ManifestRef{old, newer} {
		f.store(t, manifest.NewManifestKey(pinnedHostKey, ref.GetMeta()), ref)
	}

	// An unrelated retained revision lives in a closed Space. Exact lookup must
	// filter its identity without opening that unavailable bucket.
	closed, _ := storeTestWorldManifest(t, f.ctx, f.ws, "colors", "js", 3)
	closed.GetManifestRef().BucketId = "closed-space"
	f.store(t, "a-closed-artifact", closed)

	// A newer installed revision does not alter the pinned execution.
	instance := f.pinned(old.GetManifestRef().GetRootRef().GetHash().MarshalString())
	f.selectManifest(t, instance)
	selected := instance.executePluginRoutine.GetState()
	if selected == nil || !selected.manifestSnapshot.GetManifestRef().EqualVT(old.GetManifestRef()) {
		t.Fatalf("pinned selection = %#v, want %s", selected, old.GetManifestRef().String())
	}

	// Losing the exact artifact is unavailable, even while the newer one remains.
	if err := f.ws.DeleteGraphQuad(f.ctx, manifest_world.NewManifestQuad(pinnedHostKey, oldKey, "colors")); err != nil {
		t.Fatal(err)
	}
	f.selectManifest(t, instance)
	if instance.executePluginRoutine.GetState() != nil {
		t.Fatal("missing pinned artifact fell back to another executable")
	}
	assertPinnedTransient(t, instance)

	// Restoring the artifact makes the same pinned request usable without new code.
	if err := f.ws.SetGraphQuad(f.ctx, manifest_world.NewManifestQuad(pinnedHostKey, oldKey, "colors")); err != nil {
		t.Fatal(err)
	}
	f.selectManifest(t, instance)
	selected = instance.executePluginRoutine.GetState()
	if selected == nil || !selected.manifestSnapshot.GetManifestRef().EqualVT(old.GetManifestRef()) {
		t.Fatal("restored pinned artifact was not selected")
	}
}

// TestPinnedManifestMissingIsProven fails a pinned load as missing only when
// the host set is complete and the manifest at the root was read whole and
// names a platform no host runs. A host set taken while the lookup is busy or
// failing, or a root whose block cannot be read, stays a failure that may pass.
func TestPinnedManifestMissingIsProven(t *testing.T) {
	// Store a web-only build while only a native host is registered.
	f := newPinnedFixture(t)
	web, _ := storeTestWorldManifest(t, f.ctx, f.ws, "colors", "js", 1)
	f.store(t, manifest.NewManifestKey(pinnedHostKey, web.GetMeta()), web)
	root := web.GetManifestRef().GetRootRef().GetHash().MarshalString()
	native := []plugin_host.PluginHost{&testPluginHost{id: "desktop/darwin/arm64"}}

	// A host set taken before the lookup settled may still lack the web host.
	f.hosts = &pluginHostSet{pluginHosts: native}
	instance := f.pinned(root)
	f.selectManifest(t, instance)
	assertPinnedTransient(t, instance)

	// A host set carrying a resolver error may lack the failing host.
	f.hosts = &pluginHostSet{pluginHosts: native, err: errors.New("host resolver failed"), settled: true}
	instance = f.pinned(root)
	f.selectManifest(t, instance)
	assertPinnedTransient(t, instance)

	// A settled, error-free set proves the pinned root has no build here.
	f.hosts = &pluginHostSet{pluginHosts: native, settled: true}
	instance = f.pinned(root)
	f.selectManifest(t, instance)
	state := instance.pluginLoadStateCtr.GetValue()
	var missing *bldr_plugin.PinnedManifestMissingError
	if !errors.As(state.GetStartupError(), &missing) || missing.ManifestRoot != root || missing.PluginID != "colors" {
		t.Fatalf("web-only pinned build: startup error %v, want PinnedManifestMissingError", state.GetStartupError())
	}

	// Plant a "js" build whose root block was never written.
	absent, err := hash.Sum(hash.RecommendedHashType, []byte("absent manifest block"))
	if err != nil {
		t.Fatal(err)
	}
	unread, _ := storeTestWorldManifest(t, f.ctx, f.ws, "colors", "js", 2)
	unread.GetManifestRef().GetRootRef().Hash = absent
	f.store(t, "an-unread-artifact", unread)

	// A root that could not be read is not proven missing: the load waits.
	f.hosts = &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}, settled: true}
	instance = f.pinned(absent.MarshalString())
	f.selectManifest(t, instance)
	if instance.executePluginRoutine.GetState() != nil {
		t.Fatal("unreadable pinned root selected an executable")
	}
	assertPinnedTransient(t, instance)
}

// assertPinnedTransient fails unless instance's load failed with no cause, the
// state that waits for the World or its storage to change.
func assertPinnedTransient(t *testing.T, instance *pluginInstance) {
	t.Helper()
	state := instance.pluginLoadStateCtr.GetValue()
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationFailed || state.GetStartupError() != nil {
		t.Fatalf("pinned load = %v, %v; want a failure with no cause", state.GetInitialCapabilityRegistrationState(), state.GetStartupError())
	}
}
