//go:build !js

package web_pkg_http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/s4wave/spacewave/bldr/core"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_controller "github.com/s4wave/spacewave/bldr/web/pkg/controller"
	web_pkg_mock "github.com/s4wave/spacewave/bldr/web/pkg/mock"
	web_pkg_static "github.com/s4wave/spacewave/bldr/web/pkg/static"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

func TestWebPkgHttpServer(t *testing.T) {
	// Initialize the small Web Package server test and its logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Register the in-memory package served by the HTTP test.
	mockWebPkg := web_pkg_mock.NewMockWebPkg()
	ctrl := web_pkg_controller.NewControllerWithWebPkg(
		le,
		controller.NewInfo("web/pkg/static/test", controller.MustParseVersion("0.0.1"), "test web pkg"),
		mockWebPkg,
	)

	// Create the controller bus that resolves Web Package directives.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Attach the mock Web Package controller to the test bus.
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Serve package requests through the production HTTP adapter.
	server := NewServer(le, b, true)
	httpServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		server.ServeWebModuleHTTP(req.URL.Path, rw, req)
	}))
	httpClient := httpServer.Client()

	// Request a nested file from the mock Web Package.
	req, err := http.NewRequest("GET", httpServer.URL+"/"+mockWebPkg.GetId()+"/testdir/testing.txt", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the Web Package file through the HTTP server.
	resp, err := httplog.DoRequest(le, httpClient, req, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status code 200 got %v", resp.StatusCode)
	}

	// Read the returned package file body.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(data, []byte("file within a directory")) {
		t.Fatalf("got unexpected contents: %v", string(data))
	}
}

func TestWebPkgHttpServerServesLargeModuleBody(t *testing.T) {
	// Initialize the large-module response test and its logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Prepare a module body large enough to exercise streaming responses.
	const modulePath = "dist/index.mjs"
	moduleBody := bytes.Repeat([]byte("export const chunk = '0123456789abcdef';\n"), 900)
	if len(moduleBody) <= 32*1024 {
		t.Fatalf("test module body length = %d, want > 32768", len(moduleBody))
	}

	// Expose the generated module body through an in-memory filesystem.
	testFS := fstest.MapFS{
		modulePath: &fstest.MapFile{
			Data:    moduleBody,
			Mode:    0o644,
			ModTime: time.Now(),
		},
	}

	// Wrap the in-memory filesystem as a static Web Package.
	testWebPkg, err := web_pkg_static.NewStaticWebPkg(&web_pkg.WebPkgInfo{
		Id: "@aperturerobotics/large-module-test",
	}, func(context.Context) (*unixfs.FSHandle, error) {
		fsc, err := unixfs_iofs.NewFSCursor(testFS)
		if err != nil {
			return nil, err
		}
		return unixfs.NewFSHandle(fsc)
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the static package in a Web Package controller.
	ctrl := web_pkg_controller.NewControllerWithWebPkg(
		le,
		controller.NewInfo("web/pkg/static/large-body-test", controller.MustParseVersion("0.0.1"), "test web pkg"),
		testWebPkg,
	)

	// Create the controller bus that resolves the test package.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Attach the package controller to the test bus.
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// Serve package requests through the production HTTP adapter.
	server := NewServer(le, b, true)
	httpServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		server.ServeWebModuleHTTP(req.URL.Path, rw, req)
	}))
	defer httpServer.Close()
	httpClient := httpServer.Client()

	// Request the large module from the static Web Package.
	req, err := http.NewRequest("GET", httpServer.URL+"/"+testWebPkg.GetId()+"/"+modulePath, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the large Web Package module through the HTTP server.
	resp, err := httplog.DoRequest(le, httpClient, req, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200 got %v", resp.StatusCode)
	}

	// Read the complete module body returned by the package server.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(data) != len(moduleBody) {
		t.Fatalf("large module body length = %d, want %d", len(data), len(moduleBody))
	}
	if !bytes.Equal(data, moduleBody) {
		t.Fatal("large module body changed while serving web package")
	}
}
