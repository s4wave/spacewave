package bifrost_http

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// testHTTPFileSystem exposes filesystem opens to the asset-serving tests.
type testHTTPFileSystem func(string) (http.File, error)

// Open opens a file through the test's instrumented filesystem.
func (f testHTTPFileSystem) Open(name string) (http.File, error) {
	return f(name)
}

// TestEncodedAssetFileServerPlainOpensOnce checks the ordinary module read path.
func TestEncodedAssetFileServerPlainOpensOnce(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			// Count opens against a real in-memory file.
			files := http.FS(fstest.MapFS{
				"app.mjs": &fstest.MapFile{Data: []byte("export default 1")},
			})
			opens := 0
			handler := NewEncodedAssetFileServer(testHTTPFileSystem(func(name string) (http.File, error) {
				opens++
				return files.Open(name)
			}))

			// Serve the module without a separate metadata open.
			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, httptest.NewRequest(method, "/app.mjs", nil))
			if rw.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rw.Code)
			}
			if opens != 1 {
				t.Fatalf("filesystem opens = %d, want 1", opens)
			}
			if got := rw.Header().Get("Content-Type"); got != "application/javascript" {
				t.Fatalf("Content-Type = %q, want application/javascript", got)
			}
			wantBody := "export default 1"
			if method == http.MethodHead {
				wantBody = ""
			}
			if got := rw.Body.String(); got != wantBody {
				t.Fatalf("body = %q, want %q", got, wantBody)
			}
		})
	}
}

// TestEncodedAssetFileServerMissingModuleKeepsErrorType checks HTTP error metadata.
func TestEncodedAssetFileServerMissingModuleKeepsErrorType(t *testing.T) {
	handler := NewEncodedAssetFileServer(http.FS(fstest.MapFS{}))
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/missing.mjs", nil))
	if rw.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rw.Code)
	}
	if got := rw.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
}

// TestEncodedAssetFileServerGzipHeaders checks encoded response metadata.
func TestEncodedAssetFileServerGzipHeaders(t *testing.T) {
	// Populate the supported precompressed asset formats.
	tests := []struct {
		path        string
		contentType string
	}{
		{path: "/entrypoint/runtime.wasm.gz", contentType: "application/wasm"},
		{path: "/entrypoint/entrypoint.mjs.gz", contentType: "application/javascript"},
		{path: "/entrypoint/entrypoint.js.gz", contentType: "application/javascript"},
		{path: "/entrypoint/entrypoint.css.gz", contentType: "text/css; charset=utf-8"},
		{path: "/assets-deadbeef.kvfile.gz", contentType: "application/octet-stream"},
		{path: "/assets-deadbeef.asset.gz", contentType: "application/octet-stream"},
	}
	files := fstest.MapFS{}
	for _, test := range tests {
		files[test.path[1:]] = &fstest.MapFile{Data: []byte("gzip-bytes")}
	}
	handler := NewEncodedAssetFileServer(http.FS(files))

	// Verify every format preserves the stored bytes and advertises gzip.
	for _, test := range tests {
		rw := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://spacewave.local"+test.path, nil)
		handler.ServeHTTP(rw, req)
		if rw.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", test.path, rw.Code)
		}
		if got := rw.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("%s Content-Encoding = %q, want gzip", test.path, got)
		}
		if got := rw.Header().Get("Content-Type"); got != test.contentType {
			t.Fatalf("%s Content-Type = %q, want %q", test.path, got, test.contentType)
		}
		if got := rw.Header().Get("Vary"); got != encodedAssetVary {
			t.Fatalf("%s Vary = %q, want %q", test.path, got, encodedAssetVary)
		}
		if got := rw.Header().Get("Content-Length"); got != "10" {
			t.Fatalf("%s Content-Length = %q, want 10", test.path, got)
		}
		if got := rw.Body.String(); got != "gzip-bytes" {
			t.Fatalf("%s body = %q, want gzip-bytes", test.path, got)
		}
	}
}

