package plugin_host_scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestInstalledManifestSelection proves explicit installation beats catalog
// order and keeps independent bindings when one installation changes.
func TestInstalledManifestSelection(t *testing.T) {
	// Build a controller with one JavaScript plugin host.
	le := logrus.NewEntry(logrus.New())
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}}
	c := NewController(le, nil, &Config{})
	c.pluginHostsCtr = ccontainer.NewCContainer(hosts)
	_, first := c.newPluginInstance(pluginReference{pluginID: "colors", instanceKey: "first"})
	_, second := c.newPluginInstance(pluginReference{pluginID: "colors", instanceKey: "second"})
	old := newTestManifestRef("colors", "js", 1, "old")
	newer := newTestManifestRef("colors", "js", 99, "newer")

	// Give each manifest ref a resolvable root hash.
	for _, ref := range []*manifest.ManifestRef{old, newer} {
		digest, err := hash.Sum(hash.RecommendedHashType, []byte(ref.GetManifestRef().GetBucketId()))
		if err != nil {
			t.Fatal(err)
		}
		ref.ManifestRef.RootRef = block.NewBlockRef(digest)
	}

	// Install the old artifact on the first binding and the newer artifact
	// on the second binding.
	firstRelease := first.addManifestSelection("peer", old)
	defer firstRelease()
	secondRelease := second.addManifestSelection("peer", newer)
	defer secondRelease()

	// assertSelection fails when the instance's execution state differs.
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
	releaseReplacement := second.addManifestSelection("peer", old)
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
		release := instance.addManifestSelection("peer", installation...)

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

// TestInstalledManifestServesAssets proves a browser artifact without a host
// is served only while a reference demands its files.
func TestInstalledManifestServesAssets(t *testing.T) {
	// Build a controller whose only host cannot run the browser artifact.
	le := logrus.NewEntry(logrus.New())
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "desktop/darwin/arm64"}}}
	c := NewController(le, nil, &Config{})
	c.pluginHostsCtr = ccontainer.NewCContainer(hosts)
	_, instance := c.newPluginInstance(pluginReference{pluginID: "app"})
	release := instance.addManifestSelection("peer", newTestManifestRef("app", "js", 1, "app"))
	defer release()

	// serving reports whether the selection serves the artifact's files.
	serving := func() bool {
		state := instance.executePluginRoutine.GetState()
		if state == nil || state.pluginHost != nil {
			t.Fatal("browser artifact selected a host")
		}
		return state.serveAssets
	}
	if serving() {
		t.Fatal("served files without demand")
	}

	// Demand serves the files until its last release.
	releaseAssets := instance.addAssetsDemand()
	if !serving() {
		t.Fatal("demand did not serve files")
	}
	releaseAssets()
	releaseAssets()
	if serving() {
		t.Fatal("served files after release")
	}
}

// TestInstalledManifestPrefersNative proves a peer with native and JavaScript
// hosts runs the native artifact of an installation whose last deploy repinned
// the JavaScript build in front of it, and never falls back to JavaScript.
func TestInstalledManifestPrefersNative(t *testing.T) {
	// Build the installation in deploy repin order: linux, js, darwin, then
	// the rollback pins of the previous deploy.
	linux := newTestManifestRef("core", "desktop/linux/amd64", 146, "linux")
	js := newTestManifestRef("core", "js", 181, "js")
	darwin := newTestManifestRef("core", "desktop/darwin/arm64", 181, "darwin")
	jsOld := newTestManifestRef("core", "js", 180, "js-old")
	darwinOld := newTestManifestRef("core", "desktop/darwin/arm64", 180, "darwin-old")
	installation := []*manifest.ManifestRef{linux, js, darwin, jsOld, darwinOld}
	for _, ref := range installation {
		digest, err := hash.Sum(hash.RecommendedHashType, []byte(ref.GetManifestRef().GetBucketId()))
		if err != nil {
			t.Fatal(err)
		}
		ref.ManifestRef.RootRef = block.NewBlockRef(digest)
	}

	// Run a darwin peer that also hosts JavaScript plugins.
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{
		&testPluginHost{id: "js"},
		&testPluginHost{id: "desktop/darwin/arm64"},
	}}
	c := NewController(logrus.NewEntry(logrus.New()), nil, &Config{})
	c.pluginHostsCtr = ccontainer.NewCContainer(hosts)
	_, instance := c.newPluginInstance(pluginReference{pluginID: "core", instanceKey: "space"})
	release := instance.addManifestSelection("peer", installation...)
	defer release()

	// The native artifact runs, with only the older native artifact behind it.
	state := instance.executePluginRoutine.GetState()
	if state == nil || !state.manifestSnapshot.GetManifestRef().EqualVT(darwin.GetManifestRef()) {
		t.Fatal("installation did not select the native artifact")
	}
	if len(state.fallbacks) != 1 || !state.fallbacks[0].manifestSnapshot.GetManifestRef().EqualVT(darwinOld.GetManifestRef()) {
		t.Fatal("native artifact fell back to another platform")
	}
}

