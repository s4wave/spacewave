package plugin_host_scheduler

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	block "github.com/s4wave/spacewave/db/block"
	bucket "github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestManifestSelectionPlatformPreference checks the same platform ordering
// through direct fetches and persisted World manifests.
func TestManifestSelectionPlatformPreference(t *testing.T) {
	for _, tc := range []struct {
		name      string
		platforms []string
		want      string
	}{
		{"browser", []string{"web/js/wasm", "js"}, "js"},
		{"desktop", []string{"js", "desktop/darwin/arm64"}, "desktop/darwin/arm64"},
		{"web-only", []string{"web/js/wasm"}, "web/js/wasm"},
	} {
		for _, persisted := range []bool{false, true} {
			mode := "direct"
			if persisted {
				mode = "world"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				// Use the real World representation for persisted selection.
				ctx := t.Context()
				le := logrus.NewEntry(logrus.New())

				// Open a World testbed and a mock World state over it.
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
				ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
				if err != nil {
					t.Fatal(err)
				}

				// Create the plugin-host manifest store.
				const hostKey = "plugin-host"
				if _, err := manifest_world.CreateManifestStore(ctx, ws, hostKey); err != nil {
					t.Fatal(err)
				}

				// Offer all platforms together, independent of their arrival order.
				hosts := &pluginHostSet{}
				refs := make([]*manifest.ManifestRef, 0, len(tc.platforms))
				for _, platform := range tc.platforms {
					hosts.pluginHosts = append(hosts.pluginHosts, &testPluginHost{id: platform})
					ref, key := storeTestWorldManifest(t, ctx, ws, "spacewave-core", platform, 7)
					refs = append(refs, ref)
					if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, key, "spacewave-core")); err != nil {
						t.Fatal(err)
					}
				}
				instance := &pluginInstance{
					c:                       &Controller{conf: &Config{}, objKey: hostKey},
					le:                      le,
					pluginID:                "spacewave-core",
					downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
					executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
				}
				if persisted {
					obj, found, err := ws.GetObject(ctx, hostKey)
					defer world.ReleaseObjectState(obj)
					if err != nil || !found {
						t.Fatalf("host object: found=%t, error=%v", found, err)
					}
					if _, _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
						t.Fatal(err)
					}
				}
				if !persisted {
					handler := instance.newDirectFetchHandler(ctx, hosts)
					handler.HandleValueAdded(nil, directive.NewAttachedValue(1, manifest.NewFetchManifestValue(refs)))
				}

				// Check the chosen execution host, not only the comparator result.
				selected := instance.executePluginRoutine.GetState()
				if selected == nil || selected.pluginHost.GetPlatformId() != tc.want {
					t.Fatalf("selected %v, want platform %s", selected, tc.want)
				}
			})
		}
	}
}

