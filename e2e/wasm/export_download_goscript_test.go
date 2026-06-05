//go:build !skip_e2e && !js

package wasm

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
	space_http_export "github.com/s4wave/spacewave/core/space/http/export"
)

const projectedExportPluginPathPrefix = "/p/spacewave-core"

func TestGoScriptProjectedExportDownloadBrowserParity(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := testHarness.NewCleanSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during projected export/download gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during projected export/download gate: %+v", report)
		}
	}()

	scenario := CreateDriveScenario(t, testHarness, sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, testHarness, page)

	seed := seedGoScriptProjectedExportFixtures(t, page)
	projectedObject := buildProjectedObjectContentPath(
		scenario.GetSessionIndex(),
		scenario.GetSpaceID(),
		seed.objectKey,
		"",
	)

	fileDownload := downloadViaAnchor(
		t,
		page,
		projectedExportPluginPathPrefix+"/fs/"+buildProjectedObjectContentPath(
			scenario.GetSessionIndex(),
			scenario.GetSpaceID(),
			seed.objectKey,
			seed.fileName,
		),
		seed.fileName,
	)
	if string(fileDownload) != "row7 single file\n" {
		t.Fatalf("single-file download body mismatch: %q", string(fileDownload))
	}

	wholeSpaceZip := readZipEntries(t, downloadViaAnchor(
		t,
		page,
		projectedExportPluginPathPrefix+"/export/"+buildProjectedSpaceRootPath(
			scenario.GetSessionIndex(),
			scenario.GetSpaceID(),
		),
		"",
	))
	assertZipText(t, wholeSpaceZip, path.Join(seed.objectKey, "-", seed.fileName), "row7 single file\n")
	assertZipText(t, wholeSpaceZip, path.Join(seed.objectKey, "-", seed.dirName, "alpha.txt"), "row7 dir alpha\n")
	assertZipText(t, wholeSpaceZip, path.Join(seed.objectKey, "-", seed.dirName, "nested", "beta.txt"), "row7 dir beta\n")

	directoryZip := readZipEntries(t, downloadViaAnchor(
		t,
		page,
		projectedExportPluginPathPrefix+"/export/"+buildProjectedObjectContentPath(
			scenario.GetSessionIndex(),
			scenario.GetSpaceID(),
			seed.objectKey,
			seed.dirName,
		),
		seed.dirName+".zip",
	))
	assertZipText(t, directoryZip, path.Join(seed.dirName, "alpha.txt"), "row7 dir alpha\n")
	assertZipText(t, directoryZip, path.Join(seed.dirName, "nested", "beta.txt"), "row7 dir beta\n")

	batchZip := readZipEntries(t, downloadViaAnchor(
		t,
		page,
		projectedExportPluginPathPrefix+"/export-batch/"+projectedObject+"/"+
			encodeExportBatchRequest(t, []string{seed.dirName, seed.fileName})+
			"/selection.zip",
		"selection.zip",
	))
	assertZipText(t, batchZip, path.Join(seed.dirName, "alpha.txt"), "row7 dir alpha\n")
	assertZipText(t, batchZip, path.Join(seed.dirName, "nested", "beta.txt"), "row7 dir beta\n")
	assertZipText(t, batchZip, seed.fileName, "row7 single file\n")
}

type projectedExportSeed struct {
	objectKey string
	fileName  string
	dirName   string
}

