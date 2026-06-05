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

// TestSession holds one browser page and optional resource connections for a
// single test. Clean-session helpers allocate a fresh BrowserContext; retained
// state helpers reuse the harness-owned warm BrowserContext.
type TestSession struct {
	h              *Harness
	browserCtx     playwright.BrowserContext
	ownsBrowserCtx bool
	retainedState  bool
	page           playwright.Page
	workersMu      sync.Mutex
	workers        []playwright.Worker
	consoleMu      sync.Mutex
	console        map[chan string]struct{}
	timingMu       sync.Mutex

	browserClient  srpc.Client
	resClient      *resource_client.Client
	root           *s4wave_root.Root
	browserPeer    peer.ID
	peerAfterSeq   uint64
	resourceTiming ResourceConnectionTiming
}

// NewSession creates an isolated browser session for a single test.
//
// Deprecated: use NewCleanSession so call sites communicate that they require
// clean browser storage and a dedicated WASM process.
func (h *Harness) NewSession(t testing.TB) *TestSession {
	t.Helper()
	return h.NewCleanSession(t)
}

// NewCleanSession creates a Resource SDK session with a fresh BrowserContext,
// clean browser storage, a dedicated WASM process, and SDK resource
// connections through the devtool bus. Use this when the test requires strict
// browser-state isolation.
func (h *Harness) NewCleanSession(t testing.TB) *TestSession {
	t.Helper()

	s := h.NewCleanBlankSession(t)
	if err := h.loadAppPageURL(s, h.baseURL+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}
	WaitForApp(t, s.page)

	ctx, cancel := context.WithCancel(h.ctx)
	t.Cleanup(cancel)
	if err := s.ConnectResources(ctx); err != nil {
		t.Fatalf("connect resources: %v", err)
	}

	return s
}

// NewBlankSession creates an isolated blank browser session.
//
// Deprecated: use NewCleanBlankSession so call sites communicate that they
// require clean browser storage and a dedicated WASM process.
func (h *Harness) NewBlankSession(t testing.TB) *TestSession {
	t.Helper()
	return h.NewCleanBlankSession(t)
}