// TestWorldManifestSelectionKeepsCurrentPlatform exercises late platform
// arrival, a newer release, and removal of the current candidate.
func TestWorldManifestSelectionKeepsCurrentPlatform(t *testing.T) {
	// Start with a valid web-only manifest and both available browser hosts.

	// Bind the test context and logger.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Open a World testbed and a mock World state over it.
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
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create the plugin-host manifest store and both browser hosts.
	const hostKey = "plugin-host"
	if _, err := manifest_world.CreateManifestStore(ctx, ws, hostKey); err != nil {
		t.Fatal(err)
	}
	jsHost := &testPluginHost{id: "js"}
	webHost := &testPluginHost{id: "web/js/wasm"}
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{jsHost, webHost}}

	// Build the plugin instance bound to the host object.
	instance := &pluginInstance{
		c:                       &Controller{conf: &Config{}, objKey: hostKey},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// selectManifest loads the host object and runs World selection over it.
	selectManifest := func() *executePluginArgs {
		// Load the host object, releasing it before returning.
		t.Helper()
		obj, found, err := ws.GetObject(ctx, hostKey)
		defer world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("host object: found=%t, error=%v", found, err)
		}

		// Run selection over the loaded host object.
		if _, _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
			t.Fatal(err)
		}
		return instance.executePluginRoutine.GetState()
	}
	addManifest := func(platform string, rev uint64) (*manifest.ManifestRef, string) {
		t.Helper()
		ref, key := storeTestWorldManifest(t, ctx, ws, "spacewave-core", platform, rev)
		if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, key, "spacewave-core")); err != nil {
			t.Fatal(err)
		}
		return ref, key
	}
	_, _ = addManifest("web/js/wasm", 7)
	current := selectManifest()
	if current == nil || current.pluginHost != webHost {
		t.Fatal("initial web-only manifest was not selected")
	}

	// A late alternative for the same revision must not restart active work.
	_, _ = addManifest("js", 7)
	if selected := selectManifest(); selected != current {
		t.Fatal("late same-revision platform replaced the active plugin")
	}

	// A newer supported release must still replace the running generation.
	newer, newerKey := addManifest("js", 8)
	selected := selectManifest()
	if selected == nil || selected.pluginHost != jsHost || !selected.manifestSnapshot.ManifestRef.EqualVT(newer.ManifestRef) {
		t.Fatal("newer JavaScript release did not replace the current generation")
	}

	// Removing the admitted candidate must allow a remaining valid platform.
	if err := ws.DeleteGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, newerKey, "spacewave-core")); err != nil {
		t.Fatal(err)
	}
	if selected := selectManifest(); selected == nil || selected.pluginHost != webHost {
		t.Fatal("removed current candidate prevented fallback to the remaining platform")
	}

	// Match staging's arrival order: JavaScript starts before the web variant.
	_, _ = addManifest("js", 9)
	current = selectManifest()
	if current == nil || current.pluginHost != jsHost {
		t.Fatal("new JavaScript generation was not selected")
	}
	_, _ = addManifest("web/js/wasm", 9)
	if selectManifest() != current {
		t.Fatal("late web variant replaced the active JavaScript generation")
	}
}

// TestWorldManifestSelectionSkipsIncompatible checks that World selection
// falls back past manifests whose startup proved a protocol mismatch, and
// keeps the current target when no compatible manifest remains.
func TestWorldManifestSelectionSkipsIncompatible(t *testing.T) {
	// Bind the test context and logger.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Open a World testbed over the bounded context.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Build a mock World state over an empty testbed cursor.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create the plugin-host manifest store and both hosts.
	const hostKey = "plugin-host"
	if _, err := manifest_world.CreateManifestStore(ctx, ws, hostKey); err != nil {
		t.Fatal(err)
	}
	jsHost := &testPluginHost{id: "js"}
	webHost := &testPluginHost{id: "web/js/wasm"}
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{jsHost, webHost}}

	// Build the plugin instance bound to the host object.
	instance := &pluginInstance{
		c:                       &Controller{conf: &Config{}, objKey: hostKey},
		le:                      le,
		pluginID:                "web",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Store one manifest per platform revision under the host.
	for rev, platform := range []string{"web/js/wasm", "js"} {
		_, key := storeTestWorldManifest(t, ctx, ws, "web", platform, uint64(rev+1))
		if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, key, "web")); err != nil {
			t.Fatal(err)
		}
	}

	// Select again the way execution does after it rejects a manifest.
	selectManifest := func() *executePluginArgs {
		// Reset the selection fingerprint so selection reruns fully.
		t.Helper()
		instance.manifestSelectionFingerprint.Store(nil)

		// Load the host object, releasing it before returning.
		obj, found, err := ws.GetObject(ctx, hostKey)
		defer world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("host object: found=%t, error=%v", found, err)
		}

		// Run selection over the loaded host object.
		if _, _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
			t.Fatal(err)
		}
		return instance.executePluginRoutine.GetState()
	}

	// reject marks a manifest's root as incompatible for future selection.
	reject := func(args *executePluginArgs) {
		ref := args.manifestSnapshot.GetManifestRef()
		instance.incompatibleManifests.Store(manifestRootKey(ref), struct{}{})
	}

	// The newest release is selected first and then rejected.
	newer := selectManifest()
	if newer == nil || newer.pluginHost != jsHost {
		t.Fatal("newest release was not selected")
	}
	reject(newer)

	// Selection falls back to the older release, and rejecting it too keeps it.
	older := selectManifest()
	if older == nil || older.pluginHost != webHost {
		t.Fatal("selection did not fall back past the incompatible release")
	}
	reject(older)
	if selectManifest() != older {
		t.Fatal("rejecting every release cleared the current target")
	}
}

