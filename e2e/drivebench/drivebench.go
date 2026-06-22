//go:build !js

// Package drivebench owns the run.json artifact schema shared by the Drive
// startup benches. The unbundled bench (e2e/wasm) and the bundled production
// bench (e2e/releasewasm) cannot import each other's test symbols, so the
// cell-comparable schema and its artifact paths live here as the single
// contract both harnesses write.
package drivebench

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
)

// Run is one bench cell artifact: the resolved build identity, wall-clock
// milestones from navigation start to Drive content-ready, the browser-observed
// quickstart timing, the served bundle size, the Resource SDK connection timing,
// and, when a Go trace service is available, the captured runtime trace summary.
// It is written as run.json per cell so cells compare across runtime states and
// build modes. The bundled production bench has no Go trace service and no
// Resource SDK client, so it omits Trace and leaves ResourceConnection zero.
type Run struct {
	Timestamp          string       `json:"timestamp"`
	Compiler           string       `json:"compiler"`
	BuildMode          string       `json:"buildMode"`
	RuntimeState       string       `json:"runtimeState"`
	Cell               string       `json:"cell"`
	Milestones         Milestones   `json:"milestones"`
	Browser            Browser      `json:"browser"`
	ServedBundle       *Bundle      `json:"servedBundle,omitempty"`
	ResourceConnection ResourceConn `json:"resourceConnection"`
	Trace              *Trace       `json:"trace,omitempty"`
}

// Milestones records wall-clock milliseconds from navigation start to each boot
// milestone. ContentReadyMs is the moment getting-started.md is present in the
// file browser, which is also when the first file row renders.
type Milestones struct {
	LiveAppMs       int64 `json:"liveAppMs"`
	RouteAcceptedMs int64 `json:"routeAcceptedMs"`
	UnixfsVisibleMs int64 `json:"unixfsVisibleMs"`
	ContentReadyMs  int64 `json:"contentReadyMs"`
}

// Browser carries the browser-side quickstart timing reported by the Drive
// viewer, independent of the Go-side wall clock.
type Browser struct {
	ContentReadyMs            int     `json:"contentReadyMs"`
	QuickstartState           string  `json:"quickstartState"`
	QuickstartProgressReadyMs *int    `json:"quickstartProgressReadyMs,omitempty"`
	QuickstartContentReadyMs  *int    `json:"quickstartContentReadyMs,omitempty"`
	QuickstartFinishedMs      *int    `json:"quickstartFinishedMs,omitempty"`
	QuickstartPhases          []Phase `json:"quickstartPhases,omitempty"`
	DriveSeedResourceCalls    int     `json:"driveSeedResourceCalls,omitempty"`
	DriveSeedStartedMs        *int    `json:"driveSeedStartedMs,omitempty"`
	DriveSeedFinishedMs       *int    `json:"driveSeedFinishedMs,omitempty"`
	DriveSeedElapsedMs        *int    `json:"driveSeedElapsedMs,omitempty"`
}

