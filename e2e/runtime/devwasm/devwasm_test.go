//go:build !js

package devwasm

import "testing"

func TestTransientNavigationErrorClassification(t *testing.T) {
	if !isTransientNavigationError("playwright: Execution context was destroyed, most likely because of a navigation") {
		t.Fatal("expected navigation context error to be transient")
	}
	if isTransientNavigationError("playwright: locator timeout") {
		t.Fatal("expected locator timeout not to be transient")
	}
}

func TestNeedsDocumentLoadOnlyForBlankPages(t *testing.T) {
	for _, pageURL := range []string{"", "about:blank"} {
		if !needsDocumentLoad(pageURL) {
			t.Fatalf("needsDocumentLoad(%q) = false", pageURL)
		}
	}
	if needsDocumentLoad("http://127.0.0.1:8080/#/") {
		t.Fatal("loaded page should use client-side navigation")
	}
}
