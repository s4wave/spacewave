package web_runtime_controller

import (
	"context"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/sirupsen/logrus"
)

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
	rotating := newRotatingAccess(newBlockedAccess())
	accessCtrl := unixfs_access.NewController(
		tb.Logger,
		tb.Bus,
		controller.NewInfo("bldr/web/runtime/test-assets", semver.MustParse("0.0.1"), "test plugin assets access"),
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

				started := rotating.ResetBlocked()
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

type rotatingAccess struct {
	mtx     sync.Mutex
	current unixfs_access.AccessUnixFSFunc
	waiters []func()
	started chan struct{}
}

func newRotatingAccess(current unixfs_access.AccessUnixFSFunc) *rotatingAccess {
	return &rotatingAccess{
		current: current,
		started: make(chan struct{}),
	}
}

func (r *rotatingAccess) AccessUnixFS(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
	r.mtx.Lock()
	current := r.current
	started := r.started
	r.waiters = append(r.waiters, released)
	r.mtx.Unlock()

	select {
	case <-started:
	default:
		close(started)
	}

	return current(ctx, released)
}

func (r *rotatingAccess) ResetBlocked() <-chan struct{} {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.current = newBlockedAccess()
	r.started = make(chan struct{})
	r.waiters = nil
	return r.started
}

func (r *rotatingAccess) SetCurrent(current unixfs_access.AccessUnixFSFunc) {
	r.mtx.Lock()
	waiters := append([]func(){}, r.waiters...)
	r.current = current
	r.waiters = nil
	r.mtx.Unlock()

	for _, waiter := range waiters {
		if waiter != nil {
			waiter()
		}
	}
}

func newBlockedAccess() unixfs_access.AccessUnixFSFunc {
	return func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
}
