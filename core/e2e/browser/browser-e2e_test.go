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
	"slices"
	"strings"
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

// TIER: pr
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

	// Select the requested Vitest mode.
	uiMode := os.Getenv("BROWSER_TEST_UI") != ""
	watchMode := os.Getenv("BROWSER_TEST_WATCH") != ""

	// Stop the testbed on Ctrl+C, termination, or Vitest exit.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.InfoLevel) // avoid too much log spam
	le := logrus.NewEntry(log)

	// Resolve the repository and testbed paths.
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

	// Reset disposable build and plugin state directories.
	if err := fsutil.CleanCreateDir(buildDir); err != nil {
		t.Fatal(err.Error())
	}
	if err := fsutil.CleanCreateDir(pluginStateDir); err != nil {
		t.Fatal(err.Error())
	}
	if err := fsutil.CleanCreateDir(pluginDistDir); err != nil {
		t.Fatal(err.Error())
	}

	// Check out the web distribution sources.
	err = s4wave_core_e2e.CheckoutWebDistSources(ctx, le, repoRoot, distDir)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the native bldr testbed. The browser under test consumes the native
	// core API while frontend and fixture plugins run in QuickJS.
	tb, err := s4wave_core_e2e.NewNativeTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Register the controllers required by the browser build.
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

	// Start a peer controller to serve GetPeer directives.
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

	// Start the object-type controller for LookupObjectType directives.
	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	relObjectTypeCtrl, err := tb.GetBus().AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relObjectTypeCtrl()

	// Load the Go plugin host.
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

	// Load the JavaScript plugin host.
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

	// Load the merged project configuration.
	projectConfig, err := s4wave_core_e2e.LoadProjectConfig(repoRoot)
	if err == nil {
		err = projectConfig.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Route manifest builds through the devtool remote.
	projectConfig.Remotes = map[string]*bldr_project.RemoteConfig{
		"devtool": {
			EngineId:       tb.GetWorldEngineID(),
			PeerId:         tb.GetVolume().GetPeerID().String(),
			ObjectKey:      tb.GetPluginHostObjKey(),
			LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		},
	}

	// Configure and run the project controller.
	projCtrlConf := bldr_project_controller.NewConfig(repoRoot, workDir, projectConfig, false, true)
	projCtrlConf.FetchManifestRemote = "devtool"

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

	// Start the browser test server that exposes the full resource API.
	browserServer := s4wave_core_e2e_browser.NewBrowserTestServer(le, b)
	port, err := browserServer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start browser test server: %v", err)
	}
	defer browserServer.Stop(ctx)

	t.Logf("browser test server started on port %d", port)

	// Build vitest command arguments.
	// Every mode uses the browser Vitest configuration.
	vitestBaseArgs := []string{"vitest", "--config=vitest.browser.config.ts"}

	if uiMode {
		// Use the browser UI for interactive debugging.
		vitestArgs := append(slices.Clone(vitestBaseArgs), "--ui")
		if testFilter := os.Getenv("BROWSER_TEST_FILTER"); testFilter != "" {
			vitestArgs = append(vitestArgs, "--testNamePattern", testFilter)
		}
		cmd := exec.CommandContext(ctx, "bun", vitestArgs...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))
		t.Log("running vitest browser tests with full bldr backend...")
		if err := runWithPTY(ctx, cmd); err != nil {
			t.Logf("vitest exited: %v (this is normal in interactive mode)", err)
		}
	} else if watchMode {
		// Terminal watch mode - keyboard shortcuts work (h for help, a to rerun all, etc.)
		// Omit --run so Vitest stays in watch mode.
		vitestArgs := slices.Clone(vitestBaseArgs)
		if testFilter := os.Getenv("BROWSER_TEST_FILTER"); testFilter != "" {
			vitestArgs = append(vitestArgs, "--testNamePattern", testFilter)
		}
		cmd := exec.CommandContext(ctx, "bun", vitestArgs...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))
		t.Log("running vitest browser tests with full bldr backend...")
		if err := runWithPTY(ctx, cmd); err != nil {
			t.Logf("vitest exited: %v (this is normal in interactive mode)", err)
		}
	} else {
		testFiles, err := browserE2ETestFiles(repoRoot)
		if err != nil {
			t.Fatal(err.Error())
		}
		testFilter := os.Getenv("BROWSER_TEST_FILTER")
		t.Logf("running %d vitest browser test files with full bldr backend...", len(testFiles))
		for _, testFile := range testFiles {
			vitestArgs := append(slices.Clone(vitestBaseArgs), "--run", testFile)
			if testFilter != "" {
				vitestArgs = append(vitestArgs, "--testNamePattern", testFilter)
			}
			cmd := exec.CommandContext(ctx, "bun", vitestArgs...)
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			t.Logf("running vitest browser test file: %s", testFile)
			if err := cmd.Run(); err != nil {
				t.Fatalf("vitest browser tests failed for %s: %v", testFile, err)
			}
		}
	}

	t.Log("browser E2E tests with bldr backend passed")
}

func browserE2ETestFiles(repoRoot string) ([]string, error) {
	roots := []string{"app", "web", "core", "sdk", "plugin", "cmd", "forge"}
	var out []string
	for _, root := range roots {
		rootPath := filepath.Join(repoRoot, root)
		if _, err := os.Stat(rootPath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".e2e.test.ts") && !strings.HasSuffix(name, ".e2e.test.tsx") {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
			return nil
		}); err != nil {
			return nil, err
		}
	}
	slices.Sort(out)
	return out, nil
}

// runWithPTY runs a command with a pseudo-terminal for interactive mode.
func runWithPTY(ctx context.Context, cmd *exec.Cmd) error {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	// Propagate terminal resizes to the pseudo-terminal.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	defer signal.Stop(ch)
	_ = pty.InheritSize(os.Stdin, ptmx)

	// Put terminal input in raw mode for interactive controls.
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		}
	}

	// Bridge the parent terminal to the child pseudo-terminal.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	go func() { _, _ = io.Copy(os.Stdout, ptmx) }()

	// Wait for the command to finish or its context to be canceled.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
