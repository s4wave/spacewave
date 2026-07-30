package web_runtime_controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/bldr/core"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_pkg_controller "github.com/s4wave/spacewave/bldr/web/pkg/controller"
	web_pkg_http "github.com/s4wave/spacewave/bldr/web/pkg/http"
	web_pkg_mock "github.com/s4wave/spacewave/bldr/web/pkg/mock"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/sirupsen/logrus"
)

func TestServeServiceWorkerHTTPServesBrowserIndexSeed(t *testing.T) {
	rtCtrl := &Controller{
		le: logrus.NewEntry(logrus.New()),
	}
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/b/__index.html", nil)

	rtCtrl.ServeServiceWorkerHTTP(rw, req)

	res := rw.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want text/html; charset=utf-8", got)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	text := string(body)
	if !strings.Contains(text, "<!doctype html>") || !strings.Contains(text, `<html lang="en">`) {
		t.Fatalf("index HTML missing document shell: %s", text)
	}
	if !strings.Contains(text, `<script type="module" src="/boot.mjs"></script>`) {
		t.Fatalf("index HTML missing absolute boot entrypoint wiring: %s", text)
	}
	if !strings.Contains(text, `id="bldr-root"`) {
		t.Fatalf("index HTML missing bldr root: %s", text)
	}
}

func TestServeServiceWorkerHTTPServesWebPackageModule(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	mockWebPkg := web_pkg_mock.NewMockWebPkg()
	ctrl := web_pkg_controller.NewControllerWithWebPkg(
		le,
		controller.NewInfo("web/pkg/runtime-test", controller.MustParseVersion("0.0.1"), "test web pkg"),
		mockWebPkg,
	)
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	rtCtrl := &Controller{
		le:        le,
		bus:       b,
		pkgServer: web_pkg_http.NewServer(le, b, true),
	}
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/b/pkg/"+mockWebPkg.GetId()+"/testdir/testing.txt", nil)

	rtCtrl.ServeServiceWorkerHTTP(rw, req)

	res := rw.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Cross-Origin-Embedder-Policy"); got != "require-corp" {
		t.Fatalf("unexpected COEP header: %q", got)
	}
	if got := res.Header.Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
		t.Fatalf("unexpected CORP header: %q", got)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(body) != "file within a directory" {
		t.Fatalf("unexpected web package body: %q", string(body))
	}
}

func TestServePluginAssetsFsHTTPRebindsPendingFrontendAssets(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	tb := btb
	moduleBody1 := []byte("export const generation = 'first'\n")
	styleBody1 := []byte(".app{color:red}\n")
	rootRef1, err := newTestPluginAssetsRoot(ctx, map[string][]byte{
		"/v/b/fe/app/App-next.mjs": moduleBody1,
		"/v/b/fe/app/App-next.css": styleBody1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef1.Release()
	moduleBody2 := []byte("export const generation = 'second'\n")
	styleBody2 := []byte(".app{color:blue}\n")
	rootRef2, err := newTestPluginAssetsRoot(ctx, map[string][]byte{
		"/v/b/fe/app/App-next.mjs": moduleBody2,
		"/v/b/fe/app/App-next.css": styleBody2,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef2.Release()

	pluginID := "spacewave-app"
	unixFsID := bldr_plugin.PluginAssetsFsId(pluginID)
	rotating := unixfs_access.NewRotatingAccess()
	accessCtrl := unixfs_access.NewController(
		tb.Logger,
		tb.Bus,
		controller.NewInfo("bldr/web/runtime/test-assets", controller.MustParseVersion("0.0.1"), "test plugin assets access"),
		[]string{unixFsID},
		rotating.AccessUnixFS,
	)
	accessRel, err := tb.Bus.AddController(ctx, accessCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessRel()

	rtCtrl := &Controller{
		le:  tb.Logger,
		bus: tb.Bus,
	}

	tests := []struct {
		name string
		path string
		body []byte
	}{
		{
			name: "module",
			path: "/v/b/fe/app/App-next.mjs",
			body: moduleBody2,
		},
		{
			name: "stylesheet",
			path: "/v/b/fe/app/App-next.css",
			body: styleBody2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runPendingFetch := func(body []byte, replacement *unixfs.FSHandle) {
				reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
				defer reqCancel()

				started := make(chan struct{})
				rotating.SetCurrent(func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
					close(started)
					<-ctx.Done()
					return nil, nil, ctx.Err()
				})
				rw := httptest.NewRecorder()
				req := httptest.NewRequest("GET", tt.path, nil).WithContext(reqCtx)
				done := make(chan struct{})
				go func() {
					rtCtrl.ServePluginAssetsFsHTTP(pluginID, rw, req)
					close(done)
				}()

				select {
				case <-started:
				case <-reqCtx.Done():
					t.Fatalf("blocked provider did not start: %v", reqCtx.Err())
				}

				rotating.SetCurrent(unixfs_access.NewAccessUnixFSFunc(replacement))

				select {
				case <-done:
				case <-reqCtx.Done():
					t.Fatalf("request did not complete after replacement provider: %v", reqCtx.Err())
				}

				assertHTTPAssetResponse(t, rw, body)
			}

			firstBody := moduleBody1
			if tt.path == "/v/b/fe/app/App-next.css" {
				firstBody = styleBody1
			}
			runPendingFetch(firstBody, rootRef1)
			runPendingFetch(tt.body, rootRef2)

			rw := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.path, nil)
			rtCtrl.ServePluginAssetsFsHTTP(pluginID, rw, req)
			assertHTTPAssetResponse(t, rw, tt.body)
		})
	}
}

func newTestPluginAssetsRoot(ctx context.Context, files map[string][]byte) (*unixfs.FSHandle, error) {
	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		return nil, err
	}
	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	for path, body := range files {
		if err := billy_util.WriteFile(rbfs, path, body, 0o644); err != nil {
			rootRef.Release()
			return nil, err
		}
	}
	return rootRef, nil
}

func assertHTTPAssetResponse(t *testing.T, rw *httptest.ResponseRecorder, wantBody []byte) {
	t.Helper()

	res := rw.Result()
	if res.StatusCode != 200 {
		t.Fatalf("status code: %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(body) != string(wantBody) {
		t.Fatalf("unexpected body: %q", string(body))
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache, no-store, must-revalidate" {
		t.Fatalf("unexpected cache-control header: %q", got)
	}
}
