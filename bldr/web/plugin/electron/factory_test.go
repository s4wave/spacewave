package electron

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
)

func TestFactoryCopiesQuitPolicyToElectronInit(t *testing.T) {
	factory := NewFactory(nil)
	ctrl, err := factory.Construct(context.Background(), &Config{
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

	electronCtrl := ctrl.(*Controller)
	init := electronCtrl.electronInit
	if got := init.GetQuitPolicy(); got != QuitPolicy_QUIT_POLICY_EXIT {
		t.Fatalf("quit policy = %v, want %v", got, QuitPolicy_QUIT_POLICY_EXIT)
	}
	if got := init.GetDesktopPresencePolicy(); got != DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND {
		t.Fatalf("desktop presence policy = %v, want %v", got, DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND)
	}
	if got := init.GetTrayIconPath(); got != "/icons/tray.png" {
		t.Fatalf("tray icon path = %q, want %q", got, "/icons/tray.png")
	}
	if got := init.GetMacosTemplateTrayIconPath(); got != "/icons/tray-template.png" {
		t.Fatalf("macOS template tray icon path = %q, want %q", got, "/icons/tray-template.png")
	}
}
