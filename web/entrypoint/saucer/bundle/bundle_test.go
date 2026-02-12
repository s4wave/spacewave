//go:build !js

package entrypoint_saucer_bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	web_entrypoint_index "github.com/aperturerobotics/bldr/web/entrypoint/index"
	"github.com/sirupsen/logrus"
)

func TestSaucerDefine(t *testing.T) {
	t.Run("dev mode", func(t *testing.T) {
		defines := SaucerDefine(true)
		if defines["BLDR_SAUCER"] != "true" {
			t.Errorf("expected BLDR_SAUCER to be 'true', got %q", defines["BLDR_SAUCER"])
		}
		if defines["BLDR_DEBUG"] != "true" {
			t.Errorf("expected BLDR_DEBUG to be 'true', got %q", defines["BLDR_DEBUG"])
		}
	})

	t.Run("release mode", func(t *testing.T) {
		defines := SaucerDefine(false)
		if defines["BLDR_SAUCER"] != "true" {
			t.Errorf("expected BLDR_SAUCER to be 'true', got %q", defines["BLDR_SAUCER"])
		}
		if defines["BLDR_DEBUG"] != "false" {
			t.Errorf("expected BLDR_DEBUG to be 'false', got %q", defines["BLDR_DEBUG"])
		}
	})
}

func TestGetSaucerBinName(t *testing.T) {
	t.Run("nil platform", func(t *testing.T) {
		name := GetSaucerBinName(nil)
		if name != "bldr-saucer" {
			t.Errorf("expected 'bldr-saucer', got %q", name)
		}
	})

	t.Run("darwin platform", func(t *testing.T) {
		plat, err := bldr_platform.ParseNativePlatform("native/darwin/arm64")
		if err != nil {
			t.Fatal(err)
		}
		name := GetSaucerBinName(plat)
		if name != "bldr-saucer" {
			t.Errorf("expected 'bldr-saucer', got %q", name)
		}
	})

	t.Run("linux platform", func(t *testing.T) {
		plat, err := bldr_platform.ParseNativePlatform("native/linux/amd64")
		if err != nil {
			t.Fatal(err)
		}
		name := GetSaucerBinName(plat)
		if name != "bldr-saucer" {
			t.Errorf("expected 'bldr-saucer', got %q", name)
		}
	})

	t.Run("windows platform", func(t *testing.T) {
		plat, err := bldr_platform.ParseNativePlatform("native/windows/amd64")
		if err != nil {
			t.Fatal(err)
		}
		name := GetSaucerBinName(plat)
		if name != "bldr-saucer.exe" {
			t.Errorf("expected 'bldr-saucer.exe', got %q", name)
		}
	})
}

func TestGenerateBootstrapHtml(t *testing.T) {
	importMap := web_entrypoint_index.ImportMap{
		Imports: map[string]string{
			"react": "/b/pkg/react/index.mjs",
		},
	}
	html := generateBootstrapHtml(importMap)

	if !strings.Contains(html, "<!doctype html>") {
		t.Error("expected HTML doctype")
	}
	if !strings.Contains(html, `<div id="bldr-root"></div>`) {
		t.Error("expected bldr-root div")
	}
	if !strings.Contains(html, `<script type="importmap">`) {
		t.Error("expected importmap script tag")
	}
	if !strings.Contains(html, `<script type="module" src="/entrypoint.mjs">`) {
		t.Error("expected module script tag")
	}
	if !strings.Contains(html, "/b/pkg/react/index.mjs") {
		t.Error("expected import map content to be included")
	}
}

