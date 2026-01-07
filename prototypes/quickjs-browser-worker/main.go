package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	quickjs_wasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
)

//go:embed index.html worker.js
var staticFS embed.FS

//go:embed wasi-shim/wasi-shim.esm.js
var wasiShimJS []byte

// corsMiddleware adds CORS headers to allow cross-origin requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	addr := ":8090"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	// Find bldr root (we're in prototypes/quickjs-browser-worker)
	cwd, _ := os.Getwd()
	bldrRoot := filepath.Join(cwd, "../..")

	mux := http.NewServeMux()

	// Serve the QuickJS WASM binary from the Go embed
	mux.HandleFunc("/qjs-wasi.wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(quickjs_wasi.QuickJSWASM)
	})

	// Serve static files (index.html, worker.js)
	staticHandler := http.FileServer(http.FS(staticFS))
	mux.Handle("/", staticHandler)

	// Serve node_modules from disk for development
	if nodeModules, err := fs.Stat(staticFS, "node_modules"); err == nil && nodeModules.IsDir() {
		// If node_modules is embedded (won't be), use it
	} else {
		// Serve from disk
		mux.Handle("/node_modules/", http.StripPrefix("/node_modules/", http.FileServer(http.Dir("node_modules"))))
	}

	// Serve bundled wasi-shim ES module
	mux.HandleFunc("/wasi-shim.esm.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(wasiShimJS)
	})

	// Serve the boot harness from disk (read at request time for development)
	bootHarnessPath := filepath.Join(bldrRoot, "plugin/host/wazero-quickjs/plugin-quickjs.esm.js")
	mux.HandleFunc("/boot/plugin-quickjs.esm.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(bootHarnessPath)
		if err != nil {
			http.Error(w, "Boot harness not found: "+err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(data)
	})

	fmt.Printf("QuickJS Browser Worker Prototype\n")
	fmt.Printf("QuickJS WASM version: %s\n", quickjs_wasi.Version)
	fmt.Printf("QuickJS WASM size: %d bytes\n", len(quickjs_wasi.QuickJSWASM))
	fmt.Printf("Boot harness: %s\n", bootHarnessPath)
	fmt.Printf("\nServer running at http://localhost%s\n", addr)

	server := &http.Server{
		Addr:              addr,
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
