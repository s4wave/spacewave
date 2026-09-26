//go:build !js

package releasewasm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseHandler serves static assets inside the configured root.
func TestReleaseHandler(t *testing.T) {
	// Keep a distinct outside file beside the static root to detect traversal.
	root := t.TempDir()
	staticDir := filepath.Join(root, "static")
	distDir := filepath.Join(root, "dist")
	for _, dir := range []string{staticDir, distDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"outside.txt":       "outside static root",
		"static/asset.txt":  "static asset",
		"static/app.mjs.gz": "compressed module",
		"static/help.html":  "prerendered help",
		"dist/app.js":       "release bundle",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler := releaseHandler(distDir, staticDir, "http://localhost")

	// Exercise normal dispatch, gzip headers, and encoded traversal through HTTP paths.
	for _, test := range []struct {
		// path is the request target, including any URL escapes.
		path string
		// body is the exact response body for a successful asset request.
		body string
	}{
		{path: "/static/asset.txt", body: "static asset"},
		{path: "/static/app.mjs.gz", body: "compressed module"},
		{path: "/help", body: "prerendered help"},
		{path: "/app.js", body: "release bundle"},
		{path: "/static/../outside.txt"},
		{path: "/static/%2e%2e/outside.txt"},
		{path: "/static/..%2foutside.txt"},
		{path: "/static/..%5coutside.txt"},
		{path: "/static/nested/../../outside.txt"},
	} {
		t.Run(test.path, func(t *testing.T) {
			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, test.path, nil))
			if test.body == "" {
				if rw.Code != http.StatusNotFound {
					t.Fatalf("traversal returned %d: %s", rw.Code, rw.Body.String())
				}
				return
			}
			if rw.Code != http.StatusOK {
				t.Fatalf("response status = %d, want 200", rw.Code)
			}
			if rw.Body.String() != test.body {
				t.Fatalf("response body = %q, want %q", rw.Body.String(), test.body)
			}
			if strings.HasSuffix(test.path, ".mjs.gz") {
				if got := rw.Header().Get("Content-Encoding"); got != "gzip" {
					t.Fatalf("Content-Encoding = %q", got)
				}
				if got := rw.Header().Get("Content-Type"); got != "application/javascript" {
					t.Fatalf("Content-Type = %q", got)
				}
			}
		})
	}
}
