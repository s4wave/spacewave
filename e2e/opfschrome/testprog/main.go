//go:build js

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/world"
)

// config selects one worker action and its shared workload dimensions.
type config struct {
	// scenario selects the worker action.
	scenario string
	// root identifies the disposable shared OPFS directory.
	root string
	// worker identifies this worker within the workload.
	worker int
	// workers counts the concurrent publishers expected by readers.
	workers int
	// iterations sets the loop count or total payload bytes for the selected scenario.
	iterations int
	// batch sets the batch count or read window for the selected scenario.
	batch int
	// extra carries the scenario's operation counts and durations into its
	// result.
	extra map[string]int64
}

// largeScenarioProgressEvery bounds progress-report frequency during large transfers.
const largeScenarioProgressEvery = 8 * 1024 * 1024

// lookupSpaceOp resolves the Space world operations.
var lookupSpaceOp = world.NewSchema(space_world_ops.Types).LookupOp

// main runs the selected worker scenario and waits for the harness to terminate it.
func main() {
	// Run the selected scenario and publish its result.
	start := time.Now()
	c, err := parseConfig(testArgs())
	if err == nil {
		err = run(context.Background(), c)
	}
	postResult(c, time.Since(start), err)

	// Stay alive until the harness terminates the worker on the result.
	// Controllers a scenario started keep running, and a JS callback they
	// await would throw "Go program has already exited" if main returned.
	select {}
}

// testArgs returns process arguments or the browser harness fallback.
func testArgs() []string {
	// Prefer complete process arguments, then the harness global.
	if len(os.Args) >= 7 {
		return os.Args
	}
	val := js.Global().Get("__OPFS_CHROMETEST_ARGS")
	if val.IsUndefined() || val.IsNull() {
		return os.Args
	}

	// Copy the harness arguments out of the JS array.
	n := val.Get("length").Int()
	args := make([]string, n)
	for i := range n {
		args[i] = val.Index(i).String()
	}
	return args
}

// parseConfig validates the scenario's numeric workload arguments.
func parseConfig(args []string) (*config, error) {
	// Parse the worker identity.
	if len(args) < 7 {
		return nil, errors.Errorf("expected 6 args, got %d", len(args)-1)
	}
	worker, err := strconv.Atoi(args[3])
	if err != nil {
		return nil, errors.Wrap(err, "parse worker")
	}
	workers, err := strconv.Atoi(args[4])
	if err != nil {
		return nil, errors.Wrap(err, "parse workers")
	}

	// Parse the workload size.
	iterations, err := strconv.Atoi(args[5])
	if err != nil {
		return nil, errors.Wrap(err, "parse iterations")
	}
	batch, err := strconv.Atoi(args[6])
	if err != nil {
		return nil, errors.Wrap(err, "parse batch")
	}
	return &config{
		scenario:   args[1],
		root:       args[2],
		worker:     worker,
		workers:    workers,
		iterations: iterations,
		batch:      batch,
	}, nil
}

