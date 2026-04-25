//go:build !skip_e2e && !js

package wasm

import (
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

// TestExistingDriveOpenMeasurement records a repeatable existing-drive reopen
// baseline with timings for the route, loading, UnixFS shell, and content-ready
// layers.
func TestExistingDriveOpenMeasurement(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()

	WaitForDriveReady(t, testHarness, page)
	targetHash, err := currentHash(page.URL())
	if err != nil {
		t.Fatalf("current drive hash: %v", err)
	}

	NavigateHash(t, testHarness, page, "#/u/"+uintString(scenario.GetSessionIndex()))

	raw, err := page.Evaluate(testHarness.Script("measure-existing-drive-open.ts"), map[string]any{
		"targetHash": targetHash,
		"deadlineMs": 120000,
	})
	if err != nil {
		t.Fatalf("measure existing drive open: %v", err)
	}

	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected measurement result %T", raw)
	}
	if completed, _ := result["completed"].(bool); !completed {
		t.Fatalf("existing drive open did not complete: %#v", result)
	}
	t.Logf(
		"existing drive open measurement route_ms=%v loading_ms=%v unixfs_shell_ms=%v unixfs_ready_ms=%v url=%v",
		result["routeMs"],
		result["loadingMs"],
		result["unixfsShellMs"],
		result["unixfsReadyMs"],
		result["url"],
	)
}

func currentHash(rawURL string) (string, error) {
	idx := strings.Index(rawURL, "#")
	if idx < 0 {
		return "", errors.New("missing hash route")
	}
	return rawURL[idx:], nil
}

func uintString(n uint32) string {
	return strconv.FormatUint(uint64(n), 10)
}
