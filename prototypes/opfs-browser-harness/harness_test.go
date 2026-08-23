//go:build !js

package opfsbrowserharness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	"github.com/sirupsen/logrus"
)

func TestOpfsBrowserHarness(t *testing.T) {
	root := repositoryRoot(t)
	distDir := filepath.Join(root, "prototypes", "opfs-browser-harness", ".tmp", "dist")
	if err := os.RemoveAll(distDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pageHTML, err := os.ReadFile(filepath.Join(root, "prototypes", "opfs-browser-harness", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), pageHTML, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := buildBrowserBundle(t, root, distDir); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(distDir)))
	defer server.Close()

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: new(true)})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	browserContext, err := browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer browserContext.Close()
	page, err := browserContext.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	page.On("console", func(msg playwright.ConsoleMessage) {
		t.Logf("browser console.%s: %s", msg.Type(), msg.Text())
	})
	page.On("pageerror", func(err error) {
		t.Logf("browser pageerror: %s", err)
	})
	if _, err := page.Goto(server.URL+"/index.html", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateLoad,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction("() => window.__opfsResult !== undefined", &playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(60_000),
	}); err != nil {
		t.Fatal(err)
	}
	value, err := page.Evaluate("() => window.__opfsResult", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected browser result %#v", value)
	}
	if pass, _ := result["pass"].(bool); !pass {
		t.Fatalf("OPFS browser harness failed: %v", result["detail"])
	}
	t.Logf("OPFS browser harness: %v", result["detail"])
}

func buildBrowserBundle(t *testing.T, root, distDir string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	artifactRoot := filepath.Join(root, "prototypes", "opfs-browser-harness", ".tmp", "goscript")
	if err := os.RemoveAll(artifactRoot); err != nil {
		return err
	}
	if err := gocompiler.ExecGoScriptCompile(ctx, le, gocompiler.GoScriptCompileOptions{
		WorkDir:         root,
		OutputPath:      artifactRoot,
		Packages:        []string{"github.com/s4wave/spacewave/prototypes/opfs-browser-harness"},
		BuildFlags:      []string{"-tags=goscript,purego"},
		AllDependencies: true,
	}); err != nil {
		return fmt.Errorf("goscript compile: %w", err)
	}
	entryPath := filepath.Join(root, "prototypes", "opfs-browser-harness", "browser.ts")
	_, err := bldr_web_bundler_rolldown.Build(ctx, le,
		filepath.Join(root, "prototypes", "opfs-browser-harness", ".tmp", "bun"),
		root,
		&bldr_web_bundler_rolldown.BuildRequest{
			WorkingDir:     filepath.Dir(entryPath),
			SourceRoot:     root,
			OutputRoot:     distDir,
			BldrDistRoot:   root,
			Format:         "es",
			Platform:       "browser",
			Target:         "es2024",
			EntryFileNames: "browser.js",
			ChunkFileNames: "chunks/[name]-[hash].mjs",
			AssetFileNames: "assets/[name]-[hash][extname]",
			Sourcemap:      "none",
			TreeShaking:    true,
			Entrypoints: []*bldr_web_bundler_rolldown.Entrypoint{{
				Name:      "browser",
				InputPath: entryPath,
			}},
			Goscript: &bldr_web_bundler_rolldown.GoScriptPolicy{OutputRoot: artifactRoot},
		})
	if err != nil {
		return fmt.Errorf("rolldown bundle: %w", err)
	}
	return nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