// TestEncodedAssetFileServerWasmContentType checks uncompressed WebAssembly metadata.
func TestEncodedAssetFileServerWasmContentType(t *testing.T) {
	// Serve an ordinary WebAssembly asset.
	files := fstest.MapFS{
		"entrypoint/runtime.wasm": &fstest.MapFile{Data: []byte("wasm-bytes")},
	}
	handler := NewEncodedAssetFileServer(http.FS(files))

	// Preserve its browser MIME type without an encoding header.
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://spacewave.local/entrypoint/runtime.wasm", nil)
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rw.Code)
	}
	if got := rw.Header().Get("Content-Type"); got != "application/wasm" {
		t.Fatalf("Content-Type = %q, want application/wasm", got)
	}
	if got := rw.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

// TestEncodedAssetFileServerModuleContentTypes checks MIME types for browser assets.
func TestEncodedAssetFileServerModuleContentTypes(t *testing.T) {
	// Populate the ordinary browser module and document formats.
	tests := []struct {
		path        string
		contentType string
	}{
		{path: "/b/pa/spacewave-app/v/b/fe/app/App2.mjs", contentType: "application/javascript"},
		{path: "/b/pd/spacewave-app/plugin.js", contentType: "application/javascript"},
		{path: "/entrypoint/style.css", contentType: "text/css; charset=utf-8"},
		{path: "/b/__index.html", contentType: "text/html; charset=utf-8"},
		{path: "/manifest.json", contentType: "application/json; charset=utf-8"},
	}
	files := fstest.MapFS{}
	for _, test := range tests {
		files[test.path[1:]] = &fstest.MapFile{Data: []byte("body")}
	}
	handler := NewEncodedAssetFileServer(http.FS(files))

	// Verify uncompressed responses carry their format's MIME type.
	for _, test := range tests {
		rw := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://spacewave.local"+test.path, nil)
		handler.ServeHTTP(rw, req)
		if rw.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", test.path, rw.Code)
		}
		if got := rw.Header().Get("Content-Type"); got != test.contentType {
			t.Fatalf("%s Content-Type = %q, want %q", test.path, got, test.contentType)
		}
		if got := rw.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("%s Content-Encoding = %q, want empty", test.path, got)
		}
	}
}

// TestEncodedAssetFileServerMissingGzipDoesNotSetEncodedHeaders checks error metadata.
func TestEncodedAssetFileServerMissingGzipDoesNotSetEncodedHeaders(t *testing.T) {
	handler := NewEncodedAssetFileServer(http.FS(fstest.MapFS{}))
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://spacewave.local/missing.wasm.gz", nil)
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rw.Code)
	}
	if got := rw.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

// TestCleanHTTPFilePathRejectsTraversal checks root confinement and valid normalization.
func TestCleanHTTPFilePathRejectsTraversal(t *testing.T) {
	// Reject traversal before normalizing path components.
	tests := []string{
		"../secrets/runtime.wasm.gz",
		"/entrypoint/../secrets/runtime.wasm.gz",
		`entrypoint\..\secrets\runtime.wasm.gz`,
	}
	for _, test := range tests {
		if _, err := cleanHTTPFilePath(test); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("cleanHTTPFilePath(%q) error = %v, want permission", test, err)
		}
	}

	// Preserve a valid asset path relative to the filesystem root.
	got, err := cleanHTTPFilePath("/entrypoint/runtime.wasm.gz")
	if err != nil {
		t.Fatalf("cleanHTTPFilePath(valid) error = %v", err)
	}
	if got != "entrypoint/runtime.wasm.gz" {
		t.Fatalf("cleanHTTPFilePath(valid) = %q, want entrypoint/runtime.wasm.gz", got)
	}
}

// TestEncodedAssetFileServerBrotliQualityZeroDecodes checks explicit Brotli rejection.
func TestEncodedAssetFileServerBrotliQualityZeroDecodes(t *testing.T) {
	// Populate a stored Brotli asset.
	files := fstest.MapFS{
		"entrypoint/app.js.br": &fstest.MapFile{Data: []byte{0x0b, 0x01, 0x80, 'j', 's', 0x03}},
	}
	handler := NewEncodedAssetFileServer(http.FS(files))

	// Decode it when the client's quality parameter refuses Brotli.
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://spacewave.local/entrypoint/app.js.br", nil)
	req.Header.Set("Accept-Encoding", "gzip, br;q=0")
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rw.Code)
	}
	if got := rw.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := rw.Body.String(); !strings.Contains(got, "js") {
		t.Fatalf("body = %q, want decoded brotli body", got)
	}
}
