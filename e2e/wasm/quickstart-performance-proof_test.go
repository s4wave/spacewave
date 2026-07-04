//go:build !skip_e2e && !js

package wasm

import (
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

const (
	quickstartPerformanceProofOpCount   = 25
	quickstartPerformanceProofTimeoutMS = 120000
)

func TestGoScriptQuickstartDrivePerformanceProof(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript quickstart performance proof requires %s=goscript", E2EWasmCompilerEnv)
	}

	sess := harness(t).NewCleanBlankSession(t)
	script := "globalThis.__s4waveLogQuickstartTiming = true;"
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install quickstart timing init script: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during quickstart performance proof: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during quickstart performance proof: %+v", report)
		}
	}()

	if err := harness(t).loadAppPageURL(sess, harness(t).baseURL+"/#/quickstart/drive"); err != nil {
		t.Fatalf("load direct drive route: %v", err)
	}
	page := sess.Page()
	WaitForApp(t, page)
	AssertRootImportMap(t, harness(t), page)
	ready := WaitForDriveReady(t, harness(t), page)
	AssertQuickstartContentAfterProgress(t, ready)
	AssertBrowserStartupDone(t, harness(t), page)

	postLoadSOWorkload := runQuickstartPerformanceProofPostLoadSO(t, page)
	proofSummary := collectQuickstartPerformanceProofSummary(t, page, compiler, postLoadSOWorkload)
	t.Logf("quickstart browser performance proof: %s", proofSummary)
}

func runQuickstartPerformanceProofPostLoadSO(t testing.TB, page playwright.Page) map[string]any {
	t.Helper()

	raw, err := page.Evaluate(`async (args) => {
		const debug = globalThis.__s4wave_debug
		if (!debug?.root) {
			throw new Error('debug root is not initialized')
		}
		if (typeof debug.runPostLoadSOPerfTest !== 'function') {
			throw new Error('runPostLoadSOPerfTest is not available')
		}
		const controller = new AbortController()
		const timer = setTimeout(() => controller.abort(), args.timeoutMs)
		try {
			const result = await debug.runPostLoadSOPerfTest(
				debug.root,
				args.opCount,
				controller.signal,
			)
			return JSON.parse(JSON.stringify({
				...result,
				skipped: false,
				timeoutMs: args.timeoutMs,
			}))
		} finally {
			clearTimeout(timer)
		}
	}`, map[string]any{
		"opCount":   quickstartPerformanceProofOpCount,
		"timeoutMs": quickstartPerformanceProofTimeoutMS,
	})
	if err != nil {
		t.Fatalf("run post-load SharedObject workload: %v", err)
	}
	workload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected post-load SharedObject workload result %T", raw)
	}
	return workload
}

func collectQuickstartPerformanceProofSummary(t testing.TB, page playwright.Page, compiler E2EWasmCompiler, postLoadSOWorkload map[string]any) string {
	t.Helper()

	raw, err := page.Evaluate(`(args) => {
		const roundMs = (value) =>
			typeof value === 'number' && Number.isFinite(value) ?
				Math.round(value * 1000) / 1000
			: null
		const timing =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		if (!timing) {
			throw new Error('quickstart timing is not available')
		}
		const phases = Array.isArray(timing.phases) ? timing.phases : []
		const phaseByName = new Map(phases.map((phase) => [phase.name, phase]))
		const requiredPhaseNames = [
			'create-space',
			'mount-space',
			'access-space-world',
			'populate-space',
			'init-drive-unixfs-new-transaction',
			'init-drive-unixfs-apply-op',
			'init-drive-unixfs-commit',
			'create-drive-settings-new-transaction',
			'create-drive-settings-apply-op',
			'create-drive-settings-commit',
		]
		const missingPhaseNames = requiredPhaseNames.filter((name) => !phaseByName.has(name))
		if (missingPhaseNames.length !== 0) {
			throw new Error('quickstart performance proof missing phases: ' + missingPhaseNames.join(', '))
		}
		const summarizePhase = (name) => {
			const phase = phaseByName.get(name)
			return {
				name,
				startedMs: roundMs(phase.startedMs),
				finishedMs: roundMs(phase.finishedMs),
				elapsedMs: roundMs(phase.elapsedMs),
				error: phase.error ?? null,
			}
		}
		const postLoad = args.postLoadSOWorkload
		if (!postLoad || postLoad.skipped) {
			throw new Error('post-load SharedObject workload did not run: ' + (postLoad?.skippedReason ?? 'missing result'))
		}
		for (const key of ['opCount', 'totalMs', 'opAvgMs', 'opMinMs', 'opMaxMs', 'opsPerSec']) {
			if (typeof postLoad[key] !== 'number') {
				throw new Error('post-load SharedObject workload missing numeric ' + key)
			}
		}
		const measuredPhases = phases
			.filter((phase) => typeof phase.elapsedMs === 'number')
			.map((phase) => ({
				name: phase.name,
				elapsedMs: roundMs(phase.elapsedMs),
			}))
			.sort((a, b) => b.elapsedMs - a.elapsedMs)
		const summary = {
			schemaVersion: 1,
			scenario: 'quickstart-drive-browser-performance-proof',
			build: {
				compiler: args.compiler,
				kind: 'GoScript unbundled e2e/wasm browser build',
				packageGate: 'ENABLE_E2E_WASM=true',
				compilerSelector: 'E2E_WASM_COMPILER=goscript',
			},
			quickstart: {
				state: timing.state ?? null,
				elapsedMs: roundMs(timing.elapsedMs),
				progressReadyMs: roundMs(timing.progressReadyMs),
				contentReadyMs: roundMs(timing.contentReadyMs),
				requiredPhases: requiredPhaseNames.map(summarizePhase),
				longestPhases: measuredPhases.slice(0, 10),
			},
			postLoadSharedObjectWorkload: {
				opCount: postLoad.opCount,
				totalMs: roundMs(postLoad.totalMs),
				opAvgMs: roundMs(postLoad.opAvgMs),
				opMinMs: roundMs(postLoad.opMinMs),
				opMaxMs: roundMs(postLoad.opMaxMs),
				opsPerSec: roundMs(postLoad.opsPerSec),
				operationSemantics: postLoad.operationSemantics ?? null,
			},
			attribution: {
				createSpacePhase: summarizePhase('create-space'),
				populateSpacePhase: summarizePhase('populate-space'),
				driveCommitPhases: requiredPhaseNames
					.filter((name) => name.endsWith('-commit'))
					.map(summarizePhase),
				gap: 'create-space does not split Envelope or peer crypto from other SharedObject setup in this harness',
			},
		}
		return JSON.stringify(summary)
	}`, map[string]any{
		"compiler":           string(compiler),
		"postLoadSOWorkload": postLoadSOWorkload,
	})
	if err != nil {
		t.Fatalf("collect quickstart performance proof summary: %v", err)
	}
	summary, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected quickstart performance proof summary %T", raw)
	}
	return summary
}
