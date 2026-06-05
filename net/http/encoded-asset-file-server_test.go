package bifrost_http

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEncodedAssetFileServerGzipHeaders(t *testing.T) {
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

	for _, test := range tests {
		rw := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://spacewave.local"+test.path, nil)
		handler.ServeHTTP(rw, req)
		res := rw.Result()
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", test.path, res.StatusCode)
		}
		if got := res.Header.Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("%s Content-Encoding = %q, want gzip", test.path, got)
		}
		if got := res.Header.Get("Content-Type"); got != test.contentType {
			t.Fatalf("%s Content-Type = %q, want %q", test.path, got, test.contentType)
		}
		if got := res.Header.Get("Vary"); got != encodedAssetVary {
			t.Fatalf("%s Vary = %q, want %q", test.path, got, encodedAssetVary)
		}
		if got := res.Header.Get("Content-Length"); got != "10" {
			t.Fatalf("%s Content-Length = %q, want 10", test.path, got)
		}
		if string(body) != "gzip-bytes" {
			t.Fatalf("%s body = %q, want gzip-bytes", test.path, string(body))
		}
	}
}

func TestEncodedAssetFileServerWasmContentType(t *testing.T) {
	files := fstest.MapFS{
		"entrypoint/runtime.wasm": &fstest.MapFile{Data: []byte("wasm-bytes")},
	}
	handler := NewEncodedAssetFileServer(http.FS(files))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://spacewave.local/entrypoint/runtime.wasm", nil)
	handler.ServeHTTP(rw, req)
	res := rw.Result()
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/wasm" {
		t.Fatalf("Content-Type = %q, want application/wasm", got)
	}
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestEncodedAssetFileServerMissingGzipDoesNotSetEncodedHeaders(t *testing.T) {
	handler := NewEncodedAssetFileServer(http.FS(fstest.MapFS{}))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://spacewave.local/missing.wasm.gz", nil)
	handler.ServeHTTP(rw, req)
	res := rw.Result()
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
}

func TestCleanHTTPFilePathRejectsTraversal(t *testing.T) {
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

	got, err := cleanHTTPFilePath("/entrypoint/runtime.wasm.gz")
	if err != nil {
		t.Fatalf("cleanHTTPFilePath(valid) error = %v", err)
	}
	if got != "entrypoint/runtime.wasm.gz" {
		t.Fatalf("cleanHTTPFilePath(valid) = %q, want entrypoint/runtime.wasm.gz", got)
	}
}

func TestEncodedAssetFileServerBrotliQualityZeroDecodes(t *testing.T) {
	files := fstest.MapFS{
		"entrypoint/app.js.br": &fstest.MapFile{Data: []byte{0x0b, 0x01, 0x80, 'j', 's', 0x03}},
	}
	handler := NewEncodedAssetFileServer(http.FS(files))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://spacewave.local/entrypoint/app.js.br", nil)
	req.Header.Set("Accept-Encoding", "gzip, br;q=0")
	handler.ServeHTTP(rw, req)
	res := rw.Result()
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if !strings.Contains(string(body), "js") {
		t.Fatalf("body = %q, want decoded brotli body", string(body))
	}
}
