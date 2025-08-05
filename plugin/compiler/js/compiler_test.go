//go:build !js

package bldr_plugin_compiler_js_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_manifest_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_plugin_compiler_js "github.com/aperturerobotics/bldr/plugin/compiler/js"
	plugin_host_wazero_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	"github.com/aperturerobotics/bldr/testbed"
	bldr_web_bundler_vite_compiler "github.com/aperturerobotics/bldr/web/bundler/vite/compiler"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/sirupsen/logrus"
)

func TestPluginCompilerJs(t *testing.T) {
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
	sr.AddFactory(bldr_plugin_compiler_js.NewFactory(b))
	sr.AddFactory(bldr_manifest_builder_controller.NewFactory(b))
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))
	sr.AddFactory(bldr_web_bundler_vite_compiler.NewFactory(b))

	// load the plugin host which will execute the plugin once it is ready.
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
	pluginID := "test-plugin"
	platformID := quickjsHost.GetPluginHost().GetPlatformId()

	_, pluginRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer pluginRef.Release()

	// create the plugin compiler config which defines how to build the plugin
	jsCompilerConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(
		1,
		&bldr_plugin_compiler_js.Config{
			Modules: []*bldr_plugin_compiler_js.JsModule{{
				Kind: bldr_plugin_compiler_js.JsModuleKind_JS_MODULE_KIND_BACKEND,
				Path: "./plugin/host/wazero-quickjs/plugin-quickjs_test.ts",
			}},
		},
	), false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// build the manifest
	manifestID := pluginID
	projectID := pluginID

	pluginHostKey := tb.GetPluginHostObjKey()
	manifestMeta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_DEV, platformID, 1)

	// create a working path dir
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}
	buildWorkingPath := filepath.Join(testDir, ".test")
	distSrcPath := filepath.Join(testDir, "../../..")

	// start the manifest builder controller
	engineID := tb.GetWorldEngineID()
	peerID := tb.GetVolume().GetPeerID().String()
	manifestKey := bldr_manifest.NewManifestKey(pluginHostKey, manifestMeta)
	storeLinkObjKeys := []string{pluginHostKey}
	manifestBuilderConf := &bldr_manifest_builder.BuilderConfig{
		ProjectId:      projectID,
		ManifestMeta:   manifestMeta,
		EngineId:       engineID,
		PeerId:         peerID,
		ObjectKey:      manifestKey,
		LinkObjectKeys: storeLinkObjKeys,
		// Use the actual sources as the distSrcPath.
		// esbuild/vite will automatically handle the .ts / .js resolution
		DistSourcePath: distSrcPath,
		WorkingPath:    buildWorkingPath,
		SourcePath:     distSrcPath,
	}
	builderConf := bldr_manifest_builder_controller.NewConfig(
		manifestBuilderConf,
		jsCompilerConf,
		nil,
		true,
	)

	builderCtrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*bldr_manifest_builder_controller.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(builderConf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()

	buildResult, err := builderCtrl.GetResultPromise().Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	jdat, err := buildResult.GetManifest().MarshalJSON()
	if err != nil {
		t.Fatal(err.Error())
	}

	le.Infof("compiled js plugin manifest: %v", string(jdat))

	// wait for the plugin to load fully
	pluginClient, pluginClientRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, b, pluginID, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer pluginClientRef.Release()
	_ = pluginClient

	le.Infof("plugin %q loaded successfully", pluginID)
}
