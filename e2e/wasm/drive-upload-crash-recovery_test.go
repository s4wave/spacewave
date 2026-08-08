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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

const largeUploadSize = 8 * 1024 * 1024

const (
	driveUploadFixturePathEnv         = "E2E_WASM_DRIVE_UPLOAD_FIXTURE"
	driveUploadFixtureNameEnv         = "E2E_WASM_DRIVE_UPLOAD_NAME"
	driveUploadExternalWaitTimeoutEnv = "E2E_WASM_DRIVE_UPLOAD_WAIT_TIMEOUT_MS"
)

// TestQuickstartDriveUploadTrace writes a runtime trace for the Drive UploadTree
// path, including the bounded large-file branch used by crash recovery.
func TestQuickstartDriveUploadTrace(t *testing.T) {
	skipTraceServiceWhenDisabled(t)

	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

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

func TestQuickstartDriveLargeUploadBudgetReport(t *testing.T) {
	path, name, ok := driveUploadExternalFixturePath(t)
	if !ok {
		t.Skipf("set %s to a large local video/file path", driveUploadFixturePathEnv)
	}
	fixtureInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat large upload fixture: %v", err)
	}

	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	before, ok := captureTinyGoBudgetSnapshot(t, sess)
	if !ok {
		t.Fatal("TinyGo browser budget report unavailable before upload")
	}

	started := time.Now()
	UploadPathsViaPicker(t, page, []string{path})
	t.Logf("large external drive upload %s accepted by picker", name)
	waitForExternalDriveEntry(t, page, name)
	t.Logf("large external drive upload %s row appeared after %s", name, time.Since(started).Round(time.Millisecond))

	after, ok := captureTinyGoBudgetSnapshot(t, sess)
	if !ok {
		t.Fatal("TinyGo browser budget report unavailable after upload")
	}
	assertTinyGoBudgetSnapshot(t, "before upload", before)
	assertTinyGoBudgetSnapshot(t, "after upload", after)
	if after.Budget.Totals.HighWaterBytes < before.Budget.Totals.HighWaterBytes {
		t.Fatalf(
			"TinyGo budget high-water regressed after upload: before=%d after=%d",
			before.Budget.Totals.HighWaterBytes,
			after.Budget.Totals.HighWaterBytes,
		)
	}

	artifact := driveUploadBudgetArtifact{
		FixtureName: name,
		FixtureSize: fixtureInfo.Size(),
		StartedAt:   started.Format(time.RFC3339Nano),
		DurationMS:  time.Since(started).Milliseconds(),
		Before:      before,
		After:       after,
	}
	data := marshalDriveUploadBudgetArtifact(artifact)
	artifactPath := driveUploadBudgetArtifactPath(t)
	if err := WriteTraceArtifact(artifactPath, data); err != nil {
		t.Fatalf("write TinyGo budget artifact: %v", err)
	}
	t.Logf("TinyGo budget artifact written to %s (%d bytes)", artifactPath, len(data))

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after large upload: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after large upload: %+v", report)
	}
}

