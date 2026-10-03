package unixfs_http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	iofs_mock "github.com/s4wave/spacewave/db/unixfs/iofs/mock"
	"github.com/sirupsen/logrus"
)

func TestFileSystem(t *testing.T) {
	// Prepare the request context and HTTP request logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Open a cursor over the mock filesystem and retain its expected files.
	ifs, expectedFiles := iofs_mock.NewMockIoFS()
	fsc, err := unixfs_iofs.NewFSCursor(ifs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Wrap the mock cursor in a UnixFS handle.
	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the HTTP filesystem starts active with an empty prefix.
	httpFs, err := NewFileSystem(ctx, handle, "")
	if err != nil {
		t.Fatal(err.Error())
	}
	if httpFs.CheckReleased() {
		t.FailNow()
	}
	if httpFs.GetPrefix() != "" {
		t.FailNow()
	}

	// Serve the UnixFS handle through a local HTTP test server.
	fileServer := http.FileServer(httpFs)
	httpServer := httptest.NewServer(fileServer)
	httpClient := httpServer.Client()

	// Request every expected file through the HTTP filesystem.
	for _, fileName := range expectedFiles {
		// Fetch the expected UnixFS file through the HTTP server.
		req, err := http.NewRequest("GET", httpServer.URL+"/"+fileName, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		resp, err := httplog.DoRequest(le, httpClient, req, true)
		// resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("get %s: %v", fileName, err.Error())
		}

		// Verify the served UnixFS file contains response bytes.
		respData, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(respData) == 0 {
			t.FailNow()
		}
	}
}