// NewCleanBlankSession creates a fresh BrowserContext and page, but does not
// load the app or connect SDK resources.
func (h *Harness) NewCleanBlankSession(t testing.TB) *TestSession {
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

// NewPageSession creates an isolated browser-only session.
//
// Deprecated: use NewCleanPageSession so call sites communicate that they
// require clean browser storage and a dedicated WASM process.
func (h *Harness) NewPageSession(t testing.TB) *TestSession {
	t.Helper()
	return h.NewCleanPageSession(t)
}

// NewCleanPageSession creates a fresh BrowserContext, loads the app, and leaves
// SDK resources disconnected. Use this for browser-only tests that still
// require clean storage and a dedicated WASM process.
func (h *Harness) NewCleanPageSession(t testing.TB) *TestSession {
	t.Helper()

	s := h.NewCleanBlankSession(t)
	if err := h.loadAppPageURL(s, h.baseURL+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}
	WaitForApp(t, s.page)

	return s
}

// NewRetainedStateBlankSession creates a blank page on the package-level warm
// BrowserContext. Each call creates a fresh Page and registers cleanup for
// page-owned handles only; the retained context is released by Harness.Release.
// Use this only when the test can run against retained browser storage and the
// existing WASM process.
func (h *Harness) NewRetainedStateBlankSession(t testing.TB) *TestSession {
	t.Helper()

	s := &TestSession{h: h, retainedState: true}
	t.Cleanup(s.release)

	page, err := h.newRetainedStateBrowserPage(s)
	if err != nil {
		t.Fatalf("new retained-state page: %v", err)
	}
	s.page = page
	h.registerPageSession(page, s)
	return s
}

// NewRetainedStatePageSession creates a page-only session on the package-level
// warm BrowserContext and loads the app. Use this only when the test can run
// against retained browser storage and the existing WASM process.
func (h *Harness) NewRetainedStatePageSession(t testing.TB) *TestSession {
	t.Helper()

	s := h.NewRetainedStateBlankSession(t)

	if err := h.loadAppPageURL(s, h.baseURL+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}
	WaitForApp(t, s.page)

	return s
}

// NewSharedPageSession creates a page-only session on the package-level warm
// BrowserContext.
//
// Deprecated: use NewRetainedStatePageSession so call sites explicitly
// acknowledge retained browser state.
func (h *Harness) NewSharedPageSession(t testing.TB) *TestSession {
	t.Helper()
	return h.NewRetainedStatePageSession(t)
}

// NewRetainedStateSession creates a Resource SDK session on the warm retained
// BrowserContext. This is an opt-in startup optimization helper; clean-session
// helpers keep strict BrowserContext, storage, and WASM process isolation.
func (h *Harness) NewRetainedStateSession(t testing.TB) *TestSession {
	t.Helper()

	s := h.NewRetainedStatePageSession(t)

	ctx, cancel := context.WithCancel(h.ctx)
	t.Cleanup(cancel)
	if err := s.ConnectResources(ctx); err != nil {
		t.Fatalf("connect retained-state resources: %v", err)
	}

	return s
}

// NewSharedSession creates a Resource SDK session on the package-level warm
// BrowserContext.
//
// Deprecated: use NewRetainedStateSession so call sites explicitly acknowledge
// retained browser state.
func (h *Harness) NewSharedSession(t testing.TB) *TestSession {
	t.Helper()
	return h.NewRetainedStateSession(t)
}

// Page returns the Playwright Page for this session.
func (s *TestSession) Page() playwright.Page { return s.page }

// BrowserContext returns the Playwright BrowserContext for this session.
func (s *TestSession) BrowserContext() playwright.BrowserContext { return s.browserCtx }

// ReplacePageInCurrentContext closes the current page and opens a fresh page in
// the same BrowserContext. Clean sessions keep their isolated context; retained
// sessions keep the retained warm context.
func (s *TestSession) ReplacePageInCurrentContext() error {
	if s.browserCtx == nil {
		return errors.New("browser context not initialized")
	}
	s.disconnectResources()
	if s.page != nil {
		s.h.unregisterPageSession(s.page)
		if err := s.page.Close(); err != nil {
			return errors.Wrap(err, "close page")
		}
		s.page = nil
		s.clearWorkers()
	}

	page, err := s.h.newBrowserPage(s)
	if err != nil {
		return err
	}
	s.page = page
	s.h.registerPageSession(page, s)
	return nil
}

// ReplacePageInRetainedContext closes the current page and opens a fresh page
// in the same BrowserContext.
//
// Deprecated: use ReplacePageInCurrentContext so clean-session call sites do
// not imply that they opted into shared retained state.
func (s *TestSession) ReplacePageInRetainedContext() error {
	return s.ReplacePageInCurrentContext()
}

// WatchConsole returns browser and worker console messages emitted after it is
// called.
func (s *TestSession) WatchConsole() (<-chan string, func()) {
	ch := make(chan string, 64)

	s.consoleMu.Lock()
	if s.console == nil {
		s.console = make(map[chan string]struct{})
	}
	s.console[ch] = struct{}{}
	s.consoleMu.Unlock()

	stop := func() {
		s.consoleMu.Lock()
		if _, ok := s.console[ch]; ok {
			delete(s.console, ch)
			close(ch)
		}
		s.consoleMu.Unlock()
	}
	return ch, stop
}

func (s *TestSession) consoleWatcherCount() int {
	s.consoleMu.Lock()
	defer s.consoleMu.Unlock()
	return len(s.console)
}

// LoadApp loads the app route into the session page.
func (s *TestSession) LoadApp() error {
	return s.h.loadAppPageURL(s, s.h.baseURL+"/#/")
}

// ConnectResources connects the session Resource SDK client through the
// devtool/browser RPC link.
func (s *TestSession) ConnectResources(ctx context.Context) error {
	return s.h.connectSessionResources(ctx, s, s.peerAfterSeq)
}

// addWorker tracks a worker spawned by the page.
func (s *TestSession) addWorker(w playwright.Worker) {
	s.workersMu.Lock()
	defer s.workersMu.Unlock()
	s.workers = append(s.workers, w)
}

func (s *TestSession) emitConsole(text string) {
	s.consoleMu.Lock()
	defer s.consoleMu.Unlock()
	for ch := range s.console {
		select {
		case ch <- text:
		default:
		}
	}
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

func (s *TestSession) clearWorkers() {
	s.workersMu.Lock()
	defer s.workersMu.Unlock()
	s.workers = nil
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

func (s *TestSession) disconnectResources() {
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
	s.browserClient = nil
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
	s.consoleMu.Lock()
	for ch := range s.console {
		delete(s.console, ch)
		close(ch)
	}
	s.consoleMu.Unlock()

	s.disconnectResources()
	if s.page != nil {
		s.h.unregisterPageSession(s.page)
		if !s.ownsBrowserCtx {
			s.page.Close()
		}
		s.page = nil
		s.clearWorkers()
	}
	if s.browserCtx != nil && s.ownsBrowserCtx {
		s.browserCtx.Close()
	}
	s.browserCtx = nil
	s.ownsBrowserCtx = false
	s.retainedState = false
}