// run dispatches one selected scenario.
func run(ctx context.Context, c *config) error {
	switch c.scenario {
	case "pipe-write-loop":
		return runPipeWriteLoop(c)
	case "srpc-echo-loop":
		return runSRPCEchoLoop(ctx, c)
	case "srpc-rpcstream-echo-loop":
		return runSRPCRpcStreamEchoLoop(ctx, c)
	case "resource-echo-loop":
		return runResourceEchoLoop(ctx, c)
	case "clear":
		return clearRoot(c.root)
	case "missing-delete-classify":
		return runMissingDeleteClassify(c)
	case "read-file-helper-loop":
		return runReadFileHelperLoop(c)
	case "large-write-read-list":
		return runLargeWriteReadList(c)
	case "read-at-helper-loop":
		return runReadAtHelperLoop(c)
	case "counter-init":
		return runCounterInit(c)
	case "counter-hold":
		return runCounterHold(c)
	case "counter-increment":
		return runCounterIncrement(c)
	case "counter-queued-increment":
		postReady(c)
		return runCounterIncrement(c)
	case "counter-try-lock-unavailable":
		postReady(c)
		return runCounterTryLock(c, false)
	case "counter-try-lock-available":
		return runCounterTryLock(c, true)
	case "counter-timeout-lock":
		return runCounterTimeoutLock(ctx, c)
	case "counter-verify":
		return runCounterVerify(c)
	case "volume-runtime-write":
		return runVolumeRuntimeWrite(ctx, c)
	case "volume-runtime-verify":
		return runVolumeRuntimeVerify(ctx, c)
	case "volume-runtime-delete-verify":
		return runVolumeRuntimeDeleteVerify(ctx, c)
	case "volume-kv-write-per-op":
		return runVolumeKVWritePerOp(ctx, c)
	case "volume-kv-write-single-tx":
		return runVolumeKVWriteSingleTx(ctx, c)
	case "world-init-unixfs":
		return runWorldInitUnixFS(ctx, c)
	case "world-crash-recovery":
		return runWorldCrashRecovery(ctx, c)
	case "world-large-unixfs-upload":
		return runWorldLargeUnixFSUpload(ctx, c)
	case "world-resource-large-unixfs-upload":
		return runWorldResourceLargeUnixFSUpload(ctx, c)
	case "world-resource-large-unixfs-write":
		return runWorldResourceLargeUnixFSUpload(ctx, c)
	case "world-resource-direct-upload-tree-large-unixfs-upload":
		return runWorldResourceDirectUploadTreeLargeUnixFSUpload(ctx, c)
	case "world-controller-resource-large-unixfs-upload":
		return runWorldControllerResourceLargeUnixFSUpload(ctx, c)
	case "world-cloud-overlay-resource-large-unixfs-upload":
		return runWorldCloudOverlayResourceLargeUnixFSUpload(ctx, c, false)
	case "world-cloud-sync-resource-large-unixfs-upload":
		return runWorldCloudOverlayResourceLargeUnixFSUpload(ctx, c, true)
	case "copy-walk-wrapper-concurrency":
		return runCopyWalkWrapperConcurrency(ctx, c)
	default:
		return errors.Errorf("unknown scenario %q", c.scenario)
	}
}

// clearRoot recreates only the scenario's disposable OPFS directory.
func clearRoot(rootName string) error {
	// Delete the directory, then create it empty.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	err = opfs.DeleteEntry(root, rootName, true)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	_, err = opfs.GetDirectory(root, rootName, true)
	return err
}

// runMissingDeleteClassify requires a missing-file deletion to report NotFound.
func runMissingDeleteClassify(c *config) error {
	// Delete a file that does not exist in the scenario directory.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	dir, err := opfs.GetDirectory(root, c.root, true)
	if err != nil {
		return err
	}
	err = opfs.DeleteFile(dir, "missing-delete-classify")
	if !opfs.IsNotFound(err) {
		return errors.Errorf("expected NotFoundError from missing delete, got %v", err)
	}
	return nil
}

// runReadFileHelperLoop checks repeated whole-file reads against written bytes.
func runReadFileHelperLoop(c *config) error {
	// Write the file once.
	dir, err := openTestDirectory(c.root, []string{"read-helper"})
	if err != nil {
		return err
	}
	want := []byte("tinygo-opfs-read-file-helper")
	if err := opfs.WriteFile(dir, "manifest-a", want); err != nil {
		return err
	}

	// Read it back repeatedly and compare.
	for i := range c.iterations {
		got, err := opfs.ReadFile(dir, "manifest-a")
		if err != nil {
			return errors.Wrap(err, "read manifest-a")
		}
		if !bytes.Equal(got, want) {
			return errors.Errorf("read helper mismatch iteration=%d got=%x want=%x", i, got, want)
		}
	}
	return nil
}

// runLargeWriteReadList checks large writes, sampled reads, and directory membership.
func runLargeWriteReadList(c *config) error {
	// Split the total size across the files.
	dir, err := openTestDirectory(c.root, []string{"large-helper"})
	if err != nil {
		return err
	}
	totalSize := largeFileSize(c)
	files := c.batch
	if files <= 0 {
		files = 64
	}
	chunkSize := func(i int) int {
		if i < totalSize%files {
			return totalSize/files + 1
		}
		return totalSize / files
	}

	// Write each file.
	for i := range files {
		size := chunkSize(i)
		name := "chunk-" + zeroPad(i, 3) + ".bin"
		if err := opfs.WriteFile(dir, name, deterministicLargeBytes(size, i)); err != nil {
			return errors.Wrapf(err, "write %s", name)
		}
	}

	// Read back the first, middle, and last files and sample their bytes.
	for _, i := range []int{0, files / 2, files - 1} {
		size := chunkSize(i)
		name := "chunk-" + zeroPad(i, 3) + ".bin"
		got, err := opfs.ReadFile(dir, name)
		if err != nil {
			return errors.Wrapf(err, "read %s", name)
		}
		want := deterministicLargeBytes(size, i)
		if len(got) != len(want) {
			return errors.Errorf("%s length=%d want=%d", name, len(got), len(want))
		}
		for _, idx := range []int{0, 1, 4095, 4096, size / 2, size - 2, size - 1} {
			if idx < 0 || idx >= len(want) {
				continue
			}
			if got[idx] != want[idx] {
				return errors.Errorf("%s byte[%d]=%d want=%d", name, idx, got[idx], want[idx])
			}
		}
	}

	// Check that the directory lists every file.
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return errors.Wrap(err, "list large-helper")
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	for i := range files {
		name := "chunk-" + zeroPad(i, 3) + ".bin"
		if !seen[name] {
			return errors.Errorf("%s missing from list directory result", name)
		}
	}
	return nil
}

