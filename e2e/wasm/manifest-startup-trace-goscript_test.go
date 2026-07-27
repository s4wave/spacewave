//go:build !skip_e2e && !js

package wasm

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

const e2eWasmManifestStartupTraceEnv = "E2E_WASM_MANIFEST_STARTUP_TRACE"

// TestGoScriptManifestStartupTrace captures and writes the root browser runtime
// from process initialization through configured startup completion.
func TestGoScriptManifestStartupTrace(t *testing.T) {
	if os.Getenv(e2eWasmManifestStartupTraceEnv) == "" {
		t.Skip("set " + e2eWasmManifestStartupTraceEnv + "=1 to capture the Manifest startup trace")
	}
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve e2e wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("GoScript-only Manifest startup trace; compiler=%s", compiler)
	}

	// The trace hook is a build tag, so the shared harness has to be compiled
	// with it. Once an earlier test in this slice has booted that harness there
	// is nothing left to configure, and the run would quietly measure an
	// uninstrumented worker and then fail looking for the callback.
	if sharedHarnessBooted() {
		t.Fatalf("shared harness already built without the trace tag; run this test alone with -run %s", t.Name())
	}

	t.Setenv(gocompiler.RuntimeStartupTraceEnv, "1")
	t.Setenv("E2E_WASM_STARTUP_BUILD_CACHE", "false")
	h := harness(t)
	sess := h.NewCleanBlankSession(t)
	if err := h.loadAppPageURL(sess, h.baseURL+"/#/pair/00000000"); err != nil {
		t.Fatalf("load Pairing startup route: %v", err)
	}
	WaitForApp(t, sess.Page())
	AssertBrowserStartupDone(t, h, sess.Page())

	data := stopStartupTrace(t, sess)
	tracePath := TraceArtifactPath(t)
	if err := WriteTraceArtifact(tracePath, data); err != nil {
		t.Fatalf("write startup trace: %v", err)
	}
	assertTraceParses(t, data)
	t.Logf("wrote %d trace bytes to %s", len(data), tracePath)
}

func stopStartupTrace(t testing.TB, sess *TestSession) []byte {
	t.Helper()
	const (
		stopScript = `() => {
			const stop = globalThis.BLDR_STOP_STARTUP_TRACE
			return typeof stop === 'function' ? stop() : null
		}`
		readScript  = `(arg) => globalThis.BLDR_READ_STARTUP_TRACE(arg.offset, arg.size)`
		abortScript = `() => {
			const read = globalThis.BLDR_READ_STARTUP_TRACE
			if (typeof read === 'function') read({ offset: -1, size: 0 })
		}`
		chunkSize = 256 * 1024
	)
	var workerResults []string
	for _, worker := range sess.Workers() {
		raw, err := worker.Evaluate(stopScript)
		if err != nil {
			workerResults = append(workerResults, worker.URL()+": "+err.Error())
			continue
		}
		if raw == nil {
			workerResults = append(workerResults, worker.URL()+": no callback")
			continue
		}
		traceCompleted := false
		defer func() {
			if !traceCompleted {
				_, _ = worker.Evaluate(abortScript)
			}
		}()
		if encodedErr, ok := raw.(string); ok {
			if after, ok := strings.CutPrefix(encodedErr, "error:"); ok {
				t.Fatalf("startup trace callback failed: %s", after)
			}
			t.Fatalf("startup trace callback returned unexpected string %q from %s", encodedErr, worker.URL())
		}
		var encodedLength int
		switch v := raw.(type) {
		case int:
			encodedLength = v
		case float64:
			encodedLength = int(v)
		}
		if encodedLength <= 0 {
			t.Fatalf("startup trace callback returned %T(%v) from %s", raw, raw, worker.URL())
		}

		var encoded strings.Builder
		encoded.Grow(encodedLength)
		for offset := 0; offset < encodedLength; offset += chunkSize {
			size := min(chunkSize, encodedLength-offset)
			chunkRaw, err := worker.Evaluate(readScript, map[string]any{
				"offset": offset,
				"size":   size,
			})
			if err != nil {
				t.Fatalf("read startup trace from %s at %d: %v", worker.URL(), offset, err)
			}
			chunk, ok := chunkRaw.(string)
			if !ok || len(chunk) != size {
				t.Fatalf("startup trace chunk from %s at %d = %T bytes=%d, want %d", worker.URL(), offset, chunkRaw, len(chunk), size)
			}
			encoded.WriteString(chunk)
		}
		traceCompleted = true
		data, err := base64.StdEncoding.DecodeString(encoded.String())
		if err != nil {
			t.Fatalf("decode startup trace from %s: %v", worker.URL(), err)
		}
		if len(data) == 0 {
			t.Fatalf("startup trace from %s is empty", worker.URL())
		}
		return data
	}
	t.Fatalf("root startup trace callback not found; workers=%v", workerResults)
	return nil
}