func TestQuickstartDriveUploadBudgetProfiles(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	var profiles []driveUploadBudgetProfile
	recordProfile := func(name string, fn func()) {
		t.Helper()
		before, ok := captureTinyGoBudgetSnapshot(t, sess)
		if !ok {
			t.Fatalf("TinyGo browser budget report unavailable before %s", name)
		}
		started := time.Now()
		fn()
		after, ok := captureTinyGoBudgetSnapshot(t, sess)
		if !ok {
			t.Fatalf("TinyGo browser budget report unavailable after %s", name)
		}
		assertTinyGoBudgetSnapshot(t, name+" before", before)
		assertTinyGoBudgetSnapshot(t, name+" after", after)
		profiles = append(profiles, driveUploadBudgetProfile{
			Name:       name,
			DurationMS: time.Since(started).Milliseconds(),
			Before:     before,
			After:      after,
		})
	}

	recordProfile("many-medium-files", func() {
		files := driveUploadMediumProfileFiles()
		UploadViaPicker(t, page, files)
		verifyDriveUploadFixture(t, scenario, page, files)
		clearDriveUploadDone(t, page)
	})

	recordProfile("large-overwrite-and-readback", func() {
		exerciseDriveUploadOverwriteAndReadback(t, scenario, page)
	})

	recordProfile("text-preview-pressure", func() {
		file := playwright.InputFile{
			Name:     "budget-preview.txt",
			MimeType: "text/plain",
			Buffer:   []byte(strings.Repeat("budget preview line\n", 64*1024)),
		}
		UploadViaPicker(t, page, []playwright.InputFile{file})
		verifyUploadedFile(t, scenario, page, file)
		openDriveEntry(t, page, file.Name)
		if err := page.Locator("[data-testid='unixfs-browser'] pre").First().WaitFor(); err != nil {
			t.Fatalf("wait for text preview: %v", err)
		}
	})

	if path, name, ok := driveUploadExternalFixturePath(t); ok {
		recordProfile("abort-before-commit", func() {
			UploadPathsViaPicker(t, page, []string{path})
			t.Logf("abort profile external upload %s accepted by picker", name)
			canceled := cancelDriveUploads(t, page)
			t.Logf("abort profile cancel-all clicked=%v", canceled)
		})
	} else {
		profiles = append(profiles, driveUploadBudgetProfile{
			Name:   "abort-before-commit",
			Status: "skipped: set " + driveUploadFixturePathEnv + " to a large local file",
		})
	}

	data := marshalDriveUploadBudgetProfilesArtifact(driveUploadBudgetProfilesArtifact{
		Profiles: profiles,
	})
	artifactPath := driveUploadBudgetProfilesArtifactPath(t)
	if err := WriteTraceArtifact(artifactPath, data); err != nil {
		t.Fatalf("write TinyGo budget profile artifact: %v", err)
	}
	t.Logf("TinyGo budget profile artifact written to %s (%d bytes)", artifactPath, len(data))

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after budget profiles: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after budget profiles: %+v", report)
	}
}

func TestQuickstartDriveUploadOverwriteBudgetProfile(t *testing.T) {
	sess := harness(t).NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	scenario := CreateDriveScenario(t, harness(t), sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, harness(t), page)

	before, ok := captureTinyGoBudgetSnapshot(t, sess)
	if !ok {
		t.Fatal("TinyGo browser budget report unavailable before overwrite profile")
	}
	exerciseDriveUploadOverwriteAndReadback(t, scenario, page)
	after, ok := captureTinyGoBudgetSnapshot(t, sess)
	if !ok {
		t.Fatal("TinyGo browser budget report unavailable after overwrite profile")
	}
	assertTinyGoBudgetSnapshot(t, "overwrite profile before", before)
	assertTinyGoBudgetSnapshot(t, "overwrite profile after", after)

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after overwrite profile: %+v", report)
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after overwrite profile: %+v", report)
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
	started := time.Now()
	UploadPathsViaPicker(t, page, []string{path})
	t.Logf("external drive upload %s accepted by picker", name)
	verifyUploadedPath(t, scenario, page, name, path)
	t.Logf("external drive upload %s verified after %s", name, time.Since(started).Round(time.Millisecond))
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

	waitForExternalDriveEntry(t, page, name)
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

func waitForExternalDriveEntry(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	timeout := externalUploadWaitTimeout(t)
	err := page.Locator("[data-testid='unixfs-browser'] [role='row']:has-text('" + name + "')").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: new(float64(timeout / time.Millisecond))},
	)
	if err != nil {
		t.Logf("upload diagnostics for %s: %s", name, captureUploadDiagnostics(page))
		failWithPageBody(t, page, "wait for drive entry "+name, err)
	}
	t.Logf("external drive upload %s row appeared within %s", name, timeout)
}

func externalUploadWaitTimeout(t testing.TB) time.Duration {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv(driveUploadExternalWaitTimeoutEnv))
	if raw == "" {
		return 120 * time.Second
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		t.Fatalf("invalid %s=%q: expected positive milliseconds", driveUploadExternalWaitTimeoutEnv, raw)
	}
	return time.Duration(ms) * time.Millisecond
}

