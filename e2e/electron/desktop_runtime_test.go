//go:build !skip_e2e && !js

package electron

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

const desktopRuntimeStateWaitTimeout = 45 * time.Second

type desktopRuntimeStateSnapshot struct {
	MainWindowOpen bool   `json:"mainWindowOpen"`
	StatusText     string `json:"statusText"`
}

type desktopTrayStateSnapshot struct {
	StatusText string `json:"statusText"`
}

// TIER: nightly
func TestDesktopRuntimeStateTracksLiveTrayProjectionAndActivation(t *testing.T) {
	// Require the shared Electron harness for desktop projection checks.
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	// Open an Electron app page within the test context.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if _, err := ensureAppPage(ctx, h); err != nil {
		t.Fatal(err)
	}

	// Require the live desktop and tray projections to describe the running window.
	state, err := waitForDesktopRuntimeState(ctx, h, func(state *desktopRuntimeStateSnapshot) bool {
		return state.MainWindowOpen
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.StatusText == "" {
		t.Fatal("expected desktop runtime status text")
	}
	if _, err := waitForDesktopTrayState(ctx, h, func(state *desktopTrayStateSnapshot) bool {
		return state.StatusText == "Running"
	}); err != nil {
		t.Fatal(err)
	}

	// Close the Electron app windows and require the runtime projection to follow.
	closeAppPages(t, h.AppPages())
	if err := h.WaitForNoAppPages(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForDesktopRuntimeState(ctx, h, func(state *desktopRuntimeStateSnapshot) bool {
		return !state.MainWindowOpen
	}); err != nil {
		t.Fatal(err)
	}

	// Reopen the Electron window through the desktop control endpoint.
	if err := postE2EControl(ctx, h, "/open-or-focus", url.Values{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.WaitForPage(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForDesktopRuntimeState(ctx, h, func(state *desktopRuntimeStateSnapshot) bool {
		return state.MainWindowOpen
	}); err != nil {
		t.Fatal(err)
	}
}

func ensureAppPage(ctx context.Context, h *Harness) (playwright.Page, error) {
	if len(h.AppPages()) == 0 {
		if err := postE2EControl(ctx, h, "/open-or-focus", url.Values{}); err != nil {
			return nil, err
		}
	}
	return h.WaitForPage(ctx)
}

func waitForDesktopRuntimeState(
	ctx context.Context,
	h *Harness,
	predicate func(*desktopRuntimeStateSnapshot) bool,
) (*desktopRuntimeStateSnapshot, error) {
	// Bound the wait for the requested Electron desktop projection.
	waitCtx, waitCancel := context.WithTimeout(ctx, desktopRuntimeStateWaitTimeout)
	defer waitCancel()

	// Observe Electron desktop state until the requested projection appears.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := getDesktopRuntimeState(waitCtx, h)
		if err == nil && predicate(state) {
			return state, nil
		}
		select {
		case <-waitCtx.Done():
			if err != nil {
				return nil, errors.Wrapf(waitCtx.Err(), "last desktop runtime state error: %v", err)
			}
			return nil, waitCtx.Err()
		case <-h.done:
			return nil, h.desktopRuntimeErr("desktop runtime exited before expected state")
		case <-ticker.C:
		}
	}
}

func waitForDesktopTrayState(
	ctx context.Context,
	h *Harness,
	predicate func(*desktopTrayStateSnapshot) bool,
) (*desktopTrayStateSnapshot, error) {
	// Bound the wait for the requested Electron tray projection.
	waitCtx, waitCancel := context.WithTimeout(ctx, desktopRuntimeStateWaitTimeout)
	defer waitCancel()

	// Observe Electron tray state until the requested projection appears.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := getDesktopTrayState(waitCtx, h)
		if err == nil && predicate(state) {
			return state, nil
		}
		select {
		case <-waitCtx.Done():
			if err != nil {
				return nil, errors.Wrapf(waitCtx.Err(), "last desktop tray state error: %v", err)
			}
			return nil, waitCtx.Err()
		case <-h.done:
			return nil, h.desktopRuntimeErr("desktop runtime exited before expected tray state")
		case <-ticker.C:
		}
	}
}

func getDesktopRuntimeState(
	ctx context.Context,
	h *Harness,
) (*desktopRuntimeStateSnapshot, error) {
	// Read the Electron desktop projection from its control endpoint.
	body, err := doE2EControl(ctx, h, http.MethodGet, "/desktop-state", nil)
	if err != nil {
		return nil, err
	}

	// Decode the Electron desktop projection response.
	var p fastjson.Parser
	v, err := p.ParseBytes(body)
	if err != nil {
		return nil, err
	}
	return parseDesktopRuntimeState(v), nil
}

func getDesktopTrayState(
	ctx context.Context,
	h *Harness,
) (*desktopTrayStateSnapshot, error) {
	// Read the Electron tray projection from its control endpoint.
	body, err := doE2EControl(ctx, h, http.MethodGet, "/tray-state", nil)
	if err != nil {
		return nil, err
	}

	// Decode the Electron tray projection response.
	var p fastjson.Parser
	v, err := p.ParseBytes(body)
	if err != nil {
		return nil, err
	}
	return parseDesktopTrayState(v), nil
}

func postE2EControl(
	ctx context.Context,
	h *Harness,
	path string,
	query url.Values,
) error {
	_, err := doE2EControl(ctx, h, http.MethodPost, path, query)
	return err
}

func doE2EControl(
	ctx context.Context,
	h *Harness,
	method string,
	path string,
	query url.Values,
) ([]byte, error) {
	// Prepare the Electron control request with its query parameters.
	endpoint := h.E2EControlEndpoint() + path
	if len(query) != 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}

	// Send the Electron control request and retain its response body.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the Electron control response and require a successful status.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.Errorf("e2e control %s %s returned %d: %s", method, path, resp.StatusCode, body)
	}
	return body, nil
}

func parseDesktopRuntimeState(v *fastjson.Value) *desktopRuntimeStateSnapshot {
	return &desktopRuntimeStateSnapshot{
		MainWindowOpen: v.GetBool("mainWindowOpen"),
		StatusText:     string(v.GetStringBytes("statusText")),
	}
}

func parseDesktopTrayState(v *fastjson.Value) *desktopTrayStateSnapshot {
	return &desktopTrayStateSnapshot{
		StatusText: string(v.GetStringBytes("statusText")),
	}
}