// TestInstallationRootStaysOutOfCatalog checks that a root which ran only
// because an answerer admitted it for a load's peer stays out of the host
// catalog, so a later plain or pinned load, which asks nothing, cannot start it.
func TestInstallationRootStaysOutOfCatalog(t *testing.T) {
	// Peer P's installation of R runs once its approval answers.
	h := newInstallationHarness(t)
	root := h.manifest(t, 1)
	rootKey := manifestRootKey(root.GetManifestRef())
	h.trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: rootKey})
	release := h.instance.addManifestSelection("peer-p", root)
	defer release()
	h.run(t, h.instance)
	if got := h.nextStart(t); got != rootKey {
		t.Fatalf("started root %s, want %s", got, rootKey)
	}

	// With the approval gone, neither load of the same plugin selects R.
	h.removeApprover()
	for _, pin := range []string{"", rootKey} {
		_, load := h.c.newPluginInstance(pluginReference{pluginID: "colors", manifestRoot: pin})
		h.selectCatalog(t, load)
		if load.executePluginRoutine.GetState() != nil || load.downloadManifestRoutine.GetState() != nil {
			t.Fatalf("load pinned to %q selected the installation's root from the catalog", pin)
		}
	}

	// Once R is in the catalog the pinned load selects it, so the check above
	// observes the catalog and not an empty selection path.
	h.catalog(t, root)
	_, pinned := h.c.newPluginInstance(pluginReference{pluginID: "colors", manifestRoot: rootKey})
	h.selectCatalog(t, pinned)
	if st := pinned.executePluginRoutine.GetState(); st == nil || manifestRootKey(st.manifestSnapshot.GetManifestRef()) != rootKey {
		t.Fatal("pinned load did not select a catalog root")
	}
}

// TestReleasedInstallationStopsWaiting checks that releasing an installation
// while its root awaits approval ends the wait, that a later approval starts
// nothing, and that the binding returns to its catalog demand.
func TestReleasedInstallationStopsWaiting(t *testing.T) {
	// Peer P's installation of R waits for an approval.
	h := newInstallationHarness(t)
	root := h.manifest(t, 1)
	release := h.instance.addManifestSelection("peer-p", root)
	h.run(t, h.instance)
	_, err := h.c.pluginStatusCtr.WaitValueWithValidator(h.ctx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
		for _, st := range snap.GetExecutions() {
			if st.GetSwap().GetAwaitingApproval() != nil {
				return true, nil
			}
		}
		return false, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Releasing the selection ends the wait. Approving R afterwards starts
	// nothing.
	release()
	if h.instance.executePluginRoutine.GetState() != nil {
		t.Fatal("released installation kept its execute state")
	}
	h.trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: manifestRootKey(root.GetManifestRef())})
	h.noStart(t, "approval after release")

	// The binding runs the plugin's catalog manifest again.
	catalog := h.manifest(t, 2)
	h.catalog(t, catalog)
	h.selectCatalog(t, h.instance)
	if got, want := h.nextStart(t), manifestRootKey(catalog.GetManifestRef()); got != want {
		t.Fatalf("started root %s, want catalog root %s", got, want)
	}
}

// TestReleasedInstallationKeepsCatalogWorker checks that releasing an
// installation while its root awaits approval leaves the catalog worker
// beside it untouched, whether it runs the plugin or only serves its files:
// no stop, no restart, and the execute routine returns to that worker's own
// candidate.
func TestReleasedInstallationKeepsCatalogWorker(t *testing.T) {
	for _, serveAssets := range []bool{false, true} {
		t.Run(map[bool]string{false: "runs", true: "serves-assets"}[serveAssets], func(t *testing.T) {
			testReleasedInstallationKeepsCatalogWorker(t, serveAssets)
		})
	}
}