func captureUploadDiagnostics(page playwright.Page) string {
	_ = page.Locator("button:has-text('Uploading'), button:has-text('uploaded')").Last().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(1000)},
	)
	raw, err := page.Evaluate(`() => {
		const read = (selector) => Array.from(document.querySelectorAll(selector), (el) => ({
			selector,
			text: (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 2000),
		}))
		return {
			buttons: read('button').filter((entry) =>
				/Uploading|uploaded|Cancel all|Clear done/.test(entry.text),
			),
			progress: read('[data-testid="upload-progress-overlay"], [data-testid="upload-progress-list"]'),
			browserRows: read('[data-testid="unixfs-browser"] [role="row"]'),
		}
	}`)
	if err != nil {
		return "capture failed: " + err.Error()
	}
	return fmt.Sprintf("%#v", raw)
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

func driveUploadMediumProfileFiles() []playwright.InputFile {
	files := make([]playwright.InputFile, 0, 12)
	for i := range 12 {
		files = append(files, playwright.InputFile{
			Name:     fmt.Sprintf("budget-medium-%02d.bin", i),
			MimeType: "application/octet-stream",
			Buffer:   uploadPatternBytes(512 * 1024),
		})
	}
	return files
}

func exerciseDriveUploadOverwriteAndReadback(t testing.TB, scenario *DriveScenario, page playwright.Page) {
	t.Helper()

	first := playwright.InputFile{
		Name:     "budget-overwrite.bin",
		MimeType: "application/octet-stream",
		Buffer:   uploadPatternBytes(2 * 1024 * 1024),
	}
	second := playwright.InputFile{
		Name:     first.Name,
		MimeType: first.MimeType,
		Buffer:   uploadPatternBytes(3 * 1024 * 1024),
	}
	UploadViaPicker(t, page, []playwright.InputFile{first})
	waitForDriveUploadSummary(t, page, "1/1 uploaded")
	verifyUploadedFile(t, scenario, page, first)
	clearDriveUploadDone(t, page)
	UploadViaPicker(t, page, []playwright.InputFile{second})
	waitForDriveUploadSummary(t, page, "1/1 uploaded")
	verifyUploadedFile(t, scenario, page, second)
}

func cancelDriveUploads(t testing.TB, page playwright.Page) bool {
	t.Helper()

	_ = page.Locator("button:has-text('Uploading'), button:has-text('uploaded')").Last().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(1000)},
	)
	err := page.Locator("[data-testid='upload-progress-overlay'] button:has-text('Cancel all')").First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(1000)},
	)
	return err == nil
}

func waitForDriveUploadSummary(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	if err := page.Locator("button:has-text('" + text + "')").Last().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	); err != nil {
		t.Logf("upload diagnostics while waiting for %q: %s", text, captureUploadDiagnostics(page))
		failWithPageBody(t, page, "wait for upload summary "+text, err)
	}
}

func clearDriveUploadDone(t testing.TB, page playwright.Page) {
	t.Helper()

	_ = page.Locator("button:has-text('uploaded')").Last().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(1000)},
	)
	_ = page.Locator("[data-testid='upload-progress-overlay'] button:has-text('Clear done')").First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(1000)},
	)
}

type driveUploadBudgetArtifact struct {
	FixtureName string                   `json:"fixtureName"`
	FixtureSize int64                    `json:"fixtureSize"`
	StartedAt   string                   `json:"startedAt"`
	DurationMS  int64                    `json:"durationMs"`
	Before      tinyGoBudgetDebugPayload `json:"before"`
	After       tinyGoBudgetDebugPayload `json:"after"`
}

type driveUploadBudgetProfilesArtifact struct {
	Profiles []driveUploadBudgetProfile `json:"profiles"`
}

type driveUploadBudgetProfile struct {
	Name       string                   `json:"name"`
	Status     string                   `json:"status,omitempty"`
	DurationMS int64                    `json:"durationMs,omitempty"`
	Before     tinyGoBudgetDebugPayload `json:"before"`
	After      tinyGoBudgetDebugPayload `json:"after"`
}

type tinyGoBudgetDebugPayload struct {
	Budget tinyGoBrowserBudgetReport `json:"budget"`
	JSHeap map[string]float64        `json:"jsHeap,omitempty"`
	Now    float64                   `json:"now"`
}