// Phase is one browser-observed quickstart phase. The timestamps use the page
// performance.now timebase, matching the quickstart timing object published by
// the app while the bench is running.
type Phase struct {
	Name       string `json:"name"`
	StartedMs  int    `json:"startedMs"`
	FinishedMs *int   `json:"finishedMs,omitempty"`
	ElapsedMs  *int   `json:"elapsedMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Bundle is the served code payload measured from the built bundle on disk: the
// total JavaScript-plus-WASM module bytes, the WASM subtotal, and the module
// file count. It is a bundled-build metric, so only the bundled bench populates
// it. The page resource timeline cannot supply it because the runtime loads its
// modules inside a worker, off the main-thread timeline. The GoScript build is
// all-JavaScript, so WasmBytes is zero for GoScript and non-zero only for the
// TinyGo/Go-WASM builds.
type Bundle struct {
	TotalBytes int64 `json:"totalBytes"`
	WasmBytes  int64 `json:"wasmBytes"`
	FileCount  int   `json:"fileCount"`
}

// ResourceConn summarizes the Resource SDK connection timing for the session.
// The bundled production bench leaves it zero because it has no SDK client.
type ResourceConn struct {
	DurationMs     int64 `json:"durationMs"`
	Attempts       int   `json:"attempts"`
	StartupReloads int   `json:"startupReloads"`
}

// Trace summarizes the captured runtime trace and the paths of the raw trace
// plus its tracetool extraction.
type Trace struct {
	Bytes            int    `json:"bytes"`
	RuntimeTracePath string `json:"runtimeTracePath"`
	TracetoolPath    string `json:"tracetoolPath"`
	UserTasks        int    `json:"userTasks"`
	UserRegions      int    `json:"userRegions"`
	UserLogs         int    `json:"userLogs"`
	Tasks            []Task `json:"tasks,omitempty"`
}

// Task is an aggregate runtime-trace task summary keyed by Go user task type.
type Task struct {
	Type    string `json:"type"`
	Count   int    `json:"count"`
	TotalUs int64  `json:"totalUs"`
	MaxUs   int64  `json:"maxUs"`
}

// BrowserFromQuickstartTiming builds the browser timing artifact from the
// quickstart timing object published by app/quickstart/create.ts.
func BrowserFromQuickstartTiming(contentReadyMs int, raw map[string]any) Browser {
	browser := Browser{ContentReadyMs: contentReadyMs}
	if raw == nil {
		return browser
	}
	browser.QuickstartState, _ = raw["state"].(string)
	browser.QuickstartProgressReadyMs = optionalInt(raw, "progressReadyMs")
	browser.QuickstartContentReadyMs = optionalInt(raw, "contentReadyMs")
	browser.QuickstartFinishedMs = optionalInt(raw, "finishedMs")
	browser.QuickstartPhases = parsePhases(raw["phases"])
	browser.DriveSeedResourceCalls = CountDriveSeedResourceCalls(browser.QuickstartPhases)
	setDriveSeedWindow(&browser)
	return browser
}

// CountDriveSeedResourceCalls counts the timed Resource SDK calls that the
// Drive quickstart seed issues after populate-space begins and before content is
// ready. Wrapper phases such as populate-space and init-drive-unixfs are omitted
// because their leaf transaction phases carry the actual RPC calls.
func CountDriveSeedResourceCalls(phases []Phase) int {
	count := 0
	for _, phase := range phases {
		if slices.Contains(driveSeedResourcePhaseNames, phase.Name) {
			count++
		}
	}
	return count
}

// CellDir returns the artifact directory for one bench cell under the repo-root
// .bldr tree, grouped by the run stamp shared across the run's cells.
func CellDir(runStamp string, cell string) (string, error) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		return "", errors.Wrap(err, "find repo root")
	}
	return filepath.Join(repoRoot, ".bldr", "e2e-goscript-drive-bench", runStamp, cell), nil
}

// WriteArtifact writes data to path, creating the parent directory tree. Bench
// cells use it for the raw runtime.trace and tracetool.txt artifacts alongside
// run.json.
func WriteArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "create artifact dir")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errors.Wrap(err, "write artifact")
	}
	return nil
}

// MeasureBundleDir walks root and sums the served code-payload modules
// (.mjs/.js plus .wasm), reporting the total bytes, the WASM subtotal, and the
// module file count. It measures the built bundle on disk, the size the runtime
// loads, giving the Phase bundle-size baseline a stable number the page resource
// timeline cannot provide.
func MeasureBundleDir(root string) (*Bundle, error) {
	var bundle Bundle
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".mjs" && ext != ".js" && ext != ".wasm" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		bundle.TotalBytes += info.Size()
		bundle.FileCount++
		if ext == ".wasm" {
			bundle.WasmBytes += info.Size()
		}
		return nil
	})
	if walkErr != nil {
		return nil, errors.Wrap(walkErr, "walk bundle dir")
	}
	return &bundle, nil
}

// WriteRun marshals run as indented JSON and writes it to dir/run.json,
// returning the written path.
func WriteRun(dir string, run Run) (string, error) {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "marshal run.json")
	}
	path := filepath.Join(dir, "run.json")
	if err := WriteArtifact(path, data); err != nil {
		return "", err
	}
	return path, nil
}

var driveSeedResourcePhaseNames = []string{
	"init-drive-unixfs-new-transaction",
	"init-drive-unixfs-apply-op",
	"init-drive-unixfs-commit",
	"init-drive-unixfs-discard",
	"write-drive-starter-guide-access",
	"write-drive-starter-guide-upload",
	"create-drive-settings-get-object",
	"create-drive-settings-new-transaction",
	"create-drive-settings-apply-op",
	"create-drive-settings-commit",
	"create-drive-settings-discard",
}

func parsePhases(raw any) []Phase {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	phases := make([]Phase, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name == "" {
			continue
		}
		phase := Phase{
			Name:       name,
			StartedMs:  intValue(m["startedMs"]),
			FinishedMs: optionalInt(m, "finishedMs"),
			ElapsedMs:  optionalInt(m, "elapsedMs"),
		}
		phase.Error, _ = m["error"].(string)
		phases = append(phases, phase)
	}
	return phases
}

func setDriveSeedWindow(browser *Browser) {
	for _, phase := range browser.QuickstartPhases {
		if phase.Name == "populate-space" {
			browser.DriveSeedStartedMs = &phase.StartedMs
			browser.DriveSeedFinishedMs = phase.FinishedMs
			browser.DriveSeedElapsedMs = phase.ElapsedMs
			return
		}
	}
	var start *int
	var finish *int
	for _, phase := range browser.QuickstartPhases {
		if !strings.HasPrefix(phase.Name, "init-drive-") &&
			!strings.HasPrefix(phase.Name, "write-drive-starter-guide-") &&
			!strings.HasPrefix(phase.Name, "create-drive-settings") {
			continue
		}
		started := phase.StartedMs
		if start == nil || started < *start {
			start = &started
		}
		if phase.FinishedMs != nil && (finish == nil || *phase.FinishedMs > *finish) {
			finished := *phase.FinishedMs
			finish = &finished
		}
	}
	browser.DriveSeedStartedMs = start
	browser.DriveSeedFinishedMs = finish
	if start != nil && finish != nil {
		elapsed := *finish - *start
		browser.DriveSeedElapsedMs = &elapsed
	}
}

func optionalInt(m map[string]any, key string) *int {
	switch v := m[key].(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		return &v
	default:
		return nil
	}
}

func intValue(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
