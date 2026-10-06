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
	web_pkg_mock "github.com/s4wave/spacewave/bldr/web/pkg/mock"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestImmutablePluginFilesHTTP keeps old modules and relative chunks independent
// of the current plugin, and never falls back when the exact binding is absent.
func TestImmutablePluginFilesHTTP(t *testing.T) {
	// Start a testbed and derive the old and missing plugin artifact IDs.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Derive the old and missing plugin artifact roots.
	oldRoot, err := hash.Sum(hash.RecommendedHashType, []byte("old"))
	if err != nil {
		t.Fatal(err)
	}
	missingRoot, err := hash.Sum(hash.RecommendedHashType, []byte("missing"))
	if err != nil {
		t.Fatal(err)
	}
	oldID := bldr_plugin.PluginArtifactID("colors", oldRoot.MarshalString())

	// Register an asset root controller for each immutable binding.
	for binding, contents := range map[string]string{"colors": "new", oldID: "old"} {
		root, err := newTestPluginAssetsRoot(ctx, map[string][]byte{
			"/entry.mjs": []byte(contents), "/chunks/shared.mjs": []byte(contents + " chunk"),
		})
		if err != nil {
			t.Fatal(err)
		}
		defer root.Release()
		ctrl := unixfs_access.NewController(le, tb.Bus,
			controller.NewInfo("immutable-files/"+binding, controller.MustParseVersion("0.0.1"), "test plugin files"),
			[]string{bldr_plugin.PluginDistFsId(binding), bldr_plugin.PluginAssetsFsId(binding)},
			unixfs_access.NewAccessUnixFSFunc(root))
		defer ctrl.Close()
		release, err := tb.Bus.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	runtime := NewController(le, tb.Bus, nil, "test", controller.MustParseVersion("0.0.1"))

	// Serve each immutable path through both HTTP prefixes.
	for _, prefix := range []string{bldr_plugin.PluginDistHttpPrefix, bldr_plugin.PluginAssetsHttpPrefix} {
		for path, want := range map[string]string{
			"colors/entry.mjs": "new", oldID + "/entry.mjs": "old", oldID + "/chunks/shared.mjs": "old chunk",
		} {
			rw := httptest.NewRecorder()
			runtime.ServeServiceWorkerHTTP(rw, httptest.NewRequest(http.MethodGet, prefix+path, nil).WithContext(ctx))
			if rw.Code != http.StatusOK || rw.Body.String() != want {
				t.Fatalf("%s: status=%d body=%q, want %q", path, rw.Code, rw.Body.String(), want)
			}
		}

		// Check a missing artifact does not fall back to the current plugin.
		rw := httptest.NewRecorder()
		path := prefix + bldr_plugin.PluginArtifactID("colors", missingRoot.MarshalString()) + "/entry.mjs"
		runtime.ServeServiceWorkerHTTP(rw, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
		if rw.Code == http.StatusOK {
			t.Fatal("missing immutable files fell back to current plugin")
		}
	}
}

func TestServeServiceWorkerHTTPServesBrowserIndexSeed(t *testing.T) {
	// Request the browser index seed from the service worker handler.
	rtCtrl := NewController(logrus.NewEntry(logrus.New()), nil, nil, "test", controller.MustParseVersion("0.0.1"))
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/b/__index.html", nil)

	// Serve the index seed request.
	rtCtrl.ServeServiceWorkerHTTP(rw, req)

	// Check the status and content type of the rendered index.
	res := rw.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want text/html; charset=utf-8", got)
	}

	// Check the document shell, boot wiring, and root element.
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
	// Start a core bus with a mock web pkg controller.
	ctx := t.Context()

	// Configure a debug-level logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the core bus.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the mock web pkg controller on the bus.
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

	// Serve a file inside the mock web pkg through the runtime handler.
	rtCtrl := NewController(le, b, nil, "test", controller.MustParseVersion("0.0.1"))
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/b/pkg/"+mockWebPkg.GetId()+"/testdir/testing.txt", nil)

	// Serve the web pkg file request.
	rtCtrl.ServeServiceWorkerHTTP(rw, req)

	// Check the cross-origin headers and body of the response.
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

func TestServeServiceWorkerHTTPDisablesPluginFileCaching(t *testing.T) {
	// Start a testbed for the cache header checks.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	btb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Publish a plugin asset root on the testbed bus.
	const pluginID = "cache-header-test"
	rootRef, err := newTestPluginAssetsRoot(ctx, map[string][]byte{
		"/entry.mjs": []byte("export const cached = false\n"),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef.Release()
	accessCtrl := unixfs_access.NewController(
		btb.Logger,
		btb.Bus,
		controller.NewInfo("bldr/web/runtime/test-cache-headers", controller.MustParseVersion("0.0.1"), "test plugin file cache headers"),
		[]string{
			bldr_plugin.PluginDistFsId(pluginID),
			bldr_plugin.PluginAssetsFsId(pluginID),
		},
		unixfs_access.NewAccessUnixFSFunc(rootRef),
	)
	accessRel, err := btb.Bus.AddController(ctx, accessCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessRel()

	// Serve the plugin file through each HTTP surface.
	rtCtrl := NewController(btb.Logger, btb.Bus, nil, "test", controller.MustParseVersion("0.0.1"))
	tests := []struct {
		name string
		path string
	}{
		{name: "dist", path: bldr_plugin.PluginDistHTTPPath(pluginID, "/entry.mjs")},
		{name: "assets", path: bldr_plugin.PluginAssetHTTPPath(pluginID, "/entry.mjs")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			rtCtrl.ServeServiceWorkerHTTP(rw, req)

			assertHTTPAssetResponse(
				t,
				rw,
				[]byte("export const cached = false\n"),
			)
		})
	}
}

func TestServeServiceWorkerHTTPRebindsPendingFrontendAssets(t *testing.T) {
	// Start a debug-logging testbed.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	btb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb := btb

	// Build two generations of the frontend asset roots.
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

	// Build the second generation of the frontend asset roots.
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

	// Register the rotating access controller on the testbed bus.
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

	// Build the runtime controller and the served path cases.
	rtCtrl := NewController(tb.Logger, tb.Bus, nil, "test", controller.MustParseVersion("0.0.1"))

	// Define the served path cases for the second generation.
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

	// Exercise each path case through the pending-fetch rebind sequence.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start a pending fetch against a blocked provider.
			runPendingFetch := func(body []byte, replacement *unixfs.FSHandle) {
				// Start a request context that bounds the pending fetch.
				reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
				defer reqCancel()

				// Install a provider that blocks until the request context ends.
				started := make(chan struct{})
				rotating.SetCurrent(func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
					close(started)
					<-ctx.Done()
					return nil, nil, ctx.Err()
				})

				// Serve the request in the background.
				rw := httptest.NewRecorder()
				req := httptest.NewRequest("GET", bldr_plugin.PluginAssetHTTPPath(pluginID, tt.path), nil).WithContext(reqCtx)
				done := make(chan struct{})
				go func() {
					rtCtrl.ServeServiceWorkerHTTP(rw, req)
					close(done)
				}()

				// Wait for the blocked provider to start.
				select {
				case <-started:
				case <-reqCtx.Done():
					t.Fatalf("blocked provider did not start: %v", reqCtx.Err())
				}

				// Swap in the replacement provider and wait for the request.
				rotating.SetCurrent(unixfs_access.NewAccessUnixFSFunc(replacement))

				// Wait for the request to complete against the replacement provider.
				select {
				case <-done:
				case <-reqCtx.Done():
					t.Fatalf("request did not complete after replacement provider: %v", reqCtx.Err())
				}

				// Check that the response served the expected body.
				assertHTTPAssetResponse(t, rw, body)
			}

			// Rebind twice: first generation, then the replacement.
			firstBody := moduleBody1
			if tt.path == "/v/b/fe/app/App-next.css" {
				firstBody = styleBody1
			}
			runPendingFetch(firstBody, rootRef1)
			runPendingFetch(tt.body, rootRef2)

			// Serve the final generation directly.
			rw := httptest.NewRecorder()
			req := httptest.NewRequest("GET", bldr_plugin.PluginAssetHTTPPath(pluginID, tt.path), nil)
			rtCtrl.ServeServiceWorkerHTTP(rw, req)
			assertHTTPAssetResponse(t, rw, tt.body)
		})
	}
}

func newTestPluginAssetsRoot(ctx context.Context, files map[string][]byte) (*unixfs.FSHandle, error) {
	// Open an in-memory FS handle for the plugin assets.
	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		return nil, err
	}

	// Write each file into the billy filesystem.
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
	// Assert the asset response serves the expected body and cache headers.
	t.Helper()

	// Check the status, body, and cache headers of the asset response.
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
