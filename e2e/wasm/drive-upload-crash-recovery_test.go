//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

// TestQuickstartDriveUploadCrashRecovery exercises the Drive UploadTree path
// under browser WASM and classifies the console stream for the original
// fatal-Go-plus-exited-Go-loop recovery pattern.
func TestQuickstartDriveUploadCrashRecovery(t *testing.T) {
	sess := testHarness.NewSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, testHarness, page)

	UploadViaPicker(t, page, []playwright.InputFile{
		{
			Name:     "upload-root.txt",
			MimeType: "text/plain",
			Buffer:   []byte(strings.Repeat("root upload\n", 256)),
		},
		{
			Name:     "upload-notes.md",
			MimeType: "text/markdown",
			Buffer:   []byte(strings.Repeat("# upload notes\n\nbody\n", 128)),
		},
		{
			Name:     "upload-bytes.bin",
			MimeType: "application/octet-stream",
			Buffer:   []byte(strings.Repeat("\x00\x01\x02\x03", 1024)),
		},
	})

	waitForDriveEntry(t, page, "upload-root.txt")
	waitForDriveEntry(t, page, "upload-notes.md")
	waitForDriveEntry(t, page, "upload-bytes.bin")

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after upload: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after upload: %+v", report)
	}
}
