//go:build !js

package bldr_plugin_compiler_js_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	starpc_mock "github.com/aperturerobotics/starpc/mock"
	"github.com/aperturerobotics/util/promise"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	"github.com/s4wave/spacewave/bldr/testbed"
	bldr_web_bundler_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	"github.com/sirupsen/logrus"
)

func TestPluginCompilerJs(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer ctxCancel()
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

	// create the plugin compiler config which defines how to build the plugin
	jsCompilerConf, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(
		1,
		&bldr_plugin_compiler_js.Config{
			Modules: []*bldr_plugin_compiler_js.JsModule{{
				Kind:       bldr_plugin_compiler_js.JsModuleKind_JS_MODULE_KIND_BACKEND,
				Path:       "./bldr/plugin/host/wazero-quickjs/plugin-quickjs_test.ts",
				Entrypoint: true,
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
	distSrcPath := filepath.Join(testDir, "../../../..")

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
		// Use the monorepo root as the dist source path so vendor/ and sibling
		// packages are visible during test builds.
		DistSourcePath: distSrcPath,
		WorkingPath:    buildWorkingPath,
		SourcePath:     distSrcPath,
	}
	builderConf := bldr_manifest_builder_controller.NewConfig(
		manifestBuilderConf,
		jsCompilerConf,
		nil,
		true,
		nil,
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

func TestCreateEntrypointsFromViteOutputsBackendImportPath(t *testing.T) {
	backend, frontend := bldr_plugin_compiler_js.CreateEntrypointsFromViteOutputs(
		[]*bldr_plugin_compiler_js.JsModule{{
			Kind:       bldr_plugin_compiler_js.JsModuleKind_JS_MODULE_KIND_BACKEND,
			Path:       "./plugin/notes/backend.ts",
			Entrypoint: true,
		}},
		[]*bldr_web_bundler_vite.ViteOutputMeta{{
			EntrypointPath: "plugin/notes/backend.ts",
			Path:           "b/be/plugin/notes/backend-abc123.mjs",
		}},
		nil,
		nil,
	)

	if len(frontend) != 0 {
		t.Fatalf("expected no frontend entrypoints, got %d", len(frontend))
	}
	if len(backend) != 1 {
		t.Fatalf("expected one backend entrypoint, got %d", len(backend))
	}
	if got := backend[0].GetImportPath(); got != "/assets/v/b/be/plugin/notes/backend-abc123.mjs" {
		t.Fatalf("unexpected backend import path: %q", got)
	}
}

func TestPluginCompilerJsSupportedPlatforms(t *testing.T) {
	ctrl, err := bldr_plugin_compiler_js.NewController(nil, nil, &bldr_plugin_compiler_js.Config{})
	if err != nil {
		t.Fatal(err.Error())
	}
	supported := ctrl.GetSupportedPlatforms()

	browserTarget := bldr_platform.GetBuiltinTarget(bldr_platform.TargetID_Browser)
	if got := browserTarget.SelectPlatformForCompiler(supported); got != "web/js/wasm" {
		t.Fatalf("browser target selected platform = %q, want web/js/wasm", got)
	}

	desktopTarget := bldr_platform.GetBuiltinTarget(bldr_platform.TargetID_Desktop)
	if got := desktopTarget.SelectPlatformForCompiler(supported); got != bldr_platform.PlatformID_JS {
		t.Fatalf("desktop target selected platform = %q, want js", got)
	}
}

func TestCreateEntrypointsFromViteOutputsFrontendIsIdempotent(t *testing.T) {
	backend, frontend := bldr_plugin_compiler_js.CreateEntrypointsFromViteOutputs(
		[]*bldr_plugin_compiler_js.JsModule{{
			Kind:       bldr_plugin_compiler_js.JsModuleKind_JS_MODULE_KIND_FRONTEND,
			Path:       "./app/App.tsx",
			Entrypoint: true,
		}},
		[]*bldr_web_bundler_vite.ViteOutputMeta{{
			EntrypointPath: "app/App.tsx",
			Path:           "b/fe/app/App-abc123.mjs",
		}},
		nil,
		nil,
	)

	if len(backend) != 0 {
		t.Fatalf("expected no backend entrypoints, got %d", len(backend))
	}
	if len(frontend) != 1 {
		t.Fatalf("expected one frontend entrypoint, got %d", len(frontend))
	}

	setRenderMode := frontend[0].GetSetRenderMode()
	if setRenderMode.GetRenderMode() != web_view.RenderMode_RenderMode_REACT_COMPONENT {
		t.Fatalf("unexpected render mode: %v", setRenderMode.GetRenderMode())
	}
	if got := setRenderMode.GetScriptPath(); got != "v/b/fe/app/App-abc123.mjs" {
		t.Fatalf("unexpected frontend script path: %q", got)
	}
	if setRenderMode.GetRefresh() {
		t.Fatal("frontend entrypoints should not force refresh for idempotent handler reattachment")
	}
}
