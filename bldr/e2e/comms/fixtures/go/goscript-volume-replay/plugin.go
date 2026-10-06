//go:build goscript

// Package goscript_volume_replay checks the browser volume engines' storage
// primitives and replays captured Volume workloads on them in a GoScript
// dedicated worker.
package goscript_volume_replay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/device/devicetest"
	device_opfs "github.com/s4wave/spacewave/db/volume/device/opfs"
	volume_idb "github.com/s4wave/spacewave/db/volume/idb"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// storageName prefixes the fixture's OPFS directories and IndexedDB
// databases.
const storageName = "goscript-volume-replay"

// main keeps the GoScript worker alive while the asynchronous run proceeds.
func main() {
	js.Global().Call("addEventListener", "unhandledrejection", js.FuncOf(func(this js.Value, args []js.Value) any {
		reason := args[0].Get("reason")
		postFailure(errors.Errorf("unhandled rejection: %s %s", reason.Call("toString").String(), reason.Get("stack").String()))
		return nil
	}))
	go run()
	select {}
}

// run executes the requested mode and reports its result to the harness.
func run() {
	// Translate foreign JavaScript throws into the fixture's failure message.
	defer func() {
		if recovered := recover(); recovered != nil {
			postFailure(errors.Errorf("panic: %v", recovered))
		}
	}()

	// Report readiness first: the host waits only briefly for it.
	if err := markReady(); err != nil {
		postFailure(err)
		return
	}

	// Run the mode: "check", "replay:<trace>/<target>", or
	// "reliability:<mode>:<name>[:<arg>]".
	ctx := context.Background()
	var report any
	var err error
	switch mode := readMode(); {
	case mode == "check":
		report, err = check(ctx)
	case strings.HasPrefix(mode, "replay:"):
		report, err = replay(ctx, strings.TrimPrefix(mode, "replay:"))
	case strings.HasPrefix(mode, "reliability:"):
		report, err = reliability(ctx, strings.TrimPrefix(mode, "reliability:"))
	default:
		err = errors.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		postFailure(err)
		return
	}

	// Report the result.
	encoded, err := json.Marshal(report)
	if err != nil {
		postFailure(err)
		return
	}
	postMessage(map[string]any{"type": "volume-done", "report": string(encoded)})
}

// target is one engine on one storage primitive.
type target struct {
	// name names the engine and primitive.
	name string
	// open creates fresh storage and opens the engine on it.
	open func(ctx context.Context) (engine, error)
}

// engine is an opened engine and the storage it runs on.
type engine struct {
	// target is the replay target.
	target workload.Target
	// reopen closes the engine and opens it again on the same storage.
	reopen func(ctx context.Context) (workload.Target, error)
	// counts reports the storage calls so far.
	counts func() counts
	// destroy closes the engine and deletes its storage.
	destroy func() error
}

// targets are the engines each trace replays against.
var targets = []target{
	{name: "e1-opfs", open: openE1OPFS},
	{name: "e1-idb", open: openE1IDB},
	{name: "e3-sqlite", open: openE3SQLite},
	{name: "e1-opfs-t2", open: openT2(openE1OPFS)},
	{name: "s4db-opfs", open: openS4DBOPFS},
	{name: "s4db-idb", open: openS4DBIDB},
}

// check runs the device contract checks, each followed by a reopen that must
// find the same contents.
func check(ctx context.Context) (map[string]string, error) {
	// Check the OPFS device.
	out := make(map[string]string)
	opfsPath := storageName + "/check"
	if err := device_opfs.Delete(opfsPath); err != nil {
		return nil, err
	}
	out["opfs-device"] = result(checkDevice(ctx, func() (device.Device, func() error, error) {
		d, err := device_opfs.Open(opfsPath)
		if err != nil {
			return nil, nil, err
		}
		return d, d.Close, nil
	}))
	if err := device_opfs.Delete(storageName); err != nil {
		return nil, err
	}

	// Check the IndexedDB device.
	deviceDB := storageName + "-check-device"
	if err := volume_idb.DeleteDatabase(deviceDB); err != nil {
		return nil, err
	}
	out["idb-device"] = result(checkDevice(ctx, func() (device.Device, func() error, error) {
		d, err := volume_idb.OpenDevice(ctx, deviceDB)
		if err != nil {
			return nil, nil, err
		}
		return d, d.Close, nil
	}))
	if err := volume_idb.DeleteDatabase(deviceDB); err != nil {
		return nil, err
	}
	return out, nil
}

