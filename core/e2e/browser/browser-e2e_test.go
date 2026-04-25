//go:build !skip_e2e && !js

package s4wave_core_e2e_browser_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/creack/pty"
	bldr_manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_process "github.com/s4wave/spacewave/bldr/plugin/host/process"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	"github.com/s4wave/spacewave/bldr/testbed"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	s4wave_core_e2e "github.com/s4wave/spacewave/core/e2e"
	s4wave_core_e2e_browser "github.com/s4wave/spacewave/core/e2e/browser"
	space_world_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

// TestBrowserE2EWithBldr runs the browser E2E tests with a full bldr backend.
// This test starts the complete bldr infrastructure and runs vitest browser tests.
func TestBrowserE2EWithBldr(t *testing.T) {
	if os.Getenv("RUN_BROWSER_E2E") == "" {
		t.Skip("set RUN_BROWSER_E2E=1 to run the browser E2E test")
	}

	// Skip if SKIP_BROWSER_E2E is set (for CI without browsers)
	if os.Getenv("SKIP_BROWSER_E2E") != "" {
		t.Skip("SKIP_BROWSER_E2E is set, skipping browser E2E tests")
	}

	// Determine which vitest mode to run
	uiMode := os.Getenv("BROWSER_TEST_UI") != ""
	watchMode := os.Getenv("BROWSER_TEST_WATCH") != ""

	// Use signal context - exits on Ctrl+C or when vitest exits
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.InfoLevel) // avoid too much log spam
	le := logrus.NewEntry(log)

	// get path to repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}
	repoRoot := filepath.Join(wd, "../../..")
	workDir := filepath.Join(wd, ".bldr")
	buildDir := filepath.Join(workDir, "build")
	distDir := filepath.Join(workDir, "src")
	pluginStateDir := filepath.Join(workDir, "plugin", "state")
	pluginDistDir := filepath.Join(workDir, "plugin", "dist")

	// cleanup the build dir if it exists
	if err := fsutil.CleanCreateDir(buildDir); err != nil {
		t.Fatal(err.Error())
	}
	if err := fsutil.CleanCreateDir(pluginStateDir); err != nil {
		t.Fatal(err.Error())
	}
	if err := fsutil.CleanCreateDir(pluginDistDir); err != nil {
		t.Fatal(err.Error())
	}

	// check out the web dist sources
	err = s4wave_core_e2e.CheckoutWebDistSources(ctx, le, distDir)
	if err != nil {
		t.Fatal(err.Error())
	}

	// build the bldr testbed
	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// add the controllers we will need
	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_process.NewFactory(b))
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(bldr_manifest_builder_controller.NewFactory(b))
	sr.AddFactory(bldr_plugin_compiler_go.NewFactory(b))
	sr.AddFactory(bldr_plugin_compiler_js.NewFactory(b))
	sr.AddFactory(bldr_web_bundler_vite_compiler.NewFactory(b))
	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	// start a peer controller to serve GetPeer directives
	volPeer, err := tb.GetVolume().GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	peerCtrl := peer_controller.NewController(le, volPeer)
	relPeerCtrl, err := tb.GetBus().AddController(ctx, peerCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relPeerCtrl()

	// start objecttype controller to resolve LookupObjectType directives
	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	relObjectTypeCtrl, err := tb.GetBus().AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relObjectTypeCtrl()

	// load the go plugin host
	processHost, _, processRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_process.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_process.NewConfig(pluginStateDir, pluginDistDir)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer processRef.Release()
	_ = processHost

	// load the js plugin host
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
	_ = quickjsHost

	// load the merged project config
	projectConfig, err := s4wave_core_e2e.LoadProjectConfig(repoRoot)
	if err == nil {
		err = projectConfig.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply the devtool remote for building manifests
	projectConfig.Remotes = map[string]*bldr_project.RemoteConfig{
		"devtool": {
			EngineId:       tb.GetWorldEngineID(),
			PeerId:         tb.GetVolume().GetPeerID().String(),
			ObjectKey:      tb.GetPluginHostObjKey(),
			LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		},
	}

	// configure the project controller
	projCtrlConf := bldr_project_controller.NewConfig(repoRoot, workDir, projectConfig, false, true)
	projCtrlConf.FetchManifestRemote = "devtool"

	// run the project controller, which also compiles and starts the plugins
	projCtrl, _, projCtrlRef, err := loader.WaitExecControllerRunningTyped[*bldr_project_controller.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer projCtrlRef.Release()
	_ = projCtrl

	// Start the browser test server that exposes the full resource API
	browserServer := s4wave_core_e2e_browser.NewBrowserTestServer(le, b)
	port, err := browserServer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start browser test server: %v", err)
	}
	defer browserServer.Stop(ctx)

	t.Logf("browser test server started on port %d", port)

	// Build vitest command arguments
	// Base command: vitest --config=vitest.browser.config.ts
	vitestArgs := []string{"vitest", "--config=vitest.browser.config.ts"}

	if uiMode {
		// Use browser-based UI for interactive debugging
		vitestArgs = append(vitestArgs, "--ui")
	} else if watchMode {
		// Terminal watch mode - keyboard shortcuts work (h for help, a to rerun all, etc.)
		// Don't add --run so vitest stays in watch mode
	} else {
		// Non-interactive mode exits after tests complete
		vitestArgs = append(vitestArgs, "--run")
	}

	// Add test name pattern filter if BROWSER_TEST_FILTER is set
	// This allows running specific tests: BROWSER_TEST_FILTER="OptimizedLayout" go test ...
	if testFilter := os.Getenv("BROWSER_TEST_FILTER"); testFilter != "" {
		vitestArgs = append(vitestArgs, "--testNamePattern", testFilter)
	}

	// Run vitest browser tests with the server port
	cmd := exec.CommandContext(ctx, "bun", vitestArgs...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))

	t.Log("running vitest browser tests with full bldr backend...")

	if uiMode || watchMode {
		// Use PTY for interactive modes so vitest can receive keyboard input
		if err := runWithPTY(ctx, cmd); err != nil {
			t.Logf("vitest exited: %v (this is normal in interactive mode)", err)
		}
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("vitest browser tests failed: %v", err)
		}
	}

	t.Log("browser E2E tests with bldr backend passed")
}

// runWithPTY runs a command with a pseudo-terminal for interactive mode.
func runWithPTY(ctx context.Context, cmd *exec.Cmd) error {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	// Handle pty size changes
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	defer signal.Stop(ch)
	_ = pty.InheritSize(os.Stdin, ptmx)

	// Set stdin in raw mode if it's a terminal
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		}
	}

	// Copy stdin to the pty and pty to stdout
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	go func() { _, _ = io.Copy(os.Stdout, ptmx) }()

	// Wait for the command to finish or context to be cancelled
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
