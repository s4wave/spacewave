//go:build !skip_e2e && !js

package comms

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	e2eharness "github.com/s4wave/spacewave/e2e/harness"
)

// Two browsers run outside Playwright's launcher. "safari" is Safari on
// macOS, driven over WebDriver by a safaridriver the caller starts and names
// in SAFARIDRIVER_URL. "android" is Chrome on the one device adb sees;
// see e2eharness.AndroidChrome.

// safariDoneScript resolves a WebDriver async script with window.__results
// once #log contains "DONE".
const safariDoneScript = `
const done = arguments[arguments.length - 1]
const finished = () => (document.getElementById('log')?.textContent ?? '').includes('DONE')
const finish = () => done(window.__results ?? null)
if (finished()) {
  finish()
} else {
  new MutationObserver((_, observer) => {
    if (finished()) {
      observer.disconnect()
      finish()
    }
  }).observe(document.body, { subtree: true, childList: true, characterData: true })
}
`

// runSafari runs a fixture page in a new Safari WebDriver session, waits for
// "DONE" in #log, and returns window.__results as a map. Safari's WebDriver
// exposes no console, so failures surface only through the results.
func runSafari(t *testing.T, fixture string, run fixtureRun) map[string]any {
	t.Helper()
	driver := os.Getenv("SAFARIDRIVER_URL")
	if driver == "" {
		t.Skip("SAFARIDRIVER_URL is not set")
	}
	timeout := run.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	var a fastjson.Arena
	caps := a.NewObject()
	always := a.NewObject()
	always.Set("browserName", a.NewString("safari"))
	match := a.NewObject()
	match.Set("alwaysMatch", always)
	caps.Set("capabilities", match)
	session, err := webDriver(http.MethodPost, driver+"/session", caps)
	if err != nil {
		t.Fatalf("safari session: %v", err)
	}
	base := driver + "/session/" + string(session.GetStringBytes("sessionId"))
	t.Cleanup(func() { _, _ = webDriver(http.MethodDelete, base, nil) })

	timeouts := a.NewObject()
	timeouts.Set("script", a.NewNumberInt(int(timeout.Milliseconds())))
	if _, err := webDriver(http.MethodPost, base+"/timeouts", timeouts); err != nil {
		t.Fatalf("safari timeouts: %v", err)
	}
	page := testServer.url + "/" + fixture + ".html"
	if run.query != "" {
		page += "?" + run.query
	}
	target := a.NewObject()
	target.Set("url", a.NewString(page))
	if _, err := webDriver(http.MethodPost, base+"/url", target); err != nil {
		t.Fatalf("goto %s: %v", page, err)
	}
	script := a.NewObject()
	script.Set("script", a.NewString(safariDoneScript))
	script.Set("args", a.NewArray())
	value, err := webDriver(http.MethodPost, base+"/execute/async", script)
	if err != nil {
		t.Fatalf("fixture did not complete: %v", err)
	}
	results, ok := jsonAny(value).(map[string]any)
	if !ok {
		t.Fatalf("fixture results: %s", value)
	}
	return results
}

// webDriver sends one WebDriver command with an optional body and returns
// the value of its reply.
func webDriver(method, endpoint string, body *fastjson.Value) (*fastjson.Value, error) {
	var payload []byte
	if body != nil {
		payload = body.MarshalTo(nil)
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var p fastjson.Parser
	reply, err := p.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrapf(err, "%s %s: status %d", method, endpoint, resp.StatusCode)
	}
	value := reply.Get("value")
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%s %s: status %d: %s", method, endpoint, resp.StatusCode, value)
	}
	return value, nil
}

// jsonAny converts a JSON value to the Go values Playwright's Evaluate
// returns: maps, slices, strings, float64 numbers, bools, and nil.
func jsonAny(v *fastjson.Value) any {
	if v == nil {
		return nil
	}
	switch v.Type() {
	case fastjson.TypeObject:
		out := make(map[string]any)
		v.GetObject().Visit(func(key []byte, field *fastjson.Value) {
			out[string(key)] = jsonAny(field)
		})
		return out
	case fastjson.TypeArray:
		items := v.GetArray()
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = jsonAny(item)
		}
		return out
	case fastjson.TypeString:
		return string(v.GetStringBytes())
	case fastjson.TypeNumber:
		f, _ := v.Float64()
		return f
	case fastjson.TypeTrue:
		return true
	case fastjson.TypeFalse:
		return false
	}
	return nil
}

// androidContext connects to Chrome on the adb device and returns its
// profile's context. The device reaches the test server through adb reverse
// on the same port. Cleanup clears the test origin and removes the forwards.
func androidContext(t *testing.T) playwright.BrowserContext {
	t.Helper()
	server, err := url.Parse(testServer.url)
	if err != nil {
		t.Fatal(err)
	}
	device, err := e2eharness.ConnectAndroidChrome(pwInstance, server.Port())
	if errors.Is(err, e2eharness.ErrNoAndroidDevice) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := device.ClearOrigin(testServer.url); err != nil {
			t.Logf("clear %s on the device: %v", testServer.url, err)
		}
		device.Close()
	})
	return device.Context()
}
