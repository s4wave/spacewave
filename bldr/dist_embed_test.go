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
	distGo, err := os.ReadFile("dist.go")
	if err != nil {
		t.Fatal(err)
	}

	patterns, dirs := distSDKEmbedPatterns(t, string(distGo))
	if len(patterns) == 0 {
		t.Fatal("dist.go has no SDK go:embed patterns")
	}

	seen := make(map[string]bool)
	var missing, embeddedTests []string
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.FromSlash(dir), func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}

			slashPath := filepath.ToSlash(filePath)
			if seen[slashPath] {
				return nil
			}
			seen[slashPath] = true

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
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("dist.go SDK go:embed patterns do not cover production SDK sources:\n%s", strings.Join(missing, "\n"))
	}

	sort.Strings(embeddedTests)
	if len(embeddedTests) != 0 {
		t.Fatalf("dist.go SDK go:embed patterns include SDK test sources:\n%s", strings.Join(embeddedTests, "\n"))
	}
}

func distSDKEmbedPatterns(t *testing.T, distGo string) ([]string, []string) {
	t.Helper()

	coveredDirs := make(map[string]bool)
	var patterns []string
	var dirs []string
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
