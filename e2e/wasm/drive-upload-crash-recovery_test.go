//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
)

const largeUploadSize = 8 * 1024 * 1024

const driveUploadFixturePathEnv = "E2E_WASM_DRIVE_UPLOAD_FIXTURE"
const driveUploadFixtureNameEnv = "E2E_WASM_DRIVE_UPLOAD_NAME"

// TestQuickstartDriveUploadCrashRecovery exercises the Drive UploadTree path
// under browser WASM and classifies the console stream for the original
// fatal-Go-plus-exited-Go-loop recovery pattern.
func TestQuickstartDriveUploadCrashRecovery(t *testing.T) {
	sess := testHarness.NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, testHarness, page)

	uploadAndVerifyDriveFixture(t, scenario, page, true)

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after upload: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after upload: %+v", report)
	}
}

// TestQuickstartDriveUploadTrace writes a runtime trace for the Drive UploadTree
// path, including the bounded large-file branch used by crash recovery.
func TestQuickstartDriveUploadTrace(t *testing.T) {
	skipTraceServiceForTinyGo(t)

	sess := testHarness.NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, testHarness, page)

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	data, err := sess.CaptureTrace(ctx, "quickstart-drive-upload", func(ctx context.Context) error {
		uploadAndVerifyDriveFixture(t, scenario, page, false)
		return nil
	})
	if err != nil {
		t.Fatalf("CaptureTrace: %v", err)
	}

	path := TraceArtifactPath(t)
	if err := WriteTraceArtifact(path, data); err != nil {
		t.Fatalf("WriteTraceArtifact: %v", err)
	}
	t.Logf("trace artifact written to %s (%d bytes)", path, len(data))

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after traced upload: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after traced upload: %+v", report)
	}
}

func uploadAndVerifyDriveFixture(t testing.TB, scenario *DriveScenario, page playwright.Page, includeExternal bool) {
	t.Helper()

	files := driveUploadFixtureFiles()
	UploadViaPicker(t, page, files)
	verifyDriveUploadFixture(t, scenario, page, files)

	if !includeExternal {
		return
	}
	path, name, ok := driveUploadExternalFixturePath(t)
	if !ok {
		return
	}
	UploadPathsViaPicker(t, page, []string{path})
	verifyUploadedPath(t, scenario, page, name, path)
}

func driveUploadFixtureFiles() []playwright.InputFile {
	return []playwright.InputFile{
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
		{
			Name:     "upload-large.bin",
			MimeType: "application/octet-stream",
			Buffer:   uploadPatternBytes(largeUploadSize),
		},
	}
}

func driveUploadExternalFixturePath(t testing.TB) (string, string, bool) {
	t.Helper()

	path := strings.TrimSpace(os.Getenv(driveUploadFixturePathEnv))
	if path == "" {
		return "", "", false
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s=%q: %v", driveUploadFixturePathEnv, path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s=%q is a directory", driveUploadFixturePathEnv, path)
	}
	name := strings.TrimSpace(os.Getenv(driveUploadFixtureNameEnv))
	if name == "" {
		name = filepath.Base(path)
	}
	if name != filepath.Base(path) {
		t.Fatalf("%s=%q requires %s to match path basename %q", driveUploadFixturePathEnv, path, driveUploadFixtureNameEnv, filepath.Base(path))
	}
	return path, name, true
}

func verifyDriveUploadFixture(t testing.TB, scenario *DriveScenario, page playwright.Page, files []playwright.InputFile) {
	t.Helper()

	for _, file := range files {
		waitForDriveEntry(t, page, file.Name)
		verifyUploadedFile(t, scenario, page, file)
	}
}

func verifyUploadedPath(t testing.TB, scenario *DriveScenario, page playwright.Page, name, path string) {
	t.Helper()

	waitForDriveEntry(t, page, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read uploaded fixture %s=%q: %v", driveUploadFixturePathEnv, path, err)
	}
	verifyUploadedFile(t, scenario, page, playwright.InputFile{
		Name:     name,
		MimeType: "application/octet-stream",
		Buffer:   data,
	})
}

func verifyUploadedFile(t testing.TB, scenario *DriveScenario, page playwright.Page, file playwright.InputFile) {
	t.Helper()

	raw, err := page.Evaluate(`async ({ url, wantText }) => {
			const response = await fetch(url)
			if (!response.ok) {
				throw new Error('fetch ' + url + ' failed with ' + response.status)
			}
			const bytes = new Uint8Array(await response.arrayBuffer())
			const digest = await crypto.subtle.digest('SHA-256', bytes)
			const sha256 = Array.from(new Uint8Array(digest), (b) =>
				b.toString(16).padStart(2, '0'),
			).join('')
			return {
				size: bytes.byteLength,
				sha256,
				text: wantText ? new TextDecoder().decode(bytes) : '',
			}
		}`, map[string]any{
		"url":      driveUploadFileURL(scenario, file.Name),
		"wantText": strings.HasPrefix(file.MimeType, "text/"),
	})
	if err != nil {
		t.Fatalf("verify uploaded %s: %v", file.Name, err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected upload verification payload for %s: %#v", file.Name, raw)
	}
	if got, want := int(numberValue(t, file.Name+" size", result["size"])), len(file.Buffer); got != want {
		t.Fatalf("uploaded %s size=%d want=%d", file.Name, got, want)
	}
	sha, ok := result["sha256"].(string)
	if !ok {
		t.Fatalf("unexpected uploaded sha256 payload for %s: %#v", file.Name, result["sha256"])
	}
	if got, want := sha, sha256Hex(file.Buffer); got != want {
		t.Fatalf("uploaded %s sha256=%s want=%s", file.Name, got, want)
	}
	if strings.HasPrefix(file.MimeType, "text/") {
		text, ok := result["text"].(string)
		if !ok {
			t.Fatalf("unexpected uploaded text payload for %s: %#v", file.Name, result["text"])
		}
		if text != string(file.Buffer) {
			t.Fatalf("uploaded %s text mismatch: got %q want %q", file.Name, text, string(file.Buffer))
		}
	}
}

func driveUploadFileURL(scenario *DriveScenario, name string) string {
	return fmt.Sprintf(
		"/p/spacewave-core/fs/u/%d/so/%s/-/files/-/%s?inline=1",
		scenario.GetSessionIndex(),
		scenario.GetSpaceID(),
		url.PathEscape(name),
	)
}

func uploadPatternBytes(size int) []byte {
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte((i * 31) ^ (i >> 7) ^ (i >> 15))
	}
	return buf
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
