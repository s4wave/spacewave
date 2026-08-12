//go:build !goscript

package plugin_host_wazero_quickjs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	starpc_mock "github.com/aperturerobotics/starpc/mock"
	"github.com/aperturerobotics/util/promise"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	"github.com/s4wave/spacewave/bldr/testbed"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	"github.com/sirupsen/logrus"
)

func TestPluginHostWazeroQuickjs(t *testing.T) {
	t.Run("without web packages", func(t *testing.T) {
		testPluginHostWazeroQuickjs(t, "plugin-quickjs_test.ts", nil, false)
	})
	t.Run("with web package", func(t *testing.T) {
		testPluginHostWazeroQuickjs(
			t,
			"plugin-quickjs-web-pkg_test.ts",
			[]string{"/b/pkg/@aptre/protobuf-es-lite/message.mjs"},
			true,
		)
	})
}

func testPluginHostWazeroQuickjs(t *testing.T, inputFile string, external []string, withWebPkg bool) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build the fixture through the direct internal owner.
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bldrRoot := filepath.Clean(filepath.Join(workingDir, "../../.."))
	outputRoot := t.TempDir()
	result, err := bldr_web_bundler_rolldown.Build(
		ctx,
		le,
		t.TempDir(),
		bldrRoot,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:   workingDir,
			SourceRoot:   bldrRoot,
			OutputRoot:   outputRoot,
			BldrDistRoot: bldrRoot,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      "plugin-quickjs-test",
				InputPath: filepath.Join(workingDir, inputFile),
			}},
			Format:         "es",
			Platform:       "browser",
			Target:         "es2022",
			EntryFileNames: "plugin-quickjs-test.js",
			ChunkFileNames: "[name]-[hash].js",
			AssetFileNames: "[name]-[hash][extname]",
			Sourcemap:      "none",
			TreeShaking:    true,
			External:       external,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	outputPath := result.GetEntrypointOutputs()["plugin-quickjs-test"]
	if outputPath == "" {
		t.Fatal("direct owner produced no QuickJS fixture output")
	}
	scriptBytes, err := os.ReadFile(filepath.Join(outputRoot, outputPath))
	if err != nil {
		t.Fatal(err)
	}
	scriptContents := string(scriptBytes)

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
	if withWebPkg {
		const webPkgPath = "bldr-web-pkgs/@aptre/protobuf-es-lite/message.mjs"
		if err := billy_util.WriteFile(
			assetsFS,
			webPkgPath,
			[]byte("export class Counter { constructor() { this.value = 0 } next() { return ++this.value } }"),
			0o644,
		); err != nil {
			t.Fatal(err.Error())
		}
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
