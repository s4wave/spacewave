//go:build !skip_e2e && !js

package electron

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
)

const desktopRuntimeStateWaitTimeout = 45 * time.Second

type desktopRuntimeStateSnapshot struct {
	MainWindowOpen bool                           `json:"mainWindowOpen"`
	Quitting       bool                           `json:"quitting"`
	StatusText     string                         `json:"statusText"`
	Health         int32                          `json:"health"`
	Lifecycle      int32                          `json:"lifecycle"`
	Listener       desktopRuntimeListenerSnapshot `json:"listener"`
}

type desktopRuntimeListenerSnapshot struct {
	Reachability int32  `json:"reachability"`
	Label        string `json:"label"`
	Detail       string `json:"detail"`
	SocketPath   string `json:"socketPath"`
}

type desktopTrayStateSnapshot struct {
	StatusText string                     `json:"statusText"`
	Entries    []desktopTrayEntrySnapshot `json:"entries"`
}

type desktopTrayEntrySnapshot struct {
	ID     string                    `json:"id"`
	Label  string                    `json:"label"`
	Action desktopTrayActionSnapshot `json:"action"`
}

type desktopTrayActionSnapshot struct {
	Value string `json:"value"`
}

// TIER: nightly
func TestDesktopRuntimeStateTracksLiveTrayProjectionAndActivation(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if _, err := ensureAppPage(ctx, h); err != nil {
		t.Fatal(err)
	}

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
		return state.StatusText == "Running" &&
			state.hasEntryLabel("status-runtime", "CLI reachable") &&
			state.hasActionValue("action-copy-cli-socket", h.CLISocketPath())
	}); err != nil {
		t.Fatal(err)
	}

	closeAppPages(t, h.AppPages())
	if err := h.WaitForNoAppPages(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForDesktopRuntimeState(ctx, h, func(state *desktopRuntimeStateSnapshot) bool {
		return !state.MainWindowOpen
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForDesktopTrayState(ctx, h, func(state *desktopTrayStateSnapshot) bool {
		return state.hasActionValue("action-copy-cli-socket", h.CLISocketPath())
	}); err != nil {
		t.Fatal(err)
	}

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

func (s *desktopTrayStateSnapshot) hasEntryLabel(id, labelPart string) bool {
	for _, entry := range s.Entries {
		if entry.ID == id && strings.Contains(entry.Label, labelPart) {
			return true
		}
	}
	return false
}

func (s *desktopTrayStateSnapshot) hasActionValue(id, value string) bool {
	for _, entry := range s.Entries {
		if entry.ID == id && entry.Action.Value == value {
			return true
		}
	}
	return false
}

func waitForDesktopRuntimeState(
	ctx context.Context,
	h *Harness,
	predicate func(*desktopRuntimeStateSnapshot) bool,
) (*desktopRuntimeStateSnapshot, error) {
	waitCtx, waitCancel := context.WithTimeout(ctx, desktopRuntimeStateWaitTimeout)
	defer waitCancel()

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
	waitCtx, waitCancel := context.WithTimeout(ctx, desktopRuntimeStateWaitTimeout)
	defer waitCancel()

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
	body, err := doE2EControl(ctx, h, http.MethodGet, "/desktop-state", nil)
	if err != nil {
		return nil, err
	}
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
	body, err := doE2EControl(ctx, h, http.MethodGet, "/tray-state", nil)
	if err != nil {
		return nil, err
	}
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
	endpoint := h.E2EControlEndpoint() + path
	if len(query) != 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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
		Quitting:       v.GetBool("quitting"),
		StatusText:     string(v.GetStringBytes("statusText")),
		Health:         int32(v.GetInt("health")),
		Lifecycle:      int32(v.GetInt("lifecycle")),
		Listener: desktopRuntimeListenerSnapshot{
			Reachability: int32(v.GetInt("listener", "reachability")),
			Label:        string(v.GetStringBytes("listener", "label")),
			Detail:       string(v.GetStringBytes("listener", "detail")),
			SocketPath:   string(v.GetStringBytes("listener", "socketPath")),
		},
	}
}

func parseDesktopTrayState(v *fastjson.Value) *desktopTrayStateSnapshot {
	entries := v.GetArray("entries")
	state := &desktopTrayStateSnapshot{
		StatusText: string(v.GetStringBytes("statusText")),
		Entries:    make([]desktopTrayEntrySnapshot, 0, len(entries)),
	}
	for _, entry := range entries {
		state.Entries = append(state.Entries, desktopTrayEntrySnapshot{
			ID:    string(entry.GetStringBytes("id")),
			Label: string(entry.GetStringBytes("label")),
			Action: desktopTrayActionSnapshot{
				Value: string(entry.GetStringBytes("action", "value")),
			},
		})
	}
	return state
}
