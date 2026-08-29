//go:build !js

// Package drivebench owns the run.json artifact schema shared by the Drive
// startup benches. The unbundled bench (e2e/wasm) and the bundled production
// bench (e2e/releasewasm) cannot import each other's test symbols, so the
// cell-comparable schema and its artifact paths live here as the single
// contract both harnesses write.
package drivebench

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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
	Timestamp          string          `json:"timestamp"`
	Compiler           string          `json:"compiler"`
	BuildMode          string          `json:"buildMode"`
	RuntimeState       string          `json:"runtimeState"`
	Cell               string          `json:"cell"`
	Milestones         Milestones      `json:"milestones"`
	Browser            Browser         `json:"browser"`
	ServedBundle       *Bundle         `json:"servedBundle,omitempty"`
	ResourceConnection ResourceConn    `json:"resourceConnection"`
	Trace              *Trace          `json:"trace,omitempty"`
	OperationShape     *OperationShape `json:"operationShape,omitempty"`
	BrowserProfile     *BrowserProfile `json:"browserProfile,omitempty"`
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

	StartupMarks []StartupMark `json:"startupMarks,omitempty"`
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

// StartupMark is one startup-boundary mark observed on the page performance
// timeline, emitted by markStartupBoundary. StartMs is the mark's
// performance.now time in milliseconds from navigation start, so marks share
// the timebase of the bundled milestones and quickstart phases. The
// pre-quickstart startup gap is attributed by reading the interval between
// adjacent marks: the gap belongs to whichever mark transition spans it.
type StartupMark struct {
	Label    string         `json:"label"`
	Sequence int            `json:"sequence,omitempty"`
	StartMs  int            `json:"startMs"`
	Phase    string         `json:"phase,omitempty"`
	Mode     string         `json:"mode,omitempty"`
	Source   string         `json:"source,omitempty"`
	Detail   map[string]any `json:"detail,omitempty"`
}

// StartupMarksScript reads the page startup-mark timeline into a JSON-able
// array ordered by performance.now start time. Each entry carries the mark
// label, its start time in milliseconds from navigation start, and the boundary
// detail markStartupBoundary stored on the performance mark (sequence, phase,
// mode, source). Both Drive benches evaluate it on the page after content-ready
// so the captured marks span the full boot-to-content window.
const StartupMarksScript = `() => {
  const prefix = 'spacewave.startup.'
  return performance.getEntriesByType('mark')
    .filter((m) => m.name.startsWith(prefix))
    .map((m) => ({
      label: m.name.slice(prefix.length),
      startMs: Math.round(m.startTime),
      sequence: m.detail?.sequence ?? 0,
      phase: m.detail?.phase ?? null,
      mode: m.detail?.mode ?? null,
      source: m.detail?.source ?? null,
      detail: m.detail ?? {},
    }))
}`

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
	TracetoolPath    string `json:"tracetoolPath,omitempty"`
	UserTasks        int    `json:"userTasks,omitempty"`
	UserRegions      int    `json:"userRegions,omitempty"`
	UserLogs         int    `json:"userLogs,omitempty"`
	Tasks            []Task `json:"tasks,omitempty"`
}

// Task is an aggregate runtime-trace task summary keyed by Go user task type.
type Task struct {
	Type    string `json:"type"`
	Count   int    `json:"count"`
	TotalUs int64  `json:"totalUs"`
	MaxUs   int64  `json:"maxUs"`
}

// OperationShape is the compact per-run storage/frontier summary derived from
// runtime trace tasks and numeric trace-log payloads. Raw runtime.trace remains
// the replay source; this projection is the comparison surface for plan ranking.
type OperationShape struct {
	Operations []OperationSummary `json:"operations,omitempty"`
}

// OperationSummary groups task timing and recovered numeric log payloads under a
// stable operation name, such as block-write or opfs-publish.
type OperationSummary struct {
	Name     string           `json:"name"`
	Count    int              `json:"count,omitempty"`
	TotalUs  int64            `json:"totalUs,omitempty"`
	MaxUs    int64            `json:"maxUs,omitempty"`
	LogCount int              `json:"logCount,omitempty"`
	Fields   []OperationField `json:"fields,omitempty"`
}

// OperationField summarizes one numeric field recovered from trace logs.
type OperationField struct {
	Name    string `json:"name"`
	Samples int    `json:"samples"`
	Sum     int64  `json:"sum"`
	Max     int64  `json:"max"`
	Last    int64  `json:"last"`
}