type tinyGoBrowserBudgetReport struct {
	CurrentGenerationID int                             `json:"currentGenerationId,omitempty"`
	Generations         []tinyGoBrowserBudgetGeneration `json:"generations"`
	Totals              tinyGoBrowserBudgetTotals       `json:"totals"`
}

type tinyGoBrowserBudgetGeneration struct {
	ID     int                        `json:"id"`
	State  string                     `json:"state"`
	Owners []tinyGoBrowserBudgetOwner `json:"owners"`
}

type tinyGoBrowserBudgetOwner struct {
	Owner          string `json:"owner"`
	CurrentBytes   int64  `json:"currentBytes"`
	HighWaterBytes int64  `json:"highWaterBytes"`
	CurrentCount   int64  `json:"currentCount"`
	HighWaterCount int64  `json:"highWaterCount"`
}

type tinyGoBrowserBudgetTotals struct {
	CurrentBytes   int64 `json:"currentBytes"`
	HighWaterBytes int64 `json:"highWaterBytes"`
}

func marshalDriveUploadBudgetArtifact(artifact driveUploadBudgetArtifact) []byte {
	var arena fastjson.Arena
	return marshalDriveUploadBudgetArtifactValue(&arena, artifact).MarshalTo(nil)
}

func marshalDriveUploadBudgetArtifactValue(arena *fastjson.Arena, artifact driveUploadBudgetArtifact) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("fixtureName", arena.NewString(artifact.FixtureName))
	obj.Set("fixtureSize", arena.NewNumberString(strconv.FormatInt(artifact.FixtureSize, 10)))
	obj.Set("startedAt", arena.NewString(artifact.StartedAt))
	obj.Set("durationMs", arena.NewNumberString(strconv.FormatInt(artifact.DurationMS, 10)))
	obj.Set("before", marshalTinyGoBudgetDebugPayloadValue(arena, artifact.Before))
	obj.Set("after", marshalTinyGoBudgetDebugPayloadValue(arena, artifact.After))
	return obj
}

func marshalDriveUploadBudgetProfilesArtifact(artifact driveUploadBudgetProfilesArtifact) []byte {
	var arena fastjson.Arena
	obj := arena.NewObject()
	profiles := arena.NewArray()
	for _, profile := range artifact.Profiles {
		profiles.SetArrayItem(len(profiles.GetArray()), marshalDriveUploadBudgetProfileValue(&arena, profile))
	}
	obj.Set("profiles", profiles)
	return obj.MarshalTo(nil)
}

func marshalDriveUploadBudgetProfileValue(arena *fastjson.Arena, profile driveUploadBudgetProfile) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(profile.Name))
	if profile.Status != "" {
		obj.Set("status", arena.NewString(profile.Status))
	}
	if profile.DurationMS != 0 {
		obj.Set("durationMs", arena.NewNumberString(strconv.FormatInt(profile.DurationMS, 10)))
	}
	obj.Set("before", marshalTinyGoBudgetDebugPayloadValue(arena, profile.Before))
	obj.Set("after", marshalTinyGoBudgetDebugPayloadValue(arena, profile.After))
	return obj
}

func marshalTinyGoBudgetDebugPayloadValue(arena *fastjson.Arena, payload tinyGoBudgetDebugPayload) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("budget", marshalTinyGoBrowserBudgetReportValue(arena, payload.Budget))
	if len(payload.JSHeap) != 0 {
		obj.Set("jsHeap", marshalFloat64MapValue(arena, payload.JSHeap))
	}
	obj.Set("now", arena.NewNumberString(strconv.FormatFloat(payload.Now, 'f', -1, 64)))
	return obj
}

func marshalTinyGoBrowserBudgetReportValue(arena *fastjson.Arena, report tinyGoBrowserBudgetReport) *fastjson.Value {
	obj := arena.NewObject()
	if report.CurrentGenerationID != 0 {
		obj.Set("currentGenerationId", arena.NewNumberInt(report.CurrentGenerationID))
	}
	generations := arena.NewArray()
	for _, generation := range report.Generations {
		generations.SetArrayItem(len(generations.GetArray()), marshalTinyGoBrowserBudgetGenerationValue(arena, generation))
	}
	obj.Set("generations", generations)
	obj.Set("totals", marshalTinyGoBrowserBudgetTotalsValue(arena, report.Totals))
	return obj
}

