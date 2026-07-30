//go:build !skip_e2e && !js

package electron

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

var testHarness *Harness

// TIER: nightly
func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !E2EElectronEnabled() {
		le.Info("skipping e2e/electron package; set ENABLE_E2E_ELECTRON=true to run")
		os.Exit(0)
	}

	h, err := Boot(context.Background(), le)
	if err != nil {
		le.WithError(err).Fatal("boot electron harness")
	}
	if err := h.ConnectDriver(); err != nil {
		h.Release()
		le.WithError(err).Fatal("connect electron CDP driver")
	}
	testHarness = h

	code := m.Run()
	h.Release()
	os.Exit(code)
}

func TestElectronHarnessBootCDP(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	page, err := h.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if url := page.URL(); !strings.HasPrefix(url, "app://") {
		t.Fatalf("expected app:// renderer URL, got %q", url)
	}

	var uaRaw any
	for {
		uaRaw, err = page.Evaluate(`() => navigator.userAgent`)
		if err == nil {
			break
		}
		page, err = h.WaitForPage(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	ua, ok := uaRaw.(string)
	if !ok {
		t.Fatalf("expected string user agent, got %T", uaRaw)
	}
	if !strings.Contains(ua, "Electron") {
		t.Fatalf("expected Electron renderer user agent, got %q", ua)
	}
}
