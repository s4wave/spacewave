package web_pkg_http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRedirectToPinnedBase proves the redirect pins the base, keeps the query,
// escapes a file name that contains path-significant characters, and refuses a
// relPath that climbs out of the base. An empty wantPath expects not found.
func TestRedirectToPinnedBase(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		query    string
		wantPath string
	}{
		{name: "plain", relPath: "chunk.js", wantPath: "/p/pkg/chunk.js"},
		{name: "scoped with query", relPath: "@scope/name/x.js", query: "v=1", wantPath: "/p/pkg/@scope/name/x.js?v=1"},
		{name: "question mark in name", relPath: "a?b.js", wantPath: "/p/pkg/a%3Fb.js"},
		{name: "dot dot inside base", relPath: "a/../b.js", wantPath: "/p/pkg/b.js"},
		{name: "dot dot out of base", relPath: "../../evil.com", query: "v=1"},
		{name: "dot dot to sibling base", relPath: "../pkg2/x.js"},
		{name: "double slash stays a path", relPath: "//evil.com/x.js", wantPath: "/p/pkg/evil.com/x.js"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/b/pkg/x", nil)
			req.URL.RawQuery = test.query
			rw := httptest.NewRecorder()

			RedirectToPinnedBase(rw, req, "/p/pkg/", test.relPath)

			if test.wantPath == "" {
				if rw.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want %d", rw.Code, http.StatusNotFound)
				}
				if got := rw.Header().Get("Location"); got != "" {
					t.Fatalf("location = %q, want none", got)
				}
				return
			}
			if rw.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status = %d, want %d", rw.Code, http.StatusTemporaryRedirect)
			}
			if got := rw.Header().Get("Location"); got != test.wantPath {
				t.Fatalf("location = %q, want %q", got, test.wantPath)
			}
			if got := rw.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("cache-control = %q, want no-store", got)
			}
		})
	}
}
