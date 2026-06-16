//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strings"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestRecreatedPageSharedObjectHealthGuard verifies an existing drive still
// opens with ready health after the page is recreated inside the same browser
// context.
func TestRecreatedPageSharedObjectHealthGuard(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()

	WaitForDriveReady(t, harness(t), page)
	targetHash, err := currentHash(page.URL())
	if err != nil {
		t.Fatalf("current drive hash: %v", err)
	}

	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page in current context: %v", err)
	}
	if err := harness(t).loadAppPageURL(sess, harness(t).BaseURL()+"/"+targetHash); err != nil {
		t.Fatalf("load drive route after page replacement: %v", err)
	}

	page = sess.Page()
	WaitForApp(t, page)
	AssertRootImportMap(t, harness(t), page)
	AssertBrowserStartupDone(t, harness(t), page)

	ctx, cancel := context.WithTimeout(harness(t).Context(), 90*time.Second)
	defer cancel()
	if err := sess.ConnectResources(ctx); err != nil {
		t.Fatalf("connect resources after recreated page route load: %v", err)
	}
	if len(sess.browserPeer) == 0 {
		t.Fatal("expected recreated page resource connection to attach a browser peer")
	}
	waitForSharedObjectReadyHealth(t, ctx, sess, scenario.GetSessionIndex(), scenario.GetSpaceID())

	NavigateHash(t, harness(t), page, targetHash)
	WaitForDriveReady(t, harness(t), page)
	assertNoSharedObjectHealthCard(t, page)

	t.Logf(
		"recreated-page shared-object health guard passed: session_index=%d space_id=%s url=%s",
		scenario.GetSessionIndex(),
		scenario.GetSpaceID(),
		page.URL(),
	)
}

func assertNoSharedObjectHealthCard(t testing.TB, page playwright.Page) {
	t.Helper()

	body, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatalf("read recreated page text: %v", err)
	}
	for _, marker := range []string{
		"Closed - Shared Object",
		"Closed - Body",
		"Degraded - Shared Object",
		"Degraded - Body",
		"Shared object unavailable",
		"Shared object not found",
		"Initial state rejected",
		"Required block missing",
		"Access revoked",
		"Shared object body failed",
		"Body configuration invalid",
	} {
		if strings.Contains(body, marker) {
			t.Fatalf("recreated page route rendered shared-object health card marker %q\nbody: %s", marker, trimPageText(body))
		}
	}
}

func waitForSharedObjectReadyHealth(
	t testing.TB,
	ctx context.Context,
	sess *TestSession,
	sessionIndex uint32,
	spaceID string,
) {
	t.Helper()

	sdk, err := sess.MountSessionByIdx(ctx, sessionIndex)
	if err != nil {
		t.Fatalf("mount session %d for health check: %v", sessionIndex, err)
	}
	defer sdk.Release()

	strm, err := sdk.WatchSharedObjectHealth(ctx, spaceID)
	if err != nil {
		t.Fatalf("watch shared-object health: %v", err)
	}
	defer strm.Close()

	var last *sobject.SharedObjectHealth
	for {
		resp, err := strm.Recv()
		if err != nil {
			t.Fatalf("recv shared-object health: %v (last=%s)", err, sharedObjectHealthSummary(last))
		}
		health := resp.GetHealth()
		last = health
		switch health.GetStatus() {
		case sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY:
			return
		case sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED,
			sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_DEGRADED:
			t.Fatalf("shared-object health is not ready: %s", sharedObjectHealthSummary(health))
		}
	}
}

func sharedObjectHealthSummary(health *sobject.SharedObjectHealth) string {
	if health == nil {
		return "nil"
	}
	parts := []string{
		"status=" + health.GetStatus().String(),
		"layer=" + health.GetLayer().String(),
		"reason=" + health.GetCommonReason().String(),
		"hint=" + health.GetRemediationHint().String(),
	}
	if errText := strings.TrimSpace(health.GetError()); errText != "" {
		parts = append(parts, "error="+errText)
	}
	return strings.Join(parts, " ")
}
