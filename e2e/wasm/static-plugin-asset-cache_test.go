//go:build !skip_e2e && !js

package wasm

import (
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestReturnVisitorStaticPluginAssetCache proves a fresh page in the same
// browser context can reload a static plugin asset after the first page warms
// the generation-scoped ServiceWorker cache.
func TestReturnVisitorStaticPluginAssetCache(t *testing.T) {
	h := harness(t)
	sess := h.NewCleanSession(t)
	scenario := CreateDriveScenario(t, h, sess)
	page := scenario.GetSession().Page()

	WaitForDriveReady(t, h, page)
	pluginAssetURL := serviceWorkerRestartPluginAssetURL(t, page)
	assertStaticPluginAssetFetch(t, page, pluginAssetURL, "initial visitor", "")

	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace return-visitor page: %v", err)
	}
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load return-visitor app: %v", err)
	}
	page = sess.Page()
	WaitForApp(t, page)
	assertStaticPluginAssetFetch(t, page, pluginAssetURL, "return visitor", "generation")
}

func assertStaticPluginAssetFetch(
	t testing.TB,
	page playwright.Page,
	url string,
	label string,
	wantCacheProvenance string,
) {
	t.Helper()
	raw, err := page.Evaluate(`async (arg) => {
		const [url, label] = arg
		const response = await fetch(url)
		return {
			cacheProvenance: response.headers.get('X-Bldr-Plugin-Asset-Cache'),
			ok: response.ok,
			status: response.status,
			label,
		}
	}`, []any{url, label})
	if err != nil {
		t.Fatalf("fetch static plugin asset %s: %v", label, err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected static plugin asset proof %T: %#v", raw, raw)
	}
	if stringField(result, "label") != label ||
		intField(result, "status") != 200 ||
		!boolField(result, "ok") ||
		(wantCacheProvenance != "" &&
			stringField(result, "cacheProvenance") != wantCacheProvenance) {
		t.Fatalf("static plugin asset fetch failed %s: %#v", label, result)
	}
}
