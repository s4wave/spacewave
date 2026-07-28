//go:build skip_e2e && !js

// Package lint hosts the repository-wide build-constraint guard for heavy
// end-to-end suites. The package file remains buildable without skip_e2e; the
// guard itself runs only in the tagged sweep.
package lint

import (
	"go/build/constraint"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TIER: pr
//
// heavyDriverSignals lists the source substrings that mark a _test.go file as a
// "heavy" end-to-end test: one that drives a real browser and therefore must be
// excluded from the broad skip_e2e sweep (its TestMain builds Vite/goscript
// fixtures and steers Chromium/Firefox/WebKit, which times out the sweep).
//
// The single load-bearing signal is the Playwright browser-driver import. Every
// heavy suite in the repo imports it to boot or steer the browser
// (bldr/e2e/comms, e2e/wasm, e2e/releasewasm, e2e/electron), so a NEW heavy
// suite added without the negating constraint will trip it. It is deliberately
// narrow for precision; two broader signals were considered and rejected
// because they fire on known-light packages that mention the concept
// incidentally:
//   - The raw string "vite build" also appears in bldr/web/bundler's compiler
//     unit tests (bldr/web/bundler/vite/compiler/bundler_test.go).
//   - The shared session-harness identifiers also appear in the light options
//     unit test e2e/wasm/option_test.go.
//
// The saucer suite (bldr/web/plugin/saucer/e2e) is intentionally NOT covered:
// it runs the runtime stack in-process against a simulated browser and never
// imports a real browser driver, so it is genuinely light.
var heavyDriverSignals = []string{
	`"github.com/mxschmitt/playwright-go"`,
}

// lightAllowlist names heavy-signal _test.go files that legitimately stay in
// the skip_e2e sweep because they never actually drive a browser there: they
// reference the Playwright types incidentally or gate all real browser work
// behind an env var / t.Skip that is unset during the sweep. Paths are relative
// to the repo root (slash-separated). Every entry is validated below so the
// allowlist cannot silently rot.
var lightAllowlist = map[string]string{
	"e2e/wasm/browser_gpu_test.go":           "TestChromiumLaunchOptions only builds launch-option structs; never launches a browser",
	"e2e/releasewasm/harness_gpu_test.go":    "TestChromiumLaunchOptions only builds launch-option structs; never launches a browser",
	"bldr/e2e/downstreamapp/harness_test.go": "browser tests gate on RunEnv=1 via t.Skipf; the rest are pure resolver unit tests",
	"db/opfs/chrometest/chrome_test.go":      "TestMain and every test gate on the chrome/tinygo env vars, unset in the sweep",
}

// skipDirs are directory names never worth walking for repo source.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".bldr":        true,
	"testdata":     true,
	"dist":         true,
	".tmp":         true,
}

// TestHeavyE2EFilesNegateSkipE2E walks every _test.go file in the repository and
// fails, naming the file, whenever a heavy browser test does not carry a
// //go:build constraint that negates skip_e2e. It is the one owner for this
// invariant: no per-suite copies. It catches the regression that silently
// re-admits a heavy suite (e.g. comms) into the skip_e2e sweep.
func TestHeavyE2EFilesNegateSkipE2E(t *testing.T) {
	root := repoRoot(t)

	_, selfFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to report the lint source path")
	}

	seenAllowlisted := make(map[string]bool, len(lightAllowlist))

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if path == selfFile {
			// This file names the driver import literally as its own signal; do
			// not let it flag itself.
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !isHeavyE2E(src) {
			return nil
		}
		rel := filepath.ToSlash(relPath(root, path))
		if _, allowed := lightAllowlist[rel]; allowed {
			seenAllowlisted[rel] = true
			return nil
		}
		if !excludedUnderSkipE2E(t, src) {
			t.Errorf("%s drives a browser (heavy e2e) but its //go:build constraint does not negate skip_e2e; "+
				"add `//go:build !skip_e2e && !js` so the heavy suite stays out of the skip_e2e sweep, "+
				"or add it to lightAllowlist with justification if it never drives a browser in the sweep", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo from %s: %v", root, err)
	}

	// Keep the allowlist honest: every entry must still exist and still trip the
	// heavy signal, otherwise it is stale and should be removed.
	for rel := range lightAllowlist {
		if !seenAllowlisted[rel] {
			t.Errorf("stale lightAllowlist entry %q: file is missing or no longer matches a heavy driver signal; remove it", rel)
		}
	}
}

// isHeavyE2E reports whether src references a heavy browser driver signal.
func isHeavyE2E(src []byte) bool {
	s := string(src)
	for _, sig := range heavyDriverSignals {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

// excludedUnderSkipE2E reports whether src's leading build constraint evaluates
// false in the skip_e2e sweep environment (skip_e2e set, js unset), i.e. the
// file is excluded from that build. A file with no leading build constraint is
// not excluded. It accepts both modern //go:build and legacy // +build syntax.
func excludedUnderSkipE2E(t *testing.T, src []byte) bool {
	t.Helper()
	sweep := func(tag string) bool {
		switch tag {
		case "skip_e2e":
			return true
		case "js":
			return false
		default:
			return false
		}
	}

	var legacy constraint.Expr
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if constraint.IsGoBuild(line) {
			expr, err := constraint.Parse(line)
			if err != nil {
				t.Fatalf("parse build constraint %q: %v", line, err)
			}
			// The go:build form is authoritative when both forms are
			// present, as it is in files generated by gofmt.
			return !expr.Eval(sweep)
		}
		if constraint.IsPlusBuild(line) {
			expr, err := constraint.Parse(line)
			if err != nil {
				t.Fatalf("parse build constraint %q: %v", line, err)
			}
			if legacy == nil {
				legacy = expr
			} else {
				legacy = &constraint.AndExpr{X: legacy, Y: expr}
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		break // reached the package clause without a build constraint
	}
	if legacy == nil {
		return false
	}
	return !legacy.Eval(sweep)
}

func TestExcludedUnderSkipE2EConstraints(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "modern",
			src:  "//go:build !skip_e2e && !js\n\npackage lint\n",
			want: true,
		},
		{
			name: "legacy",
			src:  "// +build !skip_e2e\n\npackage lint\n",
			want: true,
		},
		{
			name: "legacy after package",
			src:  "package lint\n\n// +build !skip_e2e\n",
			want: false,
		},
		{
			name: "unconstrained",
			src:  "package lint\n",
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := excludedUnderSkipE2E(t, []byte(test.src)); got != test.want {
				t.Fatalf("excludedUnderSkipE2E() = %t, want %t", got, test.want)
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to report the lint source path")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) above %s", thisFile)
		}
		dir = parent
	}
}

// relPath returns path relative to root, falling back to path on error.
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