// BrowserProfile records the optional same-window browser JS CPU/profile
// artifact. It is present only when the harness attempted the profile gate.
type BrowserProfile struct {
	Captured      bool            `json:"captured"`
	ProfilePath   string          `json:"profilePath,omitempty"`
	SummaryPath   string          `json:"summaryPath,omitempty"`
	CaptureWindow string          `json:"captureWindow,omitempty"`
	StartedAt     string          `json:"startedAt,omitempty"`
	StoppedAt     string          `json:"stoppedAt,omitempty"`
	Bytes         int             `json:"bytes,omitempty"`
	SkippedReason string          `json:"skippedReason,omitempty"`
	Buckets       []ProfileBucket `json:"buckets,omitempty"`
}

// ProfileBucket is one source/function bucket from the browser JS CPU profile.
type ProfileBucket struct {
	Name    string `json:"name"`
	Count   int    `json:"count,omitempty"`
	SelfUs  int64  `json:"selfUs,omitempty"`
	TotalUs int64  `json:"totalUs,omitempty"`
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

// WriteRun marshals run as JSON and writes it to dir/run.json, returning the
// written path.
func WriteRun(dir string, run Run) (string, error) {
	var arena fastjson.Arena
	data := marshalRunValue(&arena, run).MarshalTo(nil)
	data = append(data, '\n')
	path := filepath.Join(dir, "run.json")
	if err := WriteArtifact(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func marshalRunValue(arena *fastjson.Arena, run Run) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("timestamp", arena.NewString(run.Timestamp))
	obj.Set("compiler", arena.NewString(run.Compiler))
	obj.Set("buildMode", arena.NewString(run.BuildMode))
	obj.Set("runtimeState", arena.NewString(run.RuntimeState))
	obj.Set("cell", arena.NewString(run.Cell))
	obj.Set("milestones", marshalMilestonesValue(arena, run.Milestones))
	obj.Set("browser", marshalBrowserValue(arena, run.Browser))
	if run.ServedBundle != nil {
		obj.Set("servedBundle", marshalBundleValue(arena, *run.ServedBundle))
	}
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

func marshalMilestonesValue(arena *fastjson.Arena, milestones Milestones) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("liveAppMs", arena.NewNumberString(strconv.FormatInt(milestones.LiveAppMs, 10)))
	obj.Set("routeAcceptedMs", arena.NewNumberString(strconv.FormatInt(milestones.RouteAcceptedMs, 10)))
	obj.Set("unixfsVisibleMs", arena.NewNumberString(strconv.FormatInt(milestones.UnixfsVisibleMs, 10)))
	obj.Set("contentReadyMs", arena.NewNumberString(strconv.FormatInt(milestones.ContentReadyMs, 10)))
	return obj
}

func marshalBrowserValue(arena *fastjson.Arena, browser Browser) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("contentReadyMs", arena.NewNumberInt(browser.ContentReadyMs))
	obj.Set("quickstartState", arena.NewString(browser.QuickstartState))
	setOptionalIntJSONField(arena, obj, "quickstartProgressReadyMs", browser.QuickstartProgressReadyMs)
	setOptionalIntJSONField(arena, obj, "quickstartContentReadyMs", browser.QuickstartContentReadyMs)
	setOptionalIntJSONField(arena, obj, "quickstartFinishedMs", browser.QuickstartFinishedMs)
	if len(browser.QuickstartPhases) != 0 {
		obj.Set("quickstartPhases", marshalPhasesValue(arena, browser.QuickstartPhases))
	}
	if browser.DriveSeedResourceCalls != 0 {
		obj.Set("driveSeedResourceCalls", arena.NewNumberInt(browser.DriveSeedResourceCalls))
	}
	setOptionalIntJSONField(arena, obj, "driveSeedStartedMs", browser.DriveSeedStartedMs)
	setOptionalIntJSONField(arena, obj, "driveSeedFinishedMs", browser.DriveSeedFinishedMs)
	setOptionalIntJSONField(arena, obj, "driveSeedElapsedMs", browser.DriveSeedElapsedMs)
	if len(browser.StartupMarks) != 0 {
		obj.Set("startupMarks", marshalStartupMarksValue(arena, browser.StartupMarks))
	}
	return obj
}

func marshalPhasesValue(arena *fastjson.Arena, phases []Phase) *fastjson.Value {
	arr := arena.NewArray()
	for _, phase := range phases {
		obj := arena.NewObject()
		obj.Set("name", arena.NewString(phase.Name))
		obj.Set("startedMs", arena.NewNumberInt(phase.StartedMs))
		setOptionalIntJSONField(arena, obj, "finishedMs", phase.FinishedMs)
		setOptionalIntJSONField(arena, obj, "elapsedMs", phase.ElapsedMs)
		setOmitEmptyStringJSONField(arena, obj, "error", phase.Error)
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return arr
}

func marshalStartupMarksValue(arena *fastjson.Arena, marks []StartupMark) *fastjson.Value {
	arr := arena.NewArray()
	for _, mark := range marks {
		obj := arena.NewObject()
		obj.Set("label", arena.NewString(mark.Label))
		if mark.Sequence != 0 {
			obj.Set("sequence", arena.NewNumberInt(mark.Sequence))
		}
		obj.Set("startMs", arena.NewNumberInt(mark.StartMs))
		setOmitEmptyStringJSONField(arena, obj, "phase", mark.Phase)
		setOmitEmptyStringJSONField(arena, obj, "mode", mark.Mode)
		setOmitEmptyStringJSONField(arena, obj, "source", mark.Source)
		if len(mark.Detail) != 0 {
			obj.Set("detail", marshalJSONMapValue(arena, mark.Detail))
		}
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return arr
}

func marshalBundleValue(arena *fastjson.Arena, bundle Bundle) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("totalBytes", arena.NewNumberString(strconv.FormatInt(bundle.TotalBytes, 10)))
	obj.Set("wasmBytes", arena.NewNumberString(strconv.FormatInt(bundle.WasmBytes, 10)))
	obj.Set("fileCount", arena.NewNumberInt(bundle.FileCount))
	return obj
}

func marshalResourceConnValue(arena *fastjson.Arena, conn ResourceConn) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("durationMs", arena.NewNumberString(strconv.FormatInt(conn.DurationMs, 10)))
	obj.Set("attempts", arena.NewNumberInt(conn.Attempts))
	obj.Set("startupReloads", arena.NewNumberInt(conn.StartupReloads))
	return obj
}

func marshalTraceValue(arena *fastjson.Arena, trace Trace) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("bytes", arena.NewNumberInt(trace.Bytes))
	obj.Set("runtimeTracePath", arena.NewString(trace.RuntimeTracePath))
	// A cell that captured the raw trace without summarizing it has no counts to
	// report, and the tracetool path is what says whether summarization ran. Key
	// the whole summary block on it rather than on the counts themselves: a
	// summarized cell that legitimately saw no user regions must serialize that
	// zero, or a consumer cannot tell a measured zero from an unsummarized cell.
	if trace.TracetoolPath != "" {
		obj.Set("tracetoolPath", arena.NewString(trace.TracetoolPath))
		obj.Set("userTasks", arena.NewNumberInt(trace.UserTasks))
		obj.Set("userRegions", arena.NewNumberInt(trace.UserRegions))
		obj.Set("userLogs", arena.NewNumberInt(trace.UserLogs))
		obj.Set("tasks", marshalTasksValue(arena, trace.Tasks))
	}
	return obj
}

