//go:build !skip_e2e && !js

package wasm

import (
	"os"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

func readVideoFixture(t testing.TB, name string) []byte {
	t.Helper()

	b, err := os.ReadFile("fixtures/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func uploadDriveFiles(t testing.TB, page playwright.Page, files []playwright.InputFile) {
	t.Helper()

	err := page.Locator("input[type='file']").First().SetInputFiles(files)
	if err != nil {
		t.Fatalf("upload drive files: %v", err)
	}
}

func waitForDriveEntry(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	err := page.Locator("[role='row']").Locator("text=" + name).First().WaitFor()
	if err != nil {
		t.Fatalf("wait for drive entry %q: %v", name, err)
	}
}

func openDriveEntry(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	row := page.Locator("[role='row']").Locator("text=" + name).First()
	if err := row.WaitFor(); err != nil {
		t.Fatalf("wait for %s row: %v", name, err)
	}
	if err := row.Dblclick(); err != nil {
		t.Fatalf("open %s row: %v", name, err)
	}
}

func waitForVideoState(t testing.TB, page playwright.Page, label string) map[string]any {
	t.Helper()

	err := page.Locator("video").First().WaitFor()
	if err != nil {
		t.Fatalf("wait for video element %q: %v", label, err)
	}

	raw, err := page.Evaluate(`async ({label}) => {
		const node = document.querySelector('video')
		if (!(node instanceof HTMLVideoElement)) {
			throw new Error('video element not found for ' + label)
		}
		if (node.readyState < HTMLMediaElement.HAVE_METADATA || !Number.isFinite(node.duration) || node.duration <= 0) {
			await new Promise((resolve, reject) => {
				const onLoaded = () => {
					cleanup()
					resolve(undefined)
				}
				const onError = () => {
					cleanup()
					reject(new Error('video metadata failed to load for ' + label))
				}
				const cleanup = () => {
					node.removeEventListener('loadedmetadata', onLoaded)
					node.removeEventListener('error', onError)
				}
				node.addEventListener('loadedmetadata', onLoaded, { once: true })
				node.addEventListener('error', onError, { once: true })
			})
		}
		return {
			currentTime: node.currentTime,
			duration: node.duration,
			src: node.currentSrc || node.src,
		}
	}`, map[string]any{"label": label})
	if err != nil {
		t.Fatalf("wait for video state %q: %v", label, err)
	}
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected video state payload for %q: %#v", label, raw)
	}
	return state
}

func numberValue(t testing.TB, label string, value any) float64 {
	t.Helper()

	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	default:
		t.Fatalf("unexpected numeric value for %s: %#v", label, value)
		return 0
	}
}

func seekVideo(t testing.TB, page playwright.Page, label string, seconds float64) float64 {
	t.Helper()

	raw, err := page.Evaluate(`async ({label, seconds}) => {
		const node = document.querySelector('video')
		if (!(node instanceof HTMLVideoElement)) {
			throw new Error('video element not found for ' + label)
		}
		await new Promise((resolve, reject) => {
			const onSeeked = () => {
				cleanup()
				resolve(undefined)
			}
			const onError = () => {
				cleanup()
				reject(new Error('video seek failed for ' + label))
			}
			const cleanup = () => {
				node.removeEventListener('seeked', onSeeked)
				node.removeEventListener('error', onError)
			}
			node.addEventListener('seeked', onSeeked, { once: true })
			node.addEventListener('error', onError, { once: true })
			node.currentTime = seconds
		})
		return node.currentTime
	}`, map[string]any{
		"label":   label,
		"seconds": seconds,
	})
	if err != nil {
		t.Fatalf("seek video %q: %v", label, err)
	}
	currentTime, ok := raw.(float64)
	if !ok {
		t.Fatalf("unexpected seek result for %q: %#v", label, raw)
	}
	return currentTime
}

