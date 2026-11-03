//go:build !skip_e2e && !js

package comms

import (
	"runtime"
	"testing"
)

// TestGoScriptOpfsStorage checks raw storage and an actual engine's advertised
// filesystem graph across a worker restart on OPFS and IndexedDB. DEX's local-only
// bucket lookup serves the graph with production protobuf message bindings.
func TestGoScriptOpfsStorage(t *testing.T) {
	for _, browser := range []string{"chromium", "webkit"} {
		t.Run(browser, func(t *testing.T) {
			// Linux WebKit lacks the writable-file API used by this fixture.
			// The bldr-opfs-webkit CI lane runs the full regression on macOS.
			if browser == "webkit" && runtime.GOOS == "linux" {
				t.Skip("WebKit OPFS requires macOS; covered by the bldr-opfs-webkit CI lane")
			}
			t.Parallel()

			// Compile and run the real GoScript worker through a fresh browser profile.
			ensureGoScriptFixtureWorker(t, &opfsStorageGoScriptFixtureWorker)
			results := runFixture(t, browser, "goscript-opfs-storage")
			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("GoScript OPFS storage fixture failed: %v", results["detail"])
			}

			// Require both sides of the worker restart and final storage deletion.
			assertBoolResult(t, results, "workerReady", true)
			assertBoolResult(t, results, "write", true)
			assertBoolResult(t, results, "reloadRead", true)
			assertBoolResult(t, results, "cleanup", true)
		})
	}
}