// testReleasedInstallationKeepsCatalogWorker runs one catalog worker mode.
func testReleasedInstallationKeepsCatalogWorker(t *testing.T, serveAssets bool) {
	// Answer approvals through the trust controller on a real bus. None is
	// given.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	rel, err := tb.Bus.AddController(ctx, bldr_manifest_trust.NewController(bldr_manifest_trust.NewSet()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Build a root host binding whose workers start in-process. A running
	// worker also registers, an asset worker only holds its files.
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}}
	c := &Controller{
		le: le, bus: tb.Bus, conf: &Config{PeerId: "root", AdmissionPluginId: "owner"},
		pluginHostsCtr:  ccontainer.NewCContainer(hosts),
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
	type execution struct {
		worker *pluginInstance
		closed chan struct{}
	}
	started := make(chan *execution, 2)
	instance.executions = keyed.NewKeyedRefCount(func(key executionReference) (keyed.Routine, *pluginInstance) {
		_, worker := instance.newExecution(key)
		run := &execution{worker: worker, closed: make(chan struct{})}
		return func(ctx context.Context) error {
			defer close(run.closed)
			defer worker.finishExecution(nil)
			started <- run
			if !key.args.serveAssets {
				worker.updateRpcClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux()))))
				worker.finishInitialCapabilityRegistration(true)
			}
			<-ctx.Done()
			return ctx.Err()
		}, worker
	})
	instance.executions.SetContext(ctx, true)
	defer instance.executions.ClearContext()
	defer instance.clearExecution(nil)
	instance.executePluginRoutine.SetContext(ctx, true)
	defer instance.executePluginRoutine.ClearContext()

	// The catalog root C0 runs, or serves its files, as W0.
	catalog := &executePluginArgs{
		pluginHost: hosts.pluginHosts[0], rev: 1, serveAssets: serveAssets,
		manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: rootedTestManifestRef(t, 1).GetManifestRef()},
	}
	if serveAssets {
		catalog.pluginHost = nil
	}
	instance.setExecutePluginState(catalog)
	var w0 *execution
	select {
	case w0 = <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitSwap(t, ctx, c, func(swap *plugin.PluginSwap) bool { return swap.GetSettled() != nil })
	running := instance.runningPluginCtr.GetValue()

	// Peer P's installation of R1 waits for an approval while W0 is admitted.
	release := instance.addManifestSelection("peer-p", rootedTestManifestRef(t, 2))
	waitSwap(t, ctx, c, func(swap *plugin.PluginSwap) bool { return swap.GetAwaitingApproval() != nil })

	// Releasing it hands the routine back to W0, which is adopted as it is.
	release()
	if instance.executePluginRoutine.GetState() != catalog {
		t.Fatal("released installation did not return the routine to the catalog worker")
	}
	waitSwap(t, ctx, c, func(swap *plugin.PluginSwap) bool { return swap.GetSettled() != nil })
	select {
	case run := <-started:
		t.Fatalf("release started worker %p beside the catalog worker", run.worker)
	case <-w0.closed:
		t.Fatal("release stopped the catalog worker")
	case <-time.After(300 * time.Millisecond):
	}
	instance.pluginUpdateMtx.Lock()
	admitted := instance.activeExecution
	instance.pluginUpdateMtx.Unlock()
	if admitted != w0.worker {
		t.Fatal("release took the catalog worker's admission")
	}
	if instance.runningPluginCtr.GetValue() != running {
		t.Fatal("release replaced the catalog worker's running plugin")
	}
}

