package comms

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

const (
	webDocumentUnixFSFixturePath = "fs/u/1/so/01kwd6qwtkjb3z1whtxys72s4s/-/files/-/what is this.mp4"
	webDocumentUnixFSFixtureBody = "spacewave webdocument unixfs inline fixture\n"
	webDocumentQuickJSPluginPath = "b/pd/spacewave-web/plugin.mjs"
	webDocumentQuickJSPluginBody = `export default async function main(api) {
  const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
  api.handleStreamCtr.set(async (stream) => {
    const first = (await stream.source[Symbol.asyncIterator]().next()).value || new Uint8Array(0)
    await stream.sink((async function* () {
      if (first[0] === 21) {
        yield new Uint8Array([22])
        return
      }
      if (first[0] === 31) {
        yield new Uint8Array([32])
        let released = false
        for await (const packet of stream.source) {
          if (packet[0] !== 33) {
            throw new Error('unexpected in-flight reload trigger packet: ' + Array.from(packet).join(','))
          }
          released = true
          break
        }
        if (!released) {
          throw new Error('in-flight reload trigger stream closed before release')
        }
        yield new Uint8Array([34])
        await delay(0)
        const retryStream = await api.openStream()
        const responsePackets = []
        const readResponse = (async () => {
          for await (const packet of retryStream.source) {
            responsePackets.push(packet)
            break
          }
        })()
        await retryStream.sink((async function* () {
          const startInfo = new TextEncoder().encode(JSON.stringify(api.startInfo || {}))
          const request = new Uint8Array(startInfo.length + 1)
          request[0] = 11
          request.set(startInfo, 1)
          yield request
        })())
        await readResponse
        const response = responsePackets[0] || new Uint8Array(0)
        if (response[0] !== 12) {
          throw new Error('unexpected in-flight WebRuntime response packet: ' + Array.from(response).join(','))
        }
        return
      }
      yield new Uint8Array([99])
    })())
  })

  const stream = await api.openStream()
  const responsePackets = []
  const readResponse = (async () => {
    for await (const packet of stream.source) {
      responsePackets.push(packet)
      break
    }
  })()
  await stream.sink((async function* () {
    const startInfo = new TextEncoder().encode(JSON.stringify(api.startInfo || {}))
    const request = new Uint8Array(startInfo.length + 1)
    request[0] = 11
    request.set(startInfo, 1)
    yield request
  })())
  await readResponse
  const response = responsePackets[0] || new Uint8Array(0)
  if (response[0] !== 12) {
    throw new Error('unexpected WebRuntime response packet: ' + Array.from(response).join(','))
  }
  console.info('__BLDR_QUICKJS_PLUGIN_READY__')
  const keepAlive = Promise.withResolvers()
  await keepAlive.promise
}
`
)

type webDocumentRouteFixtureVariant string

const (
	webDocumentRouteBaseline          webDocumentRouteFixtureVariant = "baseline"
	webDocumentRouteDynamicRelay      webDocumentRouteFixtureVariant = "dynamic-relay"
	webDocumentRouteReleaseGeneration webDocumentRouteFixtureVariant = "release-generation"
	webDocumentRouteInFlightReload    webDocumentRouteFixtureVariant = "in-flight-reload"
)

type webDocumentRouteFixtureTrace struct {
	results      map[string]any
	eventLines   []string
	failureLines []string
}

// TestGoScriptForegroundUnixFSFetchKeepsSpacewaveWebRuntimeRoute verifies that
// a foreground WebDocument keeps the spacewave-web GoScript/QuickJS runtime
// route alive across a same-origin UnixFS inline fetch.
func TestGoScriptForegroundUnixFSFetchKeepsSpacewaveWebRuntimeRoute(t *testing.T) {
	installWebDocumentRouteFixtureAssets(t)

	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			trace := runWebDocumentRouteFixture(t, browser, webDocumentRouteBaseline)
			assertWebDocumentRouteSurvived(t, trace, true)
		})
	}
}

// TestGoScriptForegroundUnixFSFetchDynamicRelayKeepsRoute verifies that the
// foreground QuickJS route survives when the UnixFS-looking request is served
// through the ServiceWorker runtime relay seam with delayed response headers.
func TestGoScriptForegroundUnixFSFetchDynamicRelayKeepsRoute(t *testing.T) {
	installWebDocumentRouteFixtureAssets(t)

	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteDynamicRelay)
	assertWebDocumentRouteSurvived(t, trace, true)
	assertBoolResult(t, trace.results, "dynamicRelayFetch", true)
	assertBoolResult(t, trace.results, "dynamicRelayUsed", true)
	t.Logf("dynamic relay events: %s", strings.Join(webDocumentResultEventLog(trace.results), " | "))
}

