//go:build !js

package wasm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

func TestEnableTinyGoForManifest(t *testing.T) {
	goConf := &bldr_plugin_compiler_go.Config{}
	data, err := goConf.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	conf := &bldr_project.ProjectConfig{
		Manifests: map[string]*bldr_project.ManifestConfig{
			"spacewave-core": {
				Builder: &configset_proto.ControllerConfig{
					Id:     bldr_plugin_compiler_go.ConfigID,
					Config: data,
				},
			},
		},
	}

	if err := EnableTinyGoForManifest("spacewave-core")(conf); err != nil {
		t.Fatal(err)
	}

	got := &bldr_plugin_compiler_go.Config{}
	if err := got.UnmarshalJSON(conf.GetManifests()["spacewave-core"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	webConf := got.GetPlatformTypes()["web"]
	if webConf == nil {
		t.Fatal("missing web platform override")
	}
	if webConf.GetCompilerMode() != bldr_plugin_compiler_go.CompilerMode_COMPILER_MODE_TINYGO {
		t.Fatalf("compilerMode = %s, want COMPILER_MODE_TINYGO", webConf.GetCompilerMode())
	}
}

func TestEnableGoScriptForManifest(t *testing.T) {
	goConf := &bldr_plugin_compiler_go.Config{}
	data, err := goConf.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	conf := &bldr_project.ProjectConfig{
		Manifests: map[string]*bldr_project.ManifestConfig{
			"spacewave-core": {
				Builder: &configset_proto.ControllerConfig{
					Id:     bldr_plugin_compiler_go.ConfigID,
					Config: data,
				},
			},
		},
	}

	if err := EnableGoScriptForManifest("spacewave-core")(conf); err != nil {
		t.Fatal(err)
	}

	got := &bldr_plugin_compiler_go.Config{}
	if err := got.UnmarshalJSON(conf.GetManifests()["spacewave-core"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	webConf := got.GetPlatformTypes()["web"]
	if webConf == nil {
		t.Fatal("missing web platform override")
	}
	if webConf.GetCompilerMode() != bldr_plugin_compiler_go.CompilerMode_COMPILER_MODE_GOSCRIPT {
		t.Fatalf("compilerMode = %s, want COMPILER_MODE_GOSCRIPT", webConf.GetCompilerMode())
	}
}

func TestResolveE2EWasmCompiler(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		want E2EWasmCompiler
	}{
		{name: "default", want: E2EWasmCompilerGo},
		{name: "go", env: "go", want: E2EWasmCompilerGo},
		{name: "tinygo", env: "tinygo", want: E2EWasmCompilerTinyGo},
		{name: "goscript", env: "goscript", want: E2EWasmCompilerGoScript},
		{name: "case insensitive", env: "GoScript", want: E2EWasmCompilerGoScript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(E2EWasmLegacyTinyGoEnv, "")
			t.Setenv(E2EWasmCompilerEnv, tc.env)
			got, err := ResolveE2EWasmCompiler()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("compiler = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveE2EWasmCompilerRejectsInvalid(t *testing.T) {
	t.Setenv(E2EWasmLegacyTinyGoEnv, "")
	t.Setenv(E2EWasmCompilerEnv, "banana")

	if _, err := ResolveE2EWasmCompiler(); err == nil {
		t.Fatal("expected unsupported compiler to fail")
	}
}

func TestResolveE2EWasmCompilerRejectsLegacyTinyGoEnv(t *testing.T) {
	t.Setenv(E2EWasmLegacyTinyGoEnv, "true")
	t.Setenv(E2EWasmCompilerEnv, "tinygo")

	if _, err := ResolveE2EWasmCompiler(); err == nil {
		t.Fatal("expected legacy TinyGo selector to fail")
	}
}

func TestConfigureTinyGoForManifestRemovesDebugTrace(t *testing.T) {
	goConf := &bldr_plugin_compiler_go.Config{
		BuildTypes: map[string]*bldr_plugin_compiler_go.Config{
			"dev": {
				GoPkgs: []string{
					"./core/debug/trace",
					"./core/plugin/space",
				},
				ConfigSet: map[string]*configset_proto.ControllerConfig{
					"debug-trace": {Id: "debug/trace"},
					"space":       {Id: "plugin/space"},
				},
			},
		},
	}
	data, err := goConf.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	conf := &bldr_project.ProjectConfig{
		Manifests: map[string]*bldr_project.ManifestConfig{
			"spacewave-core": {
				Builder: &configset_proto.ControllerConfig{
					Id:     bldr_plugin_compiler_go.ConfigID,
					Config: data,
				},
			},
		},
	}

	if err := ConfigureTinyGoForManifest("spacewave-core")(conf); err != nil {
		t.Fatal(err)
	}

	got := &bldr_plugin_compiler_go.Config{}
	if err := got.UnmarshalJSON(conf.GetManifests()["spacewave-core"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	devConf := got.GetBuildTypes()["dev"]
	if devConf == nil {
		t.Fatal("missing dev build-type config")
	}
	for _, pkg := range devConf.GetGoPkgs() {
		if pkg == "./core/debug/trace" {
			t.Fatal("dev build-type still includes ./core/debug/trace")
		}
	}
	if _, ok := devConf.GetConfigSet()["debug-trace"]; ok {
		t.Fatal("dev build-type still includes debug-trace config")
	}
	if _, ok := devConf.GetConfigSet()["space"]; !ok {
		t.Fatal("expected unrelated dev config to remain")
	}
	webConf := got.GetPlatformTypes()["web"]
	if webConf == nil || webConf.GetCompilerMode() != bldr_plugin_compiler_go.CompilerMode_COMPILER_MODE_TINYGO {
		t.Fatalf("web tinygo config missing: %#v", webConf)
	}
}

func TestConfigureTinyGoForManifestRemovesSessionHarnessWebRTC(t *testing.T) {
	goConf := &bldr_plugin_compiler_go.Config{
		GoPkgs: []string{
			"./e2e/wasm/session",
			"github.com/s4wave/spacewave/net/transport/webrtc",
		},
		ConfigSet: map[string]*configset_proto.ControllerConfig{
			"e2e-session-harness":        {Id: "e2e/wasm/session"},
			"e2e-session-harness-webrtc": {Id: "bifrost/webrtc"},
		},
	}
	data, err := goConf.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	conf := &bldr_project.ProjectConfig{
		Manifests: map[string]*bldr_project.ManifestConfig{
			"spacewave-core": {
				Builder: &configset_proto.ControllerConfig{
					Id:     bldr_plugin_compiler_go.ConfigID,
					Config: data,
				},
			},
		},
	}

	if err := ConfigureTinyGoForManifest("spacewave-core")(conf); err != nil {
		t.Fatal(err)
	}

	got := &bldr_plugin_compiler_go.Config{}
	if err := got.UnmarshalJSON(conf.GetManifests()["spacewave-core"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range got.GetGoPkgs() {
		if pkg == "github.com/s4wave/spacewave/net/transport/webrtc" {
			t.Fatal("TinyGo config still includes net/transport/webrtc")
		}
	}
	if _, ok := got.GetConfigSet()["e2e-session-harness-webrtc"]; ok {
		t.Fatal("TinyGo config still includes e2e-session-harness-webrtc")
	}
	if _, ok := got.GetConfigSet()["e2e-session-harness"]; !ok {
		t.Fatal("expected unrelated session harness config to remain")
	}
}

func TestConfigureGoScriptForManifestRemovesSeedRuntimeConfig(t *testing.T) {
	goConf := &bldr_plugin_compiler_go.Config{
		GoPkgs: []string{
			"./e2e/wasm/session",
			"github.com/s4wave/spacewave/net/transport/webrtc",
		},
		ConfigSet: map[string]*configset_proto.ControllerConfig{
			"e2e-session-harness":        {Id: "e2e/wasm/session"},
			"e2e-session-harness-webrtc": {Id: "bifrost/webrtc"},
		},
		BuildTypes: map[string]*bldr_plugin_compiler_go.Config{
			"dev": {
				GoPkgs: []string{
					"./core/debug/trace",
					"./core/plugin/space",
				},
				ConfigSet: map[string]*configset_proto.ControllerConfig{
					"debug-trace": {Id: "debug/trace"},
					"space":       {Id: "plugin/space"},
				},
			},
		},
	}
	data, err := goConf.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	conf := &bldr_project.ProjectConfig{
		Manifests: map[string]*bldr_project.ManifestConfig{
			"spacewave-core": {
				Builder: &configset_proto.ControllerConfig{
					Id:     bldr_plugin_compiler_go.ConfigID,
					Config: data,
				},
			},
		},
	}

	if err := ConfigureGoScriptForManifest("spacewave-core")(conf); err != nil {
		t.Fatal(err)
	}

	got := &bldr_plugin_compiler_go.Config{}
	if err := got.UnmarshalJSON(conf.GetManifests()["spacewave-core"].GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	webConf := got.GetPlatformTypes()["web"]
	if webConf == nil || webConf.GetCompilerMode() != bldr_plugin_compiler_go.CompilerMode_COMPILER_MODE_GOSCRIPT {
		t.Fatalf("web GoScript config missing: %#v", webConf)
	}
	devConf := got.GetBuildTypes()["dev"]
	if slices.Contains(devConf.GetGoPkgs(), "./core/debug/trace") {
		t.Fatal("GoScript config still includes ./core/debug/trace")
	}
	if _, ok := devConf.GetConfigSet()["debug-trace"]; ok {
		t.Fatal("GoScript config still includes debug-trace config")
	}
	if slices.Contains(got.GetGoPkgs(), "github.com/s4wave/spacewave/net/transport/webrtc") {
		t.Fatal("GoScript config still includes net/transport/webrtc")
	}
	if _, ok := got.GetConfigSet()["e2e-session-harness-webrtc"]; ok {
		t.Fatal("GoScript config still includes e2e-session-harness-webrtc")
	}
	if _, ok := got.GetConfigSet()["e2e-session-harness"]; !ok {
		t.Fatal("expected session harness controller config to remain")
	}
}

func TestConfigureTinyGoForManifestDeterministicConfig(t *testing.T) {
	goConf := &bldr_plugin_compiler_go.Config{
		ConfigSet: map[string]*configset_proto.ControllerConfig{
			"zeta":  {Id: "zeta"},
			"alpha": {Id: "alpha"},
		},
		BuildTypes: map[string]*bldr_plugin_compiler_go.Config{
			"prod": {GoPkgs: []string{"./core/plugin/space"}},
			"dev": {
				GoPkgs: []string{"./core/debug/trace", "./core/plugin/space"},
				ConfigSet: map[string]*configset_proto.ControllerConfig{
					"trace": {Id: "trace"},
					"space": {Id: "space"},
				},
			},
		},
		PlatformTypes: map[string]*bldr_plugin_compiler_go.Config{
			"web":     {EsbuildFlags: []string{"--bundle"}},
			"desktop": {GoPkgs: []string{"./cmd/spacewave"}},
		},
	}
	data, err := goConf.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var first []byte
	for i := range 20 {
		conf := &bldr_project.ProjectConfig{
			Manifests: map[string]*bldr_project.ManifestConfig{
				"spacewave-core": {
					Builder: &configset_proto.ControllerConfig{
						Id:     bldr_plugin_compiler_go.ConfigID,
						Config: data,
					},
				},
			},
		}
		if err := ConfigureTinyGoForManifest("spacewave-core")(conf); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		got := conf.GetManifests()["spacewave-core"].GetBuilder().GetConfig()
		if i == 0 {
			first = append([]byte(nil), got...)
			continue
		}
		if string(got) != string(first) {
			t.Fatalf("iteration %d produced unstable config:\nfirst: %s\nnext:  %s", i, first, got)
		}
	}
}

func TestStartupBuildCacheDefaultsToTinyGo(t *testing.T) {
	t.Setenv("E2E_WASM_STARTUP_BUILD_CACHE", "")
	t.Setenv(E2EWasmLegacyTinyGoEnv, "")
	t.Setenv(E2EWasmCompilerEnv, "")
	if E2EWasmStartupBuildCacheEnabled() {
		t.Fatal("startup build cache should default off with the Go compiler")
	}
	t.Setenv(E2EWasmCompilerEnv, "tinygo")
	if !E2EWasmStartupBuildCacheEnabled() {
		t.Fatal("startup build cache should default on with TinyGo")
	}
	t.Setenv(E2EWasmCompilerEnv, "goscript")
	if E2EWasmStartupBuildCacheEnabled() {
		t.Fatal("startup build cache should default off with GoScript")
	}
	t.Setenv("E2E_WASM_STARTUP_BUILD_CACHE", "false")
	if E2EWasmStartupBuildCacheEnabled() {
		t.Fatal("explicit startup build cache false should override compiler defaults")
	}
	t.Setenv("E2E_WASM_STARTUP_BUILD_CACHE", "true")
	t.Setenv(E2EWasmCompilerEnv, "")
	if !E2EWasmStartupBuildCacheEnabled() {
		t.Fatal("explicit startup build cache true should enable cache")
	}
}

func TestWorkerModeDefaultsToDedicated(t *testing.T) {
	t.Setenv(E2EWasmWorkerModeEnv, "")

	mode, err := ResolveE2EWasmWorkerMode("")
	if err != nil {
		t.Fatal(err)
	}
	if mode != WorkerModeDedicated {
		t.Fatalf("worker mode = %q, want %q", mode, WorkerModeDedicated)
	}
}

func TestWorkerModeSelectsSharedWorker(t *testing.T) {
	t.Setenv(E2EWasmWorkerModeEnv, "shared-worker")

	mode, err := ResolveE2EWasmWorkerMode("")
	if err != nil {
		t.Fatal(err)
	}
	if mode != WorkerModeShared {
		t.Fatalf("worker mode = %q, want %q", mode, WorkerModeShared)
	}
}

func TestResolveE2EWasmManifestBuildTimeout(t *testing.T) {
	t.Setenv(E2EWasmManifestBuildTimeoutEnv, "")

	defaultTimeout := 20 * time.Minute
	if got, err := ResolveE2EWasmManifestBuildTimeout(defaultTimeout); err != nil || got != defaultTimeout {
		t.Fatalf("default timeout = %s, %v; want %s, nil", got, err, defaultTimeout)
	}

	t.Setenv(E2EWasmManifestBuildTimeoutEnv, "45m")
	if got, err := ResolveE2EWasmManifestBuildTimeout(defaultTimeout); err != nil || got != 45*time.Minute {
		t.Fatalf("explicit timeout = %s, %v; want 45m, nil", got, err)
	}
}

func TestResolveE2EWasmManifestBuildTimeoutRejectsInvalid(t *testing.T) {
	t.Setenv(E2EWasmManifestBuildTimeoutEnv, "0")

	if _, err := ResolveE2EWasmManifestBuildTimeout(20 * time.Minute); err == nil {
		t.Fatal("expected invalid manifest build timeout to fail")
	}
}

func TestApplyE2EWasmTinyGoCompilerEnvDefaultsToFastProfile(t *testing.T) {
	clearBldrTinyGoEnv(t)
	clearE2EWasmTinyGoEnv(t)

	if err := ApplyE2EWasmTinyGoCompilerEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(gocompiler.TinyGoProfileEnv); got != gocompiler.TinyGoProfileFast {
		t.Fatalf("%s=%q, want %q", gocompiler.TinyGoProfileEnv, got, gocompiler.TinyGoProfileFast)
	}
	args, err := gocompiler.GetDefaultTinygoArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-opt=1") {
		t.Fatalf("tinygo args = %v, want -opt=1", args)
	}
	if slices.Contains(args, "-opt=0") {
		t.Fatalf("default e2e TinyGo profile should not use broken -opt=0: %v", args)
	}
	if !slices.Contains(args, "-interp-timeout=10m") {
		t.Fatalf("tinygo args = %v, want -interp-timeout=10m", args)
	}
	if !slices.Contains(args, "-gc=leaking") {
		t.Fatalf("tinygo args = %v, want -gc=leaking", args)
	}
	if got := os.Getenv(gocompiler.TinyGoGCEnv); got != "leaking" {
		t.Fatalf("%s=%q, want leaking", gocompiler.TinyGoGCEnv, got)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-scheduler=") {
			t.Fatalf("default e2e TinyGo profile should not set scheduler: %v", args)
		}
	}
}

func TestApplyE2EWasmTinyGoCompilerEnvCopiesExplicitDebugKnobs(t *testing.T) {
	clearBldrTinyGoEnv(t)
	clearE2EWasmTinyGoEnv(t)
	t.Setenv(E2EWasmTinyGoProfileEnv, gocompiler.TinyGoProfileFast)
	t.Setenv(E2EWasmTinyGoSchedulerEnv, "none")
	t.Setenv(E2EWasmTinyGoInterpTimeoutEnv, "12m")

	if err := ApplyE2EWasmTinyGoCompilerEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(gocompiler.TinyGoSchedulerEnv); got != "none" {
		t.Fatalf("%s=%q, want none", gocompiler.TinyGoSchedulerEnv, got)
	}
	if got := os.Getenv(gocompiler.TinyGoInterpTimeoutEnv); got != "12m" {
		t.Fatalf("%s=%q, want 12m", gocompiler.TinyGoInterpTimeoutEnv, got)
	}
}

func TestClearHarnessStateRootPreservesStartupBuildCache(t *testing.T) {
	root := t.TempDir()
	mustWriteHarnessStateFile(t, root, "devtool.s4wave")
	mustWriteHarnessStateFile(t, root, "devtool.s4wave-lock")
	mustWriteHarnessStateFile(t, root, "devtool.db")
	mustWriteHarnessStateFile(t, root, "logs/current.log")
	mustWriteHarnessStateFile(t, root, "src/go.mod")
	mustWriteHarnessStateFile(t, root, "plugin/state/state.bin")
	mustWriteHarnessStateFile(t, root, "build/web/js/wasm/spacewave-core/out")
	mustWriteHarnessStateFile(t, root, "cli/out")

	if err := clearHarnessStateRoot(root, true); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"devtool.s4wave", "devtool.s4wave-lock", "devtool.db"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected %s to be preserved: %v", rel, err)
		}
	}
	for _, rel := range []string{"logs", "src", "plugin", "build", "cli"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", rel, err)
		}
	}
}

func TestClearHarnessStateRootRemovesStartupBuildCache(t *testing.T) {
	root := t.TempDir()
	mustWriteHarnessStateFile(t, root, "devtool.s4wave")
	mustWriteHarnessStateFile(t, root, "devtool.s4wave-lock")
	mustWriteHarnessStateFile(t, root, "devtool.db")
	mustWriteHarnessStateFile(t, root, "src/go.mod")

	if err := clearHarnessStateRoot(root, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"devtool.s4wave", "devtool.s4wave-lock", "devtool.db", "src"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", rel, err)
		}
	}
}

func mustWriteHarnessStateFile(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func clearBldrTinyGoEnv(t *testing.T) {
	t.Helper()
	for _, key := range gocompiler.TinyGoStartupCacheEnvKeys() {
		t.Setenv(key, "")
	}
}

func clearE2EWasmTinyGoEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		E2EWasmCompilerEnv,
		E2EWasmLegacyTinyGoEnv,
		E2EWasmTinyGoProfileEnv,
		E2EWasmTinyGoOptEnv,
		E2EWasmTinyGoPanicEnv,
		E2EWasmTinyGoGCEnv,
		E2EWasmTinyGoSchedulerEnv,
		E2EWasmTinyGoLLVMFeaturesEnv,
		E2EWasmTinyGoInterpTimeoutEnv,
	} {
		t.Setenv(key, "")
	}
}
