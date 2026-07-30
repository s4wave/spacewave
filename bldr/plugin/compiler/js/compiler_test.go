//go:build !js

package bldr_plugin_compiler_js_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

func TestPluginCompilerJs(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
	staleEntryPath := filepath.Join(buildWorkingPath, "dist", "plugin-stale.mjs")
	if err := os.MkdirAll(filepath.Dir(staleEntryPath), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := os.WriteFile(staleEntryPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err.Error())
	}

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
	if buildResult.GetSubManifestResults()["vite"] == nil {
		t.Fatal("expected persisted Vite builder result")
	}
	var foundModuleInput, foundEntrypointInput, foundDistDepsInput bool
	for _, inputFile := range buildResult.GetInputManifest().GetFiles() {
		inputPath := filepath.ToSlash(inputFile.GetPath())
		switch {
		case strings.HasSuffix(inputPath, "bldr/plugin/host/wazero-quickjs/plugin-quickjs_test.ts"):
			foundModuleInput = true
		case strings.HasSuffix(inputPath, "bldr/plugin/compiler/js/entrypoint.ts"):
			foundEntrypointInput = true
		case strings.HasSuffix(inputPath, "bldr/dist/deps/package.json"):
			foundDistDepsInput = true
		}
	}
	if !foundModuleInput || !foundEntrypointInput || !foundDistDepsInput {
		t.Fatalf(
			"startup inputs missing module=%t entrypoint=%t dist-deps=%t",
			foundModuleInput,
			foundEntrypointInput,
			foundDistDepsInput,
		)
	}
	if _, err := os.Stat(staleEntryPath); !os.IsNotExist(err) {
		t.Fatalf("stale plugin entrypoint survived rebuild: %v", err)
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

func TestPluginCompilerJsOwnsWorkingPathDuringBuild(t *testing.T) {
	ctrl, err := bldr_plugin_compiler_js.NewController(logrus.NewEntry(logrus.New()), nil, &bldr_plugin_compiler_js.Config{})
	if err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	args := &bldr_manifest_builder.BuildManifestArgs{
		BuilderConfig: &bldr_manifest_builder.BuilderConfig{
			ManifestMeta:   bldr_manifest.NewManifestMeta("test-plugin", bldr_manifest.BuildType_DEV, "js", 1),
			SourcePath:     t.TempDir(),
			DistSourcePath: t.TempDir(),
			WorkingPath:    workDir,
		},
	}
	buildStarted := make(chan struct{})
	continueBuild := make(chan struct{})
	sentinelErr := errors.New("stop after working-directory ownership check")
	ctrl.AddPreBuildHook(func(
		_ context.Context,
		builderConf *bldr_manifest_builder.BuilderConfig,
		_ world.Engine,
	) (*bldr_plugin_compiler_js.PreBuildHookResult, error) {
		close(buildStarted)
		<-continueBuild
		if err := os.WriteFile(filepath.Join(builderConf.GetWorkingPath(), "marker"), []byte("builder-owned"), 0o644); err != nil {
			return nil, err
		}
		return nil, sentinelErr
	})

	buildErr := make(chan error, 1)
	go func() {
		_, err := ctrl.BuildManifest(context.Background(), args, nil)
		buildErr <- err
	}()

	<-buildStarted
	if err := os.RemoveAll(workDir); err != nil {
		t.Fatal(err)
	}
	close(continueBuild)

	select {
	case err := <-buildErr:
		if !errors.Is(err, sentinelErr) {
			t.Fatalf("BuildManifest error = %v, want sentinel after builder-owned write", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BuildManifest did not finish after releasing the build seam")
	}
	if err := ctrl.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPluginCompilerJsStartupCacheRequiresStaticInputs(t *testing.T) {
	ctrl := &bldr_plugin_compiler_js.Controller{}
	if !ctrl.SupportsStartupManifestCache() {
		t.Fatal("JS compiler with static inputs should support startup cache")
	}
	ctrl.AddPreBuildHook(func(
		context.Context,
		*bldr_manifest_builder.BuilderConfig,
		world.Engine,
	) (*bldr_plugin_compiler_js.PreBuildHookResult, error) {
		return nil, nil
	})
	if ctrl.SupportsStartupManifestCache() {
		t.Fatal("JS compiler with an undeclared pre-build hook must bypass startup cache")
	}
}

func TestPluginCompilerJsStartupCacheHonorsDeclaredProvenance(t *testing.T) {
	noopHook := func(
		context.Context,
		*bldr_manifest_builder.BuilderConfig,
		world.Engine,
	) (*bldr_plugin_compiler_js.PreBuildHookResult, error) {
		return nil, nil
	}

	// A hook that declares complete deterministic provenance is cache-eligible.
	ctrl := &bldr_plugin_compiler_js.Controller{}
	ctrl.AddPreBuildHookWithProvenance(noopHook, &bldr_plugin_compiler_js.PreBuildHookProvenance{
		InputFiles: []string{"hook/input.json"},
		EnvVars:    []string{"BLDR_TEST_HOOK_ENV"},
	})
	if !ctrl.SupportsStartupManifestCache() {
		t.Fatal("JS compiler with a declared-provenance hook must support startup cache")
	}

	// A single undeclared hook among declared ones still forces always-build.
	ctrl.AddPreBuildHookWithProvenance(noopHook, nil)
	if ctrl.SupportsStartupManifestCache() {
		t.Fatal("JS compiler with any undeclared hook must bypass startup cache")
	}
}

func TestPreBuildHookProvenanceResolvesStartupInputs(t *testing.T) {
	t.Setenv("BLDR_TEST_HOOK_ENV", "declared-value")
	provenance := &bldr_plugin_compiler_js.PreBuildHookProvenance{
		InputFiles: []string{"hook/input.json", "", "/abs/input.txt"},
		EnvVars:    []string{"BLDR_TEST_HOOK_ENV", ""},
	}

	paths := provenance.StartupInputPaths("/src/root")
	wantPaths := []string{
		filepath.Join("/src/root", "hook", "input.json"),
		"/abs/input.txt",
	}
	if len(paths) != len(wantPaths) {
		t.Fatalf("expected %d startup input paths, got %d: %v", len(wantPaths), len(paths), paths)
	}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Fatalf("startup input path[%d] = %q, want %q", i, paths[i], want)
		}
	}

	envInputs := provenance.EnvStartupInputs()
	if len(envInputs) != 1 {
		t.Fatalf("expected 1 env startup input, got %d", len(envInputs))
	}
	if got := envInputs[0].GetKey(); got != "BLDR_TEST_HOOK_ENV" {
		t.Fatalf("env startup input key = %q, want BLDR_TEST_HOOK_ENV", got)
	}
	if got := envInputs[0].GetStringValue(); got != "declared-value" {
		t.Fatalf("env startup input value = %q, want declared-value", got)
	}

	// A nil provenance (undeclared hook) contributes no inputs.
	var nilProvenance *bldr_plugin_compiler_js.PreBuildHookProvenance
	if got := nilProvenance.StartupInputPaths("/src/root"); got != nil {
		t.Fatalf("nil provenance produced startup input paths: %v", got)
	}
	if got := nilProvenance.EnvStartupInputs(); got != nil {
		t.Fatalf("nil provenance produced env startup inputs: %v", got)
	}
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

func TestCreateEntrypointsFromViteOutputsQuickJSFrontendBoundary(t *testing.T) {
	backend, frontend := bldr_plugin_compiler_js.CreateEntrypointsFromViteOutputs(
		[]*bldr_plugin_compiler_js.JsModule{
			{
				Kind:       bldr_plugin_compiler_js.JsModuleKind_JS_MODULE_KIND_BACKEND,
				Path:       "./spacewave-app/backend.ts",
				Entrypoint: true,
			},
			{
				Kind:       bldr_plugin_compiler_js.JsModuleKind_JS_MODULE_KIND_FRONTEND,
				Path:       "./spacewave-app/App.tsx",
				Entrypoint: true,
			},
		},
		[]*bldr_web_bundler_vite.ViteOutputMeta{
			{
				EntrypointPath: "spacewave-app/backend.ts",
				Path:           "b/be/spacewave-app/backend-abc123.mjs",
			},
			{
				EntrypointPath: "spacewave-app/App.tsx",
				Path:           "b/fe/spacewave-app/App-def456.mjs",
			},
			{
				EntrypointPath: "spacewave-app/App.tsx",
				Path:           "b/fe/spacewave-app/App-def456.css",
			},
		},
		nil,
		nil,
	)

	if len(backend) != 1 {
		t.Fatalf("expected one backend entrypoint, got %d", len(backend))
	}
	if len(frontend) != 1 {
		t.Fatalf("expected one frontend entrypoint, got %d", len(frontend))
	}

	if got := backend[0].GetImportPath(); got != "/assets/v/b/be/spacewave-app/backend-abc123.mjs" {
		t.Fatalf("unexpected backend import path: %q", got)
	}
	if strings.Contains(backend[0].GetImportPath(), "/v/b/fe/") {
		t.Fatalf("backend import path must not point at frontend bundle assets: %q", backend[0].GetImportPath())
	}

	setRenderMode := frontend[0].GetSetRenderMode()
	if setRenderMode.GetRenderMode() != web_view.RenderMode_RenderMode_REACT_COMPONENT {
		t.Fatalf("unexpected render mode: %v", setRenderMode.GetRenderMode())
	}
	if got := setRenderMode.GetScriptPath(); got != "v/b/fe/spacewave-app/App-def456.mjs" {
		t.Fatalf("unexpected frontend script path: %q", got)
	}
	if strings.HasPrefix(setRenderMode.GetScriptPath(), "/assets/") {
		t.Fatalf("frontend script path must remain WebView asset metadata, got %q", setRenderMode.GetScriptPath())
	}

	links := frontend[0].GetSetHtmlLinks().GetSetLinks()
	link := links["css-App-def456.css"]
	if link == nil {
		t.Fatalf("expected frontend css link in %v", links)
	}
	if got := link.GetHref(); got != "v/b/fe/spacewave-app/App-def456.css" {
		t.Fatalf("unexpected frontend css href: %q", got)
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

func TestValidateFrontendEntrypointAssetClosure(t *testing.T) {
	dir := t.TempDir()
	for _, relPath := range []string{
		"v/b/fe/app/App-abc123.mjs",
		"v/b/fe/app/App-abc123.css",
	} {
		fullPath := filepath.Join(dir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("asset"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	frontend := []*bldr_plugin_compiler_js.FrontendEntrypoint{{
		SetRenderMode: &web_view.SetRenderModeRequest{
			RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
			ScriptPath: "v/b/fe/app/App-abc123.mjs",
		},
		SetHtmlLinks: &web_view.SetHtmlLinksRequest{
			SetLinks: map[string]*web_view.HtmlLink{
				"css": {Rel: "stylesheet", Href: "v/b/fe/app/App-abc123.css"},
			},
		},
	}}
	if err := bldr_plugin_compiler_js.ValidateFrontendEntrypointAssetClosure(dir, frontend); err != nil {
		t.Fatal(err)
	}

	frontend[0].SetRenderMode.ScriptPath = "v/b/fe/app/Missing-abc123.mjs"
	err := bldr_plugin_compiler_js.ValidateFrontendEntrypointAssetClosure(dir, frontend)
	if err == nil {
		t.Fatal("expected missing frontend asset validation error")
	}
	if !strings.Contains(err.Error(), `v/b/fe/app/Missing-abc123.mjs`) {
		t.Fatalf("missing asset error did not name path: %v", err)
	}
}