// TestGoScriptForegroundUnixFSFetchReleaseGenerationMismatchReloadsBeforeNormalClose
// verifies that a promoted-generation mismatch broadcast is sufficient to reload
// the foreground route while the UnixFS-looking fetch is in flight.
func TestGoScriptForegroundUnixFSFetchReleaseGenerationMismatchReloadsBeforeNormalClose(t *testing.T) {
	installWebDocumentRouteFixtureAssets(t)

	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteReleaseGeneration)
	results := trace.results
	if pass, ok := results["pass"].(bool); !ok || !pass {
		t.Fatalf("release-generation fixture failed: %v", results["detail"])
	}
	assertBoolResult(t, results, "releaseBroadcast", true)
	assertBoolResult(t, results, "reloadObserved", true)
	assertBoolResult(t, results, "reloadBeforeNormalClose", true)
	assertBoolResult(t, results, "reproduced", true)
	assertBoolResult(t, results, "restartSentinelStable", false)
	t.Logf("release-generation events: %s", strings.Join(webDocumentResultEventLog(results), " | "))
}

// TestGoScriptForegroundUnixFSFetchInFlightReloadZeroDocumentRace verifies that
// an in-flight plugin-side stream open waits across a transient zero-WebDocument
// reload window and resumes through the replacement WebDocument route.
func TestGoScriptForegroundUnixFSFetchInFlightReloadZeroDocumentRace(t *testing.T) {
	installWebDocumentRouteFixtureAssets(t)

	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteInFlightReload)
	assertWebDocumentRouteSurvived(t, trace, true)
	assertBoolResult(t, trace.results, "zeroDocumentRace", true)
	assertBoolResult(t, trace.results, "replacementRoute", true)
	assertBoolResult(t, trace.results, "inFlightOpenRecovered", true)
	assertWebDocumentRouteEventContains(
		t,
		trace,
		"PluginWorker: plugin/spacewave-web: no WebDocument available, waiting for next WebDocument",
	)
	t.Logf("in-flight reload events: %s", strings.Join(webDocumentResultEventLog(trace.results), " | "))
}

func installWebDocumentRouteFixtureAssets(t *testing.T) {
	t.Helper()
	writeWebDocumentRouteAsset(t, webDocumentUnixFSFixturePath, []byte(webDocumentUnixFSFixtureBody))
	writeWebDocumentRouteAsset(t, webDocumentQuickJSPluginPath, []byte(webDocumentQuickJSPluginBody))
	copyWebDocumentRouteAsset(
		t,
		filepath.Join(repoRoot, "..", "node_modules", "quickjs-wasi-reactor", "qjs-wasi.wasm"),
		"b/qjs/qjs-wasi.wasm",
	)
	copyWebDocumentRouteAsset(
		t,
		filepath.Join(repoRoot, "plugin", "host", "wazero-quickjs", "plugin-quickjs.esm.js"),
		"b/qjs/plugin-quickjs.esm.js",
	)
}

func writeWebDocumentRouteAsset(t *testing.T, relPath string, body []byte) {
	t.Helper()
	path := filepath.Join(distDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture asset dir: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture asset %s: %v", relPath, err)
	}
}

func copyWebDocumentRouteAsset(t *testing.T, srcPath, relPath string) {
	t.Helper()
	body, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read fixture source %s: %v", srcPath, err)
	}
	writeWebDocumentRouteAsset(t, relPath, body)
}

