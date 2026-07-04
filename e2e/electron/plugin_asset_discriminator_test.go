//go:build !skip_e2e && !js

package electron

import (
	"context"
	"fmt"
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

const desktopModuleLoadIssueAssetPath = "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs"

// TIER: nightly
func TestRetainedStatePluginAssetFetchDiscriminator(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	initialPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	initialURL := initialPage.URL()
	initialLogPath := h.LastLogFilePath()
	if initialLogPath == "" {
		t.Fatal("expected initial devtool log path")
	}

	if err := h.Relaunch(ctx); err != nil {
		t.Fatal(err)
	}
	retainedPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	retainedLogPath := h.LastLogFilePath()
	if retainedLogPath == "" || retainedLogPath == initialLogPath {
		t.Fatalf("expected distinct retained-start log path, got initial=%q retained=%q", initialLogPath, retainedLogPath)
	}
	sentinels, err := seedProtectedStateSentinels(h, retainedPage)
	if err != nil {
		t.Fatal(err)
	}

	result, err := fetchPluginAssetDiscriminator(retainedPage, desktopModuleLoadIssueAssetPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != desktopModuleLoadIssueAssetPath {
		t.Fatalf("captured URL %q, want %q", result.URL, desktopModuleLoadIssueAssetPath)
	}
	if result.Status == 0 && result.FetchError == "" {
		t.Fatalf("expected HTTP status or fetch error in discriminator: %#v", result)
	}

	artifactPath, err := writePluginAssetDiscriminatorArtifact(
		h,
		initialURL,
		retainedPage.URL(),
		initialLogPath,
		retainedLogPath,
		result,
	)
	if err != nil {
		t.Fatalf("write plugin asset discriminator artifact: %v", err)
	}
	t.Logf("retained plugin asset fetch discriminator: %s", artifactPath)

	bootResult, err := captureDesktopBootCompatibilityDiscriminator(retainedPage)
	if err != nil {
		t.Fatal(err)
	}
	if bootResult.Classification != "shared-boot-reached" {
		t.Fatalf("expected retained Electron startup to reach shared boot before entrypoint import, got %#v", bootResult)
	}
	bootArtifactPath, err := writeDesktopBootCompatibilityArtifact(
		h,
		initialURL,
		retainedPage.URL(),
		initialLogPath,
		retainedLogPath,
		bootResult,
	)
	if err != nil {
		t.Fatalf("write desktop boot compatibility artifact: %v", err)
	}
	t.Logf("retained desktop boot compatibility discriminator: %s", bootArtifactPath)

	sentinelResult, err := verifyProtectedStateSentinels(retainedPage, sentinels)
	if err != nil {
		t.Fatal(err)
	}
	sentinelArtifactPath, err := writeProtectedStateSentinelArtifact(
		h,
		initialURL,
		retainedPage.URL(),
		initialLogPath,
		retainedLogPath,
		sentinelResult,
	)
	if err != nil {
		t.Fatalf("write protected state sentinel artifact: %v", err)
	}
	t.Logf("retained protected state sentinels: %s", sentinelArtifactPath)
}

type pluginAssetFetchDiscriminator struct {
	URL               string
	Status            int
	FetchSource       string
	RuntimeError      string
	PluginAssetResult string
	ContentType       string
	BodyPrefix        string
	FetchError        string
}

func fetchPluginAssetDiscriminator(
	page playwright.Page,
	assetPath string,
) (*pluginAssetFetchDiscriminator, error) {
	raw, err := page.Evaluate(`async (url) => {
		try {
			const response = await fetch(url, { cache: 'no-store' })
			const body = await response.clone().text()
			return JSON.stringify({
				url,
				status: response.status,
				fetchSource: response.headers.get('X-Bldr-Fetch-Source') ?? '',
				runtimeError: response.headers.get('X-Bldr-Runtime-Fetch-Error') ?? '',
				pluginAssetResult: response.headers.get('X-Bldr-Plugin-Asset-Fetch-Result') ?? '',
				contentType: response.headers.get('content-type') ?? '',
				bodyPrefix: body.slice(0, 300),
				fetchError: '',
			})
		} catch (err) {
			return JSON.stringify({
				url,
				status: 0,
				fetchSource: '',
				runtimeError: '',
				pluginAssetResult: '',
				contentType: '',
				bodyPrefix: '',
				fetchError: err instanceof Error ? err.message : String(err),
			})
		}
	}`, assetPath)
	if err != nil {
		return nil, err
	}
	encoded, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected plugin asset discriminator result %T: %#v", raw, raw)
	}
	result, err := parsePluginAssetFetchDiscriminator(encoded)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func writePluginAssetDiscriminatorArtifact(
	h *Harness,
	initialURL string,
	retainedURL string,
	initialLogPath string,
	retainedLogPath string,
	result *pluginAssetFetchDiscriminator,
) (string, error) {
	var arena fastjson.Arena
	encodedResult := result.appendJSON(&arena).MarshalTo(nil)
	path := filepath.Join(h.ArtifactDir(), "retained-plugin-asset-fetch-discriminator.txt")
	classification := classifyPluginAssetFetch(result)
	ownerClass := pluginAssetFailureOwner(classification)
	lines := []string{
		"smoke=retained-plugin-asset-fetch-discriminator",
		"asset_url=" + result.URL,
		"initial_url=" + initialURL,
		"retained_url=" + retainedURL,
		"state_root=" + h.StateRoot(),
		"spacewave_data_dir=" + h.SpacewaveDataRoot(),
		"initial_log=" + initialLogPath,
		"retained_log=" + retainedLogPath,
		"status=" + strconv.Itoa(result.Status),
		"fetch_source=" + result.FetchSource,
		"runtime_error=" + result.RuntimeError,
		"plugin_asset_result=" + result.PluginAssetResult,
		"content_type=" + result.ContentType,
		"fetch_error=" + result.FetchError,
		"classification=" + classification,
		"owner_class=" + ownerClass,
		"result_json=" + string(encodedResult),
		"proof=renderer_context_fetch_captured",
		"proof=retained_state_root_reused",
		"proof=cleanup_not_performed_by_discriminator",
		"",
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func classifyPluginAssetFetch(result *pluginAssetFetchDiscriminator) string {
	switch {
	case result.FetchError != "":
		return "fetch-error"
	case result.Status == 200 && result.PluginAssetResult == "live":
		return "live-root-module"
	case result.Status == 404 || result.PluginAssetResult == "missing":
		return "plugin-asset-missing"
	case result.Status == 503 || result.RuntimeError == "runtime-unavailable" || result.PluginAssetResult == "runtime-unavailable":
		return "runtime-unavailable"
	case result.Status == 410 || result.PluginAssetResult == "generation-closed":
		return "generation-closed"
	case result.FetchSource == "":
		return "bldr-normalization-bypassed"
	default:
		return "other-typed-result"
	}
}

func pluginAssetFailureOwner(classification string) string {
	switch classification {
	case "live-root-module":
		return "nested-import-or-module-evaluation"
	case "plugin-asset-missing":
		return "selected-assets-filesystem-or-manifest-package-closure"
	case "runtime-unavailable":
		return "webdocument-runtime-bridge-or-plugin-runtime-readiness"
	case "generation-closed":
		return "runtime-plugin-generation-lease"
	case "bldr-normalization-bypassed":
		return "serviceworker-electron-forwarding-bypass"
	case "fetch-error":
		return "browser-fetch-or-network-error"
	default:
		return "typed-result-unclassified"
	}
}

type desktopBootCompatibilityDiscriminator struct {
	URL                 string
	HasBootStatus       bool
	BootStatusPhase     string
	StoredBootVersion   string
	ResetAttemptVersion string
	ScriptSources       []string
	Classification      string
}

func captureDesktopBootCompatibilityDiscriminator(
	page playwright.Page,
) (*desktopBootCompatibilityDiscriminator, error) {
	raw, err := page.Evaluate(`() => {
		const bootStatus = globalThis.__swBootStatus
		const scriptSources = Array.from(document.scripts)
			.map((script) => script.getAttribute('src') ?? '')
			.filter(Boolean)
		const storedBootVersion = localStorage.getItem('spacewave-browser-app-state-version') ?? ''
		const resetAttemptVersion = sessionStorage.getItem('spacewave-browser-app-state-reset-attempted') ?? ''
		const hasBootStatus = typeof bootStatus !== 'undefined'
		let classification = 'other'
		if (hasBootStatus || storedBootVersion || resetAttemptVersion) {
			classification = 'shared-boot-reached'
		} else if (scriptSources.includes('./entrypoint/entrypoint.mjs')) {
			classification = 'electron-direct-entrypoint-bypass'
		}
		return JSON.stringify({
			url: window.location.href,
			hasBootStatus,
			bootStatusPhase: bootStatus?.phase ?? '',
			storedBootVersion,
			resetAttemptVersion,
			scriptSources,
			classification,
		})
	}`)
	if err != nil {
		return nil, err
	}
	encoded, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected desktop boot discriminator result %T: %#v", raw, raw)
	}
	result, err := parseDesktopBootCompatibilityDiscriminator(encoded)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func writeDesktopBootCompatibilityArtifact(
	h *Harness,
	initialURL string,
	retainedURL string,
	initialLogPath string,
	retainedLogPath string,
	result *desktopBootCompatibilityDiscriminator,
) (string, error) {
	var arena fastjson.Arena
	encodedResult := result.appendJSON(&arena).MarshalTo(nil)
	path := filepath.Join(h.ArtifactDir(), "retained-desktop-boot-compatibility-discriminator.txt")
	lines := []string{
		"smoke=retained-desktop-boot-compatibility-discriminator",
		"initial_url=" + initialURL,
		"retained_url=" + retainedURL,
		"state_root=" + h.StateRoot(),
		"spacewave_data_dir=" + h.SpacewaveDataRoot(),
		"initial_log=" + initialLogPath,
		"retained_log=" + retainedLogPath,
		"classification=" + result.Classification,
		"has_boot_status=" + strconv.FormatBool(result.HasBootStatus),
		"boot_status_phase=" + result.BootStatusPhase,
		"stored_boot_version=" + result.StoredBootVersion,
		"reset_attempt_version=" + result.ResetAttemptVersion,
		"script_sources=" + strings.Join(result.ScriptSources, ","),
		"result_json=" + string(encodedResult),
		"proof=renderer_context_boot_state_captured",
		"proof=retained_state_root_reused",
		"proof=cleanup_not_performed_by_discriminator",
	}
	if result.Classification == "shared-boot-reached" {
		lines = append(lines, "proof=shared_stable_boot_before_electron_entrypoint")
	} else {
		lines = append(lines, "phase2_target=shared-stable-boot-before-electron-entrypoint")
	}
	lines = append(lines, "")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

type protectedStateSentinels struct {
	FileValues       map[string]string
	WebStorageValues map[string]string
}

type protectedStateSentinelResult struct {
	FileSurvivors       map[string]bool
	WebStorageSurvivors map[string]bool
	BrowserDurableAPIs  map[string]string
}

func seedProtectedStateSentinels(
	h *Harness,
	page playwright.Page,
) (*protectedStateSentinels, error) {
	sentinels := &protectedStateSentinels{
		FileValues: map[string]string{
			filepath.Join(h.StateRoot(), "plugin", "state", "web", "e2e-native-plugin-state-sentinel.txt"):            "native-plugin-state-survives-discriminator\n",
			filepath.Join(h.StateRoot(), "protected-state-sentinels", "sessions", "list", "e2e-session-sentinel.txt"): "session-state-survives-discriminator\n",
			filepath.Join(h.StateRoot(), "protected-state-sentinels", "browser-durable-storage", "e2e-sentinel.txt"):  "browser-durable-storage-survives-discriminator-without-opfs-indexeddb-api\n",
		},
		WebStorageValues: map[string]string{
			"app-persistent":                   `{"json":{"e2eSentinel":"app-persistent-survives-discriminator"}}`,
			"tab-state-home":                   `{"json":{"e2eSentinel":"active-tab-state-survives-discriminator"}}`,
			"spacewave-e2e-unclassified-local": "unclassified-local-storage-survives-discriminator",
		},
	}
	for path, value := range sentinels.FileValues {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			return nil, err
		}
	}
	if err := setLocalStorageSentinels(page, sentinels.WebStorageValues); err != nil {
		return nil, err
	}
	return sentinels, nil
}

func setLocalStorageSentinels(page playwright.Page, values map[string]string) error {
	var arena fastjson.Arena
	encoded := appendStringMapJSON(&arena, values).MarshalTo(nil)
	_, err := page.Evaluate(`(encoded) => {
		const values = JSON.parse(encoded)
		for (const [key, value] of Object.entries(values)) {
			localStorage.setItem(key, value)
		}
	}`, string(encoded))
	return err
}

func verifyProtectedStateSentinels(
	page playwright.Page,
	sentinels *protectedStateSentinels,
) (*protectedStateSentinelResult, error) {
	result := &protectedStateSentinelResult{
		FileSurvivors:       make(map[string]bool, len(sentinels.FileValues)),
		WebStorageSurvivors: make(map[string]bool, len(sentinels.WebStorageValues)),
		BrowserDurableAPIs: map[string]string{
			"indexedDB.deleteDatabase":       "not-called",
			"navigator.storage.getDirectory": "not-called",
		},
	}
	for path, want := range sentinels.FileValues {
		got, err := os.ReadFile(path)
		if err != nil {
			result.FileSurvivors[path] = false
			return result, fmt.Errorf("read protected sentinel %q: %w", path, err)
		}
		result.FileSurvivors[path] = string(got) == want
		if !result.FileSurvivors[path] {
			return result, fmt.Errorf("protected sentinel %q changed", path)
		}
	}

	var arena fastjson.Arena
	encoded := appendStringMapJSON(&arena, sentinels.WebStorageValues).MarshalTo(nil)
	raw, err := page.Evaluate(`(encoded) => {
		const values = JSON.parse(encoded)
		const survivors = {}
		function sentinelValue(raw) {
			try {
				const parsed = JSON.parse(raw)
				return parsed?.json?.e2eSentinel ?? null
			} catch {
				return null
			}
		}
		for (const [key, value] of Object.entries(values)) {
			const actual = localStorage.getItem(key)
			const expectedSentinel = sentinelValue(value)
			survivors[key] = expectedSentinel
				? sentinelValue(actual) === expectedSentinel
				: actual === value
		}
		return JSON.stringify(survivors)
	}`, string(encoded))
	if err != nil {
		return nil, err
	}
	survivorsJSON, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected Web Storage sentinel result %T: %#v", raw, raw)
	}
	webStorageSurvivors, err := parseBoolMapJSON(survivorsJSON)
	if err != nil {
		return nil, err
	}
	result.WebStorageSurvivors = webStorageSurvivors
	for key, survived := range result.WebStorageSurvivors {
		if !survived {
			return result, fmt.Errorf("Web Storage sentinel %q changed", key)
		}
	}
	return result, nil
}

func writeProtectedStateSentinelArtifact(
	h *Harness,
	initialURL string,
	retainedURL string,
	initialLogPath string,
	retainedLogPath string,
	result *protectedStateSentinelResult,
) (string, error) {
	var arena fastjson.Arena
	encodedResult := result.appendJSON(&arena).MarshalTo(nil)
	path := filepath.Join(h.ArtifactDir(), "retained-protected-state-sentinels.txt")
	lines := []string{
		"smoke=retained-protected-state-sentinels",
		"initial_url=" + initialURL,
		"retained_url=" + retainedURL,
		"state_root=" + h.StateRoot(),
		"spacewave_data_dir=" + h.SpacewaveDataRoot(),
		"initial_log=" + initialLogPath,
		"retained_log=" + retainedLogPath,
		"result_json=" + string(encodedResult),
		"proof=native_plugin_state_sentinel_survived",
		"proof=session_state_fixture_sentinel_survived",
		"proof=browser_durable_storage_fixture_sentinel_survived_without_opfs_indexeddb_api",
		"proof=unclassified_web_storage_sentinels_survived",
		"proof=discriminator_only_run_performed_no_cleanup",
		"",
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func parsePluginAssetFetchDiscriminator(data string) (*pluginAssetFetchDiscriminator, error) {
	var parser fastjson.Parser
	value, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	return &pluginAssetFetchDiscriminator{
		URL:               string(value.GetStringBytes("url")),
		Status:            value.GetInt("status"),
		FetchSource:       string(value.GetStringBytes("fetchSource")),
		RuntimeError:      string(value.GetStringBytes("runtimeError")),
		PluginAssetResult: string(value.GetStringBytes("pluginAssetResult")),
		ContentType:       string(value.GetStringBytes("contentType")),
		BodyPrefix:        string(value.GetStringBytes("bodyPrefix")),
		FetchError:        string(value.GetStringBytes("fetchError")),
	}, nil
}

func (d *pluginAssetFetchDiscriminator) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("url", arena.NewString(d.URL))
	obj.Set("status", arena.NewNumberInt(d.Status))
	obj.Set("fetchSource", arena.NewString(d.FetchSource))
	obj.Set("runtimeError", arena.NewString(d.RuntimeError))
	obj.Set("pluginAssetResult", arena.NewString(d.PluginAssetResult))
	obj.Set("contentType", arena.NewString(d.ContentType))
	obj.Set("bodyPrefix", arena.NewString(d.BodyPrefix))
	obj.Set("fetchError", arena.NewString(d.FetchError))
	return obj
}

func parseDesktopBootCompatibilityDiscriminator(data string) (*desktopBootCompatibilityDiscriminator, error) {
	var parser fastjson.Parser
	value, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	scriptSourceValues := value.GetArray("scriptSources")
	scriptSources := make([]string, 0, len(scriptSourceValues))
	for _, scriptSource := range scriptSourceValues {
		scriptSources = append(scriptSources, string(scriptSource.GetStringBytes()))
	}
	return &desktopBootCompatibilityDiscriminator{
		URL:                 string(value.GetStringBytes("url")),
		HasBootStatus:       value.GetBool("hasBootStatus"),
		BootStatusPhase:     string(value.GetStringBytes("bootStatusPhase")),
		StoredBootVersion:   string(value.GetStringBytes("storedBootVersion")),
		ResetAttemptVersion: string(value.GetStringBytes("resetAttemptVersion")),
		ScriptSources:       scriptSources,
		Classification:      string(value.GetStringBytes("classification")),
	}, nil
}

func (d *desktopBootCompatibilityDiscriminator) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("url", arena.NewString(d.URL))
	if d.HasBootStatus {
		obj.Set("hasBootStatus", arena.NewTrue())
	} else {
		obj.Set("hasBootStatus", arena.NewFalse())
	}
	obj.Set("bootStatusPhase", arena.NewString(d.BootStatusPhase))
	obj.Set("storedBootVersion", arena.NewString(d.StoredBootVersion))
	obj.Set("resetAttemptVersion", arena.NewString(d.ResetAttemptVersion))
	scriptSources := arena.NewArray()
	for _, scriptSource := range d.ScriptSources {
		scriptSources.SetArrayItem(len(scriptSources.GetArray()), arena.NewString(scriptSource))
	}
	obj.Set("scriptSources", scriptSources)
	obj.Set("classification", arena.NewString(d.Classification))
	return obj
}

func (r *protectedStateSentinelResult) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("fileSurvivors", appendBoolMapJSON(arena, r.FileSurvivors))
	obj.Set("webStorageSurvivors", appendBoolMapJSON(arena, r.WebStorageSurvivors))
	obj.Set("browserDurableAPIs", appendStringMapJSON(arena, r.BrowserDurableAPIs))
	return obj
}

func appendStringMapJSON(arena *fastjson.Arena, values map[string]string) *fastjson.Value {
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		obj.Set(key, arena.NewString(values[key]))
	}
	return obj
}

func appendBoolMapJSON(arena *fastjson.Arena, values map[string]bool) *fastjson.Value {
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if values[key] {
			obj.Set(key, arena.NewTrue())
		} else {
			obj.Set(key, arena.NewFalse())
		}
	}
	return obj
}

func parseBoolMapJSON(data string) (map[string]bool, error) {
	var parser fastjson.Parser
	value, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	obj := value.GetObject()
	values := make(map[string]bool, obj.Len())
	obj.Visit(func(key []byte, value *fastjson.Value) {
		values[string(key)] = value.GetBool()
	})
	return values, nil
}
