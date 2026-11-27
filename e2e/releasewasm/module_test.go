//go:build !skip_e2e && !js

package releasewasm

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

func TestGoScriptServiceWorkerPluginDistModuleIntegrity(t *testing.T) {
	// Run only against the GoScript release build.
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript ServiceWorker plugin dist module probe", E2EReleaseWasmGoScriptEnv)
	}

	// Open the release root with HTTP tracing.
	t.Setenv("E2E_RELEASE_WASM_HTTP_TRACE", "1")
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production bundle into the quickstart Drive route.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/quickstart/drive')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}

	// Wait for the probed plugins to run.
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-core",
		"plugin/spacewave-launcher",
	})

	// Reach the quickstart Drive frame.
	t.Log("drive gate: wait for quickstart route")
	waitForQuickstartAppRoute(t, page)
	t.Log("drive gate: complete intro if present")
	completeQuickstartDriveIntroIfPresent(t, page)
	t.Log("drive gate: wait for unixfs browser frame")
	err = waitForDriveFrameReady(page, browserWaitMS)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart drive frame: %v", err)
	}

	// Exercise Drive so the plugins serve real traffic before the probe.
	t.Log("drive gate: wait for content ready")
	if _, quickstartErr := waitForQuickstartDriveContentReady(t, page); quickstartErr != "" {
		t.Fatalf("wait for quickstart drive content ready: %s", quickstartErr)
	}
	t.Log("drive gate: exercise golden path")
	if _, quickstartErr := exerciseQuickstartDriveGoldenPath(t, page); quickstartErr != "" {
		t.Fatalf("exercise quickstart drive golden path: %s", quickstartErr)
	}

	// Fetch each plugin entry and its relative module graph through the
	// ServiceWorker. The entry is a small loader that imports the GoScript
	// program chunks, so the size floor applies to the whole graph.
	raw, err := page.Evaluate(`async (args) => {
		await navigator.serviceWorker.ready
		const controllerURL = navigator.serviceWorker.controller?.scriptURL || ''
		if (!controllerURL) {
			throw new Error('page is not controlled by the release ServiceWorker')
		}

		const hasDefaultExport = (text) =>
			/\bexport\s*\{[^}]*\bas\s+default\b[^}]*\}\s*;?\s*$/.test(text) ||
			/\bexport\s+default\b/.test(text)
		const relativeImports = (text, baseURL) =>
			Array.from(
				text.matchAll(/\b(?:from|import)\s*\(?\s*["'\x60](\.{1,2}\/[^"'\x60]+\.mjs)["'\x60]/g),
				(match) => new URL(match[1], baseURL).href,
			)
		const failures = []
		const graphs = []

		// fetchModule returns one module body, or '' after recording a failure.
		const fetchModule = async (url, round) => {
			const requestURL = url + '?sw_module_integrity=' + round + '-' + Date.now()
			const response = await fetch(requestURL, { cache: 'reload' })
			const text = await response.text()
			const result = {
				path: new URL(url).pathname,
				round,
				status: response.status,
				contentType: response.headers.get('content-type') ?? '',
				bodyLength: text.length,
				head: text.slice(0, 120),
			}
			if (!response.ok) {
				failures.push({ ...result, reason: 'non-OK response' })
				return ''
			}
			if (/^\s*</.test(text)) {
				failures.push({ ...result, reason: 'response looks like HTML instead of JavaScript' })
				return ''
			}
			return text
		}

		for (let round = 0; round < args.rounds; round++) {
			for (const path of args.paths) {
				const entryURL = new URL(path, location.origin).href
				const entry = await fetchModule(entryURL, round)
				if (!entry) {
					continue
				}
				if (!hasDefaultExport(entry)) {
					failures.push({ path, round, tail: entry.slice(-180), reason: 'module body has no default export shape' })
				}

				const seen = new Set([entryURL])
				const queue = relativeImports(entry, entryURL)
				let graphLength = entry.length
				while (queue.length) {
					const url = queue.shift()
					if (seen.has(url)) {
						continue
					}
					seen.add(url)
					const text = await fetchModule(url, round)
					graphLength += text.length
					queue.push(...relativeImports(text, url))
				}

				const graph = { path, round, moduleCount: seen.size, graphLength }
				graphs.push(graph)
				if (graphLength < args.minGraphLength) {
					failures.push({ ...graph, reason: 'module graph shorter than expected minimum' })
				}
			}
		}

		if (failures.length) {
			throw new Error(
				'ServiceWorker plugin dist module integrity probe failed: ' +
					JSON.stringify({ controllerURL, failures, graphs }, null, 2),
			)
		}
		return { controllerURL, graphs }
	}`, map[string]any{
		"paths": []string{
			"/b/pd/spacewave-core/spacewave-core.mjs",
			"/b/pd/spacewave-launcher/spacewave-launcher.mjs",
		},
		"rounds":         3,
		"minGraphLength": 1024 * 1024,
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("probe ServiceWorker plugin dist module integrity: %v", err)
	}
	t.Logf("ServiceWorker plugin dist module integrity probe: %#v", raw)
}