// TestDirectManifestSelectionKeepsCurrentPlatform protects a running fallback
// from same-revision arrivals while permitting an upgrade.
func TestDirectManifestSelectionKeepsCurrentPlatform(t *testing.T) {
	// Build the instance with both platform hosts.
	le := logrus.NewEntry(logrus.New())
	fallback := &testPluginHost{id: "web/js/wasm"}
	preferred := &testPluginHost{id: "js"}
	instance := &pluginInstance{
		c:                       &Controller{conf: &Config{}},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Build a direct fetch handler over both hosts.
	handler := instance.newDirectFetchHandler(t.Context(), &pluginHostSet{
		pluginHosts: []plugin_host.PluginHost{fallback, preferred},
	})

	// add delivers one manifest ref for a platform and revision.
	add := func(id uint32, platform string, rev uint64) {
		t.Helper()
		ref := newTestManifestRef("spacewave-core", platform, rev, "bucket")
		handler.HandleValueAdded(nil, directive.NewAttachedValue(id, manifest.NewFetchManifestValue([]*manifest.ManifestRef{ref})))
	}

	// The browser fallback is selected first.
	add(1, "web/js/wasm", 7)
	current := instance.executePluginRoutine.GetState()
	if current == nil || current.pluginHost != fallback {
		t.Fatal("available browser fallback was not selected")
	}
	add(2, "js", 7)
	if instance.executePluginRoutine.GetState() != current {
		t.Fatal("late same-revision variant restarted the active generation")
	}
	add(3, "js", 8)
	if selected := instance.executePluginRoutine.GetState(); selected == nil || selected.pluginHost != preferred {
		t.Fatal("newer JavaScript revision did not replace the fallback")
	}
}

// TestManifestSelectionNativeReplacesJavaScript matches a fresh desktop
// install: the JavaScript core arrives first with a higher revision counter,
// then the native build arrives and must replace it.
func TestManifestSelectionNativeReplacesJavaScript(t *testing.T) {
	// Build both hosts and an instance constructor for either selection mode.
	le := logrus.NewEntry(logrus.New())
	jsHost := &testPluginHost{id: "js"}
	nativeHost := &testPluginHost{id: "desktop/darwin/arm64"}
	hosts := &pluginHostSet{pluginHosts: []plugin_host.PluginHost{jsHost, nativeHost}}
	newInstance := func(objKey string) *pluginInstance {
		return &pluginInstance{
			c:                       &Controller{conf: &Config{}, objKey: objKey},
			le:                      le,
			pluginID:                "spacewave-core",
			downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
			executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
		}
	}

	// Direct selection orders the same platforms through fetch values.
	t.Run("direct", func(t *testing.T) {
		// Build the instance and a direct fetch handler over both hosts.
		instance := newInstance("")
		handler := instance.newDirectFetchHandler(t.Context(), hosts)

		// add delivers one manifest ref for a platform and revision.
		add := func(id uint32, platform string, rev uint64) {
			t.Helper()
			ref := newTestManifestRef("spacewave-core", platform, rev, "bucket")
			handler.HandleValueAdded(nil, directive.NewAttachedValue(id, manifest.NewFetchManifestValue([]*manifest.ManifestRef{ref})))
		}

		// The JavaScript core is selected before the native build arrives.
		add(1, "js", 16)
		if selected := instance.executePluginRoutine.GetState(); selected == nil || selected.pluginHost != jsHost {
			t.Fatal("JavaScript core was not selected before the native build arrived")
		}
		add(2, "desktop/darwin/arm64", 15)
		if selected := instance.executePluginRoutine.GetState(); selected == nil || selected.pluginHost != nativeHost {
			t.Fatal("native build did not replace the JavaScript core")
		}
	})

	// World selection orders the same platforms through stored manifests.
	t.Run("world", func(t *testing.T) {
		// Open a World testbed and a mock World state over it.
		ctx := t.Context()
		tb, err := testbed.NewTestbed(ctx, le)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tb.Release)

		// Build a cursor and the mock World state over it.
		cursor, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cursor.Release)

		// Build a mock World state over the cursor.
		ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
		if err != nil {
			t.Fatal(err)
		}

		// Create the plugin-host manifest store and the instance.
		const hostKey = "plugin-host"
		if _, err := manifest_world.CreateManifestStore(ctx, ws, hostKey); err != nil {
			t.Fatal(err)
		}
		instance := newInstance(hostKey)

		// selectAfterAdding stores a manifest and runs World selection.
		selectAfterAdding := func(platform string, rev uint64) *executePluginArgs {
			// Store the manifest and link it to the host.
			t.Helper()
			_, key := storeTestWorldManifest(t, ctx, ws, "spacewave-core", platform, rev)
			if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(hostKey, key, "spacewave-core")); err != nil {
				t.Fatal(err)
			}

			// Load the host object, releasing it before returning.
			obj, found, err := ws.GetObject(ctx, hostKey)
			defer world.ReleaseObjectState(obj)
			if err != nil || !found {
				t.Fatalf("host object: found=%t, error=%v", found, err)
			}

			// Run selection over the loaded host object.
			if _, _, err := instance.processManifestWorldState(ctx, le, hosts, ws, obj); err != nil {
				t.Fatal(err)
			}
			return instance.executePluginRoutine.GetState()
		}

		// The JavaScript core is selected first despite its higher revision.
		if selected := selectAfterAdding("js", 16); selected == nil || selected.pluginHost != jsHost {
			t.Fatal("JavaScript core was not selected before the native build arrived")
		}

		// The lower-revision native build replaces it by platform preference.
		if selected := selectAfterAdding("desktop/darwin/arm64", 15); selected == nil || selected.pluginHost != nativeHost {
			t.Fatal("native build did not replace the JavaScript core")
		}
	})
}
func TestDirectFetchHandlerPreservesCurrentStateAcrossEmptyGap(t *testing.T) {
	// Create the test logger.
	le := logrus.NewEntry(logrus.New())
	host1 := &testPluginHost{id: "desktop/linux/amd64"}
	host2 := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{},
		},
		le:                      le,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(context.Background(), &pluginHostSet{pluginHosts: []plugin_host.PluginHost{host1}})

	// Build the fetch manifest value for the handler.
	val1 := manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 1, "bucket-1"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host1 || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state to be set from first fetched manifest")
	}
	if execState.manifestSnapshot.GetManifestRef() == nil {
		t.Fatal("expected manifest snapshot ref to be set")
	}
	assertDirectSelectionRev(t, pi, 1)

	// Remove the manifest value from the handler.
	handler.HandleValueRemoved(nil, directive.NewAttachedValue(1, val1))

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host1 {
		t.Fatal("expected execute state to remain during empty fetch-manifest gap")
	}
	if pi.downloadManifestRoutine.GetState() == nil {
		t.Fatal("expected download manifest state to remain during empty fetch-manifest gap")
	}
	originalExecState := execState

	// Publish the manifest value to the handler.
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState != originalExecState {
		t.Fatal("expected re-adding the same manifest target to avoid resetting execute state")
	}

	// Build the fetch manifest value for the handler.
	val2 := manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 2, "bucket-2"),
	})
	handler = pi.newDirectFetchHandler(context.Background(), &pluginHostSet{pluginHosts: []plugin_host.PluginHost{host2}})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, val2))

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host2 {
		t.Fatal("expected execute state to update to replacement plugin host")
	}
	if execState.manifestSnapshot.GetManifestRef() == nil {
		t.Fatal("expected replacement manifest snapshot ref to be set")
	}
	assertDirectSelectionRev(t, pi, 2)
}

