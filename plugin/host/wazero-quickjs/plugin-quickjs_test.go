package plugin_host_wazero_quickjs_test

import (
	"context"
	"testing"
	"time"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_scheduler "github.com/aperturerobotics/bldr/plugin/host/scheduler"
	plugin_host_wazero_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	"github.com/aperturerobotics/bldr/testbed"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v5/memfs"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
)

func TestPluginHostWazeroQuickjs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))
	sr.AddFactory(plugin_host_scheduler.NewFactory(b))

	// load the plugin scheduler
	sched, _, schedRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_scheduler.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_scheduler.NewConfig(
			tb.GetWorldEngineID(),
			tb.GetPluginHostObjKey(),
			tb.GetVolumeInfo().GetVolumeId(),
			tb.GetVolumeInfo().GetPeerId(),
			true,
			false,
			false,
		)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer schedRef.Release()
	_ = sched

	// load the plugin host
	quickjsHost, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_wazero_quickjs.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_wazero_quickjs.NewConfig()),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer quickjsHostRef.Release()

	// create the directive to load the plugin
	// the plugin scheduler will watch the world and wait for the manifest
	// it will create a FetchManifest directive to look up the manifest.
	pluginID := "test-plugin"
	manifestID := pluginID
	platformID := quickjsHost.GetPluginHost().GetPlatformId()
	scriptPath := "test-plugin.js"
	scriptContents := `export default async function main(backendAPI) { console.log('waiting for plugin info...'); const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({}); console.log('loaded plugin info', backendAPI.protos.GetPluginInfoResponse.toJsonString(pluginInfo)); }`
	_, pluginRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer pluginRef.Release()

	// create a basic plugin manifest
	var manifest *bldr_manifest.Manifest
	var manifestRef *bldr_manifest.ManifestRef
	manifestMeta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_DEV, platformID, 1)
	err = tb.GetWorldEngine().AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransactionAtRef(nil, nil)

		assetsFS, distFS := memfs.New(), memfs.New()
		nowTs := timestamppb.Now()
		err := billy_util.WriteFile(distFS, scriptPath, []byte(scriptContents), 0644)
		if err != nil {
			return err
		}

		manifest, err = bldr_manifest.CreateManifestWithBilly(ctx, bcs, manifestMeta, scriptPath, distFS, assetsFS, nowTs)
		if err != nil {
			return err
		}

		manifestBlockRef, _, err := btx.Write(ctx, true)
		if err != nil {
			return err
		}

		manifestObjRef := bls.GetRef().Clone()
		manifestObjRef.RootRef = manifestBlockRef
		manifestRef = bldr_manifest.NewManifestRef(manifestMeta, manifestObjRef)
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// link it with the plugin host
	err = bldr_manifest_world.ExStoreManifestOp(
		ctx,
		tb.GetWorldState(),
		tb.GetVolume().GetPeerID(),
		"manifests/"+manifestID,
		[]string{tb.GetPluginHostObjKey()},
		manifestRef,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = manifest

	// expect the plugin to startup and run
	runningPlugin, _, runningPluginRef, err := bldr_plugin.ExLoadPlugin(ctx, b, false, pluginID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer runningPluginRef.Release()

	// TODO verify it ran successfully
	rpcClient := runningPlugin.GetRpcClient()
	_ = rpcClient

	<-time.After(time.Second * 2)
	<-ctx.Done()
}
