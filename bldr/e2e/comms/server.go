package comms

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
)

// fixtureHTMLTemplate renders an HTML page that loads a fixture JS module.
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
func newTestServer(distDir string) (*httptest.Server, error) {
	return newFixtureServer(distDir, true)
}

// newTestServerNoCOI creates an httptest.Server without Cross-Origin Isolation
// headers. Used to test Config A/F fallback detection.
func newTestServerNoCOI(distDir string) (*httptest.Server, error) {
	return newFixtureServer(distDir, false)
}

// newFixtureServer serves built fixture assets from distDir. When coi is set,
// every response carries Cross-Origin Isolation headers (COOP, COEP, CORP)
// required for SharedArrayBuffer, Atomics, and OPFS sync access handle.
// Binds to localhost (not 127.0.0.1) so the server is a secure context for
// ServiceWorker registration on all browsers.
func newFixtureServer(distDir string, coi bool) (*httptest.Server, error) {
	fs := http.FileServer(http.Dir(distDir))
	mux := http.NewServeMux()

	rootMessage := "worker-comms test server"
	if !coi {
		rootMessage = "worker-comms test server (no COI)"
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if coi {
			setCOIHeaders(w)
		}

		path := r.URL.Path
		if path == "/" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintln(w, rootMessage)
			return
		}

		// /<name>.html -> generated HTML loading /<name>.js
		if name, ok := fixtureNameFromPath(path); ok {
			jsPath := filepath.Join(distDir, name+".js")
			// #nosec G703 -- fixture names are restricted to alnum/dash identifiers under distDir.
			if _, err := os.Stat(jsPath); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// #nosec G705 -- validated fixture identifiers are rendered into a controlled test template.
			fmt.Fprintf(w, fixtureHTMLTemplate, name, name)
			return
		}

		// ServiceWorker scripts need Service-Worker-Allowed header.
		if filepath.Ext(path) == ".js" {
			w.Header().Set("Service-Worker-Allowed", "/")
		}

		// Static files (JS, WASM, sourcemaps, etc.)
		fs.ServeHTTP(w, r)
	})

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener = listener
	srv.Start()
	return srv, nil
}

// setCOIHeaders sets Cross-Origin Isolation headers required for
// SharedArrayBuffer, Atomics, and OPFS sync access handle.
func setCOIHeaders(w http.ResponseWriter) {
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}

func fixtureNameFromPath(path string) (string, bool) {
	if filepath.Ext(path) != ".html" || len(path) < len("/a.html") {
		return "", false
	}
	name := path[1 : len(path)-len(".html")]
	if name == "" {
		return "", false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' {
			continue
		}
		return "", false
	}
	return name, true
}