// runReadAtHelperLoop checks offset reads and exact EOF behavior.
func runReadAtHelperLoop(c *config) error {
	// Write the file and open it for offset reads.
	dir, err := openTestDirectory(c.root, []string{"read-at-helper"})
	if err != nil {
		return err
	}
	want := []byte("tinygo-opfs-read-at-helper-window")
	if err := opfs.WriteFile(dir, "pages.dat", want); err != nil {
		return err
	}
	file, err := opfs.OpenAsyncFile(dir, "pages.dat")
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the same window repeatedly and compare.
	off := int64(11)
	expected := want[off : off+12]
	for i := range c.iterations {
		got := make([]byte, len(expected))
		n, err := file.ReadAt(got, off)
		if err != nil {
			return errors.Wrap(err, "read pages.dat")
		}
		if n != len(expected) {
			return errors.Errorf("read-at helper read %d bytes, expected %d", n, len(expected))
		}
		if !bytes.Equal(got, expected) {
			return errors.Errorf("read-at helper mismatch iteration=%d got=%x want=%x", i, got, expected)
		}
	}

	// Check that a read at the end returns EOF and no bytes.
	var eof [8]byte
	n, err := file.ReadAt(eof[:], int64(len(want)))
	if err != io.EOF {
		return errors.Errorf("read-at helper EOF error=%v, expected EOF", err)
	}
	if n != 0 {
		return errors.Errorf("read-at helper EOF read %d bytes, expected 0", n)
	}
	return nil
}

// largeFileSize returns the large file size for c: the iterations count, or
// 64 MiB by default.
func largeFileSize(c *config) int {
	// Default an unset size.
	if c.iterations <= 0 {
		return 64 * 1024 * 1024
	}
	return c.iterations
}

// zeroPad renders a workload index at the requested minimum width.
func zeroPad(n, width int) string {
	// Prepend zeros to the decimal form.
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// postReady announces that the worker reached the harness's synchronization point.
func postReady(c *config) {
	// Post the ready message to the harness.
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "ready")
	obj.Set("scenario", c.scenario)
	obj.Set("worker", c.worker)
	js.Global().Call("postMessage", obj)
}

// postProgress reports the current phase and optional byte or operation counts.
func postProgress(c *config, phase string, values ...int) {
	// Post the progress message with its optional counts.
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "progress")
	if c != nil {
		obj.Set("scenario", c.scenario)
		obj.Set("worker", c.worker)
	}
	obj.Set("phase", phase)
	if len(values) > 0 {
		obj.Set("offset", values[0])
	}
	if len(values) > 1 {
		obj.Set("total", values[1])
	}
	js.Global().Call("postMessage", obj)
}

// postResult reports the terminal result and optional operation measurements.
func postResult(c *config, dur time.Duration, err error) {
	// Build the common result and optional benchmark fields.
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "result")
	if c != nil {
		obj.Set("scenario", c.scenario)
		obj.Set("worker", c.worker)
		for k, v := range c.extra {
			obj.Set(k, v)
		}
	}
	obj.Set("durationMs", dur.Milliseconds())

	// Attach the terminal status and publish the result.
	obj.Set("ok", true)
	if err != nil {
		obj.Set("ok", false)
		obj.Set("error", err.Error())
	}
	js.Global().Call("postMessage", obj)
}

// _ is a type assertion
var _ block.StoreOps = (*probeDirtyTrackingStore)(nil)