func TestGoScriptQuickstartDriveLoadsAppModule(t *testing.T) {
	// Require the GoScript compiler for the Drive module probe.
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript quickstart Drive app module probe", E2EReleaseWasmGoScriptEnv)
	}

	// Open the release root with HTTP tracing.
	t.Setenv("E2E_RELEASE_WASM_HTTP_TRACE", "1")
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the Drive quickstart from the release root.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/quickstart/drive')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}

	// Wait for the core plugins and the Drive app module.
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-core",
		"plugin/spacewave-launcher",
	})
	waitForQuickstartAppRoute(t, page)
	waitForQuickstartDriveAppModule(t, page)
}

func waitForQuickstartDriveAppModule(t *testing.T, page playwright.Page) {
	// Wait for the Drive app module to load or fail.
	t.Helper()
	raw, err := page.WaitForFunction(`() => {
		const text = document.body?.innerText || ''
		const failed = text.match(/Failed to load module\s+(\S+)/)
		if (failed) {
			return {
				state: 'failed',
				modulePath: failed[1],
				text,
			}
		}
		if (
			document.querySelector("[data-testid='unixfs-browser']") ||
			text.includes('Create a Drive') ||
			text.includes('Drive Quickstart')
		) {
			return {
				state: 'loaded',
				href: location.href,
				text,
			}
		}
		return false
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart Drive app module: %v", err)
	}

	// Decode the Drive app module probe result.
	value, err := raw.JSONValue()
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("read quickstart Drive app module probe payload: %v", err)
	}
	state, ok := value.(map[string]any)
	if !ok {
		dumpPageState(t, page)
		t.Fatalf("unexpected quickstart Drive app module probe payload %T", value)
	}

	// Collect module delivery evidence when the Drive module fails.
	if state["state"] == "failed" {
		modulePath, _ := state["modulePath"].(string)
		if modulePath != "" {
			collectQuickstartModuleLoadDifferential(t, page, modulePath)
		}
		dumpPageState(t, page)
		t.Fatalf("quickstart Drive app module failed to load: %v", state["modulePath"])
	}

	// Record the loaded Drive app module state.
	t.Logf("quickstart Drive app module loaded: %#v", state)
}

func collectQuickstartModuleLoadDifferential(t *testing.T, page playwright.Page, modulePath string) {
	// Compare browser module delivery with the server and release artifact.
	t.Helper()
	browserProbe := collectBrowserModuleLoadDifferential(t, page, modulePath)
	rootDirect := directServerModuleProbe(t, modulePath)
	sonnerDirect := directServerModuleProbe(t, sonnerModulePath)
	sonnerArtifact := releaseWebPkgArtifactProbe(t, sonnerModulePath)
	report := moduleLoadDifferentialReport{
		ModulePath:   modulePath,
		BrowserProbe: browserProbe,
		DirectServer: moduleLoadDifferentialDirectServer{
			Root:   rootDirect,
			Sonner: sonnerDirect,
		},
		ReleaseArtifact: moduleLoadDifferentialArtifacts{
			Sonner: sonnerArtifact,
		},
	}

	// Serialize and preserve the module delivery comparison.
	var arena fastjson.Arena
	reportJSON := report.appendJSON(&arena).MarshalTo(nil)
	t.Logf("quickstart module load differential: %s", string(reportJSON))
	writeModuleLoadDifferentialArtifact(t, string(reportJSON))

	// Verify browser module delivery is complete and matches the artifact.
	assertBrowserModuleFetchComplete(t, "root App module", browserProbe.RootFetch)
	assertBrowserModuleFetchMatchesArtifact(t, "Sonner module", browserProbe.SonnerFetch, sonnerArtifact)
}

func collectBrowserModuleLoadDifferential(t *testing.T, page playwright.Page, modulePath string) browserModuleLoadDifferential {
	// Collect browser fetch and import evidence for the app and Sonner modules.
	t.Helper()
	raw, err := page.Evaluate(`async (args) => {
		const textEncoder = new TextEncoder()
		const textDecoder = new TextDecoder()
		const toHex = (bytes) =>
			Array.from(new Uint8Array(bytes))
				.map((byte) => byte.toString(16).padStart(2, '0'))
				.join('')
		const sha256Bytes = async (bytes) =>
			toHex(await crypto.subtle.digest('SHA-256', bytes))
		const summarizeBody = async (bytes) => {
			const text = textDecoder.decode(bytes)
			return {
				bodyComplete: true,
				bodyLength: text.length,
				bodyByteLength: bytes.byteLength,
				sha256: await sha256Bytes(bytes),
				head: text.slice(0, 160),
				tail: text.slice(-240),
			}
		}
		const collectChunks = (chunks, byteLength) => {
			const body = new Uint8Array(byteLength)
			chunks.reduce((offset, chunk) => {
				body.set(chunk, offset)
				return offset + chunk.byteLength
			}, 0)
			return body
		}
		const chunkByteLength = (chunks) =>
			chunks.reduce((total, chunk) => total + chunk.byteLength, 0)
		const readBody = async (response) => {
			const stream = response.body
			if (!stream) {
				const text = await response.text()
				const body = textEncoder.encode(text)
				return {
					...(await summarizeBody(body)),
					bodyReader: 'text',
					bodyChunks: body.byteLength > 0 ? 1 : 0,
				}
			}
			const reader = stream.getReader()
			const chunks = []
			try {
				for (;;) {
					const read = await reader.read()
					if (read.done) {
						break
					}
					if (read.value) {
						const chunk = read.value
						chunks.push(chunk)
					}
				}
			} catch (error) {
				const bodyByteLength = chunkByteLength(chunks)
				const partialBody = collectChunks(chunks, bodyByteLength)
				const partialText = textDecoder.decode(partialBody)
				return {
					ok: false,
					bodyComplete: false,
					bodyReader: 'stream',
					bodyChunks: chunks.length,
					bodyByteLength,
					partialSha256: await sha256Bytes(partialBody),
					partialHead: partialText.slice(0, 160),
					partialTail: partialText.slice(-240),
					name: error?.name ?? '',
					message: error?.message ?? String(error),
					stack: error?.stack ?? '',
				}
			}
			const bodyByteLength = chunkByteLength(chunks)
			return {
				...(await summarizeBody(collectChunks(chunks, bodyByteLength))),
				bodyReader: 'stream',
				bodyChunks: chunks.length,
			}
		}
		const headerObject = (headers) => {
			const out = {}
			for (const [key, value] of headers.entries()) {
				if (
					key === 'content-length' ||
					key === 'content-type' ||
					key.startsWith('x-bldr-')
				) {
					out[key] = value
				}
			}
			return out
		}
		const cacheBust = (path, label) =>
			path + (path.includes('?') ? '&' : '?') + label + '=' + Date.now()
		const fetchProbe = async (path, label) => {
			const requestURL = cacheBust(path, label)
			try {
				const response = await fetch(requestURL, { cache: 'reload' })
				try {
					const body = await readBody(response)
					return {
						path,
						requestURL,
						status: response.status,
						ok: response.ok && body.bodyComplete,
						headers: headerObject(response.headers),
						...body,
					}
				} catch (error) {
					return {
						path,
						requestURL,
						ok: false,
						phase: 'body',
						status: response.status,
						headers: headerObject(response.headers),
						name: error?.name ?? '',
						message: error?.message ?? String(error),
						stack: error?.stack ?? '',
					}
				}
			} catch (error) {
				return {
					path,
					requestURL,
					ok: false,
					phase: 'fetch',
					name: error?.name ?? '',
					message: error?.message ?? String(error),
					stack: error?.stack ?? '',
				}
			}
		}
		const importProbe = async (path, label) => {
			const requestURL = cacheBust(path, label)
			try {
				const mod = await import(/* @vite-ignore */ requestURL)
				return {
					path,
					requestURL,
					ok: true,
					exportKeys: Object.keys(mod).sort(),
					hasDefault: Object.prototype.hasOwnProperty.call(mod, 'default'),
				}
			} catch (error) {
				return {
					path,
					requestURL,
					ok: false,
					name: error?.name ?? '',
					message: error?.message ?? String(error),
					stack: error?.stack ?? '',
				}
			}
		}
		const performanceEntries = performance
			.getEntriesByType('resource')
			.map((entry) => ({
				name: entry.name,
				initiatorType: entry.initiatorType,
				transferSize: entry.transferSize,
				encodedBodySize: entry.encodedBodySize,
				decodedBodySize: entry.decodedBodySize,
			}))
			.filter((entry) =>
				entry.name.includes('/b/pa/') ||
				entry.name.includes('/b/pkg/sonner'),
			)
		return JSON.stringify({
			location: location.href,
			controllerURL: navigator.serviceWorker.controller?.scriptURL ?? '',
			rootAssetStatus: globalThis.__bldrWebViewRootAssetStatus ?? null,
			moduleImportError: globalThis.__bldrWebViewModuleImportError ?? null,
			rootFetch: await fetchProbe(args.modulePath, 'root_module_probe'),
			sonnerFetch: await fetchProbe(args.sonnerPath, 'sonner_module_probe'),
			rootImport: await importProbe(args.modulePath, 'root_import_probe'),
			sonnerImport: await importProbe(args.sonnerPath, 'sonner_import_probe'),
			performanceEntries,
		})
	}`, map[string]any{
		"modulePath": modulePath,
		"sonnerPath": sonnerModulePath,
	})
	if err != nil {
		t.Fatalf("collect browser module load differential: %v", err)
	}

	// Decode the browser module comparison payload.
	encoded, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected browser module load differential payload %T", raw)
	}

	// Parse the browser module comparison record.
	probe, err := parseBrowserModuleLoadDifferential(encoded)
	if err != nil {
		t.Fatalf("parse browser module load differential payload: %v", err)
	}
	return probe
}

func directServerModuleProbe(t *testing.T, modulePath string) moduleBodyProbe {
	// Require an absolute module request path.
	t.Helper()
	if !strings.HasPrefix(modulePath, "/") {
		t.Fatalf("module path must be absolute: %q", modulePath)
	}

	// Build the direct server request for the module.
	req, err := http.NewRequest(http.MethodGet, testHarness.getBaseURL()+modulePath, nil)
	if err != nil {
		t.Fatalf("build direct module request %q: %v", modulePath, err)
	}

	// Fetch the module response and release its body.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("direct module request %q: %v", modulePath, err)
	}
	defer resp.Body.Close()

	// Read the complete direct module response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read direct module response %q: %v", modulePath, err)
	}

	// Attach HTTP response metadata to the module body summary.
	probe := summarizeModuleBody(body)
	probe.Path = modulePath
	probe.Status = resp.StatusCode
	probe.ContentType = resp.Header.Get("Content-Type")
	probe.ContentLength = resp.Header.Get("Content-Length")
	return probe
}

func releaseWebPkgArtifactProbe(t *testing.T, modulePath string) moduleBodyProbe {
	// Read the module's release package artifact.
	t.Helper()
	artifactRelPath := releaseWebPkgArtifactRelPath(t, modulePath)
	body, err := os.ReadFile(filepath.Join(testHarness.repoRoot, artifactRelPath))
	if err != nil {
		t.Fatalf("read release web package artifact %s for %s: %v", artifactRelPath, modulePath, err)
	}

	// Attach the artifact identity to its module body summary.
	probe := summarizeModuleBody(body)
	probe.Path = modulePath
	probe.ArtifactRelPath = artifactRelPath
	probe.OK = true
	return probe
}

func releaseWebPkgArtifactRelPath(t *testing.T, modulePath string) string {
	// Require the module path to identify a release web package.
	t.Helper()
	const prefix = "/b/pkg/"
	if !strings.HasPrefix(modulePath, prefix) {
		t.Fatalf("release web package artifact path must begin with %s: %q", prefix, modulePath)
	}

	// Normalize the package path and reject escapes from its artifact root.
	pkgPath := strings.TrimPrefix(modulePath, prefix)
	cleanPkgPath := path.Clean(pkgPath)
	if cleanPkgPath == "." || cleanPkgPath == ".." || strings.HasPrefix(cleanPkgPath, "../") || path.IsAbs(cleanPkgPath) {
		t.Fatalf("release web package artifact path escapes package root: %q", modulePath)
	}
	return filepath.ToSlash(filepath.Join(webPkgArtifactRelDir, filepath.FromSlash(cleanPkgPath)))
}

func summarizeModuleBody(body []byte) moduleBodyProbe {
	// Hash the module body and retain its leading diagnostic text.
	sum := sha256.Sum256(body)
	bodyText := string(body)
	head := bodyText
	if len(head) > 160 {
		head = head[:160]
	}

	// Retain the module body's trailing diagnostic text.
	tail := bodyText
	if len(tail) > 240 {
		tail = tail[len(tail)-240:]
	}
	return moduleBodyProbe{
		BodyByteLength: len(body),
		SHA256:         hex.EncodeToString(sum[:]),
		Head:           head,
		Tail:           tail,
	}
}

func assertBrowserModuleFetchComplete(t *testing.T, label string, browser moduleFetchProbe) {
	// Verify browser module delivery succeeded through the complete body.
	t.Helper()
	if browser.Status != http.StatusOK {
		t.Fatalf("%s browser probe did not return 200: %#v", label, browser)
	}
	if !browser.OK {
		t.Fatalf("%s browser body failed after headers: %#v", label, browser)
	}
}

func assertBrowserModuleFetchMatchesArtifact(t *testing.T, label string, browser moduleFetchProbe, artifact moduleBodyProbe) {
	// Verify the browser module body matches the release artifact.
	t.Helper()
	assertBrowserModuleFetchComplete(t, label, browser)
	if !artifact.OK {
		t.Fatalf("%s release artifact probe failed: %#v", label, artifact)
	}
	if browser.BodyByteLength != artifact.BodyByteLength {
		t.Fatalf("%s browser body length %d != release artifact length %d: browser=%#v artifact=%#v", label, browser.BodyByteLength, artifact.BodyByteLength, browser, artifact)
	}
	if browser.SHA256 != artifact.SHA256 {
		t.Fatalf("%s browser body hash %v != release artifact hash %v: browser=%#v artifact=%#v", label, browser.SHA256, artifact.SHA256, browser, artifact)
	}
}

func writeModuleLoadDifferentialArtifact(t *testing.T, state string) {
	// Require a configured artifact directory for module evidence.
	t.Helper()
	if testHarness == nil || testHarness.artifactDir == "" {
		return
	}

	// Create the module comparison artifact's parent directory.
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	path := filepath.Join(
		testHarness.artifactDir,
		replacer.Replace(strings.ToLower(t.Name()))+"-module-load-differential.json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Logf("write module load differential artifact mkdir %s: %v", path, err)
		return
	}

	// Write and report the module comparison artifact.
	if err := os.WriteFile(path, []byte(state), 0o644); err != nil {
		t.Logf("write module load differential artifact %s: %v", path, err)
		return
	}
	t.Logf("module load differential artifact: %s", path)
}

type moduleLoadDifferentialReport struct {
	ModulePath      string
	BrowserProbe    browserModuleLoadDifferential
	DirectServer    moduleLoadDifferentialDirectServer
	ReleaseArtifact moduleLoadDifferentialArtifacts
}

type moduleLoadDifferentialDirectServer struct {
	Root   moduleBodyProbe
	Sonner moduleBodyProbe
}

type moduleLoadDifferentialArtifacts struct {
	Sonner moduleBodyProbe
}

type browserModuleLoadDifferential struct {
	Location           string
	ControllerURL      string
	RootAssetStatus    *fastjson.Value
	ModuleImportError  *fastjson.Value
	RootFetch          moduleFetchProbe
	SonnerFetch        moduleFetchProbe
	RootImport         moduleImportProbe
	SonnerImport       moduleImportProbe
	PerformanceEntries []modulePerformanceEntry
}

type moduleFetchProbe struct {
	Path           string
	RequestURL     string
	Status         int
	OK             bool
	Headers        map[string]string
	BodyComplete   bool
	BodyReader     string
	BodyChunks     int
	BodyLength     int
	BodyByteLength int
	SHA256         string
	Head           string
	Tail           string
	PartialSHA256  string
	PartialHead    string
	PartialTail    string
	Phase          string
	Name           string
	Message        string
	Stack          string
}

type moduleImportProbe struct {
	Path       string
	RequestURL string
	OK         bool
	ExportKeys []string
	HasDefault bool
	Name       string
	Message    string
	Stack      string
}

type modulePerformanceEntry struct {
	Name            string
	InitiatorType   string
	TransferSize    int
	EncodedBodySize int
	DecodedBodySize int
}

type moduleBodyProbe struct {
	Path            string
	ArtifactRelPath string
	Status          int
	OK              bool
	ContentType     string
	ContentLength   string
	BodyByteLength  int
	SHA256          string
	Head            string
	Tail            string
}

func parseBrowserModuleLoadDifferential(data string) (browserModuleLoadDifferential, error) {
	// Parse the browser module comparison JSON record.
	var parser fastjson.Parser
	value, err := parser.Parse(data)
	if err != nil {
		return browserModuleLoadDifferential{}, err
	}
	return browserModuleLoadDifferential{
		Location:           string(value.GetStringBytes("location")),
		ControllerURL:      string(value.GetStringBytes("controllerURL")),
		RootAssetStatus:    value.Get("rootAssetStatus"),
		ModuleImportError:  value.Get("moduleImportError"),
		RootFetch:          parseModuleFetchProbe(value.Get("rootFetch")),
		SonnerFetch:        parseModuleFetchProbe(value.Get("sonnerFetch")),
		RootImport:         parseModuleImportProbe(value.Get("rootImport")),
		SonnerImport:       parseModuleImportProbe(value.Get("sonnerImport")),
		PerformanceEntries: parseModulePerformanceEntries(value.GetArray("performanceEntries")),
	}, nil
}

func parseModuleFetchProbe(value *fastjson.Value) moduleFetchProbe {
	if value == nil {
		return moduleFetchProbe{}
	}
	return moduleFetchProbe{
		Path:           string(value.GetStringBytes("path")),
		RequestURL:     string(value.GetStringBytes("requestURL")),
		Status:         value.GetInt("status"),
		OK:             value.GetBool("ok"),
		Headers:        parseStringMapValue(value.Get("headers")),
		BodyComplete:   value.GetBool("bodyComplete"),
		BodyReader:     string(value.GetStringBytes("bodyReader")),
		BodyChunks:     value.GetInt("bodyChunks"),
		BodyLength:     value.GetInt("bodyLength"),
		BodyByteLength: value.GetInt("bodyByteLength"),
		SHA256:         string(value.GetStringBytes("sha256")),
		Head:           string(value.GetStringBytes("head")),
		Tail:           string(value.GetStringBytes("tail")),
		PartialSHA256:  string(value.GetStringBytes("partialSha256")),
		PartialHead:    string(value.GetStringBytes("partialHead")),
		PartialTail:    string(value.GetStringBytes("partialTail")),
		Phase:          string(value.GetStringBytes("phase")),
		Name:           string(value.GetStringBytes("name")),
		Message:        string(value.GetStringBytes("message")),
		Stack:          string(value.GetStringBytes("stack")),
	}
}

func parseModuleImportProbe(value *fastjson.Value) moduleImportProbe {
	if value == nil {
		return moduleImportProbe{}
	}
	return moduleImportProbe{
		Path:       string(value.GetStringBytes("path")),
		RequestURL: string(value.GetStringBytes("requestURL")),
		OK:         value.GetBool("ok"),
		ExportKeys: parseStringArray(value.GetArray("exportKeys")),
		HasDefault: value.GetBool("hasDefault"),
		Name:       string(value.GetStringBytes("name")),
		Message:    string(value.GetStringBytes("message")),
		Stack:      string(value.GetStringBytes("stack")),
	}
}

func parseModulePerformanceEntries(values []*fastjson.Value) []modulePerformanceEntry {
	// Decode the module resource timing entries.
	entries := make([]modulePerformanceEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, modulePerformanceEntry{
			Name:            string(value.GetStringBytes("name")),
			InitiatorType:   string(value.GetStringBytes("initiatorType")),
			TransferSize:    value.GetInt("transferSize"),
			EncodedBodySize: value.GetInt("encodedBodySize"),
			DecodedBodySize: value.GetInt("decodedBodySize"),
		})
	}
	return entries
}

func parseStringArray(values []*fastjson.Value) []string {
	// Decode the JSON string array.
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value.GetStringBytes()))
	}
	return out
}

func parseStringMapValue(value *fastjson.Value) map[string]string {
	// Require a JSON object for the string map.
	obj := value.GetObject()
	if obj == nil {
		return nil
	}

	// Decode the JSON object's string values.
	values := make(map[string]string, obj.Len())
	obj.Visit(func(key []byte, value *fastjson.Value) {
		values[string(key)] = string(value.GetStringBytes())
	})
	return values
}

func (r moduleLoadDifferentialReport) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module identity and browser comparison.
	obj := arena.NewObject()
	obj.Set("modulePath", arena.NewString(r.ModulePath))
	obj.Set("browserProbe", r.BrowserProbe.appendJSON(arena))

	// Serialize the direct server module probes.
	directServer := arena.NewObject()
	directServer.Set("root", r.DirectServer.Root.appendJSON(arena))
	directServer.Set("sonner", r.DirectServer.Sonner.appendJSON(arena))
	obj.Set("directServer", directServer)

	// Serialize the release package artifact probes.
	releaseArtifact := arena.NewObject()
	releaseArtifact.Set("sonner", r.ReleaseArtifact.Sonner.appendJSON(arena))
	obj.Set("releaseArtifact", releaseArtifact)
	return obj
}

func (p browserModuleLoadDifferential) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the browser location and module loading diagnostics.
	obj := arena.NewObject()
	obj.Set("location", arena.NewString(p.Location))
	obj.Set("controllerURL", arena.NewString(p.ControllerURL))
	setRawJSONValue(arena, obj, "rootAssetStatus", p.RootAssetStatus)
	setRawJSONValue(arena, obj, "moduleImportError", p.ModuleImportError)

	// Serialize browser module fetch and import results.
	obj.Set("rootFetch", p.RootFetch.appendJSON(arena))
	obj.Set("sonnerFetch", p.SonnerFetch.appendJSON(arena))
	obj.Set("rootImport", p.RootImport.appendJSON(arena))
	obj.Set("sonnerImport", p.SonnerImport.appendJSON(arena))

	// Serialize the browser module resource timing entries.
	entries := arena.NewArray()
	for _, entry := range p.PerformanceEntries {
		entries.SetArrayItem(len(entries.GetArray()), entry.appendJSON(arena))
	}
	obj.Set("performanceEntries", entries)
	return obj
}

func (p moduleFetchProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module fetch path, HTTP status and headers.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("requestURL", arena.NewString(p.RequestURL))
	obj.Set("status", arena.NewNumberInt(p.Status))
	setBoolJSONField(arena, obj, "ok", p.OK)
	obj.Set("headers", appendStringMapJSON(arena, p.Headers))

	// Serialize the complete module body's reader, size and fingerprint.
	setBoolJSONField(arena, obj, "bodyComplete", p.BodyComplete)
	obj.Set("bodyReader", arena.NewString(p.BodyReader))
	obj.Set("bodyChunks", arena.NewNumberInt(p.BodyChunks))
	obj.Set("bodyLength", arena.NewNumberInt(p.BodyLength))
	obj.Set("bodyByteLength", arena.NewNumberInt(p.BodyByteLength))
	obj.Set("sha256", arena.NewString(p.SHA256))
	obj.Set("head", arena.NewString(p.Head))
	obj.Set("tail", arena.NewString(p.Tail))

	// Serialize partial module body evidence and fetch failures.
	obj.Set("partialSha256", arena.NewString(p.PartialSHA256))
	obj.Set("partialHead", arena.NewString(p.PartialHead))
	obj.Set("partialTail", arena.NewString(p.PartialTail))
	obj.Set("phase", arena.NewString(p.Phase))
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("message", arena.NewString(p.Message))
	obj.Set("stack", arena.NewString(p.Stack))
	return obj
}

func (p moduleImportProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module import request and success status.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("requestURL", arena.NewString(p.RequestURL))
	setBoolJSONField(arena, obj, "ok", p.OK)

	// Serialize the imported module's export inventory.
	exportKeys := arena.NewArray()
	for _, key := range p.ExportKeys {
		exportKeys.SetArrayItem(len(exportKeys.GetArray()), arena.NewString(key))
	}
	obj.Set("exportKeys", exportKeys)

	// Serialize the module's default export and import failure details.
	setBoolJSONField(arena, obj, "hasDefault", p.HasDefault)
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("message", arena.NewString(p.Message))
	obj.Set("stack", arena.NewString(p.Stack))
	return obj
}

func (p modulePerformanceEntry) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module resource timing record.
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("initiatorType", arena.NewString(p.InitiatorType))
	obj.Set("transferSize", arena.NewNumberInt(p.TransferSize))
	obj.Set("encodedBodySize", arena.NewNumberInt(p.EncodedBodySize))
	obj.Set("decodedBodySize", arena.NewNumberInt(p.DecodedBodySize))
	return obj
}

func (p moduleBodyProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module body probe's request and artifact identity.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("artifactRelPath", arena.NewString(p.ArtifactRelPath))
	obj.Set("status", arena.NewNumberInt(p.Status))
	setBoolJSONField(arena, obj, "ok", p.OK)
	obj.Set("contentType", arena.NewString(p.ContentType))
	obj.Set("contentLength", arena.NewString(p.ContentLength))

	// Serialize the module body size, fingerprint and diagnostic excerpts.
	obj.Set("bodyByteLength", arena.NewNumberInt(p.BodyByteLength))
	obj.Set("sha256", arena.NewString(p.SHA256))
	obj.Set("head", arena.NewString(p.Head))
	obj.Set("tail", arena.NewString(p.Tail))
	return obj
}

func appendStringMapJSON(arena *fastjson.Arena, values map[string]string) *fastjson.Value {
	// Collect the string map's keys for stable JSON output.
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	// Serialize string map values in sorted key order.
	slices.Sort(keys)
	for _, key := range keys {
		obj.Set(key, arena.NewString(values[key]))
	}
	return obj
}
