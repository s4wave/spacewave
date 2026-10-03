package billyhttp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
)

// TestFileSystem tests the HTTP filesystem.
func TestFileSystem(t *testing.T) {
	// Create an in-memory directory for the HTTP file fixture.
	mfs := memfs.New()
	err := mfs.MkdirAll("./stuff", 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the file bytes beneath the test directory.
	data := []byte("hello world!\n")
	err = util.WriteFile(mfs, "./stuff/test.txt", data, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Serve the filesystem through an HTTP handler with the test prefix.
	var hfs http.FileSystem = NewFileSystem(mfs, "/test")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(hfs))

	// Request the saved file through its prefixed HTTP path.
	req := httptest.NewRequest("GET", "/test/stuff/test.txt", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	// Verify the filesystem handler returns a successful HTTP response.
	res := rw.Result()
	if res.StatusCode != 200 {
		t.Fatalf("status code: %d", res.StatusCode)
	}

	// Read the file bytes returned by the filesystem handler.
	readData, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the HTTP response preserves the stored file contents.
	if !bytes.Equal(readData, data) {
		t.Fail()
	}
}
