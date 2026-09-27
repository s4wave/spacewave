package entrypoint_browser_bundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRendererGeneratesStartupUtilities checks that the browser stylesheet
// includes utilities used by an injected startup component.
func TestRendererGeneratesStartupUtilities(t *testing.T) {
	// Build a renderer with a startup stylesheet that declares landing tokens.
	root := testBldrRoot(t)
	buildDir := filepath.Join(t.TempDir(), "build")
	result, err := BuildRenderer(
		context.Background(),
		testBuildLogger(),
		t.TempDir(),
		root,
		buildDir,
		ConfigFreeRendererOpts{
			OutputDir:  filepath.Join(buildDir, "entrypoint"),
			PublicPath: "/entrypoint/",
			Defines: map[string]string{
				"BLDR_IS_BROWSER": "true",
				"BLDR_DEBUG":      "false",
				"BLDR_STARTUP_JS": `"./browser/bundle/testdata/tailwind-startup.tsx"`,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Read every emitted stylesheet because Vite may split CSS assets.
	var css strings.Builder
	for _, cssPath := range result.CSSPaths {
		data, err := os.ReadFile(filepath.Join(buildDir, cssPath))
		if err != nil {
			t.Fatal(err)
		}
		css.Write(data)
	}

	// Require the utilities used by the startup markup in the emitted CSS.
	for _, utility := range []string{".bg-background-landing", ".rounded-landing-launcher"} {
		if !strings.Contains(css.String(), utility) {
			t.Fatalf("renderer CSS omits %s", utility)
		}
	}
}