// checkDevice runs the device checks, reopens the device, and compares every
// file.
func checkDevice(ctx context.Context, open func() (device.Device, func() error, error)) error {
	// Check the device and read what it holds.
	d, closeDevice, err := open()
	if err != nil {
		return err
	}
	if err := devicetest.Check(ctx, d); err != nil {
		_ = closeDevice()
		return err
	}
	before, err := readDevice(ctx, d)
	_ = closeDevice()
	if err != nil {
		return err
	}

	// Reopen it and compare every file.
	d, closeDevice, err = open()
	if err != nil {
		return err
	}
	defer closeDevice()
	after, err := readDevice(ctx, d)
	if err != nil {
		return err
	}
	if len(after) != len(before) {
		return errors.Errorf("reopened with %d files, want %d", len(after), len(before))
	}
	for name, data := range before {
		if !bytes.Equal(after[name], data) {
			return errors.Errorf("reopened file %s differs", name)
		}
	}
	return nil
}

// readDevice reads every file of a device.
func readDevice(ctx context.Context, d device.Device) (map[string][]byte, error) {
	// List the files and read each one whole.
	files, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(files))
	for _, f := range files {
		data := make([]byte, f.Size)
		if err := d.Read(ctx, []device.Read{{Name: f.Name, Data: data}}); err != nil {
			return nil, err
		}
		out[f.Name] = data
	}
	return out, nil
}

// result renders a check error, "ok" when there is none.
func result(err error) string {
	if err != nil {
		return err.Error()
	}
	return "ok"
}

// replayReport is the result of one trace replay.
type replayReport struct {
	// Trace names the replayed trace.
	Trace string `json:"trace"`
	// Target names the engine and storage primitive.
	Target string `json:"target"`
	// Policy is "durable" or "ordered".
	Policy string `json:"policy"`
	// Records is the number of replayed records.
	Records int `json:"records"`
	// WallMs is the replay's wall time.
	WallMs float64 `json:"wallMs"`
	// OpenMs is the time to reopen the engine after the replay.
	OpenMs float64 `json:"openMs"`
	// RecoverMs is the time to replay the journal after the reopen.
	RecoverMs float64 `json:"recoverMs"`
	// OrderedCommits is the number of commits made without a flush.
	OrderedCommits int `json:"orderedCommits"`
	// CommitErrors is the number of failed commits.
	CommitErrors int `json:"commitErrors"`
	// Storage counts the storage calls the replay made.
	Storage counts `json:"storage"`
	// Latency holds the median and 99th percentile of each operation.
	Latency map[string][2]float64 `json:"latency"`
	// Error is the replay's failure, empty when it succeeded.
	Error string `json:"error,omitempty"`
}

// replay replays one trace against one target, given as "<trace>/<target>",
// once durably and once ordered.
func replay(ctx context.Context, spec string) ([]replayReport, error) {
	// Find the target and fetch the trace.
	name, targetName, _ := strings.Cut(spec, "/")
	i := slices.IndexFunc(targets, func(t target) bool { return t.name == targetName })
	if i < 0 {
		return nil, errors.Errorf("unknown target %q", targetName)
	}
	t := targets[i]
	recs, err := fetchRecords(name)
	if err != nil {
		return nil, err
	}

	// Replay it under each commit policy.
	out := make([]replayReport, 0, 2)
	for _, policy := range []string{"durable", "ordered"} {
		logf("replay %s %s %s", name, t.name, policy)
		rep := replayReport{Trace: name, Target: t.name, Policy: policy}
		if err := replayOne(ctx, recs, t, policy == "ordered", &rep); err != nil {
			rep.Error = err.Error()
		}
		out = append(out, rep)
	}
	return out, nil
}