// assertDirectSelectionRev requires the plugin's recorded manifest selection to
// reference the executing manifest and carry its rev, as plugin list shows
// them.
func assertDirectSelectionRev(t *testing.T, pi *pluginInstance, rev uint64) {
	t.Helper()
	row := pi.c.pluginStatus.recovery[pluginInstanceKey(pi.pluginID, pi.instanceKey)]
	if row.GetExecuteManifestRef() == "" || row.GetExecuteManifestRev() != rev {
		t.Fatalf("selection row = %v, want an execute ref at rev %d", row, rev)
	}
}

func TestDirectFetchHandlerSuppressesConfiguredBucketOnly(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	const noCopyBucketID = "dist/project"
	const worldBucketID = "plugin-host-world"
	host := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{EngineId: worldBucketID, NoCopyBucketIds: []string{noCopyBucketID}},
		},
		le:                      le,
		manifestCopyStatus:      ccontainer.NewCContainer[*manifestCopyStatus](nil),
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(ctx, &pluginHostSet{
		pluginHosts: []plugin_host.PluginHost{host},
	})

	// Build the fetch manifest value for the handler.
	suppressed := manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 1, noCopyBucketID),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, suppressed))

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected configured no-copy manifest to remain executable")
	}
	if pi.downloadManifestRoutine.GetState() != nil {
		t.Fatal("configured no-copy manifest set download state")
	}
	status := pi.manifestCopyStatus.GetValue()
	if status == nil ||
		status.phase != manifestCopyPhaseSuppressed ||
		status.class != manifestCopyClassSuppressed ||
		status.sourceBucketID != noCopyBucketID ||
		status.destinationBucketID != worldBucketID {
		t.Fatalf("suppressed copy status = %#v", status)
	}

	// Build the fetch manifest value for the handler.
	dynamic := manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 2, "dynamic-provider"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, dynamic))

	// Check the download manifest routine state.
	downloadState := pi.downloadManifestRoutine.GetState()
	if downloadState == nil || downloadState.GetManifestRef().GetBucketId() != "dynamic-provider" {
		t.Fatalf("dynamic download state = %#v, want dynamic provider", downloadState)
	}
}