func marshalTinyGoBrowserBudgetGenerationValue(arena *fastjson.Arena, generation tinyGoBrowserBudgetGeneration) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("id", arena.NewNumberInt(generation.ID))
	obj.Set("state", arena.NewString(generation.State))
	owners := arena.NewArray()
	for _, owner := range generation.Owners {
		owners.SetArrayItem(len(owners.GetArray()), marshalTinyGoBrowserBudgetOwnerValue(arena, owner))
	}
	obj.Set("owners", owners)
	return obj
}

func marshalTinyGoBrowserBudgetOwnerValue(arena *fastjson.Arena, owner tinyGoBrowserBudgetOwner) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("owner", arena.NewString(owner.Owner))
	obj.Set("currentBytes", arena.NewNumberString(strconv.FormatInt(owner.CurrentBytes, 10)))
	obj.Set("highWaterBytes", arena.NewNumberString(strconv.FormatInt(owner.HighWaterBytes, 10)))
	obj.Set("currentCount", arena.NewNumberString(strconv.FormatInt(owner.CurrentCount, 10)))
	obj.Set("highWaterCount", arena.NewNumberString(strconv.FormatInt(owner.HighWaterCount, 10)))
	return obj
}

func marshalTinyGoBrowserBudgetTotalsValue(arena *fastjson.Arena, totals tinyGoBrowserBudgetTotals) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("currentBytes", arena.NewNumberString(strconv.FormatInt(totals.CurrentBytes, 10)))
	obj.Set("highWaterBytes", arena.NewNumberString(strconv.FormatInt(totals.HighWaterBytes, 10)))
	return obj
}

func marshalFloat64MapValue(arena *fastjson.Arena, values map[string]float64) *fastjson.Value {
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		obj.Set(key, arena.NewNumberString(strconv.FormatFloat(values[key], 'f', -1, 64)))
	}
	return obj
}

func parseTinyGoBudgetDebugPayload(data []byte) (tinyGoBudgetDebugPayload, error) {
	var parser fastjson.Parser
	v, err := parser.ParseBytes(data)
	if err != nil {
		return tinyGoBudgetDebugPayload{}, err
	}
	return parseTinyGoBudgetDebugPayloadValue(v), nil
}

func parseTinyGoBudgetDebugPayloadValue(v *fastjson.Value) tinyGoBudgetDebugPayload {
	if v == nil {
		return tinyGoBudgetDebugPayload{}
	}
	return tinyGoBudgetDebugPayload{
		Budget: parseTinyGoBrowserBudgetReportValue(v.Get("budget")),
		JSHeap: parseFloat64MapValue(v.GetObject("jsHeap")),
		Now:    v.GetFloat64("now"),
	}
}

func parseTinyGoBrowserBudgetReportValue(v *fastjson.Value) tinyGoBrowserBudgetReport {
	if v == nil {
		return tinyGoBrowserBudgetReport{}
	}
	return tinyGoBrowserBudgetReport{
		CurrentGenerationID: v.GetInt("currentGenerationId"),
		Generations:         parseTinyGoBrowserBudgetGenerations(v.GetArray("generations")),
		Totals:              parseTinyGoBrowserBudgetTotalsValue(v.Get("totals")),
	}
}

func parseTinyGoBrowserBudgetGenerations(values []*fastjson.Value) []tinyGoBrowserBudgetGeneration {
	out := make([]tinyGoBrowserBudgetGeneration, 0, len(values))
	for _, value := range values {
		out = append(out, tinyGoBrowserBudgetGeneration{
			ID:     value.GetInt("id"),
			State:  string(value.GetStringBytes("state")),
			Owners: parseTinyGoBrowserBudgetOwners(value.GetArray("owners")),
		})
	}
	return out
}