// replayOne replays records against a fresh target and fills rep.
func replayOne(ctx context.Context, recs []workload.Record, t target, ordered bool, rep *replayReport) error {
	// Prepare the workload and a fresh engine.
	r, err := workload.NewReplay(recs)
	if err != nil {
		return err
	}
	r.Ordered = ordered
	e, err := t.open(ctx)
	if err != nil {
		return err
	}
	defer e.destroy()
	if err := r.Seed(ctx, e.target); err != nil {
		return err
	}

	// Replay the workload.
	seeded := e.counts()
	result, err := r.Run(ctx, e.target)
	if err != nil {
		return err
	}
	rep.Storage = e.counts().sub(seeded)

	// Reopen and recover.
	openStart := time.Now()
	reopened, err := e.reopen(ctx)
	if err != nil {
		return err
	}
	rep.OpenMs = ms(time.Since(openStart))
	recoverStart := time.Now()
	if err := reopened.ReplayJournal(ctx, workload.DiscardJournal); err != nil {
		return err
	}
	rep.RecoverMs = ms(time.Since(recoverStart))

	// Report.
	rep.Records = result.Ops
	rep.WallMs = ms(result.Wall)
	rep.OrderedCommits = result.OrderedCommits
	rep.CommitErrors = result.CommitErrors
	rep.Latency = make(map[string][2]float64)
	for op := range result.Latency {
		rep.Latency[string(op)] = [2]float64{ms(result.Percentile(op, 0.5)), ms(result.Percentile(op, 0.99))}
	}
	return nil
}

// fetchRecords fetches the text records of a trace from the fixture server.
func fetchRecords(name string) ([]workload.Record, error) {
	// Fetch the trace and check the status.
	resp, err := opfs.AwaitPromise(js.Global().Call("fetch", "/traces/"+name+".records"))
	if err != nil {
		return nil, err
	}
	if !resp.Get("ok").Bool() {
		return nil, errors.Errorf("fetch trace %s: status %d", name, resp.Get("status").Int())
	}

	// Read and parse the body.
	text, err := opfs.AwaitPromise(resp.Call("text"))
	if err != nil {
		return nil, err
	}
	return workload.ParseRecords(text.String())
}

// ms converts a duration to fractional milliseconds.
func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

// readMode reads the worker's requested mode.
func readMode() string {
	// The start info is base64 JSON whose instance key names the mode.
	encoded := js.Global().Get("BLDR_PLUGIN_START_INFO")
	if encoded.IsUndefined() || encoded.IsNull() {
		return ""
	}
	jsonText := js.Global().Call("atob", encoded.String())
	parsed := js.Global().Get("JSON").Call("parse", jsonText)
	return parsed.Get("instanceKey").String()
}

// markReady completes the production worker readiness handshake.
func markReady() error {
	ready := js.Global().Get("BLDR_PLUGIN_MARK_READY")
	if ready.IsUndefined() || ready.IsNull() || ready.Type() != js.TypeFunction {
		return errors.New("BLDR_PLUGIN_MARK_READY is not a function")
	}
	ready.Invoke()
	return nil
}

// logf posts a progress line for the page to log.
func logf(format string, args ...any) {
	postMessage(map[string]any{"type": "volume-log", "line": fmt.Sprintf(format, args...)})
}

// postFailure sends the error to the browser harness.
func postFailure(err error) {
	postMessage(map[string]any{
		"type":          "volume-failed",
		"failureReason": err.Error(),
	})
}

// postMessage sends one fixture result through the worker message boundary.
func postMessage(msg map[string]any) {
	js.Global().Call("postMessage", js.ValueOf(msg))
}
