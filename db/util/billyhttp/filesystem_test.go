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

// TestDirReaddirContinues tests that limited Readdir calls continue through
// the directory and end with io.EOF.
func TestDirReaddirContinues(t *testing.T) {
	// Create a directory with three files.
	mfs := memfs.New()
	for _, name := range []string{"a", "b", "c"} {
		if err := util.WriteFile(mfs, "dir/"+name, nil, 0o644); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Read the directory two entries at a time.
	dir := NewDir(mfs, "dir")
	var names []string
	for {
		fis, err := dir.Readdir(2)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err.Error())
		}
		for _, fi := range fis {
			names = append(names, fi.Name())
		}
	}

	// Verify each entry was returned exactly once.
	if len(names) != 3 || names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Fatalf("names: %v", names)
	}
}
