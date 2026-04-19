package bldr_web_plugin_compiler

import (
	"testing"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	electron "github.com/aperturerobotics/bldr/web/plugin/electron"
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
