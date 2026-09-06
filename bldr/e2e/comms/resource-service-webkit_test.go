//go:build !skip_e2e && !js

package comms

import "testing"

// TestGoScriptResourceServiceWebKit checks server-side resource release through
// WebKit's worker transport, where shared-memory transport may be unavailable.
func TestGoScriptResourceServiceWebKit(t *testing.T) {
	// Compile the real GoScript resource server and exercise the browser client.
	ensureGoScriptFixtureWorker(t, &resourceServiceGoScriptFixtureWorker)
	results := runFixture(t, "webkit", "goscript-resource-service")
	if pass, ok := results["pass"].(bool); !ok || !pass {
		t.Fatalf("WebKit resource lifecycle failed: %v", results["detail"])
	}
}
