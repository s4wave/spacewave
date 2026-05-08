//go:build !skip_e2e && !js

package electron

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TIER: nightly
func TestDesktopRuntimeExplicitQuitStops(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := ensureAppPage(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := postE2EControl(ctx, h, "/quit", url.Values{}); err != nil {
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	}
	if err := waitForDesktopRuntimeEndpointsDown(ctx, h); err != nil {
		t.Fatal(err)
	}
}

func waitForDesktopRuntimeEndpointsDown(ctx context.Context, h *Harness) error {
	waitCtx, waitCancel := context.WithTimeout(ctx, desktopRuntimeStateWaitTimeout)
	defer waitCancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !e2eControlAvailable(waitCtx, h) && !cdpAvailable(waitCtx, h) {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func e2eControlAvailable(ctx context.Context, h *Harness) bool {
	_, err := doE2EControl(ctx, h, http.MethodGet, "/desktop-state", nil)
	return err == nil
}

func cdpAvailable(ctx context.Context, h *Harness) bool {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		h.CDPEndpoint()+"/json/version",
		nil,
	)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}
