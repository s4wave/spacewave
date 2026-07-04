//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

// opfsFormatMarkerName is the per-volume root marker written at volume open
// (db/volume/js/opfs/runtime.go). Its presence fingerprints a live OPFS volume
// subtree, so listing markers from the page is a path-agnostic way to assert a
// volume subtree exists and, after account deletion, is gone.
const opfsFormatMarkerName = ".spacewave-opfs-format.json"

// TestGoScriptDriveAccountDeleteRemovesOpfsSubtree proves that deleting an
// account removes its OPFS volume subtree. Under E2E_WASM_WORKER_MODE=shared the
// Go runtime drives every OPFS op through the RemoteDriver bridge, so this
// exercises the bridge-backed delete path
// (DeleteAccount -> vol.Delete -> deleteRuntimeRoot -> opfs.DeleteEntry). The
// page reads OPFS directly: the main-thread window context can call
// navigator.storage.getDirectory even in shared-worker mode (only the
// SharedWorker scope throws SecurityError), and OPFS is origin-global, so the
// page observes exactly what the bridge wrote and removed.
func TestGoScriptDriveAccountDeleteRemovesOpfsSubtree(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := harness(t).NewCleanSession(t)
	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()

	WaitForDriveReady(t, harness(t), page)

	beforeMarkers := listOpfsFormatMarkers(t, page)
	if len(beforeMarkers) == 0 {
		t.Fatalf("expected an OPFS volume format marker after drive ready, found none")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	s, err := sess.MountSessionByIdx(ctx, scenario.GetSessionIndex())
	if err != nil {
		t.Fatalf("MountSessionByIdx: %v", err)
	}
	defer s.Release()

	if _, err := s.DeleteAccount(ctx, scenario.GetSessionIndex()); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	// Deleting the account must remove its volume subtree over the bridge. Assert
	// the marker set strictly shrank rather than becoming empty: a read-only dist
	// volume may also be OPFS-backed, so other markers can legitimately survive,
	// but the account's must be gone and nothing new may appear. This test is the
	// sole OPFS mutator between the two synchronous snapshots (one clean session,
	// one account, DeleteAccount the only intervening op), so subset-with-shrink
	// is exactly "the deleted account's subtree was removed".
	afterMarkers := listOpfsFormatMarkers(t, page)
	before := make(map[string]bool, len(beforeMarkers))
	for _, m := range beforeMarkers {
		before[m] = true
	}
	for _, m := range afterMarkers {
		if !before[m] {
			t.Fatalf("unexpected new OPFS marker after account delete: %q (before: %v, after: %v)", m, beforeMarkers, afterMarkers)
		}
	}
	if len(afterMarkers) >= len(beforeMarkers) {
		t.Fatalf("account delete did not remove an OPFS volume subtree: before=%v after=%v", beforeMarkers, afterMarkers)
	}
}

// listOpfsFormatMarkers walks the origin OPFS tree from the page and returns the
// full paths of every volume format marker. It runs in the main-thread window
// context where navigator.storage.getDirectory is available.
func listOpfsFormatMarkers(t testing.TB, page playwright.Page) []string {
	t.Helper()

	result, err := page.Evaluate(`async (markerName) => {
		const out = []
		const walk = async (dir, prefix) => {
			for await (const [name, handle] of dir.entries()) {
				const path = prefix + '/' + name
				if (handle.kind === 'directory') {
					await walk(handle, path)
				} else if (name === markerName) {
					out.push(path)
				}
			}
		}
		const root = await navigator.storage.getDirectory()
		await walk(root, '')
		return out
	}`, opfsFormatMarkerName)
	if err != nil {
		t.Fatalf("list OPFS format markers: %v", err)
	}

	entries, ok := result.([]any)
	if !ok {
		t.Fatalf("OPFS marker list: unexpected result type %T", result)
	}
	markers := make([]string, 0, len(entries))
	for _, entry := range entries {
		path, ok := entry.(string)
		if !ok {
			t.Fatalf("OPFS marker list: unexpected entry type %T", entry)
		}
		markers = append(markers, path)
	}
	return markers
}
