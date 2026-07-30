//go:build !skip_e2e && !js

package electron

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

// commandKeyAttemptTimeout bounds one press-and-observe attempt.
const commandKeyAttemptTimeout = 5_000

// TIER: nightly
//
// The renderer key dispatcher owns command keybindings. The main process must
// therefore claim neither the leader nor the palette accelerator, which the
// control endpoint reports directly. The key presses below only observe that
// the renderer discovery surfaces are wired: Playwright drives the debugging
// protocol, which injects below the native layer where menu accelerators and
// globalShortcut intercept keys, so a press arrives at the renderer whether or
// not a native owner would have stolen it.
func TestElectronDoesNotClaimRendererCommandKeys(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	state, err := getGlobalShortcutState(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if state.LeaderRegistered {
		t.Fatal("main process registered the leader accelerator Control+Space")
	}
	if state.PaletteRegistered {
		t.Fatal("main process registered the palette accelerator CommandOrControl+K")
	}

	page, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	closeOtherAppPages(t, h, page)

	// Dismiss whichever surface is open even when an assertion below fails, so
	// the shared window returns clean for the rest of the suite.
	t.Cleanup(func() {
		_ = page.Keyboard().Press("Escape")
	})

	if err := pressUntilVisible(
		ctx,
		page,
		"Control+Space",
		`section[aria-label="Key sequence continuations"]`,
	); err != nil {
		t.Fatalf("which-key panel did not open on the leader key: %v", err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatalf("dismiss which-key panel: %v", err)
	}

	if err := page.Keyboard().Press("ControlOrMeta+KeyK"); err != nil {
		t.Fatalf("press palette key: %v", err)
	}
	if err := waitForVisibleSelector(
		page,
		`[data-slot="command-input"]`,
		shellUIWaitTimeout,
	); err != nil {
		t.Fatalf("palette did not open on the palette key: %v", err)
	}
}

type globalShortcutStateSnapshot struct {
	LeaderRegistered  bool
	PaletteRegistered bool
}

func getGlobalShortcutState(
	ctx context.Context,
	h *Harness,
) (*globalShortcutStateSnapshot, error) {
	body, err := doE2EControl(ctx, h, http.MethodGet, "/globalshortcut-state", nil)
	if err != nil {
		return nil, err
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(body)
	if err != nil {
		return nil, err
	}
	return &globalShortcutStateSnapshot{
		LeaderRegistered:  v.GetBool("leaderRegistered"),
		PaletteRegistered: v.GetBool("paletteRegistered"),
	}, nil
}

func waitForVisibleSelector(
	page playwright.Page,
	selector string,
	timeout float64,
) error {
	return page.Locator(selector).First().WaitFor(
		playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: new(timeout),
		},
	)
}

// pressUntilVisible presses a key until the surface it drives appears.
//
// A keybinding reaches the dispatcher only once the command registry snapshot
// carrying it arrives in the renderer, and a press delivered before that is
// simply discarded. Retrying the press is therefore the only way to separate a
// key that is not wired from a key that arrived early; each attempt first
// clears any partial sequence state so presses cannot accumulate.
func pressUntilVisible(
	ctx context.Context,
	page playwright.Page,
	keys string,
	selector string,
) error {
	for {
		if err := page.Keyboard().Press(keys); err != nil {
			return err
		}
		err := waitForVisibleSelector(page, selector, commandKeyAttemptTimeout)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		if err := page.Keyboard().Press("Escape"); err != nil {
			return err
		}
	}
}
