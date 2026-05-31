//go:build !skip_e2e && !js

package wasm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const v86SmokeTimeoutMS = 180000
const v86SmokeDefaultCdnSpaceID = "01kpn3x0y79yr94ps1yae206vp"

func TestQuickstartV86BootSmoke(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_E2E")), "true") {
		t.Skip("set RUN_V86_E2E=true to run the v86 boot smoke")
	}

	sess := testHarness.NewCleanBlankSession(t)
	installV86CdnMirrorRuntimeEnv(t, sess)
	if err := testHarness.loadAppPageURL(sess, testHarness.baseURL+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during v86 smoke: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during v86 smoke: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	AssertRootImportMap(t, testHarness, page)
	cdnProbe := probeV86CdnMount(page)
	t.Logf("v86 CDN direct probe: %s", cdnProbe)
	if !strings.Contains(cdnProbe, `"defaultV86ImageFound": true`) {
		t.Fatalf("v86 CDN probe did not find the default V86Image before wizard load: %s", cdnProbe)
	}
	NavigateHash(t, testHarness, page, "#/quickstart/v86")
	WaitForApp(t, page)

	wait := playwright.LocatorWaitForOptions{Timeout: playwright.Float(v86SmokeTimeoutMS)}
	if err := page.Locator("input[placeholder='e.g. debian-lab']").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for v86 wizard name input: %v", err)
	}
	if err := page.Locator("text=/Will copy from CDN: (Aperture Linux|v86image-01kqx490m1sghtcw99sj1wzad9)/").First().WaitFor(wait); err != nil {
		t.Fatalf("wait for selected CDN V86Image: %v\n%s", err, readV86WizardDebug(page))
	}
	if err := page.Locator("input[placeholder='e.g. debian-lab']").First().Fill("v86 smoke"); err != nil {
		t.Fatalf("fill v86 VM name: %v", err)
	}
	createButton := page.Locator("button:visible:has-text('Create'):not([disabled])").Last()
	if err := createButton.Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(v86SmokeTimeoutMS)},
	); err != nil {
		body, _ := page.Locator("body").TextContent()
		t.Fatalf("create v86 VM: %v\nbody: %s", err, trimPageText(body))
	}
	if _, err := page.WaitForFunction(`() => {
		const hash = window.location.hash || ''
		if (hash.includes('/-/vm/v86/')) return true
		return Array.from(document.querySelectorAll('button')).some((button) =>
			(button.innerText || button.textContent || '').trim() === 'Creating...' && button.disabled
		)
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(1000),
	}); err != nil {
		t.Logf("Create button did not enter creating state after Playwright click; retrying DOM click")
		if _, evalErr := page.Evaluate(`() => {
			const button = Array.from(document.querySelectorAll('button')).find((candidate) =>
				(candidate.innerText || candidate.textContent || '').trim() === 'Create' && !candidate.disabled
			)
			if (!button) return false
			button.click()
			return true
		}`, nil); evalErr != nil {
			t.Fatalf("retry create v86 VM click: %v\n%s", evalErr, readV86WizardDebug(page))
		}
	}

	vmKey := waitForV86ObjectRoute(t, page)
	t.Logf("created VmV86 object %s", vmKey)
	if err := installV86SerialProbe(page, vmKey); err != nil {
		t.Fatalf("install v86 serial probe: %v", err)
	}
	if err := page.Locator("button:visible:has-text('Start')").First().Click(); err != nil {
		t.Fatalf("start v86 VM: %v", err)
	}
	serial := waitForV86SerialOutput(t, page)
	t.Logf("v86 serial output sample: %q", trimSerialSample(serial))
}

func installV86CdnMirrorRuntimeEnv(t testing.TB, sess *TestSession) {
	t.Helper()

	mirrorDir := strings.TrimSpace(os.Getenv("V86_E2E_CDN_MIRROR_DIR"))
	if mirrorDir == "" {
		return
	}
	spaceID := strings.TrimSpace(os.Getenv("V86_E2E_CDN_SPACE_ID"))
	if spaceID == "" {
		spaceID = v86SmokeDefaultCdnSpaceID
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("v86 CDN mirror request %s %s range=%q", r.Method, r.URL.Path, r.Header.Get("Range"))
		serveV86CdnMirrorHTTP(w, r, mirrorDir)
	}))
	t.Cleanup(srv.Close)

	envJSON, err := json.Marshal(map[string]string{
		"SPACEWAVE_CDN_BASE_URL": srv.URL,
		"SPACEWAVE_CDN_SPACE_ID": spaceID,
	})
	if err != nil {
		t.Fatalf("encode v86 CDN runtime env: %v", err)
	}
	script := fmt.Sprintf(
		"globalThis.BLDR_RUNTIME_WASM_ENV = Object.assign({}, globalThis.BLDR_RUNTIME_WASM_ENV || {}, %s);",
		envJSON,
	)
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install v86 CDN runtime env init script: %v", err)
	}
	t.Logf("serving v86 CDN mirror %s as %s for Space %s", mirrorDir, srv.URL, spaceID)
}

func serveV86CdnMirrorHTTP(w http.ResponseWriter, r *http.Request, mirrorDir string) {
	setV86CdnMirrorHeaders(w.Header())
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), string(filepath.Separator))
	path := filepath.Join(mirrorDir, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	body := data
	status := http.StatusOK
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		start, end, ok := parseV86CdnByteRange(rangeHeader, len(data))
		if !ok {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		status = http.StatusPartialContent
		body = data[start : end+1]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func setV86CdnMirrorHeaders(headers http.Header) {
	headers.Set("Accept-Ranges", "bytes")
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Headers", "Range,Content-Type")
	headers.Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS")
	headers.Set("Cross-Origin-Resource-Policy", "cross-origin")
	headers.Set("Cross-Origin-Embedder-Policy", "require-corp")
	headers.Set("Cross-Origin-Opener-Policy", "same-origin")
	headers.Set("Content-Type", "application/octet-stream")
}

func parseV86CdnByteRange(header string, size int) (int, int, bool) {
	const prefix = "bytes="
	if size <= 0 || !strings.HasPrefix(header, prefix) {
		return 0, 0, false
	}
	parts := strings.SplitN(strings.TrimPrefix(header, prefix), "-", 2)
	if len(parts) != 2 || parts[0] == "" {
		return 0, 0, false
	}
	start64, err := strconv.ParseInt(parts[0], 10, 0)
	if err != nil || start64 < 0 || start64 >= int64(size) {
		return 0, 0, false
	}
	end := size - 1
	if parts[1] != "" {
		end64, err := strconv.ParseInt(parts[1], 10, 0)
		if err != nil || end64 < start64 {
			return 0, 0, false
		}
		if end64 < int64(end) {
			end = int(end64)
		}
	}
	return int(start64), end, true
}

func waitForV86ObjectRoute(t testing.TB, page playwright.Page) string {
	t.Helper()

	handle, err := page.WaitForFunction(`() => {
		const hash = window.location.hash || ''
		const marker = '/-/'
		const idx = hash.indexOf(marker)
		if (idx === -1) return ''
		const rest = decodeURIComponent(hash.slice(idx + marker.length).split(/[?#]/, 1)[0])
		const body = document.body?.innerText || ''
		if (
			!rest ||
			rest.startsWith('wizard/') ||
			!/\bV86\b/.test(body) ||
			!/Start/.test(body) ||
			/No installed viewer handles this object type/.test(body)
		) {
			return ''
		}
		return rest
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(v86SmokeTimeoutMS),
	})
	if err != nil {
		t.Fatalf("wait for v86 object route: %v\n%s", err, readV86WizardDebug(page))
	}
	raw, err := handle.JSONValue()
	if err != nil {
		t.Fatalf("read v86 object route handle: %v", err)
	}
	vmKey, ok := raw.(string)
	if !ok || strings.TrimSpace(vmKey) == "" {
		t.Fatalf("unexpected v86 object key from route: %#v", raw)
	}
	return vmKey
}

func installV86SerialProbe(page playwright.Page, vmKey string) error {
	_, err := page.Evaluate(`(vmKey) => {
		const channel = new BroadcastChannel('v86-serial-' + vmKey)
		const probe = {
			vmKey,
			text: '',
			close() {
				channel.close()
			},
		}
		channel.onmessage = (ev) => {
			const frame = ev.data
			if (!frame || frame.dir !== 'out' || typeof frame.byte !== 'number') return
			probe.text += String.fromCharCode(frame.byte)
			if (probe.text.length > 8192) probe.text = probe.text.slice(-8192)
		}
		globalThis.__v86SmokeSerial = probe
		return true
	}`, vmKey)
	return err
}

func waitForV86SerialOutput(t testing.TB, page playwright.Page) string {
	t.Helper()

	handle, err := page.WaitForFunction(`() => {
		const text = globalThis.__v86SmokeSerial?.text || ''
		if (/login:|# |\$ |Welcome|Linux version|Kernel command line/.test(text)) {
			return text
		}
		const body = document.body?.innerText || ''
		if (/Runtime error/.test(body)) {
			throw new Error(body.slice(0, 2000))
		}
		return ''
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(v86SmokeTimeoutMS),
	})
	if err != nil {
		serial, _ := page.Evaluate(`() => globalThis.__v86SmokeSerial?.text || ''`, nil)
		body, _ := page.Locator("body").TextContent()
		t.Fatalf(
			"wait for v86 serial output: %v\nserial: %s\nbody: %s",
			err,
			trimSerialSample(asString(serial)),
			trimPageText(body),
		)
	}
	raw, err := handle.JSONValue()
	if err != nil {
		t.Fatalf("read v86 serial output handle: %v", err)
	}
	return asString(raw)
}

func probeV86CdnMount(page playwright.Page) string {
	raw, err := page.Evaluate(`async () => {
		const out = {
			ok: false,
			stage: 'start',
			browserEnv: globalThis.BLDR_RUNTIME_WASM_ENV ?? null,
			cdnSpaceId: '',
			objectKeys: [],
			v86ImageKeys: [],
			error: '',
			stack: '',
		}
		const root = globalThis.__s4wave_debug?.root
		if (!root) {
			out.stage = 'root'
			out.error = 'missing debug root'
			return JSON.stringify(out, null, 2)
		}
		const signal = AbortSignal.timeout(15000)
		let cdn
		let space
		let world
		try {
			out.stage = 'getCdn'
			const res = await root.getCdn('', signal)
			cdn = res?.cdn
			if (!cdn) {
				out.error = 'getCdn returned no cdn'
				return JSON.stringify(out, null, 2)
			}
			out.stage = 'getCdnSpaceId'
			out.cdnSpaceId = await cdn.getCdnSpaceId(signal)
			out.stage = 'mountCdnSpace'
			space = await cdn.mountCdnSpace(signal)
			out.stage = 'accessWorldState'
			world = await space.accessWorldState(false, signal)
			out.stage = 'iterateObjects'
			const iter = await world.iterateObjects('', false, signal)
			try {
				for (let i = 0; i < 50 && await iter.valid(signal); i++) {
					out.objectKeys.push(await iter.key(signal))
					await iter.next(signal)
				}
			} finally {
				await iter.close(signal).catch(() => {})
				iter?.[Symbol.dispose]?.()
			}
			out.stage = 'listObjectsWithType'
			out.v86ImageKeys = await world.listObjectsWithType('vm/image/v86', signal)
			const defaultKey = 'v86image-01kqx490m1sghtcw99sj1wzad9'
			out.defaultV86ImageObjectKey = defaultKey
			out.stage = 'defaultV86Image'
			using defaultObj = await world.getObject(defaultKey, signal)
			if (defaultObj) {
				using defaultCursor = await defaultObj.accessWorldState(undefined, signal)
				const defaultResp = await defaultCursor.unmarshal({}, signal)
				out.defaultV86ImageFound = !!defaultResp.found && !!defaultResp.data?.length
				out.defaultV86ImageDataLength = defaultResp.data?.length ?? 0
			} else {
				out.defaultV86ImageFound = false
				out.defaultV86ImageDataLength = 0
			}
			out.stage = 'listed'
			out.ok = out.v86ImageKeys.length > 0
			return JSON.stringify(out, null, 2)
		} catch (err) {
			out.error = String(err?.message ?? err)
			out.stack = String(err?.stack ?? '')
			return JSON.stringify(out, null, 2)
		} finally {
			try {
				world?.[Symbol.dispose]?.()
			} catch {}
			try {
				space?.[Symbol.dispose]?.()
			} catch {}
			try {
				cdn?.[Symbol.dispose]?.()
			} catch {}
		}
	}`, nil)
	if err != nil {
		return "probe failed: " + err.Error()
	}
	return asString(raw)
}

func readV86WizardDebug(page playwright.Page) string {
	raw, err := page.Evaluate(`() => {
		const body = document.body
		const text = body?.innerText || body?.textContent || ''
		return JSON.stringify({
			url: window.location.href,
			hash: window.location.hash,
			text: text.slice(0, 3000),
			textTail: text.slice(-3000),
			inputs: Array.from(document.querySelectorAll('input')).map((input) => ({
				placeholder: input.getAttribute('placeholder') || '',
				value: input.value || '',
				disabled: input.disabled,
			})),
			buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
				text: (button.innerText || button.textContent || '').trim().slice(0, 160),
				disabled: button.disabled,
			})),
			startup: {
				status: globalThis.__swBootStatus ?? null,
				hasDebugRoot: !!globalThis.__s4wave_debug?.root,
				quickstartTiming: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
			},
		}, null, 2)
	}`, nil)
	if err != nil {
		return "debug: " + err.Error()
	}
	return "debug: " + asString(raw)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func trimSerialSample(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= 1000 {
		return s
	}
	return s[len(s)-1000:]
}