func TestDirectFetchHandlerPrefersCurrentStateAcrossEqualRevOverlap(t *testing.T) {
	// Create the test logger.
	le := logrus.NewEntry(logrus.New())
	host := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{},
		},
		le:                      le,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(context.Background(), &pluginHostSet{pluginHosts: []plugin_host.PluginHost{host}})

	// Build the fetch manifest value for the handler.
	val1 := manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 7, "bucket-a"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state after first manifest")
	}
	firstRef := execState.manifestSnapshot.GetManifestRef()
	if firstRef == nil {
		t.Fatal("expected first manifest ref")
	}

	// Build the fetch manifest value for the handler.
	val2 := manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 7, "bucket-b"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, val2))

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state during equal-rev overlap")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(firstRef) {
		t.Fatal("expected equal-rev overlap to preserve the current execute target")
	}

	// Remove the manifest value from the handler.
	handler.HandleValueRemoved(nil, directive.NewAttachedValue(1, val1))

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state after removing original candidate")
	}
	if execState.manifestSnapshot.GetManifestRef().EqualVT(firstRef) {
		t.Fatal("expected execute target to switch once the original candidate is removed")
	}
}

func TestExecutePluginArgsEqualHandlesNilManifestRefs(t *testing.T) {
	if !executePluginArgsEqual(
		&executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{}},
		&executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{}},
	) {
		t.Fatal("expected args with nil manifest refs to compare equal")
	}

	withRef := &executePluginArgs{
		manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: &bucket.ObjectRef{BucketId: "bucket"},
		},
	}
	withoutRef := &executePluginArgs{manifestSnapshot: &manifest.ManifestSnapshot{}}
	if executePluginArgsEqual(withRef, withoutRef) {
		t.Fatal("expected args with one nil manifest ref to differ")
	}
}