func parseTinyGoBrowserBudgetOwners(values []*fastjson.Value) []tinyGoBrowserBudgetOwner {
	out := make([]tinyGoBrowserBudgetOwner, 0, len(values))
	for _, value := range values {
		out = append(out, tinyGoBrowserBudgetOwner{
			Owner:          string(value.GetStringBytes("owner")),
			CurrentBytes:   value.GetInt64("currentBytes"),
			HighWaterBytes: value.GetInt64("highWaterBytes"),
			CurrentCount:   value.GetInt64("currentCount"),
			HighWaterCount: value.GetInt64("highWaterCount"),
		})
	}
	return out
}

func parseTinyGoBrowserBudgetTotalsValue(v *fastjson.Value) tinyGoBrowserBudgetTotals {
	if v == nil {
		return tinyGoBrowserBudgetTotals{}
	}
	return tinyGoBrowserBudgetTotals{
		CurrentBytes:   v.GetInt64("currentBytes"),
		HighWaterBytes: v.GetInt64("highWaterBytes"),
	}
}

func parseFloat64MapValue(obj *fastjson.Object) map[string]float64 {
	if obj == nil {
		return nil
	}
	values := map[string]float64{}
	obj.Visit(func(key []byte, value *fastjson.Value) {
		values[string(key)] = value.GetFloat64()
	})
	return values
}

func captureTinyGoBudgetSnapshot(t testing.TB, sess *TestSession) (tinyGoBudgetDebugPayload, bool) {
	t.Helper()

	script := harness(t).Script("tinygo-budget-debug.ts")
	args := map[string]any{"op": "snapshot"}
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, w := range sess.Workers() {
			raw, err := w.Evaluate(script, args)
			if err != nil {
				continue
			}
			rawJSON, ok := raw.(string)
			if !ok || rawJSON == "null" {
				continue
			}
			payload, err := parseTinyGoBudgetDebugPayload([]byte(rawJSON))
			if err != nil {
				t.Fatalf("parse TinyGo budget snapshot: %v", err)
			}
			return payload, true
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return tinyGoBudgetDebugPayload{}, false
}

func assertTinyGoBudgetSnapshot(t testing.TB, label string, payload tinyGoBudgetDebugPayload) {
	t.Helper()

	if len(payload.Budget.Generations) == 0 {
		t.Fatalf("%s TinyGo budget report has no generations", label)
	}
	owners := map[string]tinyGoBrowserBudgetOwner{}
	for _, generation := range payload.Budget.Generations {
		if generation.ID <= 0 {
			t.Fatalf("%s TinyGo budget generation has invalid id: %+v", label, generation)
		}
		if generation.State != "running" && generation.State != "exited" {
			t.Fatalf("%s TinyGo budget generation has invalid state: %+v", label, generation)
		}
		for _, owner := range generation.Owners {
			if owner.CurrentBytes < 0 || owner.HighWaterBytes < 0 || owner.CurrentCount < 0 || owner.HighWaterCount < 0 {
				t.Fatalf("%s TinyGo budget owner has negative value: %+v", label, owner)
			}
			if owner.HighWaterBytes < owner.CurrentBytes {
				t.Fatalf("%s TinyGo budget owner high-water bytes below current: %+v", label, owner)
			}
			if owner.HighWaterCount < owner.CurrentCount {
				t.Fatalf("%s TinyGo budget owner high-water count below current: %+v", label, owner)
			}
			owners[owner.Owner] = owner
		}
	}
	for _, owner := range []string{
		"wasm-linear-memory",
		"stored-bytes",
		"fetch-requests",
		"web-lock-requests",
		"opfs-write-streams",
		"opfs-runtime-tasks",
		"callback-queue",
	} {
		if _, ok := owners[owner]; !ok {
			t.Fatalf("%s TinyGo budget report missing owner %q", label, owner)
		}
	}
	if payload.Budget.Totals.HighWaterBytes < payload.Budget.Totals.CurrentBytes {
		t.Fatalf("%s TinyGo budget total high-water below current: %+v", label, payload.Budget.Totals)
	}
}

func driveUploadBudgetArtifactPath(t testing.TB) string {
	return filepath.Join("testdata", sanitizeTestName(t.Name())+".budget.json")
}

func driveUploadBudgetProfilesArtifactPath(t testing.TB) string {
	return filepath.Join("testdata", sanitizeTestName(t.Name())+".profiles.budget.json")
}
