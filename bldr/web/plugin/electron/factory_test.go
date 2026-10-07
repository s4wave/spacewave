package electron

import (
	"runtime"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
)

// TestFactoryCopiesPoliciesToElectronInit checks the platform-adjusted shell configuration.
func TestFactoryCopiesPoliciesToElectronInit(t *testing.T) {

	// Construct the controller from a fully populated config.
	factory := NewFactory(nil)
	ctrl, err := factory.Construct(t.Context(), &Config{
		ElectronPath:              "electron",
		RendererPath:              "app.asar/index.mjs",
		WebRuntimeId:              "runtime",
		QuitPolicy:                QuitPolicy_QUIT_POLICY_EXIT,
		ExternalLinks:             ExternalLinks_EXTERNAL_LINKS_DENY,
		AppName:                   "Spacewave",
		WindowTitle:               "Spacewave",
		WindowWidth:               1200,
		WindowHeight:              800,
		DevTools:                  true,
		ThemeSource:               "dark",
		DesktopPresencePolicy:     DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
		TrayIconPath:              "/icons/tray.png",
		MacosTemplateTrayIconPath: "/icons/tray-template.png",
	}, controller.ConstructOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Assert the electron init carried the configured policies.
	electronCtrl := ctrl.(*Controller)
	init := electronCtrl.electronInit
	if got := init.GetQuitPolicy(); got != QuitPolicy_QUIT_POLICY_EXIT {
		t.Fatalf("quit policy = %v, want %v", got, QuitPolicy_QUIT_POLICY_EXIT)
	}

	// Darwin ties daemon-controlled shell demand to its window.
	wantPresence := DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND
	if runtime.GOOS == "darwin" {
		wantPresence = DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME
	}
	if got := init.GetDesktopPresencePolicy(); got != wantPresence {
		t.Fatalf("desktop presence policy = %v, want %v", got, wantPresence)
	}

	// Preserve both configured tray icon paths.
	if got := init.GetTrayIconPath(); got != "/icons/tray.png" {
		t.Fatalf("tray icon path = %q, want %q", got, "/icons/tray.png")
	}
	if got := init.GetMacosTemplateTrayIconPath(); got != "/icons/tray-template.png" {
		t.Fatalf("macOS template tray icon path = %q, want %q", got, "/icons/tray-template.png")
	}
}

// TestEffectiveDesktopPresencePolicy checks the shell lifetime for each platform.
func TestEffectiveDesktopPresencePolicy(t *testing.T) {
	for _, tc := range []struct {
		// name identifies the platform policy case.
		name string
		// configured is the requested shell lifetime.
		configured DesktopPresencePolicy
		// goos selects the platform policy.
		goos string
		// want is the effective shell lifetime.
		want DesktopPresencePolicy
	}{
		{
			name:       "darwin overrides tray background",
			configured: DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
			goos:       "darwin",
			want:       DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
		},
		{
			name:       "darwin preserves window lifetime",
			configured: DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
			goos:       "darwin",
			want:       DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
		},
		{
			name:       "linux uses configured",
			configured: DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
			goos:       "linux",
			want:       DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND,
		},
		{
			name:       "windows uses configured",
			configured: DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
			goos:       "windows",
			want:       DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveDesktopPresencePolicy(tc.configured, tc.goos)
			if got != tc.want {
				t.Fatalf("effectiveDesktopPresencePolicy(%v, %q) = %v, want %v", tc.configured, tc.goos, got, tc.want)
			}
		})
	}
}
