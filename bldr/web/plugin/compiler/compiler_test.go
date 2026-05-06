package bldr_web_plugin_compiler

import (
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
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

func TestGetNativeAppSourcePath(t *testing.T) {
	if got := getNativeAppSourcePath("/src", ""); got != "" {
		t.Fatalf("empty native path = %q, want empty", got)
	}
	if got := getNativeAppSourcePath("/src", "assets/tray.png"); got != "/src/assets/tray.png" {
		t.Fatalf("native source path = %q, want %q", got, "/src/assets/tray.png")
	}
}
