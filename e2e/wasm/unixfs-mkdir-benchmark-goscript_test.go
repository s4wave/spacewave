//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/s4wave/spacewave/e2e/drivebench"
)

func measureGoScriptUnixFSMkdir(t testing.TB, sess *TestSession, page playwright.Page) {
	t.Helper()

	prepareUnixFSMkdirBenchmark(t, page)
	defer releaseUnixFSMkdirBenchmark(t, page)

	samples := make([]float64, 3)
	for idx := range samples {
		samples[idx] = runUnixFSMkdirBenchmark(t, page, "mkdir-sample-"+strconv.Itoa(idx))
	}

	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve compiler for UnixFS mkdir benchmark: %v", err)
	}
	artifactDir := unixFSMkdirArtifactDir(t)
	var arena fastjson.Arena
	measurement := arena.NewObject()
	measurement.Set("compiler", arena.NewString(string(compiler)))
	measurement.Set("operation", arena.NewString("unixfs-mkdir-all-in-space"))
	measurementSamples := arena.NewArray()
	for idx, sample := range samples {
		measurementSamples.SetArrayItem(
			idx,
			arena.NewNumberString(strconv.FormatFloat(sample, 'f', 6, 64)),
		)
	}
	measurement.Set("samplesMs", measurementSamples)
	writeMeasurement := func() {
		measurementPath := filepath.Join(artifactDir, "benchmark.json")
		if err := WriteTraceArtifact(measurementPath, append(measurement.MarshalTo(nil), '\n')); err != nil {
			t.Fatalf("write UnixFS mkdir measurements: %v", err)
		}
	}
	t.Logf(
		"goscript UnixFS mkdir samples: %.3fms %.3fms %.3fms",
		samples[0],
		samples[1],
		samples[2],
	)

	if !E2EWasmTraceServiceEnabled(compiler) {
		writeMeasurement()
		return
	}
	var tracedSampleMs float64
	traceData, err := sess.CaptureTrace(t.Context(), "goscript-unixfs-mkdir", func(context.Context) error {
		tracedSampleMs = runUnixFSMkdirBenchmark(t, page, "mkdir-traced")
		return nil
	})
	if err != nil {
		t.Fatalf("capture UnixFS mkdir trace: %v", err)
	}
	tracePath := filepath.Join(artifactDir, "runtime.trace")
	if err := WriteTraceArtifact(tracePath, traceData); err != nil {
		t.Fatalf("write UnixFS mkdir runtime trace: %v", err)
	}
	summary, _, _, _, _, operationShape := summarizeTrace(t, traceData)
	if operationShape != nil {
		measurement.Set("operationShape", drivebench.MarshalOperationShapeValue(&arena, *operationShape))
	}
	measurement.Set("tracedSampleMs", arena.NewNumberString(strconv.FormatFloat(tracedSampleMs, 'f', 6, 64)))
	writeMeasurement()
	if err := WriteTraceArtifact(filepath.Join(artifactDir, "tracetool.txt"), []byte(summary)); err != nil {
		t.Fatalf("write UnixFS mkdir trace summary: %v", err)
	}
	t.Logf("goscript UnixFS traced mkdir completed in %.3fms", tracedSampleMs)
}

func prepareUnixFSMkdirBenchmark(t testing.TB, page playwright.Page) {
	t.Helper()

	raw, err := page.Evaluate(`async () => {
		if (globalThis.__s4waveUnixFSMkdirBenchmark) {
			return { error: 'UnixFS mkdir benchmark is already mounted' }
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		const FSHandle = debug?.FSHandle
		const unixfsObjectKey = debug?.UNIXFS_OBJECT_KEY
		if (!match || !root || !mountSpace || !FSHandle || !unixfsObjectKey) {
			return { error: 'missing direct Drive route or debug FSHandle context' }
		}
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		let session = null
		let world = null
		let rootHandle = null
		try {
			const abort = AbortSignal.timeout(120000)
			const mounted = await root.mountSessionByIdx({ sessionIdx: Number(match[1]) }, abort)
			session = mounted?.session ?? null
			if (!session) return { error: 'mountSessionByIdx returned no session' }
			const space = await mountSpace({
				session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: { id: decodeURIComponent(match[2]) },
					},
				},
				abortSignal: abort,
				cleanup,
			})
			world = await space.accessWorldState(true, abort)
			const access = await world.accessTypedObject(unixfsObjectKey, abort)
			if (!access?.resourceId) return { error: 'accessTypedObject returned no UnixFS resource id' }
			rootHandle = new FSHandle(world.getResourceRef().createRef(access.resourceId))
			globalThis.__s4waveUnixFSMkdirBenchmark = {
				cleanupStack,
				names: [],
				rootHandle,
				session,
				world,
			}
			return { ready: true }
		} catch (err) {
			rootHandle?.release?.()
			world?.release?.()
			while (cleanupStack.length) cleanupStack.pop()?.release?.()
			session?.release?.()
			return { error: String(err?.stack ?? err) }
		}
	}`, nil)
	if err != nil {
		t.Fatalf("prepare UnixFS mkdir benchmark: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok || !boolField(result, "ready") {
		t.Fatalf("prepare UnixFS mkdir benchmark: %#v", raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("prepare UnixFS mkdir benchmark: %s", errMsg)
	}
}

func runUnixFSMkdirBenchmark(t testing.TB, page playwright.Page, name string) float64 {
	t.Helper()

	raw, err := page.Evaluate(`async (name) => {
		const bench = globalThis.__s4waveUnixFSMkdirBenchmark
		if (!bench?.rootHandle) return { error: 'UnixFS mkdir benchmark is not mounted' }
		try {
			const abort = AbortSignal.timeout(120000)
			const startedAt = performance.now()
			await bench.rootHandle.mkdirAll([name], 0o755, abort)
			const durationMs = performance.now() - startedAt
			const dir = await bench.rootHandle.lookup(name, abort)
			const entries = await dir.readdirAll(0n, abort)
			dir.release()
			if (!Array.isArray(entries)) return { error: 'created directory readdir did not return entries' }
			bench.names.push(name)
			return { durationMs }
		} catch (err) {
			return { error: String(err?.stack ?? err) }
		}
	}`, name)
	if err != nil {
		t.Fatalf("run UnixFS mkdir benchmark %q: %v", name, err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("run UnixFS mkdir benchmark %q: %#v", name, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("run UnixFS mkdir benchmark %q: %s", name, errMsg)
	}
	durationMs, ok := result["durationMs"].(float64)
	if !ok || durationMs <= 0 {
		t.Fatalf("run UnixFS mkdir benchmark %q duration: %#v", name, result["durationMs"])
	}
	return durationMs
}

func releaseUnixFSMkdirBenchmark(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const bench = globalThis.__s4waveUnixFSMkdirBenchmark
		if (!bench) return
		delete globalThis.__s4waveUnixFSMkdirBenchmark
		try {
			if (bench.names.length) {
				await bench.rootHandle.remove(bench.names, AbortSignal.timeout(120000))
			}
		} finally {
			bench.rootHandle?.release?.()
			bench.world?.release?.()
			while (bench.cleanupStack.length) bench.cleanupStack.pop()?.release?.()
			bench.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Errorf("release UnixFS mkdir benchmark: %v", err)
	}
}

func unixFSMkdirArtifactDir(t testing.TB) string {
	t.Helper()

	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("find repo root for UnixFS mkdir artifacts: %v", err)
	}
	return filepath.Join(repoRoot, ".bldr", "e2e-goscript-unixfs-mkdir")
}
