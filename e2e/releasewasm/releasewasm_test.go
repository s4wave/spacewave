//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/sirupsen/logrus"
)

var testHarness *harness

const browserWaitMS = 60000

// TIER: nightly
func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !E2EReleaseWasmEnabled() {
		le.Info("skipping e2e/releasewasm package; set ENABLE_E2E_RELEASE_WASM=true to run")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	h, err := boot(ctx, le)
	if err != nil {
		le.WithError(err).Fatal("boot release wasm harness")
	}
	testHarness = h

	code := m.Run()
	h.release(le)
	os.Exit(code)
}

func TestBrowserReleaseDescriptorIncludesPrerenderedWasmShell(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if desc.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", desc.SchemaVersion)
	}
	if desc.GenerationID == "" {
		t.Fatal("expected generation id")
	}
	if desc.ShellAssets.Entrypoint == "" {
		t.Fatal("expected shellAssets.entrypoint")
	}
	if desc.ShellAssets.ServiceWorker == "" {
		t.Fatal("expected shellAssets.serviceWorker")
	}
	if desc.ShellAssets.SharedWorker == "" {
		t.Fatal("expected shellAssets.sharedWorker")
	}
	if desc.ShellAssets.Wasm == "" {
		t.Fatal("expected shellAssets.wasm")
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/") {
		t.Fatalf("expected / in prerendered routes: %v", desc.PrerenderedRoutes)
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/quickstart/drive") {
		t.Fatalf("expected /quickstart/drive in prerendered routes: %v", desc.PrerenderedRoutes)
	}
}

func TestRootPrerenderLoadsProductionWasmBundle(t *testing.T) {
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err := page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)
}

func TestProductionRuntimeMatchesReleaseDescriptor(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)

	raw, err := page.Evaluate(`async () => {
		const registration = await navigator.serviceWorker.ready
		return {
			generationId: globalThis.__swGenerationId || '',
			controllerURL: navigator.serviceWorker.controller?.scriptURL || '',
			activeURL: registration.active?.scriptURL || '',
		}
	}`)
	if err != nil {
		t.Fatalf("read production runtime state: %v", err)
	}
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected production runtime state %T", raw)
	}
	generationID, _ := state["generationId"].(string)
	if generationID != desc.GenerationID {
		t.Fatalf("generation id=%q want %q", generationID, desc.GenerationID)
	}
	controllerURL, _ := state["controllerURL"].(string)
	if !strings.HasSuffix(controllerURL, "/"+desc.ShellAssets.ServiceWorker) {
		t.Fatalf("controller service worker=%q want suffix %q", controllerURL, desc.ShellAssets.ServiceWorker)
	}
	activeURL, _ := state["activeURL"].(string)
	if !strings.HasSuffix(activeURL, "/"+desc.ShellAssets.ServiceWorker) {
		t.Fatalf("active service worker=%q want suffix %q", activeURL, desc.ShellAssets.ServiceWorker)
	}
}

func TestQuickstartPrerenderAutoBootsProductionWasmBundle(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := testHarness.quickstartSmokeArtifactPath(t)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove previous quickstart smoke artifact: %v", err)
	}
	source := sourceRevision(t)
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}
	enableQuickstartTimingLogs(t, page)

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	waitForCanonicalQuickstartURL(t, page)
	err = page.Locator("[data-testid='unixfs-browser']").WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart drive shell: %v", err)
	}
	driveShellVisibleMs := browserNowMs(t, page)
	err = page.Locator("text=getting-started.md").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart drive content: %v", err)
	}
	driveReadyMs := browserNowMs(t, page)
	logQuickstartTiming(t, page)

	data, err := collectQuickstartSmokeArtifact(page, desc, source, driveShellVisibleMs, driveReadyMs)
	if err != nil {
		t.Fatalf("collect quickstart smoke artifact: %v", err)
	}
	if err := writeQuickstartSmokeArtifact(path, data); err != nil {
		t.Fatalf("write quickstart smoke artifact: %v", err)
	}
	t.Logf("quickstart smoke artifact written to %s (%d bytes)", path, len(data))
}

