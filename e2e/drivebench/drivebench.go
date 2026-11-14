//go:build !js

// Package drivebench owns the run.json artifact schema shared by the Drive
// startup benches. The unbundled bench (e2e/wasm) and the bundled production
// bench (e2e/releasewasm) cannot import each other's test symbols, so the
// cell-comparable schema and its artifact paths live here as the single
// contract both harnesses write.
package drivebench

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
)

// Run is one bench cell artifact: the resolved build identity, wall-clock
// milestones from navigation start to Drive content-ready, the browser-observed
// quickstart timing, the served bundle size, the Resource SDK connection timing,
// and optional trace/profile summaries. It is written as run.json per cell so
// cells compare across runtime states and build modes. The bundled production
// bench has no Resource SDK client, so ResourceConnection remains zero; an
// opt-in startup trace may populate Trace.
type Run struct {
	// Timestamp is when the run was recorded.
	Timestamp string `json:"timestamp"`
	// Compiler names the toolchain that built the runtime.
	Compiler string `json:"compiler"`
	// BuildMode names the bundled or unbundled build.
	BuildMode string `json:"buildMode"`
	// RuntimeState is the state the run started from, such as cold or warm.
	RuntimeState string `json:"runtimeState"`
	// Cell names the bench cell, its runtime state and build mode.
	Cell string `json:"cell"`
	// Milestones holds the wall-clock boot milestones.
	Milestones Milestones `json:"milestones"`
	// Browser holds the browser-observed quickstart timing.
	Browser Browser `json:"browser"`
	// ServedBundle is the served code payload, set only by the bundled bench.
	ServedBundle *Bundle `json:"servedBundle,omitempty"`
	// ResourceConnection is the Resource SDK connection timing.
	ResourceConnection ResourceConn `json:"resourceConnection"`
	// Trace summarizes the runtime trace, when one was captured.
	Trace *Trace `json:"trace,omitempty"`
	// OperationShape summarizes storage operations from the trace, when captured.
	OperationShape *OperationShape `json:"operationShape,omitempty"`
	// BrowserProfile records the browser JS profile, when attempted.
	BrowserProfile *BrowserProfile `json:"browserProfile,omitempty"`
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

// WriteRun marshals run as JSON and writes it to dir/run.json, returning the
// written path.
func WriteRun(dir string, run Run) (string, error) {
	// Encode the run artifact as newline-terminated JSON.
	var arena fastjson.Arena
	data := marshalRunValue(&arena, run).MarshalTo(nil)
	data = append(data, '\n')

	// Write the run artifact to its cell directory.
	path := filepath.Join(dir, "run.json")
	if err := WriteArtifact(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func marshalRunValue(arena *fastjson.Arena, run Run) *fastjson.Value {
	// Encode the run's build identity and cell metadata.
	obj := arena.NewObject()
	obj.Set("timestamp", arena.NewString(run.Timestamp))
	obj.Set("compiler", arena.NewString(run.Compiler))
	obj.Set("buildMode", arena.NewString(run.BuildMode))
	obj.Set("runtimeState", arena.NewString(run.RuntimeState))
	obj.Set("cell", arena.NewString(run.Cell))

	// Encode the run's milestones, browser timing, and served bundle.
	obj.Set("milestones", marshalMilestonesValue(arena, run.Milestones))
	obj.Set("browser", marshalBrowserValue(arena, run.Browser))
	if run.ServedBundle != nil {
		obj.Set("servedBundle", marshalBundleValue(arena, *run.ServedBundle))
	}

	// Encode the run's resource connection and optional trace and profiles.
	obj.Set("resourceConnection", marshalResourceConnValue(arena, run.ResourceConnection))
	if run.Trace != nil {
		obj.Set("trace", marshalTraceValue(arena, *run.Trace))
	}
	if run.OperationShape != nil && len(run.OperationShape.Operations) != 0 {
		obj.Set("operationShape", MarshalOperationShapeValue(arena, *run.OperationShape))
	}
	if run.BrowserProfile != nil {
		obj.Set("browserProfile", marshalBrowserProfileValue(arena, *run.BrowserProfile))
	}
	return obj
}

func marshalJSONMapValue(arena *fastjson.Arena, values map[string]any) *fastjson.Value {
	// Collect the JSON object's keys for deterministic ordering.
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	// Encode the JSON object's values in sorted key order.
	slices.Sort(keys)
	for _, key := range keys {
		obj.Set(key, marshalJSONAnyValue(arena, values[key]))
	}
	return obj
}

func marshalJSONArrayValue(arena *fastjson.Arena, values []any) *fastjson.Value {
	arr := arena.NewArray()
	for _, value := range values {
		arr.SetArrayItem(len(arr.GetArray()), marshalJSONAnyValue(arena, value))
	}
	return arr
}

func marshalJSONAnyValue(arena *fastjson.Arena, value any) *fastjson.Value {
	switch typed := value.(type) {
	case nil:
		return arena.NewNull()
	case bool:
		if typed {
			return arena.NewTrue()
		}
		return arena.NewFalse()
	case string:
		return arena.NewString(typed)
	case int:
		return arena.NewNumberInt(typed)
	case int64:
		return arena.NewNumberString(strconv.FormatInt(typed, 10))
	case uint64:
		return arena.NewNumberString(strconv.FormatUint(typed, 10))
	case float64:
		return arena.NewNumberString(strconv.FormatFloat(typed, 'f', -1, 64))
	case []any:
		return marshalJSONArrayValue(arena, typed)
	case map[string]any:
		return marshalJSONMapValue(arena, typed)
	default:
		return arena.NewNull()
	}
}

func setOmitEmptyStringJSONField(arena *fastjson.Arena, obj *fastjson.Value, key, value string) {
	if value != "" {
		obj.Set(key, arena.NewString(value))
	}
}

func setOptionalIntJSONField(arena *fastjson.Arena, obj *fastjson.Value, key string, value *int) {
	if value != nil {
		obj.Set(key, arena.NewNumberInt(*value))
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