func marshalTasksValue(arena *fastjson.Arena, tasks []Task) *fastjson.Value {
	arr := arena.NewArray()
	for _, task := range tasks {
		obj := arena.NewObject()
		obj.Set("type", arena.NewString(task.Type))
		obj.Set("count", arena.NewNumberInt(task.Count))
		obj.Set("totalUs", arena.NewNumberString(strconv.FormatInt(task.TotalUs, 10)))
		obj.Set("maxUs", arena.NewNumberString(strconv.FormatInt(task.MaxUs, 10)))
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return arr
}

// MarshalOperationShapeValue encodes an operation-shape projection in an existing JSON arena.
func MarshalOperationShapeValue(arena *fastjson.Arena, shape OperationShape) *fastjson.Value {
	obj := arena.NewObject()
	if len(shape.Operations) != 0 {
		arr := arena.NewArray()
		for _, op := range shape.Operations {
			arr.SetArrayItem(len(arr.GetArray()), marshalOperationSummaryValue(arena, op))
		}
		obj.Set("operations", arr)
	}
	return obj
}

func marshalOperationSummaryValue(arena *fastjson.Arena, op OperationSummary) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(op.Name))
	if op.Count != 0 {
		obj.Set("count", arena.NewNumberInt(op.Count))
	}
	if op.TotalUs != 0 {
		obj.Set("totalUs", arena.NewNumberString(strconv.FormatInt(op.TotalUs, 10)))
	}
	if op.MaxUs != 0 {
		obj.Set("maxUs", arena.NewNumberString(strconv.FormatInt(op.MaxUs, 10)))
	}
	if op.LogCount != 0 {
		obj.Set("logCount", arena.NewNumberInt(op.LogCount))
	}
	if len(op.Fields) != 0 {
		arr := arena.NewArray()
		for _, field := range op.Fields {
			arr.SetArrayItem(len(arr.GetArray()), marshalOperationFieldValue(arena, field))
		}
		obj.Set("fields", arr)
	}
	return obj
}

