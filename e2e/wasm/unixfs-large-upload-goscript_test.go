//go:build !skip_e2e && !js

package wasm

import (
	"strconv"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
)

const (
	unixFSLargeDropFileCount = 8
	unixFSLargeDropFileSize  = 3 * 1024 * 1024
	unixFSLargeDropWait      = 5 * time.Minute
)

func TestGoScriptUnixFSLargeMultiFileDropCompletes(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := harness(t).NewCleanSession(t)
	page := sess.Page()
	if err := page.SetViewportSize(1440, 900); err != nil {
		t.Fatalf("set viewport size: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page = scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	files := unixFSLargeDropFiles()
	started := time.Now()
	UploadViaDnd(t, page, files)
	waitForUnixFSLargeDropUploadSummary(t, page, "8/8 uploaded")
	for _, file := range files {
		waitForDriveEntry(t, page, file.Name)
		verifyUploadedFile(t, scenario, page, file)
	}
	duration := time.Since(started)
	totalBytes := int64(unixFSLargeDropFileCount * unixFSLargeDropFileSize)
	t.Logf(
		"goscript UnixFS large multi-file drop completed: files=%d bytes=%d duration=%s throughput_mib_s=%.2f",
		len(files),
		totalBytes,
		duration.Round(time.Millisecond),
		float64(totalBytes)/(1024*1024)/duration.Seconds(),
	)

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after GoScript UnixFS large multi-file drop: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after GoScript UnixFS large multi-file drop: %+v", report)
	}
}

func unixFSLargeDropFiles() []playwright.InputFile {
	files := make([]playwright.InputFile, 0, unixFSLargeDropFileCount)
	for i := range unixFSLargeDropFileCount {
		files = append(files, playwright.InputFile{
			Name:     "goscript-large-drop-" + strconv.Itoa(i) + ".bin",
			MimeType: "application/octet-stream",
			Buffer:   uploadPatternBytes(unixFSLargeDropFileSize),
		})
	}
	return files
}

func waitForUnixFSLargeDropUploadSummary(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	if err := page.Locator("button:has-text('" + text + "')").Last().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(float64(unixFSLargeDropWait / time.Millisecond))},
	); err != nil {
		t.Logf("upload diagnostics while waiting for %q: %s", text, captureUploadDiagnostics(page))
		failWithPageBody(t, page, "wait for large UnixFS drop upload summary "+text, err)
	}
}
