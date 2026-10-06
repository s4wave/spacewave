package plugin_host_scheduler

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/routine"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
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
