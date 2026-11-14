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

// distCompleteRoots are the source trees whose every production TypeScript
// file ships in DistSources. dist.go names their files one by one, because an
// embed pattern cannot exclude files and a glob would also embed their tests.
var distCompleteRoots = []string{
	"sdk/",
	"web/bldr/",
	"web/bldr-react/",
	"web/boot/",
	"web/electron/",
	"web/entrypoint/",
	"web/runtime/goscript/",
}

// TestDistSourcesExcludeTests checks that no test source ships in the binary.
func TestDistSourcesExcludeTests(t *testing.T) {
	// Collect every embedded path that is a test source or test fixture.
	var tests []string
	err := fs.WalkDir(DistSources, ".", func(slashPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && isDistTestSource(slashPath) {
			tests = append(tests, slashPath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Report the test sources the embed patterns matched.
	if len(tests) != 0 {
		t.Fatalf("dist.go go:embed patterns include test sources:\n%s", strings.Join(tests, "\n"))
	}
}

// TestDistEmbedPatternsCoverProductionSources checks that each complete root
// embeds every production TypeScript file under the directories it names.
func TestDistEmbedPatternsCoverProductionSources(t *testing.T) {
	// Read dist.go, whose embed directives define production coverage.
	distGo, err := os.ReadFile("dist.go")
	if err != nil {
		t.Fatal(err)
	}

	// Extract the complete roots' embed patterns and their source directories.
	patterns, dirs := distEmbedPatterns(t, string(distGo))
	if len(patterns) == 0 {
		t.Fatal("dist.go has no go:embed patterns under the complete roots")
	}

	// Walk each covered directory and collect production sources no pattern
	// embeds. Overlapping directories reach some paths twice.
	seen := make(map[string]bool)
	var missing []string
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.FromSlash(dir), func(filePath string, entry fs.DirEntry, walkErr error) error {
			// Propagate walk errors and skip directories.
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}

			// Normalize the path and visit it once.
			slashPath := filepath.ToSlash(filePath)
			if seen[slashPath] {
				return nil
			}
			seen[slashPath] = true

			// Record a production source no pattern embeds.
			if isDistProductionSource(slashPath) && !distEmbedPatternMatches(t, patterns, slashPath) {
				missing = append(missing, slashPath)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	// Report omissions in a stable order.
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("dist.go go:embed patterns do not cover production sources:\n%s", strings.Join(missing, "\n"))
	}
}

// distEmbedPatterns returns the go:embed patterns under the complete roots and
// the source directories they name, in a stable order.
func distEmbedPatterns(t *testing.T, distGo string) ([]string, []string) {
	// Attribute helper failures to the calling test.
	t.Helper()

	// Parse every go:embed directive and retain paths under a complete root.
	coveredDirs := make(map[string]bool)
	var patterns []string
	var dirs []string
	for line := range strings.SplitSeq(distGo, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "//go:embed") {
			continue
		}

		for pattern := range strings.FieldsSeq(strings.TrimPrefix(line, "//go:embed")) {
			if !hasDistCompleteRoot(pattern) {
				continue
			}

			patterns = append(patterns, pattern)
			dir := path.Dir(pattern)
			if hasGlobMeta(dir) {
				t.Fatalf("go:embed pattern %q uses a glob in its directory; update this guard before relying on it", pattern)
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

// distEmbedPatternMatches reports whether a pattern names or matches slashPath.
func distEmbedPatternMatches(t *testing.T, patterns []string, slashPath string) bool {
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
			t.Fatalf("invalid go:embed glob %q: %v", pattern, err)
		}
		if matched {
			return true
		}
	}
	return false
}

// hasDistCompleteRoot reports whether slashPath lies under a complete root.
func hasDistCompleteRoot(slashPath string) bool {
	for _, root := range distCompleteRoots {
		if strings.HasPrefix(slashPath, root) {
			return true
		}
	}
	return false
}

// isDistProductionSource reports whether slashPath is TypeScript that ships.
func isDistProductionSource(slashPath string) bool {
	if !strings.HasSuffix(slashPath, ".ts") && !strings.HasSuffix(slashPath, ".tsx") {
		return false
	}
	return !isDistTestSource(slashPath)
}

// isDistTestSource reports whether slashPath is a test source, fixture or
// spec view. A TLA+ spec's .view.ts lives beside the code it models and never
// ships.
func isDistTestSource(slashPath string) bool {
	return strings.HasSuffix(slashPath, ".test.ts") ||
		strings.HasSuffix(slashPath, ".view.ts") ||
		strings.HasSuffix(slashPath, ".test.tsx") ||
		strings.HasSuffix(slashPath, "_test.go") ||
		strings.Contains(slashPath, "/testdata/")
}

// hasGlobMeta reports whether s contains a path.Match metacharacter.
func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*[?")
}
