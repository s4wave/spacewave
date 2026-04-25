//go:build !js

package wasm

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestSession holds an isolated browser context and resource connections for
// a single test. Each test creates its own session via Harness.NewSession so
// browser state (localStorage, cookies, WASM process, workers) is isolated.
type TestSession struct {
	h          *Harness
	browserCtx playwright.BrowserContext
	page       playwright.Page
	workersMu  sync.Mutex
	workers    []playwright.Worker

	browserClient srpc.Client
	resClient     *resource_client.Client
	root          *s4wave_root.Root
	browserPeer   peer.ID
}

// NewSession creates an isolated browser session for a single test. A fresh
// BrowserContext with clean storage is created, the app is loaded, and
// resources are connected through the devtool bus. The session is released
// automatically when the test finishes via t.Cleanup.
func (h *Harness) NewSession(t testing.TB) *TestSession {
	t.Helper()

	s := h.NewBlankSession(t)
	if err := h.loadAppPageURL(s, h.baseURL+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}

	ctx, cancel := context.WithCancel(h.ctx)
	t.Cleanup(cancel)
	if err := s.ConnectResources(ctx); err != nil {
		t.Fatalf("connect resources: %v", err)
	}

	return s
}

// NewBlankSession creates an isolated browser session with a fresh browser
// context and page, but does not load the app or connect SDK resources.
func (h *Harness) NewBlankSession(t testing.TB) *TestSession {
	t.Helper()

	s := &TestSession{h: h}
	t.Cleanup(s.release)

	page, err := h.newBrowserContext(s)
	if err != nil {
		t.Fatalf("new browser context: %v", err)
	}
	s.page = page
	h.registerPageSession(page, s)
	return s
}

// NewPageSession creates an isolated browser session with only a browser
// context and page, without connecting SDK resources. Use this for tests
// that only need browser interaction (e.g. heap profiling, screenshot tests).
func (h *Harness) NewPageSession(t testing.TB) *TestSession {
	t.Helper()

	s := h.NewBlankSession(t)
	if err := s.LoadApp(); err != nil {
		t.Fatalf("load app: %v", err)
	}

	return s
}

// Page returns the Playwright Page for this session.
func (s *TestSession) Page() playwright.Page { return s.page }

// BrowserContext returns the Playwright BrowserContext for this session.
func (s *TestSession) BrowserContext() playwright.BrowserContext { return s.browserCtx }

// LoadApp loads the app base URL into the session page.
func (s *TestSession) LoadApp() error {
	return s.h.loadAppPage(s)
}

// ConnectResources connects the session Resource SDK client through the
// devtool/browser RPC link.
func (s *TestSession) ConnectResources(ctx context.Context) error {
	return s.h.connectSessionResources(ctx, s)
}

// addWorker tracks a worker spawned by the page.
func (s *TestSession) addWorker(w playwright.Worker) {
	s.workersMu.Lock()
	defer s.workersMu.Unlock()
	s.workers = append(s.workers, w)
}

// removeWorker removes a tracked worker after close.
func (s *TestSession) removeWorker(w playwright.Worker) {
	s.workersMu.Lock()
	defer s.workersMu.Unlock()
	s.workers = slices.DeleteFunc(s.workers, func(ew playwright.Worker) bool {
		return ew == w
	})
}

// Workers returns a snapshot of the tracked page workers.
func (s *TestSession) Workers() []playwright.Worker {
	s.workersMu.Lock()
	defer s.workersMu.Unlock()
	return append([]playwright.Worker(nil), s.workers...)
}

// BrowserClient returns the SRPC client connected to the browser peer, or
// nil if resources are not connected.
func (s *TestSession) BrowserClient() srpc.Client { return s.browserClient }

// ResourceClient returns the Resource SDK client, or nil if not connected.
func (s *TestSession) ResourceClient() *resource_client.Client { return s.resClient }

// Root returns the Root resource wrapper, or nil if not connected.
func (s *TestSession) Root() *s4wave_root.Root { return s.root }

// Release tears down the session's browser context and resource connections.
func (s *TestSession) Release() {
	s.release()
}

// MountSessionByIdx mounts a session by its 1-based index and returns the
// Session SDK wrapper. The caller must call Release on the returned Session.
func (s *TestSession) MountSessionByIdx(ctx context.Context, idx uint32) (*s4wave_session.Session, error) {
	if s.root == nil {
		return nil, errors.New("resources not connected")
	}
	resp, err := s.root.MountSessionByIdx(ctx, idx)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		return nil, errors.Errorf("no session at index %d", idx)
	}

	sessRef := s.resClient.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(s.resClient, sessRef)
	if err != nil {
		sessRef.Release()
		return nil, errors.Wrap(err, "session resource")
	}
	return sess, nil
}

// release tears down the session's browser context and resource connections.
func (s *TestSession) release() {
	s.h.releaseBrowserPeerLease(s, s.browserPeer)
	s.browserPeer = ""
	if s.root != nil {
		s.root.Release()
		s.root = nil
	}
	if s.resClient != nil {
		s.resClient.Release()
		s.resClient = nil
	}
	if s.browserCtx != nil {
		if s.page != nil {
			s.h.unregisterPageSession(s.page)
		}
		s.browserCtx.Close()
		s.browserCtx = nil
		s.page = nil
	}
}
