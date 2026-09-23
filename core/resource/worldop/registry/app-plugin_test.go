//go:build !js && !goscript

package resource_worldop_registry

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/bldr/testbed"
	bundler "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	objecttypes "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	"github.com/s4wave/spacewave/core/resource/registration"
	viewers "github.com/s4wave/spacewave/core/resource/viewer/registry"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	viewer "github.com/s4wave/spacewave/sdk/viewer/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// TestNativeApplicationPlugin executes actual immutable TypeScript modules through
// the native QuickJS host, World bridge, supplied transaction, and KV Resources.
func TestNativeApplicationPlugin(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	b := tb.GetBus()
	tb.GetStaticResolver().AddFactory(plugin_host.NewFactory(b))
	host, _, hostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host.Controller](
		ctx, b, resolver.NewLoadControllerWithConfig(plugin_host.NewConfig()), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer hostRef.Release()

	// Typed KV access and plugin operations use the production controller owners.
	types := objecttype_controller.NewController(func(_ context.Context, id string) (objecttype.ObjectType, error) {
		if id == kv_world.KvStoreTypeID {
			return kv_world.KvStoreType, nil
		}
		return nil, nil
	})
	release, err := b.AddController(ctx, types, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// Serve the real registration Resource tree as the core plugin capability.
	generations := registration.NewRegistry()
	registeredTypes := objecttypes.NewObjectTypeRegistryResource(generations)
	registeredViewers := viewers.NewViewerRegistryResource(generations)
	registeredOps := NewWorldOpRegistryResource(generations)
	coreRoot := srpc.NewMux(registeredTypes.GetMux(), registeredViewers.GetMux(), registeredOps.GetMux())
	if err := generations.Register(coreRoot); err != nil {
		t.Fatal(err)
	}
	coreMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(coreRoot).Register(coreMux); err != nil {
		t.Fatal(err)
	}
	core := newApplicationCore(coreMux)
	releaseCore, err := b.AddController(ctx, core, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCore()
	bridge := NewWorldOpRegistryBridgeController(le, b, registeredOps)
	releaseBridge, err := b.AddController(ctx, bridge, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBridge()

	// Compile two distinguishable executables and retain both installed manifests.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "../../../.."))
	distRoot := filepath.Join(root, "bldr")
	var revisions []string
	var installed []*manifest.ManifestRef
	var installationReleases []func()
	var current bldr_plugin.RunningPluginRef
	var running bldr_plugin.RunningPlugin
	for version, step := range []int{1, 10, 99} {
		out := t.TempDir()
		result, err := bundler.Build(ctx, le, t.TempDir(), distRoot, &bundler.BuildRequest{
			WorkingDir:     root,
			SourceRoot:     root,
			OutputRoot:     out,
			BldrDistRoot:   distRoot,
			Entrypoints:    []*bundler.Entrypoint{{Name: "app", InputPath: filepath.Join(cwd, "testdata/app.ts")}},
			Format:         "es",
			Platform:       "browser",
			Target:         "es2022",
			EntryFileNames: "app.mjs",
			ChunkFileNames: "[name]-[hash].mjs",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      "none",
			TreeShaking:    true,
			Defines:        map[string]string{"STEP": strconv.Itoa(step)},
			PrefixAliases: map[string]string{
				"@go/github.com/s4wave/spacewave/": root,
				"@go/":                             filepath.Join(root, "vendor"),
				"@aptre/bldr-sdk/":                 filepath.Join(root, "bldr/sdk"),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		script, err := os.ReadFile(filepath.Join(out, result.GetEntrypointOutputs()["app"]))
		if err != nil {
			t.Fatal(err)
		}
		dist := memfs.New()
		if err := billy_util.WriteFile(dist, "app.mjs", script, 0o644); err != nil {
			t.Fatal(err)
		}
		meta := manifest.NewManifestMeta("test-colors", manifest.BuildType_DEV, host.GetPluginHost().GetPlatformId(), uint64(version+1))
		assets := memfs.New()
		if version != 2 {
			if err := assets.MkdirAll("v/b/fe/.vite", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := billy_util.WriteFile(assets, "v/b/fe/.vite/manifest.json", []byte(`{"colors.tsx":{"file":"colors.js"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := billy_util.WriteFile(assets, "v/b/fe/colors.js", []byte(`export const Colors = () => null`), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		_, ref, err := tb.CreateManifestWithBilly(ctx, meta, "app.mjs", dist, assets, timestamppb.Now())
		if err != nil {
			t.Fatal(err)
		}
		key := manifest.NewManifestKey(tb.GetPluginHostObjKey(), meta)
		if err := manifest_world.ExStoreManifestOp(
			ctx, tb.GetWorldState(), tb.GetVolume().GetPeerID(), key,
			[]string{tb.GetPluginHostObjKey()}, ref,
		); err != nil {
			t.Fatal(err)
		}
		revisions = append(revisions, ref.GetManifestRef().GetRootRef().GetHash().MarshalString())

		// Install and replace the actual native TypeScript worker while its logical
		// reference remains open. An invalid third viewer must preserve revision two.
		installed = append([]*manifest.ManifestRef{ref}, installed...)
		next, release := tb.GetScheduler().AddSelectedPluginReference("test-colors", "space/first", installed...)
		installationReleases = append(installationReleases, release)
		defer release()
		if current != nil && current != next {
			t.Fatal("installation replaced the logical plugin binding")
		}
		current = next
		if version == 2 {
			_, err := tb.GetScheduler().GetPluginStatusCtr().WaitValueWithValidator(ctx, func(status *scheduler.PluginStatusSnapshot) (bool, error) {
				for _, item := range status.Plugins {
					if item.GetPluginId() == "test-colors" && item.GetInstanceKey() == "space/first" && item.GetLastErrorMessage() != "" {
						return true, nil
					}
				}
				return false, nil
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if current.GetRunningPluginCtr().GetValue() != running {
				t.Fatal("failed native candidate replaced the admitted worker")
			}
			continue
		}
		running, err = current.GetRunningPluginCtr().WaitValueChange(ctx, running, nil)
		if err != nil || running == nil {
			t.Fatalf("native candidate did not become ready: %v", err)
		}
		if registeredTypes.LookupRegistration("sync/app/test/colors", "space/first") == nil {
			t.Fatal("ready plugin has no admitted ObjectType")
		}
		list, err := registeredViewers.ListViewers(ctx, &viewer.ListViewersRequest{Surface: viewer.ViewerSurface_VIEWER_SURFACE_WEB, InstanceKey: "space/first"})
		if err != nil {
			t.Fatal(err)
		}
		expectedPath := "/b/pa/test-colors/manifest/" + revisions[version] + "/v/b/fe/colors.js"
		if len(list.GetRegistrations()) != 1 || list.GetRegistrations()[0].GetScriptPath() != expectedPath {
			t.Fatalf("viewer does not match ready worker: %v", list)
		}
	}

	// Closing the last installation reference releases its worker. Reopening the
	// same persisted choices must recover revision two after revision three fails.
	for _, release := range installationReleases {
		release()
	}
	if _, err := current.GetRunningPluginCtr().WaitValueWithValidator(ctx, func(value bldr_plugin.RunningPlugin) (bool, error) {
		return value == nil, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	reopened, releaseReopened := tb.GetScheduler().AddSelectedPluginReference("test-colors", "space/first", installed...)
	defer releaseReopened()
	if _, err := reopened.GetRunningPluginCtr().WaitValue(ctx, nil); err != nil {
		t.Fatal(err)
	}
	list, err := registeredViewers.ListViewers(ctx, &viewer.ListViewersRequest{Surface: viewer.ViewerSurface_VIEWER_SURFACE_WEB, InstanceKey: "space/first"})
	if err != nil {
		t.Fatal(err)
	}
	expectedPath := "/b/pa/test-colors/manifest/" + revisions[1] + "/v/b/fe/colors.js"
	if len(list.GetRegistrations()) != 1 || list.GetRegistrations()[0].GetScriptPath() != expectedPath {
		t.Fatalf("reopened installation did not recover the last working viewer: %v", list)
	}

	// A second Space retains revision one while the first remains on revision two.
	second, releaseSecond := tb.GetScheduler().AddSelectedPluginReference("test-colors", "space/second", installed[len(installed)-1])
	defer releaseSecond()
	if _, err := second.GetRunningPluginCtr().WaitValue(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for instanceKey, revision := range map[string]string{"space/first": revisions[1], "space/second": revisions[0]} {
		list, err := registeredViewers.ListViewers(ctx, &viewer.ListViewersRequest{Surface: viewer.ViewerSurface_VIEWER_SURFACE_WEB, InstanceKey: instanceKey})
		if err != nil {
			t.Fatal(err)
		}
		path := "/b/pa/test-colors/manifest/" + revision + "/v/b/fe/colors.js"
		if len(list.GetRegistrations()) != 1 || list.GetRegistrations()[0].GetScriptPath() != path {
			t.Fatalf("%s selected another Space's viewer: %v", instanceKey, list)
		}
		if registeredTypes.LookupRegistration("sync/app/test/colors", instanceKey) == nil {
			t.Fatalf("%s has no admitted ObjectType", instanceKey)
		}
	}
	global, err := registeredViewers.ListViewers(ctx, &viewer.ListViewersRequest{Surface: viewer.ViewerSurface_VIEWER_SURFACE_WEB})
	if err != nil || len(global.GetRegistrations()) != 0 {
		t.Fatalf("Space registrations escaped to the global catalog: %v, %v", global, err)
	}

	// The original executable still increments by one after a newer one is installed.
	execute := func(ctx context.Context, revision, action, key, request, mutation, input string, accept bool) error {
		id := registry.PinnedOperationID("test-colors", revision, "test/colors/"+action)
		lookups, _, ref, err := world.ExLookupWorldOp(ctx, b, le, id, tb.GetWorldEngineID())
		if err != nil {
			return err
		}
		defer ref.Release()
		var op world.Operation
		for _, lookup := range lookups {
			op, err = lookup(ctx, id)
			if err != nil {
				return err
			}
			if op != nil {
				break
			}
		}
		if op == nil {
			return world.ErrUnhandledOp
		}
		value, err := fastjson.Parse(input)
		if err != nil {
			return err
		}
		var arena fastjson.Arena
		intent := arena.NewObject()
		intent.Set("kind", arena.NewString("mutate"))
		intent.Set("name", arena.NewString(mutation))
		intent.Set("input", value)
		operation := arena.NewObject()
		operation.Set("objectKey", arena.NewString(key))
		operation.Set("requestId", arena.NewString(request))
		if action == "upgrade" {
			operation.Set("from", value)
		} else {
			operation.Set("mutation", intent)
		}
		data := operation.MarshalTo(nil)
		if err := op.UnmarshalBlock(data); err != nil {
			return err
		}
		tx, err := tb.GetWorldEngine().NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		if _, err := op.ApplyWorldOp(ctx, le, tx, tb.GetVolume().GetPeerID()); err != nil {
			return err
		}
		if accept {
			return tx.Commit(ctx)
		}
		return nil
	}
	for _, request := range []struct{ revision, action, key, id string }{
		{revisions[0], "create", "colors/old", "create-old"},
		{revisions[1], "create", "colors/new", "create-new"},
		{revisions[0], "mutate", "colors/old", "like-old"},
		{revisions[0], "mutate", "colors/old", "like-old"},
	} {
		if err := execute(ctx, request.revision, request.action, request.key, request.id, "like", `"blue"`, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := execute(ctx, revisions[0], "mutate", "colors/old", "invalid", "like", "12", true); err == nil {
		t.Fatal("invalid input was accepted")
	}
	if err := execute(ctx, revisions[0], "mutate", "colors/old", "fail", "fail", `"blue"`, true); err == nil {
		t.Fatal("failed callback was accepted")
	}
	// A cooperative callback stops with its call and releases the supplied writer.
	// A subsequent accepted operation proves that cancellation did not retain it.
	canceled, cancelCall := context.WithTimeout(ctx, time.Second)
	err = execute(canceled, revisions[0], "mutate", "colors/old", "cancel-old", "cancel", `"blue"`, true)
	cancelCall()
	if err == nil || canceled.Err() != context.DeadlineExceeded {
		t.Fatalf("cooperative cancellation: %v, call: %v", err, canceled.Err())
	}

	// Adopt the new executable atomically; old calls cannot enter the changed instance.
	oldObject, err := world.MustGetObject(ctx, tb.GetWorldState(), "colors/old")
	if err != nil {
		t.Fatal(err)
	}
	var originalBinding []byte
	err = oldObject.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		var err error
		originalBinding, _, err = cursor.GetBlock(ctx, cursor.GetRef().GetRootRef())
		return err
	})
	world.ReleaseObjectState(oldObject)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := execute(ctx, revisions[1], "upgrade", "colors/old", "upgrade-old", "", string(originalBinding), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := execute(ctx, revisions[0], "mutate", "colors/old", "late-old", "like", `"blue"`, true); err == nil {
		t.Fatal("old executable accepted work after the instance changed")
	}
	if err := execute(ctx, revisions[1], "mutate", "colors/old", "new-like", "like", `"blue"`, true); err != nil {
		t.Fatal(err)
	}

	for key, expected := range map[string]string{"colors/old": "12", "colors/new": "10"} {
		object, err := world.MustGetObject(ctx, tb.GetWorldState(), key)
		if err != nil {
			t.Fatal(err)
		}
		var instance string
		err = object.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
			data, _, err := cursor.GetBlock(ctx, cursor.GetRef().GetRootRef())
			if err != nil {
				return err
			}
			binding, err := fastjson.ParseBytes(data)
			if err == nil {
				instance = string(binding.GetStringBytes("instance"))
			}
			return err
		})
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
		collection := "sync/v1/" + hex.EncodeToString([]byte(instance)) + "/" + hex.EncodeToString([]byte("shared")) + "/collections/" + hex.EncodeToString([]byte("counts"))
		object, err = world.MustGetObject(ctx, tb.GetWorldState(), collection)
		if err != nil {
			t.Fatal(err)
		}
		err = object.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
			_, blockCursor := cursor.BuildTransaction(nil)
			reader, err := kvtx_block.BuildKvTransaction(ctx, blockCursor, false)
			if err != nil {
				return err
			}
			defer reader.Discard()
			value, _, err := reader.Get(ctx, []byte("blue"))
			if err == nil && string(value) != expected {
				t.Errorf("%s: value=%s, want %s", key, value, expected)
			}
			return err
		})
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
	}
}