// TestReleasedInstallationLeavesGuardedWorker checks that releasing an
// installation of a guarded plugin clears its pending guarded update, so the
// running catalog worker is never asked to quiesce for it.
func TestReleasedInstallationLeavesGuardedWorker(t *testing.T) {
	// Run catalog root C0 on a binding whose plugin guards its updates.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "js"}}}
	c := &Controller{
		le: logrus.NewEntry(logrus.New()), conf: &Config{PeerId: "root", AdmissionPluginId: "owner", UpdateGuardPluginIds: []string{"colors"}},
		pluginHostsCtr: ccontainer.NewCContainer(hosts),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	catalog := &executePluginArgs{
		pluginHost: hosts.pluginHosts[0], rev: 1,
		manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: rootedTestManifestRef(t, 1).GetManifestRef()},
	}
	instance.setExecutePluginState(catalog)
	guard := &countingUpdateGuard{}
	mux := srpc.NewMux()
	if err := plugin.SRPCRegisterUpdateGuard(mux, guard); err != nil {
		t.Fatal(err)
	}
	instance.runningPluginCtr.SetValue(plugin.NewRunningPlugin(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))

	// Selecting R1 routes R1 through the guarded update. Releasing the
	// installation clears the guarded update.
	release := instance.addManifestSelection("peer-p", rootedTestManifestRef(t, 2))
	pending := instance.updatePluginRoutine.GetState()
	if pending == nil || pending.installation == nil {
		t.Fatal("guarded installation did not wait in the update routine")
	}
	release()
	if instance.updatePluginRoutine.GetState() != nil {
		t.Fatal("update routine kept the released installation")
	}
	if instance.executePluginRoutine.GetState() != catalog {
		t.Fatal("release replaced the catalog candidate")
	}

	// A guarded update still holding the released candidate sends no Prepare.
	if err := instance.execGuardedPluginUpdate(ctx, pending); err == nil {
		t.Fatal("released candidate passed the guarded update")
	}
	if n := guard.calls.Load(); n != 0 {
		t.Fatalf("running worker was asked to quiesce %d times for a released installation", n)
	}
}

// countingUpdateGuard counts Prepare calls and permits each one.
type countingUpdateGuard struct {
	calls atomic.Int32
}

// Prepare records the call and permits the replacement.
func (g *countingUpdateGuard) Prepare(context.Context, *plugin.PrepareUpdateRequest) (*plugin.PrepareUpdateResponse, error) {
	g.calls.Add(1)
	return &plugin.PrepareUpdateResponse{}, nil
}

// rootedTestManifestRef returns a colors js reference at rev with a root
// hash derived from rev.
func rootedTestManifestRef(t *testing.T, rev uint64) *manifest.ManifestRef {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{0xb0, byte(rev)})
	if err != nil {
		t.Fatal(err)
	}
	ref := newTestManifestRef("colors", "js", rev, "candidate")
	ref.ManifestRef.RootRef = &block.BlockRef{Hash: h}
	return ref
}

// waitSwap waits until the colors binding's published swap state satisfies ok.
func waitSwap(t *testing.T, ctx context.Context, c *Controller, ok func(*plugin.PluginSwap) bool) {
	t.Helper()
	_, err := c.pluginStatusCtr.WaitValueWithValidator(ctx, func(snap *plugin.PluginStatusSnapshot) (bool, error) {
		for _, st := range snap.GetExecutions() {
			if st.GetPluginId() == "colors" && ok(st.GetSwap()) {
				return true, nil
			}
		}
		return false, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// TestCatalogRemovalReleasesWorker checks that unlinking the last catalog
// manifest the binding may run releases the worker the catalog admitted:
// whether it runs the plugin or only serves its files, when the catalog keeps
// a manifest only quarantined for the plugin, and for a binding pinned to the
// removed root while the plugin keeps another.
func TestCatalogRemovalReleasesWorker(t *testing.T) {
	for _, mode := range []string{"runs", "serves-assets", "quarantined", "pinned"} {
		t.Run(mode, func(t *testing.T) {
			// The catalog's manifest runs, or serves its files, as W0.
			h := newInstallationHarness(t)
			load := h.instance
			ref := h.manifest(t, 1)
			h.catalog(t, ref)
			switch mode {
			case "serves-assets":
				h.hosts = &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "desktop/darwin/arm64"}}}
				defer h.instance.addAssetsDemand()()
			case "quarantined":
				// A shapes manifest linked as colors reads as quarantined.
				other := h.manifestOf(t, "shapes", 1)
				key := manifest.NewManifestKey(installationHostKey, other.GetMeta())
				if err := manifest_world.ExStoreManifestOp(h.ctx, h.ws, "", key, []string{installationHostKey}, other); err != nil {
					t.Fatal(err)
				}
				if err := h.ws.SetGraphQuad(h.ctx, manifest_world.NewManifestQuad(installationHostKey, key, "colors")); err != nil {
					t.Fatal(err)
				}
			case "pinned":
				h.catalog(t, h.manifest(t, 2))
				load = h.pinned(t, manifestRootKey(ref.GetManifestRef()))
			}
			h.run(t, load)
			h.selectCatalog(t, load)
			h.awaitAdmitted(t, load)

			// Unlinking it leaves no manifest the binding may run, which
			// releases W0 and publishes that nothing runs.
			key := manifest.NewManifestKey(installationHostKey, ref.GetMeta())
			if err := manifest_world.UnlinkManifest(h.ctx, h.ws, key, installationHostKey); err != nil {
				t.Fatal(err)
			}
			h.selectCatalog(t, load)
			if load.executePluginRoutine.GetState() != nil || admitted(load) != nil {
				t.Fatal("catalog removal kept the catalog worker")
			}
			if h.runningRef(load) != "" {
				t.Fatal("catalog removal kept the running manifest")
			}
		})
	}
}

