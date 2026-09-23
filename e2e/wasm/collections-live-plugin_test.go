//go:build !skip_e2e && !js

package wasm

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestSpaceCollectionsPlugin builds Space-stored TypeScript on a real local
// Forge worker, installs it without closing the Space, and uses its custom UI
// from two pages. Each page owns its selection while both observe accepted votes.
func TestSpaceCollectionsPlugin(t *testing.T) {
	// Keep the real browser Session and its mounted Space alive across two pages.
	h := harness(t)
	ctx := t.Context()
	first := h.NewRetainedStatePageSession(t)
	page := first.Page()
	scenario := CreateDriveScenario(t, h, first)
	if err := first.ConnectResources(ctx); err != nil {
		t.Fatal(err)
	}
	mounted := mountForgeSpace(ctx, t, first, scenario.sessionIndex, scenario.spaceID)
	defer mounted.Release()

	// Source files and the build grant are normal accepted World state.
	session, err := first.MountSessionByIdx(ctx, scenario.sessionIndex)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Release()
	info, err := session.GetSessionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := peer.IDB58Decode(info.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	deviceBus, deviceResolver, deviceEngine, devicePeer := mountPluginBuildDevice(t, ctx, session, scenario.spaceID)
	seedColorsProject(t, ctx, mounted.eng, sender, devicePeer)

	// The paired native device owns its World writes and compiler transport.
	const engineID = "collections-browser-build"
	bus, resolver := deviceBus, deviceResolver
	resolver.AddFactory(worker_controller.NewFactory(bus))
	resolver.AddFactory(execution_controller.NewFactory(bus))
	resolver.AddFactory(forge_lib_kvtx.NewFactory(bus))
	for _, factory := range space_exec.BridgeFactories(space_exec.NewDefaultRegistryWithBus(bus)) {
		resolver.AddFactory(factory)
	}
	release, err := bus.AddController(ctx, world.NewEngineController(engineID, deviceEngine), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, workerRef, err := worker_controller.StartControllerWithConfig(ctx, bus,
		worker_controller.NewConfig(engineID, "workers/plugin-build", devicePeer, true))
	if err != nil {
		t.Fatal(err)
	}
	defer workerRef.Release()

	// Build and install through the developer's controls while the Space stays open.
	click := func(locator playwright.Locator) {
		t.Helper()
		if err := locator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(15000)}); err != nil {
			body, _ := page.Locator("body").InnerText()
			t.Fatalf("click: %v\n%s", err, body)
		}
	}
	spaceHash := "#/u/" + strconv.FormatUint(uint64(scenario.sessionIndex), 10) + "/so/" + scenario.spaceID + "/-/"
	NavigateHash(t, h, page, spaceHash+"projects/colors/-/ColorViewer.tsx")
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Edit file", Exact: new(true)}))
	editor := page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "File contents", Exact: new(true)})
	source, err := editor.InputValue()
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.Fill(strings.ReplaceAll(source, "Pick a color.", "Choose a color.")); err != nil {
		t.Fatal(err)
	}
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save file", Exact: new(true)}))
	if err := page.GetByText("File saved", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}

	// The builder reads the edited, accepted source through the selected Device.
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Open shared object menu", Exact: new(true)}))
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Plugins", Exact: new(true)}))
	click(page.Locator("summary").Filter(playwright.LocatorFilterOptions{HasText: "Build a TypeScript plugin"}))
	for label, value := range map[string]string{"Source folder": "projects/colors", "Build device": "devices/plugin-build"} {
		values := []string{value}
		if _, err := page.GetByLabel(label, playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Values: &values}); err != nil {
			t.Fatal(err)
		}
	}
	if err := page.GetByLabel("Plugin manifest ID", playwright.PageGetByLabelOptions{Exact: new(true)}).Fill("space-colors"); err != nil {
		t.Fatal(err)
	}
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Build", Exact: new(true)}))

	// Open the second Space during compilation so it observes the installation
	// and custom ObjectType registration without reloading its existing client.
	var other playwright.Page
	if os.Getenv("E2E_WASM_FRONTEND_DEVELOPMENT") != "true" {
		other = h.NewRetainedStatePageSession(t).Page()
		NavigateHash(t, h, other, spaceHash)
	}

	install := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Install in this Space", Exact: new(true)})
	if err := install.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(30000)}); err != nil {
		body, _ := page.Locator("body").InnerText()
		t.Fatalf("build did not finish: %v\n%s", err, body)
	}
	click(install)
	if err := page.GetByText("Plugin added. Its status appears in the installed list.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}

	// The installation pins the artifact before the plugin creates its typed object.
	settings, err := space_world.LookupSpaceSettingsBody(ctx, mounted.engWs)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.GetPluginInstallations()["space-colors"].GetManifestKeys()) != 1 {
		t.Fatal("installation did not retain its exact manifest")
	}
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create Color votes", Exact: new(true)}))
	if err := page.WaitForURL("**/-/colors", playwright.PageWaitForURLOptions{Timeout: playwright.Float(15000)}); err != nil {
		body, _ := page.Locator("body").InnerText()
		t.Fatalf("create Colors object: %v\n%s", err, body)
	}
	if os.Getenv("E2E_WASM_FRONTEND_DEVELOPMENT") == "true" {
		checkColorsLivePreview(t, page, click)
		return
	}

	// The Space menu can remain mounted after navigating to the new object.
	closeMenu := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Close shared object menu", Exact: new(true)})
	if visible, _ := closeMenu.IsVisible(); visible {
		click(closeMenu)
	}
	colors := page.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Colors ranked by votes"})
	if err := colors.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(15000)}); err != nil {
		body, _ := page.Locator("body").InnerText()
		t.Fatalf("custom viewer: %v\n%s", err, body)
	}
	if err := page.GetByText("Choose a color. Every vote is shared with this Space.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor(); err != nil {
		t.Fatalf("built viewer omitted the source edit: %v", err)
	}

	// Votes converge across pages while React selection remains local to each viewer.
	NavigateHash(t, h, other, spaceHash+"colors")
	otherColors := other.GetByRole("list", playwright.PageGetByRoleOptions{Name: "Colors ranked by votes"})
	if err := otherColors.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(15000)}); err != nil {
		t.Fatal(err)
	}
	blue := colors.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Ocean blue 0 votes", Exact: new(true)})
	green := otherColors.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Sea glass 0 votes", Exact: new(true)})
	click(blue)
	click(green)
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Vote for Ocean blue", Exact: new(true)}))
	if err := otherColors.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Ocean blue 1 vote", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if pressed, err := green.GetAttribute("aria-pressed"); err != nil || pressed != "true" {
		t.Fatalf("remote vote changed personal selection: %q, %v", pressed, err)
	}
	click(other.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Vote for Sea glass", Exact: new(true)}))
	if err := colors.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Sea glass 1 vote", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}

	// A new viewer reads the accepted records after its previous resources close.
	if _, err := other.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := otherColors.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Ocean blue 1 vote", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
}