func seedGoScriptProjectedExportFixtures(t testing.TB, page playwright.Page) projectedExportSeed {
	t.Helper()

	raw, err := page.Evaluate(`async () => {
		function streamFromText(text) {
			return new ReadableStream({
				start(controller) {
					controller.enqueue(new TextEncoder().encode(text))
					controller.close()
				},
			})
		}
		async function readText(handle, length = 0n) {
			const read = await handle.readAt(0n, length)
			return new TextDecoder().decode(read.data)
		}
		async function expectText(rootHandle, filePath, text, abort) {
			const file = await rootHandle.lookupPath(filePath, abort)
			try {
				const got = await readText(file.handle, BigInt(new TextEncoder().encode(text).length))
				if (got !== text) {
					throw new Error('seed readback mismatch for ' + filePath + ': ' + got)
				}
			} finally {
				file.handle.release()
			}
		}

		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		const FSHandle = debug?.FSHandle
		const unixfsObjectKey = debug?.UNIXFS_OBJECT_KEY
		if (!match || !root || !mountSpace || !FSHandle || !unixfsObjectKey) {
			return { error: 'missing direct Drive route or debug FSHandle context' }
		}

		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		const mountedResources = {
			session: null,
			space: null,
			world: null,
			rootHandle: null,
		}
		let step = 'mount-session'
		try {
			const abort = AbortSignal.timeout(120000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session', step }

			step = 'mount-space'
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
				spaceResp: {
					sharedObjectRef: {
						providerResourceRef: {
							id: sharedObjectId,
						},
					},
				},
				abortSignal: abort,
				cleanup,
			})

			step = 'access-world'
			mountedResources.world = await mountedResources.space.accessWorldState(true, abort)
			step = 'access-unixfs'
			const access = await mountedResources.world.accessTypedObject(unixfsObjectKey, abort)
			if (!access?.resourceId) return { error: 'accessTypedObject returned no UnixFS resource id', step }
			mountedResources.rootHandle = new FSHandle(
				mountedResources.world.getResourceRef().createRef(access.resourceId),
			)
			const rootHandle = mountedResources.rootHandle

			step = 'write-fixtures'
			const suffix = String(Date.now()) + '-' + Math.random().toString(36).slice(2, 8)
			const fileName = 'row7-file-' + suffix + '.txt'
			const dirName = 'row7-dir-' + suffix
			await rootHandle.uploadFile(
				fileName,
				17n,
				streamFromText('row7 single file\n'),
				0o644,
				undefined,
				abort,
			)
			await rootHandle.uploadTree(
				[
					{ kind: 'directory', path: dirName, mode: 0o755 },
					{
						kind: 'file',
						path: dirName + '/alpha.txt',
						totalSize: 15n,
						stream: streamFromText('row7 dir alpha\n'),
						mode: 0o644,
					},
					{ kind: 'directory', path: dirName + '/nested', mode: 0o755 },
					{
						kind: 'file',
						path: dirName + '/nested/beta.txt',
						totalSize: 14n,
						stream: streamFromText('row7 dir beta\n'),
						mode: 0o644,
					},
				],
				undefined,
				abort,
			)
			await expectText(rootHandle, fileName, 'row7 single file\n', abort)
			await expectText(rootHandle, dirName + '/alpha.txt', 'row7 dir alpha\n', abort)
			await expectText(rootHandle, dirName + '/nested/beta.txt', 'row7 dir beta\n', abort)

			return {
				objectKey: unixfsObjectKey,
				fileName,
				dirName,
			}
		} catch (err) {
			return { error: String(err?.stack ?? err), step }
		} finally {
			mountedResources.rootHandle?.release?.()
			mountedResources.world?.release?.()
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("seed projected export fixtures: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected projected export seed result %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("projected export seed failed at %s: %s", stringField(result, "step"), errMsg)
	}
	objectKey := stringField(result, "objectKey")
	if objectKey == "" {
		t.Fatalf("projected export seed returned no object key: %#v", result)
	}
	fileName := stringField(result, "fileName")
	dirName := stringField(result, "dirName")
	if fileName == "" || dirName == "" {
		t.Fatalf("projected export seed returned incomplete fixture names: %#v", result)
	}
	return projectedExportSeed{
		objectKey: objectKey,
		fileName:  fileName,
		dirName:   dirName,
	}
}

func downloadViaAnchor(t testing.TB, page playwright.Page, targetURL, filename string) []byte {
	t.Helper()

	probe := probeDownloadURL(t, page, targetURL)
	if probe.status < 200 || probe.status >= 300 {
		if probe.status == 0 && len(probe.body) != 0 {
			t.Logf(
				"preflight fetch %s returned browser status=0 with content-type=%q content-disposition=%q bytes=%d; continuing to download proof",
				targetURL,
				probe.contentType,
				probe.contentDisposition,
				len(probe.body),
			)
		}
		if probe.status != 0 || len(probe.body) == 0 {
			t.Fatalf(
				"preflight fetch %s status=%d content-type=%q content-disposition=%q body=%q",
				targetURL,
				probe.status,
				probe.contentType,
				probe.contentDisposition,
				string(probe.body),
			)
		}
	}

	timeout := float64(120000)
	download, err := page.ExpectDownload(func() error {
		_, evalErr := page.Evaluate(`async ({ url, filename }) => {
			const downloadURL = globalThis.__s4wave_debug?.downloadURL
			if (typeof downloadURL !== 'function') {
				throw new Error('debug downloadURL helper is not available')
			}
			await downloadURL(url, filename ?? '')
		}`, map[string]any{
			"url":      targetURL,
			"filename": filename,
		})
		return evalErr
	}, playwright.PageExpectDownloadOptions{Timeout: &timeout})
	if err != nil {
		t.Fatalf("download %s: %v", targetURL, err)
	}
	if filename != "" && download.SuggestedFilename() != filename {
		t.Fatalf("download %s suggested filename=%q want %q", targetURL, download.SuggestedFilename(), filename)
	}
	if err := download.Failure(); err != nil {
		t.Fatalf(
			"download %s failed after preflight status=%d content-type=%q content-disposition=%q bytes=%d: %v",
			targetURL,
			probe.status,
			probe.contentType,
			probe.contentDisposition,
			len(probe.body),
			err,
		)
	}

	savePath := filepath.Join(t.TempDir(), download.SuggestedFilename())
	if savePath == "" || strings.HasSuffix(savePath, string(os.PathSeparator)) {
		savePath = filepath.Join(t.TempDir(), "download")
	}
	if err := download.SaveAs(savePath); err != nil {
		t.Fatalf("save download %s: %v", targetURL, err)
	}
	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read download %s: %v", targetURL, err)
	}
	return data
}

type downloadProbe struct {
	status             int
	contentType        string
	contentDisposition string
	body               []byte
}

func probeDownloadURL(t testing.TB, page playwright.Page, targetURL string) downloadProbe {
	t.Helper()

	raw, err := page.Evaluate(`async ({ url }) => {
		function encodeBase64(bytes) {
			let binary = ''
			for (let i = 0; i < bytes.length; i += 0x8000) {
				binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
			}
			return btoa(binary)
		}
		try {
			const resp = await fetch(url, {
				cache: 'no-store',
				signal: AbortSignal.timeout(120000),
			})
			const bytes = new Uint8Array(await resp.arrayBuffer())
			return {
				status: resp.status,
				contentType: resp.headers.get('content-type') ?? '',
				contentDisposition: resp.headers.get('content-disposition') ?? '',
				bodyBase64: encodeBase64(bytes),
			}
		} catch (err) {
			return {
				error: String(err?.stack ?? err),
				status: 0,
				contentType: '',
				contentDisposition: '',
				bodyBase64: '',
			}
		}
	}`, map[string]any{"url": targetURL})
	if err != nil {
		t.Fatalf("preflight fetch %s: %v", targetURL, err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected preflight fetch result %T: %#v", raw, raw)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("preflight fetch %s failed: %s", targetURL, errMsg)
	}
	bodyBase64 := stringField(result, "bodyBase64")
	body, err := base64.StdEncoding.DecodeString(bodyBase64)
	if err != nil {
		t.Fatalf("decode preflight fetch body for %s: %v", targetURL, err)
	}
	return downloadProbe{
		status:             int(numberField(result, "status")),
		contentType:        stringField(result, "contentType"),
		contentDisposition: stringField(result, "contentDisposition"),
		body:               body,
	}
}

func numberField(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	default:
		return 0
	}
}

func buildProjectedSpaceRootPath(sessionIndex uint32, sharedObjectID string) string {
	return "u/" + strconv.FormatUint(uint64(sessionIndex), 10) + "/so/" + url.PathEscape(sharedObjectID)
}

func buildProjectedObjectContentPath(sessionIndex uint32, sharedObjectID, objectKey, objectPath string) string {
	projectedPath := buildProjectedSpaceRootPath(sessionIndex, sharedObjectID) + "/-/" + escapeProjectedSegments(objectKey)
	if normalizeProjectedSubpath(objectPath) == "" {
		return projectedPath + "/-"
	}
	return projectedPath + "/-/" + escapeProjectedSegments(objectPath)
}

func escapeProjectedSegments(value string) string {
	segments := strings.Split(value, "/")
	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(segment))
	}
	return strings.Join(escaped, "/")
}

func normalizeProjectedSubpath(value string) string {
	segments := strings.Split(value, "/")
	normalized := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		normalized = append(normalized, segment)
	}
	return strings.Join(normalized, "/")
}

func encodeExportBatchRequest(t testing.TB, paths []string) string {
	t.Helper()

	data, err := (&space_http_export.ExportBatchRequest{Paths: paths}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal export batch request: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func readZipEntries(t testing.TB, data []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		name := strings.TrimPrefix(file.Name, "/")
		if file.FileInfo().IsDir() {
			entries[name] = ""
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		body, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close zip entry %s: %v", file.Name, closeErr)
		}
		entries[name] = string(body)
	}
	return entries
}

func assertZipText(t testing.TB, entries map[string]string, name, want string) {
	t.Helper()

	got, ok := entries[name]
	if !ok {
		names := make([]string, 0, len(entries))
		for entryName := range entries {
			names = append(names, entryName)
		}
		slices.Sort(names)
		t.Fatalf("zip missing %s; entries=%v", name, names)
	}
	if got != want {
		t.Fatalf("zip entry %s body=%q want %q", name, got, want)
	}
}