func assertWebDocumentRouteSurvived(t *testing.T, trace webDocumentRouteFixtureTrace, assertRestartSentinel bool) {
	t.Helper()
	results := trace.results

	if pass, ok := results["pass"].(bool); !ok || !pass {
		t.Fatalf("WebDocument UnixFS route fixture failed: %v", results["detail"])
	}

	assertBoolResult(t, results, "workerReady", true)
	assertBoolResult(t, results, "startInfo", true)
	assertBoolResult(t, results, "pluginToHostStream", true)
	assertBoolResult(t, results, "preFetchStream", true)
	assertBoolResult(t, results, "fetchSuccess", true)
	assertBoolResult(t, results, "postFetchStream", true)
	if assertRestartSentinel {
		assertBoolResult(t, results, "restartSentinelStable", true)
	}

	if failureReason, _ := results["failureReason"].(string); failureReason != "" {
		t.Fatalf("unexpected runtime failure: %s", failureReason)
	}

	for _, line := range trace.eventLines {
		for _, forbidden := range webDocumentRouteForbiddenEvents() {
			if strings.Contains(line, forbidden) {
				t.Fatalf("forbidden lifecycle event observed: %s", line)
			}
		}
	}
	if len(trace.failureLines) > 0 {
		t.Fatalf("browser failures: %s", strings.Join(trace.failureLines, "; "))
	}

	t.Logf("detail: %s", results["detail"])
}

func runWebDocumentRouteFixture(
	t *testing.T,
	browserName string,
	variant webDocumentRouteFixtureVariant,
) webDocumentRouteFixtureTrace {
	t.Helper()

	bt := browserType(browserName)
	browser, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(true),
	})
	if err != nil {
		if shouldSkipBrowserLaunch(browserName, err) {
			t.Skipf("skip %s: %v", browserName, err)
		}
		t.Fatalf("launch %s: %v", browserName, err)
	}
	defer browser.Close()

	ctx, err := browser.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	var eventMu sync.Mutex
	var eventLines []string
	var failureLines []string

	page.On("console", func(msg playwright.ConsoleMessage) {
		line := "[" + browserName + " console." + msg.Type() + "] " + msg.Text()
		t.Log(line)
		eventMu.Lock()
		eventLines = append(eventLines, line)
		if msg.Type() == "error" {
			failureLines = append(failureLines, "console.error: "+msg.Text())
		}
		eventMu.Unlock()
	})
	page.On("pageerror", func(err error) {
		line := "[" + browserName + " pageerror] " + err.Error()
		t.Log(line)
		eventMu.Lock()
		eventLines = append(eventLines, line)
		failureLines = append(failureLines, "pageerror: "+err.Error())
		eventMu.Unlock()
	})

	url := testServer.url + "/goscript-webdocument-unixfs-fetch.html"
	if variant != webDocumentRouteBaseline {
		url += "?variant=" + string(variant)
	}
	if _, err := page.Goto(url); err != nil {
		t.Fatalf("goto %s: %v", url, err)
	}

	logSel := page.Locator("#log")
	if err := logSel.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(30000),
	}); err != nil {
		t.Fatalf("wait for #log visible: %v", err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(logSel).ToContainText("DONE", playwright.LocatorAssertionsToContainTextOptions{
		Timeout: playwright.Float(30000),
	}); err != nil {
		text, _ := logSel.TextContent()
		eventMu.Lock()
		failures := strings.Join(failureLines, "; ")
		eventMu.Unlock()
		if failures != "" {
			t.Fatalf("fixture did not complete (text=%q, browser failures=%s): %v", text, failures, err)
		}
		t.Fatalf("fixture did not complete (text=%q): %v", text, err)
	}

	results, err := page.Evaluate("window.__results")
	if err != nil {
		t.Fatalf("evaluate window.__results: %v", err)
	}
	resultsMap, ok := results.(map[string]any)
	if !ok {
		t.Fatalf("window.__results is not an object: %T", results)
	}

	eventMu.Lock()
	trace := webDocumentRouteFixtureTrace{
		results:      resultsMap,
		eventLines:   append([]string(nil), eventLines...),
		failureLines: append([]string(nil), failureLines...),
	}
	eventMu.Unlock()
	return trace
}

func webDocumentResultEventLog(results map[string]any) []string {
	raw, ok := results["eventLog"].([]any)
	if !ok {
		return nil
	}
	lines := make([]string, 0, len(raw))
	for _, value := range raw {
		line, ok := value.(string)
		if ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func webDocumentRouteForbiddenEvents() []string {
	return []string{
		"PluginWorker: plugin/spacewave-web: no WebDocument available, exiting!",
		"closed while waiting for WebDocument",
	}
}

func assertWebDocumentRouteEventContains(t *testing.T, trace webDocumentRouteFixtureTrace, expected string) {
	t.Helper()
	for _, line := range trace.eventLines {
		if strings.Contains(line, expected) {
			return
		}
	}
	t.Fatalf("expected lifecycle event containing %q", expected)
}
