package unixfs_http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	iofs_mock "github.com/aperturerobotics/hydra/unixfs/iofs/mock"
	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/sirupsen/logrus"
)

func TestFileSystem(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	ifs, expectedFiles := iofs_mock.NewMockIoFS()
	fsc, err := unixfs_iofs.NewFSCursor(ifs)
	if err != nil {
		t.Fatal(err.Error())
	}

	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}

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

	fileServer := http.FileServer(httpFs)
	httpServer := httptest.NewServer(fileServer)
	httpClient := httpServer.Client()

	for _, fileName := range expectedFiles {
		req, err := http.NewRequest("GET", httpServer.URL+"/"+fileName, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		resp, err := httplog.DoRequest(le, httpClient, req, true)
		// resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("get %s: %v", fileName, err.Error())
		}
		respData, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(respData) == 0 {
			t.FailNow()
		}
	}
}
