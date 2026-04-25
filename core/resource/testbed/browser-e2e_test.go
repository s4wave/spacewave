//go:build !js

package resource_testbed_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	browser_testbed "github.com/s4wave/spacewave/testbed/browser"
	"github.com/sirupsen/logrus"
)

// TestBrowserE2E runs the browser E2E tests with a live Go backend.
func TestBrowserE2E(t *testing.T) {
	// Skip if SKIP_BROWSER_E2E is set (for CI without browsers)
	if os.Getenv("SKIP_BROWSER_E2E") != "" {
		t.Skip("SKIP_BROWSER_E2E is set, skipping browser E2E tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create and start the browser test server using the consolidated LayoutServer
	server := browser_testbed.NewLayoutServer(le)
	port, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start browser test server: %v", err)
	}
	defer server.Stop(ctx)

	t.Logf("browser test server started on port %d", port)

	// Set up initial layout model for tests
	initialModel := &s4wave_layout.LayoutModel{
		Layout: &s4wave_layout.RowDef{
			Id: "root",
			Children: []*s4wave_layout.RowOrTabSetDef{
				{
					Node: &s4wave_layout.RowOrTabSetDef_TabSet{
						TabSet: &s4wave_layout.TabSetDef{
							Id: "tabset-1",
							Children: []*s4wave_layout.TabDef{
								{Id: "tab-1", Name: "Tab 1"},
								{Id: "tab-2", Name: "Tab 2"},
								{Id: "tab-closable", Name: "Closable Tab", EnableClose: true},
							},
						},
					},
				},
			},
		},
	}
	server.SetLayoutModel(initialModel)

	// Run vitest browser tests with the server port
	// Only run layout-related tests - the backend tests require full TestbedResourceService
	// which is provided by core/e2e/browser tests instead.
	projectRoot := findProjectRoot(t)
	cmd := exec.CommandContext(ctx, "bun", "vitest", "--config=vitest.browser.config.ts", "--run", "web/layout/")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), fmt.Sprintf("VITE_E2E_SERVER_PORT=%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	t.Log("running vitest browser tests...")
	if err := cmd.Run(); err != nil {
		t.Fatalf("vitest browser tests failed: %v", err)
	}

	t.Log("browser E2E tests passed")
}

// findProjectRoot finds the project root directory by looking for go.mod.
func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}