// seedColorsProject registers the local worker and copies the shipped example
// into an ordinary Space UnixFS object before the developer opens the build UI.
func seedColorsProject(t *testing.T, ctx context.Context, engine world.Engine, sender, devicePeer peer.ID) {
	t.Helper()

	// Publish the example and its compiler configuration in one source transaction.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	const sourceKey = "projects/colors"
	_, _, err = unixfs_world.FsInit(ctx, tx, sender, sourceKey, unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"bldr.yaml": []byte(`{
  "id": "space-colors",
  "manifests": {"space-colors": {"builder": {
    "id": "bldr/plugin/compiler/js",
    "config": {
      "webPkgs": [{"id": "@s4wave/web", "exclude": true}],
      "viteConfigPaths": ["vite.config.ts"],
      "viteDisableProjectConfig": true,
      "modules": [
        {"kind": "JS_MODULE_KIND_BACKEND", "path": "./backend.ts", "entrypoint": true},
        {"kind": "JS_MODULE_KIND_FRONTEND", "path": "./ColorViewer.tsx"}
      ]
    }
  }}}
}`), "package.json": []byte(`{"type":"module","dependencies":{"zod":"4.3.6"},"devDependencies":{"@vitejs/plugin-react":"6.0.5","vite":"8.2.2"}}`), "vite.config.ts": []byte(`import react from '@vitejs/plugin-react'
export default { plugins: [react()] }
`)}
	for _, name := range []string{"backend.ts", "app.ts", "ColorViewer.tsx"} {
		data, err := os.ReadFile(filepath.Join("../../plugin/colors", name))
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ReplaceAll(string(data), "../../sdk/", "@go/github.com/s4wave/spacewave/sdk/")
		files[name] = []byte(strings.ReplaceAll(source, "plugin/colors/ColorViewer.tsx", "./ColorViewer.tsx"))
	}
	object, err := world.MustGetObject(ctx, tx, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(object)
	for name, data := range files {
		_, _, err := unixfs_world.FsMknodWithContent(ctx, object, sender, unixfs_world.FSType_FSType_FS_NODE,
			[]string{name}, unixfs.NewFSCursorNodeType_File(), int64(len(data)), bytes.NewReader(data), 0o644, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	// Grant the disposable Session's worker access through the Device capability.
	publicKey, err := devicePeer.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(publicKey, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = forge_worker.CreateWorker(ctx, tx, "workers/plugin-build", "plugin-builder", []*identity.Keypair{keypair}, sender)
	if err != nil {
		t.Fatal(err)
	}
	device := &s4wave_device.Device{
		PeerId: devicePeer.String(), Label: "Local plugin builder",
		SetupState: s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id: "worker", Kind: s4wave_device.DeviceCapabilityKindForgeWorker,
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link:  &s4wave_device.DeviceCapabilityLink{ObjectKey: "workers/plugin-build", TypeId: forge_worker.WorkerTypeID},
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "plugin-builder", GrantPolicyRef: "space/plugin-builder",
				LocalState: s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState: s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
			},
		}},
	}
	_, _, err = world.AccessWorldObject(ctx, tx, "devices/plugin-build", true, func(cursor *block.Cursor) error {
		cursor.SetBlock(device, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tx, "devices/plugin-build", s4wave_device.DeviceTypeID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// checkColorsLivePreview saves source through the workbench without losing the
// custom viewer's local selection or replacing its installed backend.
func checkColorsLivePreview(t *testing.T, page playwright.Page, click func(playwright.Locator)) {
	t.Helper()
	click(page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Edit with live preview", Exact: new(true)}))
	if err := page.GetByText("Live preview connected. Save a source file to apply its changes.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(25000)}); err != nil {
		body, _ := page.Locator("body").InnerText()
		t.Fatalf("preview startup: %v\n%s", err, body)
	}
	values := []string{"colors"}
	if _, err := page.GetByLabel("Preview object", playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Values: &values}); err != nil {
		t.Fatal(err)
	}
	preview := page.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Plugin preview"})
	click(preview.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Ocean blue 0 votes", Exact: new(true)}))
	sourcePane := page.GetByRole("region", playwright.PageGetByRoleOptions{Name: "Plugin source"})
	if err := sourcePane.GetByText("ColorViewer.tsx", playwright.LocatorGetByTextOptions{Exact: new(true)}).Dblclick(); err != nil {
		t.Fatal(err)
	}
	click(sourcePane.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Edit file", Exact: new(true)}))
	editor := sourcePane.GetByRole("textbox", playwright.LocatorGetByRoleOptions{Name: "File contents"})
	source, err := editor.InputValue()
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.Fill(strings.ReplaceAll(source, "Choose a color.", "Live colors.")); err != nil {
		t.Fatal(err)
	}
	click(sourcePane.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save file", Exact: new(true)}))
	if err := preview.GetByText("Live colors. Every vote is shared with this Space.", playwright.LocatorGetByTextOptions{Exact: new(true)}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(15000)}); err != nil {
		body, _ := page.Locator("body").InnerText()
		t.Fatalf("preview did not apply the source edit: %v\n%s", err, body)
	}
	if pressed, err := preview.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Ocean blue 0 votes", Exact: new(true)}).GetAttribute("aria-pressed"); err != nil || pressed != "true" {
		t.Fatalf("source edit reset selection: %q, %v", pressed, err)
	}
	click(preview.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Vote for Ocean blue", Exact: new(true)}))
	if err := preview.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Ocean blue 1 vote", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
}
