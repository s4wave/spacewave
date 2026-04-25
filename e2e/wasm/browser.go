//go:build !js

package wasm

import (
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/sirupsen/logrus"
)

// LaunchBrowser starts a Playwright-managed Chromium instance. The browser
// process is shared across all test sessions; each test creates its own
// BrowserContext via Harness.NewSession.
//
// Call this after Boot returns. Headless mode is the default; pass
// WithHeadless(false) at Boot time to see the browser.
func (h *Harness) LaunchBrowser() error {
	pw, err := playwright.Run()
	if err != nil {
		return errors.Wrap(err, "start playwright")
	}
	h.pw = pw

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(h.headless),
		Args: []string{
			"--allow-loopback-in-peer-connection",
			"--disable-features=WebRtcHideLocalIpsWithMdns",
		},
	})
	if err != nil {
		pw.Stop()
		h.pw = nil
		return errors.Wrap(err, "launch chromium")
	}
	h.browser = browser

	return nil
}

// Browser returns the raw Playwright Browser handle, or nil if not launched.
func (h *Harness) Browser() playwright.Browser { return h.browser }

// newBrowserContext creates a fresh BrowserContext on the shared browser and
// wires up console/error forwarding. The context and page are stored on the
// provided TestSession.
func (h *Harness) newBrowserContext(s *TestSession) (playwright.Page, error) {
	if h.browser == nil {
		return nil, errors.New("browser not launched")
	}

	ctx, err := h.browser.NewContext()
	if err != nil {
		return nil, errors.Wrap(err, "new browser context")
	}
	s.browserCtx = ctx

	page, err := ctx.NewPage()
	if err != nil {
		ctx.Close()
		s.browserCtx = nil
		return nil, errors.Wrap(err, "new page")
	}

	// Forward browser console output to the test log.
	page.On("console", func(msg playwright.ConsoleMessage) {
		logrus.WithFields(logrus.Fields{
			"type":    msg.Type(),
			"browser": true,
		}).Info(msg.Text())
	})
	// Forward worker console output (SharedWorkers, dedicated workers).
	page.OnWorker(func(w playwright.Worker) {
		url := w.URL()
		s.addWorker(w)
		logrus.WithField("worker", url).Debug("worker spawned")
		w.OnConsole(func(msg playwright.ConsoleMessage) {
			le := logrus.WithFields(logrus.Fields{
				"type":   msg.Type(),
				"worker": url,
			})
			switch msg.Type() {
			case "error":
				le.Error(msg.Text())
			case "warning":
				le.Warn(msg.Text())
			default:
				le.Info(msg.Text())
			}
		})
		w.OnClose(func(_ playwright.Worker) {
			s.removeWorker(w)
			logrus.WithField("worker", url).Debug("worker closed")
		})
	})

	page.On("pageerror", func(err error) {
		logrus.WithField("browser", true).Error("page error: " + err.Error())
	})
	page.On("response", func(resp playwright.Response) {
		if resp.Status() >= 400 {
			logrus.WithFields(logrus.Fields{
				"url":     resp.URL(),
				"status":  resp.Status(),
				"browser": true,
			}).Warn("HTTP error response")
		}
	})

	return page, nil
}

// loadAppPage loads the app base URL into the session page.
func (h *Harness) loadAppPage(s *TestSession) error {
	return h.loadAppPageURL(s, h.baseURL)
}

func (h *Harness) loadAppPageURL(s *TestSession, targetURL string) error {
	if s.page == nil {
		return errors.New("session page not initialized")
	}

	waitUntil := playwright.WaitUntilStateDomcontentloaded
	timeout := float64(120000)
	resp, err := s.page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: waitUntil,
		Timeout:   &timeout,
	})
	if err != nil {
		return errors.Wrap(err, "load app")
	}
	if resp != nil && resp.Status() >= 400 {
		return errors.Errorf("app returned HTTP %d", resp.Status())
	}
	return nil
}

// closeBrowser tears down the shared Playwright browser process.
func (h *Harness) closeBrowser() {
	if h.browser != nil {
		h.browser.Close()
		h.browser = nil
	}
	if h.pw != nil {
		h.pw.Stop()
		h.pw = nil
	}
}