func marshalOperationFieldValue(arena *fastjson.Arena, field OperationField) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(field.Name))
	obj.Set("samples", arena.NewNumberInt(field.Samples))
	obj.Set("sum", arena.NewNumberString(strconv.FormatInt(field.Sum, 10)))
	obj.Set("max", arena.NewNumberString(strconv.FormatInt(field.Max, 10)))
	obj.Set("last", arena.NewNumberString(strconv.FormatInt(field.Last, 10)))
	return obj
}

func marshalBrowserProfileValue(arena *fastjson.Arena, profile BrowserProfile) *fastjson.Value {
	obj := arena.NewObject()
	if profile.Captured {
		obj.Set("captured", arena.NewTrue())
	} else {
		obj.Set("captured", arena.NewFalse())
	}
	setOmitEmptyStringJSONField(arena, obj, "profilePath", profile.ProfilePath)
	setOmitEmptyStringJSONField(arena, obj, "summaryPath", profile.SummaryPath)
	setOmitEmptyStringJSONField(arena, obj, "captureWindow", profile.CaptureWindow)
	setOmitEmptyStringJSONField(arena, obj, "startedAt", profile.StartedAt)
	setOmitEmptyStringJSONField(arena, obj, "stoppedAt", profile.StoppedAt)
	if profile.Bytes != 0 {
		obj.Set("bytes", arena.NewNumberInt(profile.Bytes))
	}
	setOmitEmptyStringJSONField(arena, obj, "skippedReason", profile.SkippedReason)
	if len(profile.Buckets) != 0 {
		arr := arena.NewArray()
		for _, bucket := range profile.Buckets {
			arr.SetArrayItem(len(arr.GetArray()), marshalProfileBucketValue(arena, bucket))
		}
		obj.Set("buckets", arr)
	}
	return obj
}

func marshalProfileBucketValue(arena *fastjson.Arena, bucket ProfileBucket) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(bucket.Name))
	if bucket.Count != 0 {
		obj.Set("count", arena.NewNumberInt(bucket.Count))
	}
	if bucket.SelfUs != 0 {
		obj.Set("selfUs", arena.NewNumberString(strconv.FormatInt(bucket.SelfUs, 10)))
	}
	if bucket.TotalUs != 0 {
		obj.Set("totalUs", arena.NewNumberString(strconv.FormatInt(bucket.TotalUs, 10)))
	}
	return obj
}

func marshalJSONMapValue(arena *fastjson.Arena, values map[string]any) *fastjson.Value {
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
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

// ParseStartupMarks parses the page startup-mark timeline captured with
// StartupMarksScript. Marks arrive ordered by performance.now start time; the
// parser preserves that order so the pre-quickstart gap reads as the interval
// between adjacent marks. Entries without a label are skipped.
func ParseStartupMarks(raw any) []StartupMark {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	marks := make([]StartupMark, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label, _ := m["label"].(string)
		if label == "" {
			continue
		}
		mark := StartupMark{
			Label:    label,
			Sequence: intValue(m["sequence"]),
			StartMs:  intValue(m["startMs"]),
		}
		mark.Phase, _ = m["phase"].(string)
		mark.Mode, _ = m["mode"].(string)
		mark.Source, _ = m["source"].(string)
		if detail, ok := m["detail"].(map[string]any); ok && len(detail) > 0 {
			mark.Detail = detail
		}
		marks = append(marks, mark)
	}
	return marks
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
