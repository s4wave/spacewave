//go:build !skip_e2e && !js

package electron

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/e2e/electron/cdpretry"
	"github.com/sirupsen/logrus"
)

var testHarness *Harness

// TIER: nightly
func TestMain(m *testing.M) {
	// Prepare debug logging for the Electron test harness.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Skip the Electron suite unless its runtime is explicitly enabled.
	if !E2EElectronEnabled() {
		le.Info("skipping e2e/electron package; set ENABLE_E2E_ELECTRON=true to run")
		os.Exit(0)
	}

	// Boot Electron and attach its shared Playwright driver for the suite.
	h, err := Boot(context.Background(), le)
	if err != nil {
		le.WithError(err).Fatal("boot electron harness")
	}
	if err := h.ConnectDriver(); err != nil {
		h.Release()
		le.WithError(err).Fatal("connect electron CDP driver")
	}
	testHarness = h

	// Run the Electron suite and release the runtime before exiting.
	code := m.Run()
	h.Release()
	os.Exit(code)
}

func TestElectronHarnessBootCDP(t *testing.T) {
	// Require the shared Electron harness and its configured endpoints.
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}
	if h.CDPEndpoint() == "" {
		t.Fatal("expected CDP endpoint")
	}
	if h.StateRoot() == "" {
		t.Fatal("expected isolated state root")
	}

	// Wait for an Electron renderer page within the test context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	page, err := h.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Require the Electron renderer to use the app URL scheme.
	if url := page.URL(); !strings.HasPrefix(url, "app://") {
		t.Fatalf("expected app:// renderer URL, got %q", url)
	}

	// Require the renderer user agent to identify Electron.
	ua, err := cdpretry.EvaluateUserAgent(ctx, page, func(ctx context.Context) (cdpretry.Page, error) {
		return h.WaitForPage(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ua, "Electron") {
		t.Fatalf("expected Electron renderer user agent, got %q", ua)
	}
}
