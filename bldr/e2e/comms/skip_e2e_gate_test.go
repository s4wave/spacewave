//go:build skip_e2e && !js

package comms

import (
	"go/build/constraint"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TIER: pr
// TestSkipE2EExcludesHeavySuite is compiled ONLY under the skip_e2e tag, so it
// runs inside the broad skip_e2e Go sweep. It is a regression canary for the
// heavy browser suite in this package (comms_test.go / TestMain builds the Vite
// and goscript fixtures and drives Chromium via Playwright). If comms_test.go
// ever lost its `//go:build !skip_e2e` constraint, its TestMain would boot here;
// this source assertion would fail after the package's TestMain setup, proving
// the heavy files re-entered the skip_e2e sweep.
func TestSkipE2EExcludesHeavySuite(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	// Heavy files that must never enter the skip_e2e sweep.
	for _, name := range []string{"comms_test.go", "webdocument_unixfs_fetch_test.go"} {
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !excludedUnderSkipE2E(t, src) {
			t.Fatalf("%s must carry a //go:build constraint that negates skip_e2e; "+
				"without it the heavy browser suite re-enters the skip_e2e sweep and times out", name)
		}
	}
}

// excludedUnderSkipE2E reports whether src's leading //go:build constraint
// evaluates false in the skip_e2e sweep environment (skip_e2e set, js unset),
// i.e. the file is excluded from that build.
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
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//go:build") {
			expr, err := constraint.Parse(line)
			if err != nil {
				t.Fatalf("parse build constraint %q: %v", line, err)
			}
			return !expr.Eval(sweep)
		}
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		break // reached the package clause without a build constraint
	}
	return false
}
