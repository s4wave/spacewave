package bldr_web_plugin_compiler

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	electron "github.com/s4wave/spacewave/bldr/web/plugin/electron"
)

func TestGetElectronQuitPolicy(t *testing.T) {
	if got := getElectronQuitPolicy(bldr_manifest.BuildType_DEV, nil); got != electron.QuitPolicy_QUIT_POLICY_RESTART {
		t.Fatalf("dev default quit policy = %v, want %v", got, electron.QuitPolicy_QUIT_POLICY_RESTART)
	}
	if got := getElectronQuitPolicy(bldr_manifest.BuildType_RELEASE, nil); got != electron.QuitPolicy_QUIT_POLICY_EXIT {
		t.Fatalf("release default quit policy = %v, want %v", got, electron.QuitPolicy_QUIT_POLICY_EXIT)
	}

	nativeApp := &NativeAppConfig{QuitPolicy: QuitPolicy_QUIT_POLICY_RESTART}
	if got := getElectronQuitPolicy(bldr_manifest.BuildType_RELEASE, nativeApp); got != electron.QuitPolicy_QUIT_POLICY_RESTART {
		t.Fatalf("explicit restart quit policy = %v, want %v", got, electron.QuitPolicy_QUIT_POLICY_RESTART)
	}

	nativeApp = &NativeAppConfig{QuitPolicy: QuitPolicy_QUIT_POLICY_EXIT}
	if got := getElectronQuitPolicy(bldr_manifest.BuildType_DEV, nativeApp); got != electron.QuitPolicy_QUIT_POLICY_EXIT {
		t.Fatalf("explicit exit quit policy = %v, want %v", got, electron.QuitPolicy_QUIT_POLICY_EXIT)
	}
}

func TestGetElectronDesktopPresencePolicy(t *testing.T) {
	if got := getElectronDesktopPresencePolicy(nil); got != electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_UNSPECIFIED {
		t.Fatalf("nil desktop presence policy = %v, want %v", got, electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_UNSPECIFIED)
	}

	nativeApp := &NativeAppConfig{}
	if got := getElectronDesktopPresencePolicy(nativeApp); got != electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_UNSPECIFIED {
		t.Fatalf("unspecified desktop presence policy = %v, want %v", got, electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_UNSPECIFIED)
	}

	nativeApp = &NativeAppConfig{
		DesktopPresencePolicy: DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
	}
	if got := getElectronDesktopPresencePolicy(nativeApp); got != electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME {
		t.Fatalf("window lifetime desktop presence policy = %v, want %v", got, electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME)
	}

	nativeApp = &NativeAppConfig{
		DesktopPresencePolicy: DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
	}
	if got := getElectronDesktopPresencePolicy(nativeApp); got != electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND {
		t.Fatalf("tray background desktop presence policy = %v, want %v", got, electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND)
	}
}

func TestShouldBundleNativeWebRendererHonorsSkipEnv(t *testing.T) {
	t.Setenv(SkipNativeWebRendererEnvVar, "")
	if !shouldBundleNativeWebRenderer() {
		t.Fatal("empty skip env should bundle native web renderer")
	}

	t.Setenv(SkipNativeWebRendererEnvVar, "true")
	if shouldBundleNativeWebRenderer() {
		t.Fatal("true skip env should not bundle native web renderer")
	}

	t.Setenv(SkipNativeWebRendererEnvVar, "false")
	if !shouldBundleNativeWebRenderer() {
		t.Fatal("false skip env should bundle native web renderer")
	}
}

func TestElectronNoSandboxEnabledHonorsEnv(t *testing.T) {
	t.Setenv(ElectronNoSandboxEnvVar, "")
	if electronNoSandboxEnabled() {
		t.Fatal("empty no-sandbox env should not add Electron no-sandbox flag")
	}

	t.Setenv(ElectronNoSandboxEnvVar, "true")
	if !electronNoSandboxEnabled() {
		t.Fatal("true no-sandbox env should add Electron no-sandbox flag")
	}

	t.Setenv(ElectronNoSandboxEnvVar, "false")
	if electronNoSandboxEnabled() {
		t.Fatal("false no-sandbox env should not add Electron no-sandbox flag")
	}
}

func TestAddWebPluginStartupInputsIncludesSkipRendererEnv(t *testing.T) {
	t.Setenv("BLDR_WEB_RENDERER", "")
	t.Setenv(SkipNativeWebRendererEnvVar, "true")
	t.Setenv(ElectronNoSandboxEnvVar, "true")
	result := &bldr_manifest_builder.BuilderResult{}
	if err := addWebPluginStartupInputs(
		&bldr_manifest_builder.BuilderConfig{SourcePath: t.TempDir()},
		result,
	); err != nil {
		t.Fatal(err)
	}

	wantInputs := map[string]string{
		"BLDR_WEB_RENDERER":         "",
		SkipNativeWebRendererEnvVar: "true",
		ElectronNoSandboxEnvVar:     "true",
	}
	for _, input := range result.GetInputManifest().GetStartupInputs() {
		want, ok := wantInputs[input.GetKey()]
		if !ok {
			continue
		}
		if input.GetStringValue() != want {
			t.Fatalf("%s input = %q, want %q", input.GetKey(), input.GetStringValue(), want)
		}
		delete(wantInputs, input.GetKey())
	}
	for key := range wantInputs {
		t.Fatalf("missing startup input for %s", key)
	}
}

func TestAddWebPluginStartupInputsIncludesBrowserShimSources(t *testing.T) {
	sourcePath := t.TempDir()
	for _, relPath := range []string{
		".bldr/src/web/plugin/browser/web-plugin-browser.ts",
		".bldr/src/web/plugin/browser/browser_srpc.pb.ts",
		".bldr/src/plugin/plugin_srpc.pb.ts",
		".bldr/src/manifest/manifest.pb.ts",
	} {
		absPath := filepath.Join(sourcePath, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte("export {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := &bldr_manifest_builder.BuilderResult{}
	if err := addWebPluginStartupInputs(
		&bldr_manifest_builder.BuilderConfig{SourcePath: sourcePath},
		result,
	); err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, input := range result.GetInputManifest().GetFiles() {
		paths = append(paths, input.GetPath())
	}
	for _, relPath := range []string{
		".bldr/src/manifest/manifest.pb.ts",
		".bldr/src/plugin/plugin_srpc.pb.ts",
		".bldr/src/web/plugin/browser/browser_srpc.pb.ts",
		".bldr/src/web/plugin/browser/web-plugin-browser.ts",
	} {
		if !slices.Contains(paths, relPath) {
			t.Fatalf("startup input files=%v, want %s", paths, relPath)
		}
	}
}
