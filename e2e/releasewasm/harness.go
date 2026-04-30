//go:build !js

package releasewasm

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/sirupsen/logrus"
)

const (
	releaseDistRelPath   = ".bldr-dist/build/js/spacewave-dist/dist"
	prerenderDistRelPath = "app/prerender/dist"
)

type browserReleaseDescriptor struct {
	SchemaVersion        int                       `json:"schemaVersion"`
	GenerationID         string                    `json:"generationId"`
	ShellAssets          browserReleaseShellAssets `json:"shellAssets"`
	PrerenderedRoutes    []string                  `json:"prerenderedRoutes"`
	RequiredStaticAssets []string                  `json:"requiredStaticAssets"`
}

type browserReleaseShellAssets struct {
	Entrypoint    string   `json:"entrypoint"`
	ServiceWorker string   `json:"serviceWorker"`
	SharedWorker  string   `json:"sharedWorker"`
	Wasm          string   `json:"wasm"`
	CSS           []string `json:"css"`
}

type harness struct {
	baseURL string
	server  *http.Server
	pw      *playwright.Playwright
	browser playwright.Browser
}

func boot(ctx context.Context, le *logrus.Entry) (_ *harness, retErr error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return nil, errors.Wrap(err, "find repo root")
	}

	if err := os.RemoveAll(filepath.Join(repoRoot, prerenderDistRelPath)); err != nil {
		return nil, errors.Wrap(err, "clean prerender dist")
	}

	le.Info("building release web bundle")
	if err := runBun(ctx, repoRoot, "run", "build:release:web"); err != nil {
		return nil, errors.Wrap(err, "build release web bundle")
	}

	distDir := filepath.Join(repoRoot, releaseDistRelPath)
	le.Info("building prerender hydrate bundle")
	if err := runBun(ctx, repoRoot, "run", "vite", "build", "--config", "app/prerender/vite.hydrate.config.ts"); err != nil {
		return nil, errors.Wrap(err, "build prerender hydrate bundle")
	}
	le.Info("building prerender ssr bundle")
	if err := runBun(ctx, repoRoot, "run", "vite", "build", "--config", "app/prerender/vite.ssr.config.ts"); err != nil {
		return nil, errors.Wrap(err, "build prerender ssr bundle")
	}
	le.Info("running prerender build")
	if err := runBun(ctx, repoRoot, "./app/prerender/ssr-dist/build.js", "--dist-dir", distDir); err != nil {
		return nil, errors.Wrap(err, "run prerender build")
	}

	if _, err := os.Stat(filepath.Join(distDir, "browser-release.json")); err != nil {
		return nil, errors.Wrap(err, "stat browser-release.json")
	}
	staticDir := filepath.Join(repoRoot, prerenderDistRelPath)
	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err != nil {
		return nil, errors.Wrap(err, "stat prerender index.html")
	}

	port, err := findFreePort()
	if err != nil {
		return nil, errors.Wrap(err, "find free port")
	}
	baseURL := "http://127.0.0.1:" + port
	h := &harness{baseURL: baseURL}
	defer func() {
		if retErr != nil {
			h.release(le)
		}
	}()

	h.server = &http.Server{
		Addr:              "127.0.0.1:" + port,
		Handler:           releaseHandler(distDir, staticDir),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		if err := h.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			le.WithError(err).Error("release wasm server exited")
		}
	}()
	if err := h.waitForReady(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for release server")
	}

	le.Info("installing playwright chromium driver")
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}); err != nil {
		return nil, errors.Wrap(err, "install playwright")
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, errors.Wrap(err, "start playwright")
	}
	h.pw = pw

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(true),
		Args: []string{
			"--allow-loopback-in-peer-connection",
			"--disable-features=WebRtcHideLocalIpsWithMdns",
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "launch chromium")
	}
	h.browser = browser

	return h, nil
}

func (h *harness) getBaseURL() string { return h.baseURL }

