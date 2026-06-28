//go:build !skip_e2e && !js

package wasm

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGoScriptSharedWorkerOPFSProbe(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve e2e wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript-only SharedWorker OPFS probe; compiler=%s", compiler)
	}

	h := harness(t)
	if h.BrowserName() != "chromium" {
		t.Skipf("Chromium-only SharedWorker OPFS probe; browser=%s", h.BrowserName())
	}

	sess := h.NewCleanBlankSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during SharedWorker OPFS probe: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during SharedWorker OPFS probe: %+v", report)
		}
	}()

	if err := h.loadAppPageURL(sess, h.baseURL+"/#/quickstart/drive"); err != nil {
		t.Fatalf("load direct drive route: %v", err)
	}
	page := sess.Page()
	WaitForApp(t, page)
	ready := WaitForDriveReady(t, h, page)

	topologyRaw, err := page.Evaluate(`() => {
		const marks = globalThis.__swStartupMarks ?? []
		let runtimeMode = null
		for (let i = marks.length - 1; i >= 0; i--) {
			const mark = marks[i]
			if (mark.label === 'runtime.mode-selected') {
				runtimeMode = mark.detail?.mode ?? null
				break
			}
		}
		const pluginDispatches = marks
			.filter((mark) => mark.label === 'worker.create-dispatch-start' && mark.detail?.plugin)
			.map((mark) => ({
				workerId: mark.detail?.workerId ?? null,
				shared: mark.detail?.shared ?? null,
				detectConfig: mark.detail?.detectConfig ?? null,
				workerMode: mark.detail?.workerMode ?? null,
				path: mark.detail?.path ?? null,
			}))
		const opfsBridgeMarks = marks
			.filter((mark) => mark.label === 'runtime.opfs-bridge-ready')
			.map((mark) => ({
				workerId: mark.detail?.workerId ?? null,
				documentId: mark.detail?.documentId ?? null,
				runtimeId: mark.detail?.runtimeId ?? null,
				enabled: mark.detail?.enabled ?? null,
			}))
		return {
			crossOriginIsolated: !!globalThis.crossOriginIsolated,
			runtimeMode,
			pluginDispatches,
			opfsBridgeMarks,
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read browser worker topology: %v", err)
	}
	topology, ok := topologyRaw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected topology result %T: %#v", topologyRaw, topologyRaw)
	}
	// The runtime-worker OPFS bridge only activates when the engine runtime runs
	// in a SharedWorker, which is the staging/production topology and the only
	// scope that hits the OPFS getDirectory SecurityError. The shared harness
	// defaults to dedicated workers, so skip unless booted with
	// E2E_WASM_WORKER_MODE=shared rather than hard-failing on the wrong topology.
	if got, _ := topology["runtimeMode"].(string); got != "shared-worker" {
		t.Skipf("runtime worker mode = %q, want shared-worker; run with E2E_WASM_WORKER_MODE=shared to exercise the runtime OPFS bridge; topology=%#v", got, topology)
	}
	if got, _ := topology["crossOriginIsolated"].(bool); !got {
		t.Fatalf("page crossOriginIsolated = false; topology=%#v", topology)
	}
	opfsBridgeReady := false
	if marks, ok := topology["opfsBridgeMarks"].([]any); ok {
		for _, item := range marks {
			mark, ok := item.(map[string]any)
			if !ok {
				continue
			}
			enabled, _ := mark["enabled"].(bool)
			if enabled {
				opfsBridgeReady = true
				break
			}
		}
	}
	if !opfsBridgeReady {
		t.Fatalf("no enabled runtime.opfs-bridge-ready mark observed; the engine runtime SharedWorker did not broker an OPFS DedicatedWorker; topology=%#v", topology)
	}

	probeRaw, err := page.Evaluate(`async () => {
		const script = [
			"self.onconnect = (event) => {",
			"  const port = event.ports[0]",
			"  port.onmessage = async () => {",
			"    const result = {",
			"      scope: self.constructor?.name ?? null,",
			"      crossOriginIsolated: !!self.crossOriginIsolated,",
			"      hasGetDir: typeof navigator.storage?.getDirectory,",
			"      estimateOK: false,",
			"      estimateErrName: null,",
			"      getDirectoryOK: false,",
			"      getDirectoryErrName: null,",
			"      getDirectoryErrMessage: null,",
			"    }",
			"    try {",
			"      await navigator.storage?.estimate?.()",
			"      result.estimateOK = true",
			"    } catch (err) {",
			"      result.estimateErrName = err?.name ?? String(err)",
			"    }",
			"    try {",
			"      await navigator.storage.getDirectory()",
			"      result.getDirectoryOK = true",
			"    } catch (err) {",
			"      result.getDirectoryErrName = err?.name ?? String(err)",
			"      result.getDirectoryErrMessage = err?.message ?? String(err)",
			"    }",
			"    port.postMessage(result)",
			"  }",
			"  port.start()",
			"}",
		].join("\n")
		const url = URL.createObjectURL(new Blob([script], { type: 'text/javascript' }))
		try {
			const worker = new SharedWorker(url, {
				name: 'spacewave-opfs-probe-' + Math.random().toString(16).slice(2),
			})
			return await new Promise((resolve) => {
				const timeout = setTimeout(() => {
					URL.revokeObjectURL(url)
					resolve({ stage: 'timeout' })
				}, 10000)
				worker.port.onmessage = (event) => {
					clearTimeout(timeout)
					URL.revokeObjectURL(url)
					resolve({ stage: 'result', ...event.data })
				}
				worker.port.start()
				worker.port.postMessage('probe')
			})
		} catch (err) {
			URL.revokeObjectURL(url)
			return {
				stage: 'construct-error',
				name: err?.name ?? String(err),
				message: err?.message ?? String(err),
			}
		}
	}`, nil)
	if err != nil {
		t.Fatalf("run browser SharedWorker OPFS probe: %v", err)
	}
	probe, ok := probeRaw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected SharedWorker probe result %T: %#v", probeRaw, probeRaw)
	}
	t.Logf(
		"shared-worker opfs probe: ready_ms=%d topology=%#v probe=%#v",
		ready.ContentReadyMs,
		topology,
		probe,
	)
	if got, _ := probe["stage"].(string); got != "result" {
		t.Fatalf("SharedWorker probe stage = %q, want result; probe=%#v", got, probe)
	}
	if got, _ := probe["scope"].(string); got != "SharedWorkerGlobalScope" {
		t.Fatalf("SharedWorker probe scope = %q, want SharedWorkerGlobalScope; probe=%#v", got, probe)
	}
	if got, _ := probe["hasGetDir"].(string); got != "function" {
		t.Fatalf("SharedWorker getDirectory type = %q, want function; probe=%#v", got, probe)
	}
	if got, _ := probe["getDirectoryOK"].(bool); !got {
		t.Fatalf("SharedWorker getDirectory failed; probe=%#v", probe)
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("E2E_WASM_OPFS_SHAREDWORKER_HOLD_OPEN"))) {
	case "true", "1", "yes", "on":
		t.Logf("holding browser open for manual SharedWorker inspection: %s", page.URL())
		<-time.After(30 * time.Minute)
	}
}