// TestCatalogHostLossKeepsWorker checks that losing the host of the running
// manifest keeps the catalog worker, both when another host remains and when
// none does, and that the host's return restarts nothing.
func TestCatalogHostLossKeepsWorker(t *testing.T) {
	// The catalog manifest runs on the js host as W0.
	h := newInstallationHarness(t)
	h.catalog(t, h.manifest(t, 1))
	h.run(t, h.instance)
	h.selectCatalog(t, h.instance)
	h.nextStart(t)
	w0 := h.awaitAdmitted(t, h.instance)
	state := h.instance.executePluginRoutine.GetState()

	// Each flap leaves the manifest with no present host, then restores it.
	up := h.hosts
	for _, flap := range []*pluginHostSet{
		{pluginHosts: []plugin_host.PluginHost{&testPluginHost{id: "desktop/darwin/arm64"}}},
		{},
	} {
		h.hosts = flap
		h.selectCatalog(t, h.instance)
		h.hosts = up
		h.selectCatalog(t, h.instance)
		if h.instance.executePluginRoutine.GetState() != state || admitted(h.instance) != w0 {
			t.Fatalf("host flap to %d hosts replaced the catalog worker", len(flap.pluginHosts))
		}
	}
	h.noStart(t, "host return")
}

// TestInstallationWorkerSurvivesEmptyCatalog checks that a catalog holding no
// manifest of the plugin leaves an installation's worker alone.
func TestInstallationWorkerSurvivesEmptyCatalog(t *testing.T) {
	// Peer P's installation of R runs as W.
	h := newInstallationHarness(t)
	root := h.manifest(t, 1)
	h.trusted.Add(bldr_manifest_trust.Root{PluginID: "colors", ManifestRoot: manifestRootKey(root.GetManifestRef())})
	release := h.instance.addManifestSelection("peer-p", root)
	defer release()
	h.run(t, h.instance)
	h.nextStart(t)
	w := h.awaitAdmitted(t, h.instance)
	state := h.instance.executePluginRoutine.GetState()

	// Selecting from the empty catalog changes nothing.
	h.selectCatalog(t, h.instance)
	if h.instance.executePluginRoutine.GetState() != state || admitted(h.instance) != w {
		t.Fatal("empty catalog released the installation's worker")
	}
	h.noStart(t, "empty catalog")
}

// TestReleasedEmptyInstallationReselectsCatalog checks that releasing the last
// installation returns the binding to its catalog even when the installation
// never placed a candidate: an empty catalog pass during it released nothing,
// and the release must not leave the catalog worker on that pass's decision.
func TestReleasedEmptyInstallationReselectsCatalog(t *testing.T) {
	// The catalog manifest runs as W0.
	h := newInstallationHarness(t)
	ref := h.manifest(t, 1)
	h.catalog(t, ref)
	h.run(t, h.instance)
	h.selectCatalog(t, h.instance)
	h.awaitAdmitted(t, h.instance)

	// An installation with no artifacts places no candidate, and the empty
	// catalog pass it overlaps releases nothing.
	release := h.instance.addManifestSelection("peer-p")
	key := manifest.NewManifestKey(installationHostKey, ref.GetMeta())
	if err := manifest_world.UnlinkManifest(h.ctx, h.ws, key, installationHostKey); err != nil {
		t.Fatal(err)
	}
	h.selectCatalog(t, h.instance)
	if admitted(h.instance) == nil {
		t.Fatal("the catalog released its worker while an installation was selected")
	}

	// Releasing the installation reruns the catalog pass, which releases W0.
	release()
	h.selectCatalog(t, h.instance)
	if h.instance.executePluginRoutine.GetState() != nil || admitted(h.instance) != nil {
		t.Fatal("released installation left the catalog worker of an empty catalog")
	}
}

