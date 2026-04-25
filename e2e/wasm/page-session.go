//go:build !js

package wasm

import playwright "github.com/playwright-community/playwright-go"

// LookupSessionByPage returns the test session associated with a page.
func (h *Harness) LookupSessionByPage(page playwright.Page) *TestSession {
	h.pageSessionMu.Lock()
	defer h.pageSessionMu.Unlock()
	return h.pageSessions[page]
}

func (h *Harness) registerPageSession(page playwright.Page, s *TestSession) {
	h.pageSessionMu.Lock()
	defer h.pageSessionMu.Unlock()

	if h.pageSessions == nil {
		h.pageSessions = make(map[playwright.Page]*TestSession)
	}
	h.pageSessions[page] = s
}

func (h *Harness) unregisterPageSession(page playwright.Page) {
	h.pageSessionMu.Lock()
	defer h.pageSessionMu.Unlock()

	delete(h.pageSessions, page)
}
