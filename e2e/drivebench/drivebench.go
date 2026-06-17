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
	ContentReadyMs            int    `json:"contentReadyMs"`
	QuickstartState           string `json:"quickstartState"`
	QuickstartProgressReadyMs *int   `json:"quickstartProgressReadyMs,omitempty"`
	QuickstartContentReadyMs  *int   `json:"quickstartContentReadyMs,omitempty"`
	QuickstartFinishedMs      *int   `json:"quickstartFinishedMs,omitempty"`
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