func TestBuildSaucerJSBundle(t *testing.T) {
	// This test requires the full bldr source tree to be available
	// Skip if we can't find the source directory
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Navigate up to find the bldr root (from web/entrypoint/saucer/bundle/)
	bldrRoot := filepath.Join(testDir, "../../../..")

	// Check if the entrypoint exists
	entrypointPath := filepath.Join(bldrRoot, "web/entrypoint/entrypoint.tsx")
	if _, err := os.Stat(entrypointPath); os.IsNotExist(err) {
		t.Skipf("skipping: entrypoint not found at %s", entrypointPath)
	}

	// Create a temporary build directory
	buildDir := filepath.Join(testDir, ".test-build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build the JS bundle (minified for faster build)
	bundle, err := BuildSaucerJSBundle(le, bldrRoot, buildDir, true)
	if err != nil {
		t.Fatalf("BuildSaucerJSBundle failed: %v", err)
	}

	// Verify the bundle
	if bundle.BootstrapHTML == "" {
		t.Error("expected non-empty BootstrapHTML")
	}

	if !strings.Contains(bundle.BootstrapHTML, "<!doctype html>") {
		t.Error("expected HTML doctype in bootstrap")
	}

	if !strings.Contains(bundle.BootstrapHTML, `<div id="bldr-root"></div>`) {
		t.Error("expected bldr-root div in bootstrap")
	}

	// EntrypointJS should contain module code
	if bundle.EntrypointJS == "" {
		t.Error("expected non-empty EntrypointJS")
	}

	// Check for import map in HTML
	if !strings.Contains(bundle.BootstrapHTML, `<script type="importmap">`) {
		t.Error("expected importmap script tag in bootstrap")
	}

	t.Logf("Successfully built saucer JS bundle, HTML size: %d bytes, JS size: %d bytes",
		len(bundle.BootstrapHTML), len(bundle.EntrypointJS))
}

func TestGetSaucerPlatformPkgName(t *testing.T) {
	tests := []struct {
		platform string
		expected string
	}{
		{"native/darwin/arm64", "bldr-saucer-darwin-arm64"},
		{"native/darwin/amd64", "bldr-saucer-darwin-x64"},
		{"native/linux/amd64", "bldr-saucer-linux-x64"},
		{"native/linux/arm64", "bldr-saucer-linux-arm64"},
		{"native/windows/amd64", "bldr-saucer-win32-x64"},
	}
	for _, tc := range tests {
		t.Run(tc.platform, func(t *testing.T) {
			plat, err := bldr_platform.ParseNativePlatform(tc.platform)
			if err != nil {
				t.Fatal(err)
			}
			name := getSaucerPlatformPkgName(plat)
			if name != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, name)
			}
		})
	}
}

func TestFindSaucerBinary(t *testing.T) {
	// Create a fake npm directory structure.
	tmpDir := t.TempDir()
	plat, err := bldr_platform.ParseNativePlatform("native/darwin/arm64")
	if err != nil {
		t.Fatal(err)
	}

	// Create the platform-specific package binary.
	binDir := filepath.Join(tmpDir, "node_modules", "@aptre", "bldr-saucer-darwin-arm64", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(binDir, "bldr-saucer")
	if err := os.WriteFile(binPath, []byte("fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Should find the platform-specific binary.
	found := findSaucerBinary(tmpDir, plat)
	if found != binPath {
		t.Errorf("expected %q, got %q", binPath, found)
	}

	// Remove platform binary, create source build fallback.
	os.RemoveAll(filepath.Join(tmpDir, "node_modules", "@aptre", "bldr-saucer-darwin-arm64"))
	fallbackDir := filepath.Join(tmpDir, "node_modules", "@aptre", "bldr-saucer", "build")
	if err := os.MkdirAll(fallbackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fallbackPath := filepath.Join(fallbackDir, "bldr-saucer")
	if err := os.WriteFile(fallbackPath, []byte("fallback-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	found = findSaucerBinary(tmpDir, plat)
	if found != fallbackPath {
		t.Errorf("expected fallback %q, got %q", fallbackPath, found)
	}

	// Remove everything, should return empty.
	os.RemoveAll(filepath.Join(tmpDir, "node_modules"))
	found = findSaucerBinary(tmpDir, plat)
	if found != "" {
		t.Errorf("expected empty string, got %q", found)
	}
}
