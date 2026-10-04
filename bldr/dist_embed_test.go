package bldr

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDistSDKEmbedPatternsCoverProductionSources(t *testing.T) {
	// Read dist.go, whose embed directives define production SDK coverage.
	distGo, err := os.ReadFile("dist.go")
	if err != nil {
		t.Fatal(err)
	}

	// Extract SDK embed patterns and their source directories.
	patterns, dirs := distSDKEmbedPatterns(t, string(distGo))
	if len(patterns) == 0 {
		t.Fatal("dist.go has no SDK go:embed patterns")
	}

	// Track SDK paths once while collecting missing sources and embedded tests.
	seen := make(map[string]bool)
	var missing, embeddedTests []string

	// Walk each covered SDK directory and classify its source files.
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.FromSlash(dir), func(filePath string, entry fs.DirEntry, walkErr error) error {
			// Propagate errors from the source-directory walk.
			if walkErr != nil {
				return walkErr
			}

			// Skip directories because embed coverage applies to files.
			if entry.IsDir() {
				return nil
			}

			// Normalize each filesystem path to the embed pattern form.
			slashPath := filepath.ToSlash(filePath)

			// Avoid classifying a source path covered by overlapping directories twice.
			if seen[slashPath] {
				return nil
			}
			seen[slashPath] = true

			// Record missing production sources and patterns that include tests.
			switch {
			case isProductionSDKSource(slashPath):
				if !distSDKEmbedPatternMatches(t, patterns, slashPath) {
					missing = append(missing, slashPath)
				}
			case isSDKTestSource(slashPath):
				if distSDKEmbedPatternMatches(t, patterns, slashPath) {
					embeddedTests = append(embeddedTests, slashPath)
				}
			}
			return nil
		})

		// Fail if walking an SDK source directory failed.
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	// Sort production omissions for stable test diagnostics.
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("dist.go SDK go:embed patterns do not cover production SDK sources:\n%s", strings.Join(missing, "\n"))
	}

	// Sort embedded test paths for stable test diagnostics.
	sort.Strings(embeddedTests)
	if len(embeddedTests) != 0 {
		t.Fatalf("dist.go SDK go:embed patterns include SDK test sources:\n%s", strings.Join(embeddedTests, "\n"))
	}
}

func distSDKEmbedPatterns(t *testing.T, distGo string) ([]string, []string) {
	// Attribute helper failures to the calling test.
	t.Helper()

	// Track source directories represented by SDK embed patterns.
	coveredDirs := make(map[string]bool)
	var patterns []string
	var dirs []string

	// Parse every go:embed directive and retain SDK paths.
	for line := range strings.SplitSeq(distGo, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "//go:embed") {
			continue
		}

		for pattern := range strings.FieldsSeq(strings.TrimPrefix(line, "//go:embed")) {
			if !strings.HasPrefix(pattern, "sdk/") {
				continue
			}

			patterns = append(patterns, pattern)
			dir := path.Dir(pattern)
			if hasGlobMeta(dir) {
				t.Fatalf("SDK go:embed pattern %q uses a glob in its directory; update this guard before relying on it", pattern)
			}
			if !coveredDirs[dir] {
				coveredDirs[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}

	// Return source directories in a stable order.
	sort.Strings(dirs)
	return patterns, dirs
}

func distSDKEmbedPatternMatches(t *testing.T, patterns []string, slashPath string) bool {
	t.Helper()

	for _, pattern := range patterns {
		if pattern == slashPath {
			return true
		}
		if !hasGlobMeta(pattern) {
			continue
		}

		matched, err := path.Match(pattern, slashPath)
		if err != nil {
			t.Fatalf("invalid SDK go:embed glob %q: %v", pattern, err)
		}
		if matched {
			return true
		}
	}
	return false
}

func isProductionSDKSource(slashPath string) bool {
	if !strings.HasSuffix(slashPath, ".ts") && !strings.HasSuffix(slashPath, ".tsx") {
		return false
	}
	return !strings.HasSuffix(slashPath, ".test.ts") && !strings.HasSuffix(slashPath, ".test.tsx")
}

func isSDKTestSource(slashPath string) bool {
	return strings.HasSuffix(slashPath, ".test.ts") || strings.HasSuffix(slashPath, ".test.tsx")
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*[?")
}