// TestQuickstartDriveVideoPreview verifies projected mp4/webm previews load,
// seek, and survive file-viewer navigation without stale player state.
func TestQuickstartDriveVideoPreview(t *testing.T) {
	sess := testHarness.NewSession(t)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()

	WaitForDriveReady(t, testHarness, page)

	mp4Name := "video-preview.mp4"
	webmName := "video-preview.webm"
	uploadDriveFiles(t, page, []playwright.InputFile{
		{
			Name:     mp4Name,
			MimeType: "video/mp4",
			Buffer:   readVideoFixture(t, mp4Name),
		},
		{
			Name:     webmName,
			MimeType: "video/webm",
			Buffer:   readVideoFixture(t, webmName),
		},
	})
	waitForDriveEntry(t, page, mp4Name)
	waitForDriveEntry(t, page, webmName)

	openDriveEntry(t, page, mp4Name)
	mp4State := waitForVideoState(t, page, mp4Name)
	mp4Src, _ := mp4State["src"].(string)
	if !strings.Contains(mp4Src, "/p/spacewave-core/fs/") || !strings.Contains(mp4Src, "inline=1") {
		t.Fatalf("expected projected inline mp4 source, got %q", mp4Src)
	}
	if duration := numberValue(t, "mp4 duration", mp4State["duration"]); duration <= 0 {
		t.Fatalf("expected positive mp4 duration, got %#v", mp4State["duration"])
	}
	if currentTime := seekVideo(t, page, mp4Name, 0.75); currentTime < 0.5 {
		t.Fatalf("expected mp4 seek to advance playback, got %f", currentTime)
	}

	if err := page.Locator("button[title='Up']").First().Click(); err != nil {
		t.Fatalf("click up from mp4 preview: %v", err)
	}
	WaitForDriveReady(t, testHarness, page)
	openDriveEntry(t, page, webmName)

	webmState := waitForVideoState(t, page, webmName)
	webmSrc, _ := webmState["src"].(string)
	if !strings.Contains(webmSrc, "/p/spacewave-core/fs/") || !strings.Contains(webmSrc, "inline=1") {
		t.Fatalf("expected projected inline webm source, got %q", webmSrc)
	}
	if duration := numberValue(t, "webm duration", webmState["duration"]); duration <= 0 {
		t.Fatalf("expected positive webm duration, got %#v", webmState["duration"])
	}
	if currentTime := numberValue(t, "webm currentTime", webmState["currentTime"]); currentTime > 0.1 {
		t.Fatalf("expected fresh webm preview to start near zero, got %f", currentTime)
	}
	if currentTime := seekVideo(t, page, webmName, 0.75); currentTime < 0.5 {
		t.Fatalf("expected webm seek to advance playback, got %f", currentTime)
	}

	if err := page.Locator("button[title='Back']").First().Click(); err != nil {
		t.Fatalf("click back to mp4 preview: %v", err)
	}
	backState := waitForVideoState(t, page, mp4Name)
	backSrc, _ := backState["src"].(string)
	if !strings.Contains(backSrc, mp4Name) {
		t.Fatalf("expected back navigation to restore %q, got %q", mp4Name, backSrc)
	}
	if currentTime := numberValue(t, "back mp4 currentTime", backState["currentTime"]); currentTime > 0.1 {
		t.Fatalf("expected remounted mp4 preview to reset playback, got %f", currentTime)
	}

	if err := page.Locator("button[title='Forward']").First().Click(); err != nil {
		t.Fatalf("click forward to webm preview: %v", err)
	}
	forwardState := waitForVideoState(t, page, webmName)
	forwardSrc, _ := forwardState["src"].(string)
	if !strings.Contains(forwardSrc, webmName) {
		t.Fatalf("expected forward navigation to restore %q, got %q", webmName, forwardSrc)
	}
	if currentTime := numberValue(t, "forward webm currentTime", forwardState["currentTime"]); currentTime > 0.1 {
		t.Fatalf("expected remounted webm preview to reset playback, got %f", currentTime)
	}
}