// awaitAdmitted waits until load publishes a running manifest, which it does
// in the critical section that admits a worker, and returns that worker.
func (h *installationHarness) awaitAdmitted(t *testing.T, load *pluginInstance) *pluginInstance {
	t.Helper()
	_, err := h.c.pluginStatusCtr.WaitValueWithValidator(h.ctx, func(*plugin.PluginStatusSnapshot) (bool, error) {
		return h.runningRef(load) != "", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return admitted(load)
}

// runningRef returns the running manifest load last published.
func (h *installationHarness) runningRef(load *pluginInstance) string {
	for _, status := range h.c.pluginStatusCtr.GetValue().GetExecutions() {
		if status.GetPluginId() == load.pluginID && status.GetInstanceKey() == load.instanceKey {
			return status.GetRunningManifestRef()
		}
	}
	return ""
}

// admitted returns the worker load admitted.
func admitted(load *pluginInstance) *pluginInstance {
	load.pluginUpdateMtx.Lock()
	defer load.pluginUpdateMtx.Unlock()
	return load.activeExecution
}

// pinned adds a colors binding pinned to root and readies its executions.
func (h *installationHarness) pinned(t *testing.T, root string) *pluginInstance {
	t.Helper()
	key := pluginReference{pluginID: "colors", manifestRoot: root}
	_, load := h.c.newPluginInstance(key)
	h.c.setPluginStatus("colors", key.executionKey(), plugin.PluginState_PluginState_REQUESTED)
	load.executions.SetContext(h.ctx, true)
	t.Cleanup(func() { load.executions.ClearContext() })
	t.Cleanup(func() { load.clearExecution(nil) })
	return load
}

// installationHarness runs one colors binding on a root host scheduler: an
// installation asks AdmitPluginRoot for its own peer, a catalog load does not.
type installationHarness struct {
	ctx context.Context
	le  *logrus.Entry
	ws  world.WorldState
	c   *Controller
	// instance is the plain colors binding.
	instance *pluginInstance
	hosts    *pluginHostSet
	// started receives the root of each plugin the host starts.
	started chan string
	// trusted answers AdmitPluginRoot until removeApprover is called.
	trusted        *bldr_manifest_trust.Set
	removeApprover func()
}

// installationHostKey is the plugin host object holding the harness catalog.
const installationHostKey = "plugin-host"

// newInstallationHarness builds the harness on a fresh testbed and World.
func newInstallationHarness(t *testing.T) *installationHarness {
	// Answer approvals through the trust controller on a real bus.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	trusted := bldr_manifest_trust.NewSet()
	rel, err := tb.Bus.AddController(ctx, bldr_manifest_trust.NewController(trusted), nil)
	if err != nil {
		t.Fatal(err)
	}
	removeApprover := sync.OnceFunc(rel)
	t.Cleanup(removeApprover)

	// Resolve every plugin host root lookup with one Root.
	hostRoot := plugin_host_root.NewRoot()
	rootCtrl := callback.NewCallbackController(
		controller.NewInfo("test/plugin-host-root", controller.MustParseVersion("0.0.1"), ""),
		nil,
		func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			if _, ok := inst.GetDirective().(plugin_host_root.LookupRoot); !ok {
				return nil, nil
			}
			return directive.R(directive.NewValueResolver([]plugin_host_root.LookupRootValue{hostRoot}), nil)
		},
		nil,
	)
	releaseRoot, err := tb.Bus.AddController(ctx, rootCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseRoot)

	// Open a World with an empty catalog for the host.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest_world.CreateManifestStore(ctx, ws, installationHostKey); err != nil {
		t.Fatal(err)
	}

	// Build the root host scheduler and its colors binding.
	started := make(chan string, 4)
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{&startedPluginHost{testPluginHost: testPluginHost{id: "js"}, started: started}}}
	var wsv world.WorldState = ws
	c := &Controller{
		le: le, bus: tb.Bus, conf: &Config{PeerId: "root", AdmissionPluginId: "owner", WatchFetchManifest: true}, objKey: installationHostKey,
		worldStateCtr: ccontainer.NewCContainer(wsv),
		hostVolumeCtr: ccontainer.NewCContainer(&hostVol{
			vol:  tb.Volume,
			info: &volume.VolumeInfo{VolumeId: tb.Volume.GetID()},
		}),
		pluginHostsCtr:  ccontainer.NewCContainer(hosts),
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
	instance.executions.SetContext(ctx, true)
	t.Cleanup(func() { instance.executions.ClearContext() })
	t.Cleanup(func() { instance.clearExecution(nil) })
	return &installationHarness{
		ctx: ctx, le: le, ws: ws, c: c, instance: instance, hosts: hosts,
		started: started, trusted: trusted, removeApprover: removeApprover,
	}
}

