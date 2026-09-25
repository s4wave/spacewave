//go:build !js

package harness

import (
	"os/exec"
	"strings"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

// ErrNoAndroidDevice is returned when adb is missing or sees no device.
var ErrNoAndroidDevice = errors.New("no adb device")

// AndroidChrome is Chrome on the one Android device adb sees, reached over the
// DevTools protocol through adb port forwarding. Chrome must be open and the
// device unlocked; a locked device freezes its tabs.
//
// Chrome on Android cannot create browser contexts, so tests share the
// profile's context and remove what they leave with ClearOrigin.
type AndroidChrome struct {
	// Browser is the connected browser. Closing it only disconnects.
	Browser playwright.Browser

	reverse string
	forward string
}

// ConnectAndroidChrome makes serverPort on the device reach the same port on
// this host and connects to the device's Chrome.
func ConnectAndroidChrome(pw *playwright.Playwright, serverPort string) (*AndroidChrome, error) {
	if _, err := exec.LookPath("adb"); err != nil {
		return nil, ErrNoAndroidDevice
	}
	if state, err := adb("get-state"); err != nil || state != "device" {
		return nil, errors.Wrap(ErrNoAndroidDevice, state)
	}

	a := &AndroidChrome{reverse: "tcp:" + serverPort}
	if _, err := adb("reverse", a.reverse, a.reverse); err != nil {
		return nil, errors.Wrap(err, "adb reverse")
	}
	port, err := adb("forward", "tcp:0", "localabstract:chrome_devtools_remote")
	if err != nil {
		a.Close()
		return nil, errors.Wrap(err, "adb forward")
	}
	a.forward = "tcp:" + port

	a.Browser, err = pw.Chromium.ConnectOverCDP("http://127.0.0.1:" + port)
	if err != nil {
		a.Close()
		return nil, errors.Wrap(err, "connect to Chrome on the device (is it open and unlocked?)")
	}
	return a, nil
}

// Context returns the profile's browser context.
func (a *AndroidChrome) Context() playwright.BrowserContext {
	return a.Browser.Contexts()[0]
}

// ClearOrigin closes the pages of origin and clears its storage. Pages close
// first because Chrome cannot clear OPFS under an open sync access handle.
func (a *AndroidChrome) ClearOrigin(origin string) error {
	ctx := a.Context()
	for _, page := range ctx.Pages() {
		if strings.HasPrefix(page.URL(), origin) {
			_ = page.Close()
		}
	}
	page, err := ctx.NewPage()
	if err != nil {
		return err
	}
	defer page.Close()
	cdp, err := ctx.NewCDPSession(page)
	if err != nil {
		return err
	}
	_, err = cdp.Send("Storage.clearDataForOrigin", map[string]any{
		"origin":       origin,
		"storageTypes": "all",
	})
	return err
}

// Close disconnects from Chrome and removes the port forwards.
func (a *AndroidChrome) Close() {
	if a.Browser != nil {
		_ = a.Browser.Close()
	}
	if a.forward != "" {
		_, _ = adb("forward", "--remove", a.forward)
	}
	_, _ = adb("reverse", "--remove", a.reverse)
}

// adb runs one adb command and returns its trimmed output.
func adb(args ...string) (string, error) {
	out, err := exec.Command("adb", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