func (h *harness) newPage(t testing.TB) playwright.Page {
	t.Helper()

	ctx, err := h.browser.NewContext()
	if err != nil {
		t.Fatalf("new browser context: %v", err)
	}
	t.Cleanup(func() {
		if err := ctx.Close(); err != nil {
			t.Logf("close browser context: %v", err)
		}
	})

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	var errs []string
	page.On("console", func(msg playwright.ConsoleMessage) {
		switch msg.Type() {
		case "error":
			if !ignoreBrowserError(msg.Text()) {
				errs = append(errs, "console error: "+msg.Text())
			}
		case "warning":
			t.Logf("browser warning: %s", msg.Text())
		default:
			t.Logf("browser %s: %s", msg.Type(), msg.Text())
		}
	})
	page.On("pageerror", func(err error) {
		if !ignoreBrowserError(err.Error()) {
			errs = append(errs, "page error: "+err.Error())
		}
	})
	page.On("response", func(resp playwright.Response) {
		if resp.Status() < 400 {
			return
		}
		url := resp.URL()
		if strings.HasPrefix(url, h.baseURL) && !strings.HasSuffix(url, "/.vite/manifest.json") {
			errs = append(errs, "http "+resp.StatusText()+": "+resp.URL())
			return
		}
		t.Logf("browser http warning: %d %s", resp.Status(), url)
	})
	t.Cleanup(func() {
		if len(errs) != 0 {
			t.Fatalf("browser errors: %v", errs)
		}
	})

	return page
}

func ignoreBrowserError(msg string) bool {
	return strings.Contains(msg, "cache disabled") ||
		strings.Contains(msg, "detected ctrl+shift+r") ||
		strings.Contains(msg, "web document is closed")
}

func (h *harness) browserRelease(ctx context.Context) (*browserReleaseDescriptor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/browser-release.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("browser-release.json returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	desc := &browserReleaseDescriptor{
		SchemaVersion: int(v.GetInt("schemaVersion")),
		GenerationID:  string(v.GetStringBytes("generationId")),
		ShellAssets: browserReleaseShellAssets{
			Entrypoint:    string(v.GetStringBytes("shellAssets", "entrypoint")),
			ServiceWorker: string(v.GetStringBytes("shellAssets", "serviceWorker")),
			SharedWorker:  string(v.GetStringBytes("shellAssets", "sharedWorker")),
			Wasm:          string(v.GetStringBytes("shellAssets", "wasm")),
		},
	}
	for _, css := range v.GetArray("shellAssets", "css") {
		desc.ShellAssets.CSS = append(desc.ShellAssets.CSS, string(css.GetStringBytes()))
	}
	for _, route := range v.GetArray("prerenderedRoutes") {
		desc.PrerenderedRoutes = append(desc.PrerenderedRoutes, string(route.GetStringBytes()))
	}
	for _, asset := range v.GetArray("requiredStaticAssets") {
		desc.RequiredStaticAssets = append(desc.RequiredStaticAssets, string(asset.GetStringBytes()))
	}
	return desc, nil
}

func (h *harness) release(le *logrus.Entry) {
	if h.browser != nil {
		if err := h.browser.Close(); err != nil {
			le.WithError(err).Warn("close browser")
		}
	}
	if h.pw != nil {
		if err := h.pw.Stop(); err != nil {
			le.WithError(err).Warn("stop playwright")
		}
	}
	if h.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.server.Shutdown(ctx); err != nil {
			le.WithError(err).Warn("shutdown release server")
		}
	}
}

func releaseHandler(distDir, staticDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(distDir))
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		if strings.HasSuffix(req.URL.Path, ".wasm.gz") {
			rw.Header().Set("Content-Encoding", "gzip")
			rw.Header().Set("Content-Type", "application/wasm")
		}
		if after, ok := strings.CutPrefix(req.URL.Path, "/static/"); ok {
			http.ServeFile(rw, req, filepath.Join(staticDir, after))
			return
		}
		if staticPath, ok := resolveStaticHTML(staticDir, req.URL.Path); ok {
			http.ServeFile(rw, req, staticPath)
			return
		}
		fileServer.ServeHTTP(rw, req)
	})
}

func resolveStaticHTML(staticDir, reqPath string) (string, bool) {
	clean := strings.Trim(strings.Split(reqPath, "?")[0], "/")
	if clean == "" {
		clean = "index"
	}
	if strings.Contains(clean, "..") {
		return "", false
	}
	path := filepath.Join(staticDir, clean+".html")
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

func runBun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (h *harness) waitForReady(ctx context.Context) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/browser-release.json", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func findFreePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}