func TestExecutePluginArgsEqualIgnoresBucketForSameManifestRoot(t *testing.T) {
	// Build the root block ref for the fixture.
	rootRef := block.NewBlockRef(hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{1, 2, 3}))
	remote := &executePluginArgs{
		manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: &bucket.ObjectRef{
				BucketId: "remote",
				RootRef:  rootRef.Clone(),
			},
		},
	}
	local := &executePluginArgs{
		manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: &bucket.ObjectRef{
				BucketId: "local",
				RootRef:  rootRef.Clone(),
			},
		},
	}
	if !executePluginArgsEqual(remote, local) {
		t.Fatal("expected local manifest copy to preserve execute state")
	}

	// Create the test logger.
	le := logrus.NewEntry(logrus.New())
	ctr := routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le)
	ctr.SetState(remote)
	if _, changed, _, _ := ctr.SetState(local); changed {
		t.Fatal("expected local manifest copy not to reset the execute routine")
	}

	// Clone the root ref for the changed manifest.
	changedRoot := rootRef.Clone()
	changedRoot.Hash.Hash[0] ^= 0xff
	local.manifestSnapshot.ManifestRef.RootRef = changedRoot
	if executePluginArgsEqual(remote, local) {
		t.Fatal("expected a changed manifest root to reset execute state")
	}
}

func TestFilterPluginPlatformIDsHonorsPlatformPolicy(t *testing.T) {
	// Build the platform allowlist config.
	conf := webPlatformAllowlistConfig("spacewave-v86")
	got := conf.FilterPluginPlatformIDs("spacewave-core", []string{
		"js",
		"web/js/wasm",
		"desktop/darwin/arm64",
	})
	want := []string{"js", "desktop/darwin/arm64"}
	if len(got) != len(want) {
		t.Fatalf("platform ids: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("platform ids: got %v, want %v", got, want)
		}
	}

	// Filter the platform ids through the config.
	got = conf.FilterPluginPlatformIDs("spacewave-v86", []string{"js", "web/js/wasm"})
	want = []string{"js", "web/js/wasm"}
	if len(got) != len(want) {
		t.Fatalf("allowed platform ids: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("allowed platform ids: got %v, want %v", got, want)
		}
	}
}

func TestFilterPluginPlatformIDsHonorsDeniedPlugins(t *testing.T) {
	conf := &Config{PlatformSelectionPolicies: []*PlatformSelectionPolicy{{
		PlatformId:      "web/js/wasm",
		DeniedPluginIds: []string{"core"},
	}}}
	platforms := []string{"js", "web/js/wasm"}
	if got, want := conf.FilterPluginPlatformIDs("core", platforms), []string{"js"}; !slices.Equal(got, want) {
		t.Fatalf("denied plugin platforms = %v, want %v", got, want)
	}
	if got := conf.FilterPluginPlatformIDs("external", platforms); !slices.Equal(got, platforms) {
		t.Fatalf("unlisted plugin platforms = %v, want %v", got, platforms)
	}
}

func TestDirectFetchHandlerFiltersWebPlatformForUnlistedPlugin(t *testing.T) {
	// Create the test logger.
	le := logrus.NewEntry(logrus.New())
	webHost := &testPluginHost{id: "web/js/wasm"}
	jsHost := &testPluginHost{id: "js"}
	pi := &pluginInstance{
		c: &Controller{
			conf: webPlatformAllowlistConfig("spacewave-v86"),
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(context.Background(), &pluginHostSet{
		pluginHosts: []plugin_host.PluginHost{jsHost, webHost},
	})

	// Publish the manifest value to the handler.
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, manifest.NewFetchManifestValue([]*manifest.ManifestRef{
		newTestManifestRef("spacewave-core", "web/js/wasm", 99, "bucket-web"),
		newTestManifestRef("spacewave-core", "js", 1, "bucket-js"),
	})))

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != jsHost {
		t.Fatal("expected unlisted plugin to use js fallback instead of web/js/wasm")
	}
}
