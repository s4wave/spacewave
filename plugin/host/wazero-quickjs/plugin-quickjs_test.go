package plugin_host_wazero_quickjs_test

import (
	"context"
	"testing"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_wazero_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	"github.com/aperturerobotics/bldr/testbed"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	starpc_mock "github.com/aperturerobotics/starpc/mock"
	"github.com/aperturerobotics/util/promise"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

func TestPluginHostWazeroQuickjs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build the TypeScript source to ESM format ES2022
	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{"plugin-quickjs_test.ts"},
		Bundle:      true,
		Format:      esbuild_api.FormatESModule,
		Target:      esbuild_api.ES2022,
		TreeShaking: esbuild_api.TreeShakingTrue,
		Platform:    esbuild_api.PlatformBrowser,
	})

	if len(result.Errors) > 0 {
		t.Fatalf("esbuild errors: %v", result.Errors)
	}

	if len(result.OutputFiles) == 0 {
		t.Fatal("no output files from esbuild")
	}

	scriptContents := string(result.OutputFiles[0].Contents)

	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))

	// run a service on the plugin host that our plugin will call
	calledPromise := promise.NewPromise[*starpc_mock.MockMsg]()
	mockServer := &starpc_mock.MockServer{
		MockRequestCb: func(ctx context.Context, msg *starpc_mock.MockMsg) (*starpc_mock.MockMsg, error) {
			calledPromise.SetResult(msg, nil)
			return &starpc_mock.MockMsg{Body: "hello from js compiler test"}, nil
		},
	}
	mux := tb.GetMux()
	mockServer.Register(mux)

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
	_, pluginRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer pluginRef.Release()

	// create the contents of the plugin manifest
	assetsFS, distFS := memfs.New(), memfs.New()
	nowTs := timestamppb.Now()
	err = billy_util.WriteFile(distFS, scriptPath, []byte(scriptContents), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a basic plugin manifest
	manifestMeta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_DEV, platformID, 1)
	manifest, manifestRef, err := tb.CreateManifestWithBilly(ctx, manifestMeta, scriptPath, distFS, assetsFS, nowTs)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = manifestRef
	manifest.GetMeta().Logger(le).Info("created manifest")

	// expect the plugin to startup and run
	runningPlugin, _, runningPluginRef, err := bldr_plugin.ExLoadPlugin(ctx, b, false, pluginID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer runningPluginRef.Release()

	le.Info("plugin started successfully")

	// TODO call the plugin service
	rpcClient := runningPlugin.GetRpcClient()
	_ = rpcClient

	// wait for rpc to be called
	calledMsg, err := calledPromise.Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	calledMsgDat, err := calledMsg.MarshalJSON()
	if err != nil {
		t.Fatal(err.Error())
	}

	le.Infof("plugin successfully called host rpc with message: %v", string(calledMsgDat))
}
