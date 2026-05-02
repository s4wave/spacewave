package electron

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
)

func TestFactoryCopiesQuitPolicyToElectronInit(t *testing.T) {
	factory := NewFactory(nil)
	ctrl, err := factory.Construct(context.Background(), &Config{
		ElectronPath:  "electron",
		RendererPath:  "app.asar/index.mjs",
		WebRuntimeId:  "runtime",
		QuitPolicy:    QuitPolicy_QUIT_POLICY_EXIT,
		ExternalLinks: ExternalLinks_EXTERNAL_LINKS_DENY,
		AppName:       "Spacewave",
		WindowTitle:   "Spacewave",
		WindowWidth:   1200,
		WindowHeight:  800,
		DevTools:      true,
		ThemeSource:   "dark",
	}, controller.ConstructOpts{})
	if err != nil {
		t.Fatal(err)
	}

	electronCtrl := ctrl.(*Controller)
	init := electronCtrl.electronInit
	if got := init.GetQuitPolicy(); got != QuitPolicy_QUIT_POLICY_EXIT {
		t.Fatalf("quit policy = %v, want %v", got, QuitPolicy_QUIT_POLICY_EXIT)
	}
}
