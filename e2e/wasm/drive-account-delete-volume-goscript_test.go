//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"slices"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestGoScriptDriveAccountDeleteRemovesVolume proves that deleting an account
// removes its browser volume storage, whichever device holds it. The page reads
// storage directly: OPFS and IndexedDB are origin-global, so the page observes
// exactly what the runtime wrote and removed.
func TestGoScriptDriveAccountDeleteRemovesVolume(t *testing.T) {
	// This case exercises the GoScript runtime's volume deletion.
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	// Create one account through the drive quickstart.
	sess := harness(t).NewCleanSession(t)
	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	// Record the browser volumes present before deletion.
	before := listBrowserVolumes(t, page)
	if len(before) == 0 {
		t.Fatal("expected a browser volume after drive ready, found none")
	}

	// Delete through the account API that owns its volume lifetime.
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

	// The volumes after deletion must be a strict subset of those before: other
	// volumes may legitimately survive, but the account's must be gone and
	// nothing new may appear. DeleteAccount is the only storage mutation
	// between the two snapshots.
	after := listBrowserVolumes(t, page)
	for _, v := range after {
		if !slices.Contains(before, v) {
			t.Fatalf("unexpected new volume after account delete: %q (before: %v, after: %v)", v, before, after)
		}
	}
	if len(after) >= len(before) {
		t.Fatalf("account delete did not remove a volume: before=%v after=%v", before, after)
	}
}

// listBrowserVolumes returns the storage of every browser volume: its OPFS
// directory under volumes/ and its IndexedDB database named volumes/<id>.
func listBrowserVolumes(t testing.TB, page playwright.Page) []string {
	// Inspect the origin's storage directly, independently of worker routing.
	t.Helper()
	result, err := page.Evaluate(`async () => {
		const out = []
		const root = await navigator.storage.getDirectory()
		try {
			const dir = await root.getDirectoryHandle('volumes')
			for await (const [name, handle] of dir.entries()) {
				if (handle.kind === 'directory') {
					out.push('opfs:volumes/' + name)
				}
			}
		} catch (err) {
			if (err?.name !== 'NotFoundError') {
				throw err
			}
		}
		for (const db of await indexedDB.databases()) {
			if (db.name?.startsWith('volumes/')) {
				out.push('idb:' + db.name)
			}
		}
		return out
	}`)
	if err != nil {
		t.Fatalf("list browser volumes: %v", err)
	}

	// Decode the volume names.
	entries, ok := result.([]any)
	if !ok {
		t.Fatalf("browser volume list: unexpected result type %T", result)
	}
	volumes := make([]string, 0, len(entries))
	for _, entry := range entries {
		name, ok := entry.(string)
		if !ok {
			t.Fatalf("browser volume list: unexpected entry type %T", entry)
		}
		volumes = append(volumes, name)
	}
	return volumes
}