// manifest writes a runnable colors manifest at rev into the World, outside
// any catalog, and returns its reference.
func (h *installationHarness) manifest(t *testing.T, rev uint64) *manifest.ManifestRef {
	t.Helper()
	return h.manifestOf(t, "colors", rev)
}

// manifestOf writes a runnable js manifest of id at rev into the World,
// outside any catalog, and returns its reference.
func (h *installationHarness) manifestOf(t *testing.T, id string, rev uint64) *manifest.ManifestRef {
	// Write the dist and the manifest blocks.
	t.Helper()
	distFS := memfs.New()
	if err := billy_util.WriteFile(distFS, "colors.js", []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := manifest.NewManifest(manifest.NewManifestMeta(id, manifest.BuildType_RELEASE, "js", rev), "colors.js")
	ref, err := world.AccessObject(h.ctx, h.ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return manifest.CreateManifestWithBilly(h.ctx, bcs, m, distFS, nil, timestamppb.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest.NewManifestRef(m.GetMeta(), ref)
}

// catalog stores ref in the host catalog.
func (h *installationHarness) catalog(t *testing.T, ref *manifest.ManifestRef) {
	t.Helper()
	key := manifest.NewManifestKey(installationHostKey, ref.GetMeta())
	if err := manifest_world.ExStoreManifestOp(h.ctx, h.ws, "", key, []string{installationHostKey}, ref); err != nil {
		t.Fatal(err)
	}
}

// selectCatalog runs one catalog selection pass for load.
func (h *installationHarness) selectCatalog(t *testing.T, load *pluginInstance) {
	// Load the host object, releasing it before returning.
	t.Helper()
	obj, found, err := h.ws.GetObject(h.ctx, installationHostKey)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		t.Fatalf("host object: %t, %v", found, err)
	}
	if _, _, err := load.processManifestWorldState(h.ctx, h.le, h.hosts, h.ws, obj); err != nil {
		t.Fatal(err)
	}
}

// run starts the execute routine of load.
func (h *installationHarness) run(t *testing.T, load *pluginInstance) {
	load.executePluginRoutine.SetContext(h.ctx, true)
	t.Cleanup(func() { load.executePluginRoutine.ClearContext() })
}

// nextStart waits for the host to start a plugin and returns its root.
func (h *installationHarness) nextStart(t *testing.T) string {
	t.Helper()
	select {
	case root := <-h.started:
		return root
	case <-h.ctx.Done():
		t.Fatal(h.ctx.Err())
		return ""
	}
}

// noStart asserts that the host starts no plugin for a while.
func (h *installationHarness) noStart(t *testing.T, phase string) {
	t.Helper()
	select {
	case root := <-h.started:
		t.Fatalf("%s: host started root %s", phase, root)
	case <-time.After(300 * time.Millisecond):
	}
}

// startedPluginHost reports each started root, then runs it until released.
type startedPluginHost struct {
	testPluginHost
	started chan<- string
}

// ExecutePlugin reports manifestRoot and blocks until ctx ends.
func (h *startedPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	executionKey,
	manifestRoot,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	select {
	case h.started <- manifestRoot:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	<-ctx.Done()
	return context.Cause(ctx)
}
