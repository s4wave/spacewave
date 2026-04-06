package comms

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
)

// fixtureHTML generates an HTML page that loads a fixture JS module.
const fixtureHTMLTemplate = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>%s</title></head>
<body>
<div id="log">LOADING</div>
<script type="module" src="/%s.js"></script>
</body>
</html>`

// newTestServer creates an httptest.Server that serves built fixture assets
// from distDir with Cross-Origin Isolation headers (COOP, COEP, CORP).
// Requests to /<name>.html serve a generated HTML page loading <name>.js.
// All other requests serve static files from distDir.
func newTestServer(distDir string) *httptest.Server {
	fs := http.FileServer(http.Dir(distDir))
	mux := http.NewServeMux()

	// Serve generated HTML pages for fixtures.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setCOIHeaders(w)

		path := r.URL.Path
		if path == "/" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintln(w, "worker-comms test server")
			return
		}

		// /<name>.html -> generated HTML loading /<name>.js
		if ext := filepath.Ext(path); ext == ".html" {
			name := path[1 : len(path)-len(ext)]
			// Check if the corresponding JS exists.
			jsPath := filepath.Join(distDir, name+".js")
			if _, err := os.Stat(jsPath); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, fixtureHTMLTemplate, name, name)
			return
		}

		// Static files (JS, WASM, sourcemaps, etc.)
		fs.ServeHTTP(w, r)
	})

	return httptest.NewServer(mux)
}

// setCOIHeaders sets Cross-Origin Isolation headers required for
// SharedArrayBuffer, Atomics, and OPFS sync access handle.
func setCOIHeaders(w http.ResponseWriter) {
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}