func waitForCanonicalQuickstartURL(t *testing.T, page playwright.Page) {
	t.Helper()

	expectedURL := testHarness.getBaseURL() + "/#/quickstart/drive"
	if page.URL() == expectedURL {
		return
	}
	err := page.WaitForURL(expectedURL, playwright.PageWaitForURLOptions{
		Timeout:   playwright.Float(30000),
		WaitUntil: playwright.WaitUntilStateCommit,
	})
	if err != nil {
		t.Fatalf("wait for canonical quickstart URL: %v", err)
	}
}

func waitForLiveApp(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		await Promise.race([
			globalThis.__swReady,
			new Promise((_, reject) => setTimeout(() => reject(new Error('runtime did not become ready')), 30000)),
		])
		const deadline = performance.now() + 30000
		while (document.querySelector('#bldr-root')?.hasAttribute('data-prerendered')) {
			if (performance.now() > deadline) {
				throw new Error('prerender did not switch to live app')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for live app: %v", err)
	}
}

func waitForPrerenderRoot(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (!document.querySelector('#bldr-root[data-prerendered]')) {
			if (performance.now() > deadline) {
				throw new Error('missing prerendered bldr root')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for prerender root: %v", err)
	}
}

func waitForBootFunction(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (typeof globalThis.__swBoot !== 'function' || !globalThis.__swReady) {
			if (performance.now() > deadline) {
				throw new Error('production boot function did not initialize')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for boot function: %v", err)
	}
}

func dumpPageState(t *testing.T, page playwright.Page) {
	t.Helper()

	state, err := page.Evaluate(`() => ({
		href: window.location.href,
		title: document.title,
		text: document.body?.innerText?.slice(0, 4000) ?? '',
		rootHtml: document.querySelector('#bldr-root')?.outerHTML?.slice(0, 4000) ?? '',
		hasDebugRoot: !!globalThis.__s4wave_debug?.root,
		quickstartTiming: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
		testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
			testid: el.getAttribute('data-testid'),
			text: el.textContent?.slice(0, 200) ?? '',
		})),
	})`)
	if err != nil {
		t.Logf("dump page state: %v", err)
		return
	}
	t.Logf("page state: %#v", state)
}

func enableQuickstartTimingLogs(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`() => {
		globalThis.__s4waveLogQuickstartTiming = true
	}`)
	if err != nil {
		t.Fatalf("enable quickstart timing logs: %v", err)
	}
}

func logQuickstartTiming(t *testing.T, page playwright.Page) {
	t.Helper()

	timing, err := page.Evaluate(`() => JSON.stringify(globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null)`)
	if err != nil {
		t.Logf("quickstart timing: %v", err)
		return
	}
	t.Logf("quickstart timing: %v", timing)
}

func browserNowMs(t *testing.T, page playwright.Page) int {
	t.Helper()

	raw, err := page.Evaluate(`() => Math.round(performance.now())`)
	if err != nil {
		t.Fatalf("read browser performance.now: %v", err)
	}
	val, ok := raw.(int)
	if ok {
		return val
	}
	fval, ok := raw.(float64)
	if !ok {
		t.Fatalf("unexpected performance.now value %T", raw)
	}
	return int(fval)
}

func collectQuickstartSmokeArtifact(
	page playwright.Page,
	desc *browserReleaseDescriptor,
	source map[string]any,
	driveShellVisibleMs int,
	driveReadyMs int,
) ([]byte, error) {
	raw, err := page.Evaluate(`async (args) => {
		const startupPrefix = 'spacewave.startup.'
		const roundMs = (value) =>
			typeof value === 'number' && Number.isFinite(value) ?
				Math.round(value * 1000) / 1000
			: null
		const stableAliases = new Map()
		const stableAlias = (kind, value) => {
			if (typeof value !== 'string' || value === '') return value ?? null
			const existingAliases = stableAliases.get(kind)
			const aliases = existingAliases ?? new Map()
			if (!existingAliases) {
				stableAliases.set(kind, aliases)
			}
			const existingAlias = aliases.get(value)
			if (existingAlias) {
				return existingAlias
			}
			const alias = kind + '-' + (aliases.size + 1)
			aliases.set(value, alias)
			return alias
		}
		const normalizeDetailValue = (key, value) => {
			if (key === 'documentId') return stableAlias('document', value)
			if (key === 'from') return stableAlias('sender', value)
			if (key === 'path') return stableAlias('asset-path', value)
			if (key === 'workerId') return stableAlias('worker', value)
			if (Array.isArray(value)) {
				return value.map((item) => normalizeDetailValue(key, item))
			}
			if (value && typeof value === 'object') {
				return Object.fromEntries(
					Object.keys(value)
						.sort()
						.map((childKey) => [
							childKey,
							normalizeDetailValue(childKey, value[childKey]),
						]),
				)
			}
			return value ?? null
		}
		const normalizeDetail = (detail) => {
			if (!detail || typeof detail !== 'object') return null
			return Object.fromEntries(
				Object.keys(detail)
					.sort()
					.map((key) => [key, normalizeDetailValue(key, detail[key])]),
			)
		}
		const detectBrowserFamily = () => {
			const ua = navigator.userAgent
			if (ua.includes('Firefox/')) return 'firefox'
			if (ua.includes('Edg/')) return 'edge'
			if (ua.includes('Chrome/') || ua.includes('Chromium/')) return 'chromium'
			if (ua.includes('Safari/')) return 'webkit'
			return 'unknown'
		}
		const detectWorkerComms = async () => {
			const caps = {
				crossOriginIsolated: !!globalThis.crossOriginIsolated,
				sabAvailable: false,
				opfsAvailable: false,
				webLocksAvailable: !!navigator.locks,
				broadcastChannelAvailable: typeof BroadcastChannel === 'function',
			}
			try {
				const buf = new SharedArrayBuffer(8)
				caps.sabAvailable = buf.byteLength === 8
			} catch {}
			try {
				if (navigator.storage?.getDirectory) {
					await navigator.storage.getDirectory()
					caps.opfsAvailable = true
				}
			} catch {}
			const config =
				!caps.crossOriginIsolated || !caps.sabAvailable ? 'A'
				: caps.opfsAvailable && caps.webLocksAvailable ? 'C'
				: 'B'
			return { config, caps }
		}
		const storageEstimate =
			navigator.storage?.estimate ?
				await navigator.storage.estimate().catch(() => null)
			: null
		const persisted =
			navigator.storage?.persisted ?
				await navigator.storage.persisted().catch(() => null)
			: null
		const startupMarks = performance
			.getEntriesByType('mark')
			.filter((entry) => entry.name.startsWith(startupPrefix))
			.map((entry, index) => {
				const detail = normalizeDetail(entry.detail)
				return {
					name: entry.name,
					label: entry.name.slice(startupPrefix.length),
					startTimeMs: roundMs(entry.startTime),
					collectionOrdinal: index + 1,
					sequence:
						typeof detail?.sequence === 'number' ? detail.sequence : null,
					detail,
					_sortStartTimeMs: entry.startTime,
				}
			})
			.sort(
				(a, b) =>
					a._sortStartTimeMs - b._sortStartTimeMs ||
					(a.sequence ?? Number.MAX_SAFE_INTEGER) -
						(b.sequence ?? Number.MAX_SAFE_INTEGER) ||
					a.collectionOrdinal - b.collectionOrdinal ||
					a.name.localeCompare(b.name),
			)
			.map((mark, index) => {
				const { _sortStartTimeMs: _, ...stableMark } = mark
				return {
					...stableMark,
					timelineOrdinal: index + 1,
				}
			})
		const labels = new Set(startupMarks.map((mark) => mark.label))
		const expectedStartupMarks = [
			'shell.entrypoint-loaded',
			'web-document.construct-start',
			'worker-comms.detected',
			'storage.mode-detected',
			'runtime.mode-selected',
			'runtime.worker-created',
			'service-worker.register-ready',
			'runtime.connected',
			'worker.first-ready',
			'plugin.running',
		]
		const markMatches = (mark, label, predicate) =>
			mark.label === label && (!predicate || predicate(mark))
		const firstMark = (label, predicate) =>
			startupMarks.find((mark) => markMatches(mark, label, predicate)) ?? null
		const lastMark = (label, predicate) => {
			for (let i = startupMarks.length - 1; i >= 0; i--) {
				const mark = startupMarks[i]
				if (markMatches(mark, label, predicate)) return mark
			}
			return null
		}
		const isPluginMark = (mark) =>
			mark.detail?.plugin === true ||
			(typeof mark.detail?.workerId === 'string' &&
				mark.detail.workerId.startsWith('plugin/'))
		const lastPluginReady =
			lastMark('worker.ready', isPluginMark) ??
			lastMark('worker.first-ready', isPluginMark)
		const firstWorkerReady =
			firstMark('worker.first-ready') ??
			firstMark('worker.ready')
		const pluginRunning =
			firstMark('plugin.running', isPluginMark) ??
			lastPluginReady
		const firstPluginStart =
			firstMark('worker.first-create-start', isPluginMark) ??
			firstMark('worker.construct-start', isPluginMark)
		const shellEntrypoint = firstMark('shell.entrypoint-loaded')
		const shellBootRequested = firstMark('shell.boot-requested')
		const runtimeConnected = firstMark('runtime.connected')
		const nav = performance.getEntriesByType('navigation')[0]
		const navigation =
			nav ?
				{
					type: nav.type,
					startTimeMs: roundMs(nav.startTime),
					responseEndMs: roundMs(nav.responseEnd),
					domInteractiveMs: roundMs(nav.domInteractive),
					domContentLoadedEventEndMs: roundMs(nav.domContentLoadedEventEnd),
					loadEventEndMs: roundMs(nav.loadEventEnd),
				}
			: null
		const paint = performance.getEntriesByType('paint').map((entry) => ({
			name: entry.name,
			startTimeMs: roundMs(entry.startTime),
		}))
		const brands =
			navigator.userAgentData?.brands?.map((brand) => ({
				brand: brand.brand,
				version: brand.version,
			})) ?? []
		const quickstartTiming =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		const makeSegment = (name, startMs, endMs, attribution, evidence) => ({
			name,
			startMs: startMs ?? null,
			endMs: endMs ?? null,
			elapsedMs:
				typeof startMs === 'number' && typeof endMs === 'number' ?
					Math.max(0, endMs - startMs)
				: null,
			attribution,
			evidence,
		})
		const makeReadinessMark = (name, timestampMs, evidence, sourceMark) => ({
			name,
			timestampMs: timestampMs ?? null,
			evidence,
			sourceMark: sourceMark ?? null,
		})
		const readinessTimeline = [
			makeReadinessMark(
				'worker-ready',
				firstWorkerReady?.startTimeMs,
				[firstWorkerReady?.label ?? 'worker.ready'],
				firstWorkerReady,
			),
			makeReadinessMark(
				'plugin-running',
				pluginRunning?.startTimeMs,
				[pluginRunning?.label ?? 'plugin.running'],
				pluginRunning,
			),
			makeReadinessMark(
				'progress-ready',
				quickstartTiming?.progressReadyMs,
				['quickstart.progressReadyMs'],
				null,
			),
			makeReadinessMark(
				'frame-ready',
				args.driveShellVisibleMs,
				['driveShellVisibleMs', "[data-testid='unixfs-browser']"],
				null,
			),
			makeReadinessMark(
				'content-ready',
				args.driveReadyMs,
				['driveReadyMs', 'getting-started.md'],
				null,
			),
		]
		const missingReadinessMarks = readinessTimeline
			.filter((mark) => typeof mark.timestampMs !== 'number')
			.map((mark) => mark.name)
		const startupAttributionSegments = [
			makeSegment(
				'navigation-to-entrypoint',
				navigation?.startTimeMs,
				shellEntrypoint?.startTimeMs,
				'HTML, static assets, hydration entrypoint load',
				['navigation.startTimeMs', 'shell.entrypoint-loaded'],
			),
			makeSegment(
				'entrypoint-to-runtime-connected',
				shellEntrypoint?.startTimeMs,
				runtimeConnected?.startTimeMs,
				'Bldr web document and runtime connection',
				['shell.entrypoint-loaded', 'runtime.connected'],
			),
			makeSegment(
				'boot-request-to-first-plugin-worker',
				shellBootRequested?.startTimeMs,
				firstPluginStart?.startTimeMs,
				'Live app boot before the first plugin worker is constructed',
				['shell.boot-requested', firstPluginStart?.label ?? 'worker.construct-start'],
			),
			makeSegment(
				'plugin-worker-startup',
				firstPluginStart?.startTimeMs,
				lastPluginReady?.startTimeMs,
				'Plugin worker construction and readiness',
				[firstPluginStart?.label ?? 'worker.construct-start', lastPluginReady?.label ?? 'worker.ready'],
			),
			makeSegment(
				'plugin-ready-to-quickstart-start',
				lastPluginReady?.startTimeMs,
				quickstartTiming?.startedMs,
				'App shell, root resource access, routing, and quickstart resource scheduling before createQuickstartSetup begins',
				[lastPluginReady?.label ?? 'worker.ready', 'quickstart.startedMs'],
			),
			makeSegment(
				'quickstart-progress-setup',
				quickstartTiming?.startedMs,
				quickstartTiming?.progressReadyMs,
				'createQuickstartSetup RPC flow through session, space, world, and Drive frame prerequisites',
				['quickstart.startedMs', 'quickstart.progressReadyMs'],
			),
			makeSegment(
				'quickstart-content-seed',
				quickstartTiming?.progressReadyMs,
				quickstartTiming?.finishedMs,
				'Quickstart-specific seed content population before final navigation',
				['quickstart.progressReadyMs', 'quickstart.finishedMs'],
			),
			makeSegment(
				'quickstart-finished-to-drive-shell',
				quickstartTiming?.finishedMs,
				args.driveShellVisibleMs,
				'Post-setup redirect, session mount, space mount, and drive shell render',
				['quickstart.finishedMs', 'driveShellVisibleMs'],
			),
			makeSegment(
				'drive-shell-to-content-ready',
				args.driveShellVisibleMs,
				args.driveReadyMs,
				'Drive content watch and file list render',
				['driveShellVisibleMs', 'driveReadyMs'],
			),
		]
		const measuredSegments = startupAttributionSegments.filter(
			(segment) => typeof segment.elapsedMs === 'number',
		)
		const longestSegment =
			measuredSegments.length ?
				measuredSegments.reduce((longest, segment) =>
					segment.elapsedMs > longest.elapsedMs ? segment : longest,
				)
			: null
		const artifact = {
			schemaVersion: 3,
			scenario: 'quickstart-drive-production-smoke',
			collectedAt: new Date().toISOString(),
			baseURL: args.baseURL,
			finalURL: window.location.href,
			release: args.release,
			source: args.source,
			browser: {
				family: detectBrowserFamily(),
				harnessName: args.browserName,
				userAgent: navigator.userAgent,
				brands,
			},
			page: {
				visibilityState: document.visibilityState,
				focused: document.hasFocus(),
			},
			workerComms: await detectWorkerComms(),
			storage: {
				mode:
					navigator.storage?.getDirectory ?
						'browser-opfs-indexeddb'
					: 'browser-indexeddb',
				persistSupported: !!navigator.storage?.persist,
				persistedSupported: !!navigator.storage?.persisted,
				persisted,
				estimate: storageEstimate,
			},
			timing: {
				browserNowMs: roundMs(performance.now()),
				driveShellVisibleMs: args.driveShellVisibleMs,
				driveReadyMs: args.driveReadyMs,
				quickstart: quickstartTiming,
				navigation,
				paint,
			},
			timeline: {
				base: 'navigation.startTimeMs',
				unit: 'ms',
				ordering: [
					'performance.mark.startTime',
					'detail.sequence',
					'collectionOrdinal',
					'name',
				],
				normalizedVolatileDetailFields: [
					'documentId',
					'from',
					'path',
					'workerId',
				],
			},
			readiness: {
				frameReadyMs: args.driveShellVisibleMs,
				progressReadyMs: quickstartTiming?.progressReadyMs ?? null,
				contentReadyMs: args.driveReadyMs,
				workerReadyMs: firstWorkerReady?.startTimeMs ?? null,
				pluginRunningMs: pluginRunning?.startTimeMs ?? null,
				missingReadinessMarks,
				timeline: readinessTimeline,
			},
			startupMarks,
			missingStartupMarks: expectedStartupMarks.filter((label) => !labels.has(label)),
			startupAttribution: {
				range: makeSegment(
					'last-plugin-ready-to-drive-ready',
					lastPluginReady?.startTimeMs,
					args.driveReadyMs,
					'Previously unattributed post-plugin startup tail',
					[lastPluginReady?.label ?? 'worker.ready', 'driveReadyMs'],
				),
				longestSegment,
				segments: startupAttributionSegments,
			},
		}
		return JSON.stringify(artifact, null, 2)
	}`, map[string]any{
		"baseURL":             testHarness.getBaseURL(),
		"browserName":         testHarness.browserName,
		"driveShellVisibleMs": driveShellVisibleMs,
		"driveReadyMs":        driveReadyMs,
		"release": map[string]any{
			"generationId": desc.GenerationID,
			"shellAssets": map[string]any{
				"entrypoint":    desc.ShellAssets.Entrypoint,
				"serviceWorker": desc.ShellAssets.ServiceWorker,
				"sharedWorker":  desc.ShellAssets.SharedWorker,
				"wasm":          desc.ShellAssets.Wasm,
				"css":           desc.ShellAssets.CSS,
			},
			"prerenderedRoutes":    desc.PrerenderedRoutes,
			"requiredStaticAssets": desc.RequiredStaticAssets,
		},
		"source": source,
	})
	if err != nil {
		return nil, err
	}
	data, ok := raw.(string)
	if !ok {
		return nil, errors.Errorf("unexpected artifact payload %T", raw)
	}
	return []byte(data + "\n"), nil
}

func writeQuickstartSmokeArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func sourceRevision(t testing.TB) map[string]any {
	t.Helper()

	headCmd := exec.Command("git", "rev-parse", "HEAD")
	headRaw, err := headCmd.Output()
	if err != nil {
		t.Fatalf("read git HEAD: %v", err)
	}
	statusCmd := exec.Command("git", "status", "--short")
	statusRaw, err := statusCmd.Output()
	if err != nil {
		t.Fatalf("read git status: %v", err)
	}
	status := strings.TrimSpace(string(statusRaw))
	statusLines := []string{}
	if status != "" {
		statusLines = strings.Split(status, "\n")
	}
	return map[string]any{
		"head":        strings.TrimSpace(string(headRaw)),
		"dirty":       len(statusLines) != 0,
		"statusShort": statusLines,
	}
}
