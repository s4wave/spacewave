package unixfs_rpc_server

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
)

func TestResolveFSCursorProxyTracksEveryClient(t *testing.T) {
	ctx := t.Context()
	proxy := newTestFSCursor(nil)
	root := newTestFSCursor(proxy)
	service := NewFSCursorService(root)
	service.clients[1] = &fsCursorClient{}
	service.clients[2] = &fsCursorClient{}

	_, proxyHandleID, err := service.resolveFSCursorProxy(ctx, 1, 1)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, secondHandleID, err := service.resolveFSCursorProxy(ctx, 1, 2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if secondHandleID != proxyHandleID {
		t.Fatalf("proxy handle changed: first=%d second=%d", proxyHandleID, secondHandleID)
	}

	service.mtx.Lock()
	registered := service.handleIDToCursor[proxyHandleID]
	if registered == nil {
		service.mtx.Unlock()
		t.Fatal("proxy cursor was not registered")
	}
	if len(registered.clients) != 2 || registered.clients[0] != 1 || registered.clients[1] != 2 {
		service.mtx.Unlock()
		t.Fatalf("unexpected proxy clients: %v", registered.clients)
	}
	if clients := service.handleIDToCursor[1].clients; len(clients) != 0 {
		service.mtx.Unlock()
		t.Fatalf("proxy clients were registered on the parent: %v", clients)
	}
	for _, clientID := range []uint64{1, 2} {
		if cursors := service.clients[clientID].cursors; !slices.Equal(cursors, []uint64{proxyHandleID}) {
			service.mtx.Unlock()
			t.Fatalf("unexpected client %d cursors: %v", clientID, cursors)
		}
	}
	releaseClient := func(clientID uint64) {
		for _, cursorHandleID := range service.clients[clientID].cursors {
			if err := service.removeFSCursorRefLocked(cursorHandleID, clientID, false); err != nil {
				service.mtx.Unlock()
				t.Fatal(err.Error())
			}
		}
	}
	releaseClient(1)
	if service.handleIDToCursor[proxyHandleID] == nil {
		service.mtx.Unlock()
		t.Fatal("first client release removed the shared proxy cursor")
	}
	releaseClient(2)
	if service.handleIDToCursor[proxyHandleID] != nil {
		service.mtx.Unlock()
		t.Fatal("last client release retained the proxy cursor")
	}
	service.mtx.Unlock()

	select {
	case <-proxy.releasedCh:
	case <-time.After(time.Second):
		t.Fatal("last client release did not release the proxy cursor")
	}
}

type testFSCursor struct {
	proxy       unixfs.FSCursor
	released    atomic.Bool
	releaseOnce sync.Once
	releasedCh  chan struct{}
}

func newTestFSCursor(proxy unixfs.FSCursor) *testFSCursor {
	return &testFSCursor{proxy: proxy, releasedCh: make(chan struct{})}
}

func (c *testFSCursor) CheckReleased() bool {
	return c.released.Load()
}

func (c *testFSCursor) GetProxyCursor(context.Context) (unixfs.FSCursor, error) {
	return c.proxy, nil
}

func (c *testFSCursor) AddChangeCb(unixfs.FSCursorChangeCb) {}

func (c *testFSCursor) GetCursorOps(context.Context) (unixfs.FSCursorOps, error) {
	return nil, nil
}

func (c *testFSCursor) Release() {
	c.released.Store(true)
	c.releaseOnce.Do(func() {
		close(c.releasedCh)
	})
}
