package billyhttp

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
)

// TestFileSystem tests the HTTP filesystem.
func TestFileSystem(t *testing.T) {
	mfs := memfs.New()

	err := mfs.MkdirAll("./stuff", 0755)
	if err != nil {
		t.Fatal(err.Error())
	}

	data := []byte("hello world!\n")
	err = util.WriteFile(mfs, "./stuff/test.txt", data, 0755)
	if err != nil {
		t.Fatal(err.Error())
	}

	var hfs http.FileSystem = NewFileSystem(mfs, "/test")

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(hfs))

	req := httptest.NewRequest("GET", "/test/stuff/test.txt", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	res := rw.Result()
	if res.StatusCode != 200 {
		t.Fatalf("status code: %d", res.StatusCode)
	}

	readData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readData, data) {
		t.Fail()
	}
}
