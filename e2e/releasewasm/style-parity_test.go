//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseStyleParity captures the branch through its signed local CDN fixture.
func TestReleaseStyleParity(t *testing.T) {
	// Require explicit artifact output and the isolated branch distribution.
	out := os.Getenv("STYLE_PARITY_OUTPUT")
	if out == "" {
		t.Skip("set STYLE_PARITY_OUTPUT to capture the release interface")
	}
	if os.Getenv(localCDNEnv) != "1" {
		t.Fatal("style parity requires the local CDN fixture")
	}

	// Resolve the manifest and output relative to the source checkout.
	manifest := os.Getenv("STYLE_PARITY_MANIFEST")
	if manifest == "" {
		manifest = "scripts/style-parity/public.manifest.ts"
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(testHarness.repoRoot, out)
	}

	// Attribute the served fixture to the checkout that built its artifacts.
	gitValue := func(args ...string) string {
		// Read the checkout metadata used to build the served artifacts.
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = testHarness.repoRoot
		value, err := command.Output()
		if err != nil {
			t.Fatalf("read fixture source: %v", err)
		}
		return strings.TrimSpace(string(value))
	}
	revision := gitValue("rev-parse", "HEAD")
	status := gitValue("status", "--porcelain", "--untracked-files=normal")
	if status == "" {
		status = "clean"
	}

	// Keep repeated captures on one origin and one distribution generation.
	paths := []string{out}
	repeat := os.Getenv("STYLE_PARITY_REPEAT") == "1"
	if repeat {
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(out, 0o755); err != nil {
			t.Fatal(err)
		}
		paths = []string{filepath.Join(out, "before"), filepath.Join(out, "after")}
	}

	// Build the collector, capture each matrix, and compare repeated renders.
	commands := [][]string{{"bun", "run", "style-parity:build"}}
	for _, path := range paths {
		commands = append(commands, []string{
			"node", ".bldr/style-parity-tools/style-parity.mjs", "capture",
			"--base", testHarness.baseURL, "--manifest", manifest, "--out", path,
			"--revision", revision, "--source-status", status,
		})
	}
	if repeat {
		commands = append(commands, []string{
			"node", ".bldr/style-parity-tools/style-parity.mjs", "compare",
			"--before", paths[0], "--after", paths[1],
			"--out", filepath.Join(out, "differences.jsonl"),
		})
	}
	for _, args := range commands {
		// Stream child output and keep browser traffic on the fixture's network.
		command := exec.CommandContext(t.Context(), args[0], args[1:]...)
		command.Dir = testHarness.repoRoot
		command.Env = append(os.Environ(), "STYLE_PARITY_PROXY="+testHarness.baseURL)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			t.Fatalf("style parity %s: %v", args[0], err)
		}
	}
}
