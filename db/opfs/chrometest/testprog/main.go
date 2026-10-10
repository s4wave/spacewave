//go:build js

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	stderrors "errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
	"time"

	cbconfig "github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/kvtx"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	packfile_writer "github.com/s4wave/spacewave/db/packfile/writer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

// config selects one worker action and its shared workload dimensions.
type config struct {
	// scenario selects the worker action.
	scenario string
	// root names the disposable shared OPFS directory.
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

// main runs one selected worker scenario, reports its terminal result, and
// waits for the harness to terminate the worker.
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
	case "world-deferred-crash-recovery":
		return runWorldDeferredCrashRecovery(ctx, c)
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
		return runWorldCloudOverlayResourceLargeUnixFSUpload(ctx, c)
	case "world-cloud-sync-resource-large-unixfs-upload":
		return runWorldCloudSyncResourceLargeUnixFSUpload(ctx, c)
	case "copy-walk-wrapper-concurrency":
		return runCopyWalkWrapperConcurrency(ctx, c)
	default:
		return errors.Errorf("unknown scenario %q", c.scenario)
	}
}

// pipeReadResult returns the bytes drained and terminal error from a pipe reader.
type pipeReadResult struct {
	// n counts the bytes drained before termination.
	n int
	// err is the terminal pipe error, excluding normal EOF.
	err error
}

// runPipeWriteLoop checks deterministic streaming through a Go pipe.
func runPipeWriteLoop(c *config) error {
	// Drain the pipe in the background, counting the bytes read.
	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 4 * 1024 * 1024
	}
	pr, pw := io.Pipe()
	done := make(chan pipeReadResult, 1)
	go func() {
		buf := make([]byte, 32*1024)
		var total int
		for {
			n, err := pr.Read(buf)
			total += n
			if err == io.EOF {
				done <- pipeReadResult{n: total}
				return
			}
			if err != nil {
				done <- pipeReadResult{n: total, err: err}
				return
			}
		}
	}()

	// Stream deterministic chunks into the pipe, reporting progress.
	postProgress(c, "pipe-write-start", 0, totalSize)
	const chunkSize = 64 * 1024
	const progressEvery = 1024 * 1024
	for offset := 0; offset < totalSize; offset += chunkSize {
		n := min(chunkSize, totalSize-offset)
		written, err := pw.Write(deterministicLargeWindow(offset, n, 0))
		if err != nil {
			return errors.Wrapf(err, "pipe write offset=%d", offset)
		}
		if written != n {
			return errors.Errorf("pipe write offset=%d wrote=%d want=%d", offset, written, n)
		}
		next := offset + n
		if next == totalSize || next%progressEvery == 0 {
			postProgress(c, "pipe-write-stream", next, totalSize)
		}
	}

	// Close the writer and check the reader drained every byte.
	postProgress(c, "pipe-close-start", totalSize, totalSize)
	if err := pw.Close(); err != nil {
		return errors.Wrap(err, "pipe close")
	}
	res := <-done
	if res.err != nil {
		return errors.Wrap(res.err, "pipe read")
	}
	if res.n != totalSize {
		return errors.Errorf("pipe read=%d want=%d", res.n, totalSize)
	}
	postProgress(c, "pipe-close-complete", totalSize, totalSize)
	return nil
}

// runSRPCEchoLoop checks the echo contract over a real multiplexed connection.
func runSRPCEchoLoop(ctx context.Context, c *config) error {
	// Serve the echo service over an in-memory multiplexed connection.
	serverMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(serverMux); err != nil {
		return errors.Wrap(err, "register echo server")
	}
	conn, cleanup, err := serveMuxedPipe(ctx, serverMux)
	if err != nil {
		return err
	}

	// Run the echo loop against the served service, then join the server.
	client := echo.NewSRPCEchoerClient(conn)
	return stderrors.Join(runEchoClientLoop(ctx, c, client, "srpc-echo-loop"), cleanup())
}

// runSRPCRpcStreamEchoLoop checks echo calls through a nested RPC stream.
func runSRPCRpcStreamEchoLoop(ctx context.Context, c *config) error {
	// Serve an echo service whose RPC stream forwards to an inner echo service.
	innerMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(innerMux); err != nil {
		return errors.Wrap(err, "register inner echo server")
	}
	serverMux := srpc.NewMux()
	if err := echo.NewEchoServer(innerMux).Register(serverMux); err != nil {
		return errors.Wrap(err, "register outer echo server")
	}
	conn, cleanup, err := serveMuxedPipe(ctx, serverMux)
	if err != nil {
		return err
	}

	// Run the echo loop against the inner service through the outer stream,
	// then join the server.
	outerClient := echo.NewSRPCEchoerClient(conn)
	nestedClient := rpcstream.NewRpcStreamClient(
		func(ctx context.Context) (echo.SRPCEchoer_RpcStreamClient, error) {
			return outerClient.RpcStream(ctx)
		},
		"echo",
		true,
	)
	client := echo.NewSRPCEchoerClient(nestedClient)
	return stderrors.Join(runEchoClientLoop(ctx, c, client, "srpc-rpcstream-echo-loop"), cleanup())
}

// serveMuxedPipe serves mux on one end of an in-memory multiplexed connection
// and returns a client for the other end. The cleanup stops the server, closes
// both ends, and returns the server's error unless it is a normal close.
func serveMuxedPipe(ctx context.Context, mux srpc.Mux) (srpc.Client, func() error, error) {
	// Open both multiplexed ends of the pipe.
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open client muxed conn")
	}
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		_ = clientMp.Close()
		_ = serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open server muxed conn")
	}

	// Serve the mux until the cleanup cancels it.
	serverCtx, cancelServer := context.WithCancel(ctx)
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srpc.NewServer(mux).AcceptMuxedConn(serverCtx, serverMp)
	}()
	cleanup := func() error {
		// Stop the server and join it.
		cancelServer()
		_ = clientMp.Close()
		_ = serverMp.Close()
		if err := <-serverErrCh; err != nil && !isExpectedMuxCloseError(err) {
			return errors.Wrap(err, "server mux")
		}
		return nil
	}
	return srpc.NewClientWithMuxedConn(clientMp), cleanup, nil
}

// runEchoClientLoop verifies repeated echo responses and reports stream progress.
func runEchoClientLoop(
	ctx context.Context,
	c *config,
	client echo.SRPCEchoerClient,
	phase string,
) error {
	// Default the call count and payload size.
	iterations := c.iterations
	if iterations <= 0 {
		iterations = 128
	}
	payloadSize := c.batch
	if payloadSize <= 0 {
		payloadSize = 4096
	}

	// Echo the payload repeatedly and check each response.
	body := strings.Repeat("x", payloadSize)
	postProgress(c, phase+"-start", 0, iterations)
	for i := range iterations {
		resp, err := client.Echo(ctx, &echo.EchoMsg{Body: body})
		if err != nil {
			return errors.Wrapf(err, "echo call %d", i)
		}
		if resp.GetBody() != body {
			return errors.Errorf("echo call %d body len=%d want=%d", i, len(resp.GetBody()), len(body))
		}
		next := i + 1
		if next == 1 || next == iterations || next%16 == 0 {
			postProgress(c, phase+"-stream", next, iterations)
		}
	}
	postProgress(c, phase+"-complete", iterations, iterations)
	return nil
}

// runResourceEchoLoop checks echo calls through the resource reference lifecycle.
func runResourceEchoLoop(ctx context.Context, c *config) error {
	// Serve the echo service as the root resource.
	rootMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(rootMux); err != nil {
		return errors.Wrap(err, "register root echo server")
	}
	resClient, cleanup, err := openResourceClient(ctx, rootMux)
	if err != nil {
		return err
	}
	defer cleanup()

	// Run the echo loop through a reference to the root resource.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get root resource client")
	}
	client := echo.NewSRPCEchoerClient(rootClient)
	return runEchoClientLoop(ctx, c, client, "resource-echo-loop")
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

// runVolumeRuntimeWrite persists a block and the metadata needed to find it after remount.
func runVolumeRuntimeWrite(ctx context.Context, c *config) error {
	// Store the block durably.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	ref, _, err := vol.PutBlock(ctx, volumeBlockValue(), nil)
	if err != nil {
		return errors.Wrap(err, "put volume block")
	}
	if _, err := vol.Sync(ctx); err != nil {
		return err
	}

	// Store the metadata and the block reference in one transaction.
	refData, err := ref.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal volume block ref")
	}
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open volume write tx")
	}
	defer tx.Discard()
	if err := tx.Set(ctx, volumeMetaKey(), volumeMetaValue()); err != nil {
		return errors.Wrap(err, "set volume meta")
	}
	if err := tx.Set(ctx, volumeRefKey(), refData); err != nil {
		return errors.Wrap(err, "set volume block ref")
	}

	// Commit and sync the transaction.
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit volume meta")
	}
	_, err = vol.Sync(ctx)
	return err
}

// volumeKVKey returns the benchmark key for one write iteration.
func volumeKVKey(i int) []byte {
	return []byte("bench/kv/" + strconv.Itoa(i))
}

// volumeKVValue returns the benchmark payload; batch carries the size in bytes.
func volumeKVValue(c *config) []byte {
	return bytes.Repeat([]byte{0x61}, c.batch)
}

// runVolumeKVWritePerOp commits one key per write transaction. opNanos covers
// every iteration including transaction open and commit.
func runVolumeKVWritePerOp(ctx context.Context, c *config) error {
	// Open the volume's key/value store.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()

	// Commit each key in its own transaction, timing the loop.
	start := time.Now()
	for i := range c.iterations {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open kv write tx")
		}
		if err := tx.Set(ctx, volumeKVKey(i), volumeKVValue(c)); err != nil {
			tx.Discard()
			return errors.Wrap(err, "set kv")
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Discard()
			return errors.Wrap(err, "commit kv tx")
		}
	}
	c.extra = map[string]int64{
		"opNanos": time.Since(start).Nanoseconds(),
		"ops":     int64(c.iterations),
	}
	return nil
}

// runVolumeKVWriteSingleTx puts all values into one write transaction and
// commits once. opNanos covers every set plus the single commit.
func runVolumeKVWriteSingleTx(ctx context.Context, c *config) error {
	// Open one write transaction on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open kv write tx")
	}
	defer tx.Discard()

	// Set every key and commit once, timing both.
	start := time.Now()
	for i := range c.iterations {
		if err := tx.Set(ctx, volumeKVKey(i), volumeKVValue(c)); err != nil {
			return errors.Wrap(err, "set kv")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit kv tx")
	}
	c.extra = map[string]int64{
		"opNanos": time.Since(start).Nanoseconds(),
		"ops":     int64(c.iterations),
	}
	return nil
}

// runVolumeRuntimeVerify checks saved metadata, referenced block content, and storage totals.
func runVolumeRuntimeVerify(ctx context.Context, c *config) error {
	// Open a read transaction on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open volume read tx")
	}
	defer tx.Discard()

	// Check the metadata and read the block reference.
	meta, found, err := tx.Get(ctx, volumeMetaKey())
	if err != nil {
		return errors.Wrap(err, "get volume meta")
	}
	if !found || !bytes.Equal(meta, volumeMetaValue()) {
		return errors.Errorf("volume meta mismatch found=%v value=%q", found, string(meta))
	}
	refData, found, err := tx.Get(ctx, volumeRefKey())
	if err != nil {
		return errors.Wrap(err, "get volume block ref")
	}
	if !found {
		return errors.New("volume block ref missing")
	}

	// Check the referenced block's content.
	ref := &block.BlockRef{}
	if err := ref.UnmarshalVT(refData); err != nil {
		return errors.Wrap(err, "unmarshal volume block ref")
	}
	data, found, err := vol.GetBlock(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "get volume block")
	}
	if !found || !bytes.Equal(data, volumeBlockValue()) {
		return errors.Errorf("volume block mismatch found=%v value=%q", found, string(data))
	}

	// Check that the statistics count the block.
	stats, err := vol.GetStorageStats(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume stats")
	}
	if stats.GetBlockCount() != 1 {
		return errors.Errorf("volume block count=%d want=1", stats.GetBlockCount())
	}
	if stats.GetTotalBytes() < uint64(len(data)) {
		return errors.Errorf("volume total bytes=%d want at least %d", stats.GetTotalBytes(), len(data))
	}
	return nil
}

// runVolumeRuntimeDeleteVerify requires explicit Volume.Delete to remove its subtree.
func runVolumeRuntimeDeleteVerify(ctx context.Context, c *config) error {
	// Delete the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	if err := vol.Delete(); err != nil {
		return err
	}

	// Check that its directory is gone.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	_, err = opfs.GetDirectoryPath(root, strings.Split(c.root+"/volume", "/"), false)
	if !opfs.IsNotFound(err) {
		return errors.Errorf("volume root after delete: %v", err)
	}
	return nil
}

// runWorldInitUnixFS initializes and reads an empty UnixFS root through the product volume.
func runWorldInitUnixFS(ctx context.Context, c *config) error {
	// Commit an empty UnixFS root in a world on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, vol)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Read the root back and check that it is empty.
	handle, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	var entries []string
	if err := handle.ReaddirAll(ctx, 0, func(ent unixfs_sdk.FSCursorDirent) error {
		entries = append(entries, ent.GetName())
		return nil
	}); err != nil {
		return errors.Wrap(err, "read unixfs root")
	}
	if len(entries) != 0 {
		return errors.Errorf("unixfs root entries = %v, want empty", entries)
	}
	return nil
}

// initUnixFSWorld builds a writable world state on the bucket c.root+"/world"
// of bkt and commits an empty UnixFS root named "files" in it. The release
// discards the state and its cursor.
func initUnixFSWorld(
	ctx context.Context,
	le *logrus.Entry,
	c *config,
	vol volume.Volume,
	bkt bucket.BucketOps,
) (*world_block.WorldState, func(), error) {
	// Build the world state on a cursor at the bucket's empty root.
	bucketID := c.root + "/world"
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		bkt,
		nil,
		&bucket.ObjectRef{BucketId: bucketID},
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	ws, err := world_block.BuildWorldStateFromCursor(
		ctx,
		le,
		true,
		cursor,
		world.NewWorldStorageFromCursor(cursor),
		space_world_ops.LookupWorldOp,
		false,
	)
	if err != nil {
		cursor.Release()
		return nil, nil, errors.Wrap(err, "build world state")
	}
	release := func() {
		ws.Discard()
		cursor.Release()
	}

	// Commit the UnixFS root.
	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, vol.GetPeerID(), "files", time.Now()); err != nil {
		release()
		return nil, nil, errors.Wrap(err, "init unixfs")
	}
	if err := ws.Commit(ctx); err != nil {
		release()
		return nil, nil, errors.Wrap(err, "commit initial world state")
	}
	return ws, release, nil
}

// openUnixFSHandle opens a handle on the UnixFS root "files" in ws. The
// release frees the handle and its cursor.
func openUnixFSHandle(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (*unixfs_sdk.FSHandle, func(), error) {
	// Follow the root, then open a handle on the cursor.
	fsCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: "files",
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		sender,
		true,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "follow unixfs")
	}
	handle, err := unixfs_sdk.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, nil, errors.Wrap(err, "open fs handle")
	}
	return handle, func() {
		handle.Release()
		fsCursor.Release()
	}, nil
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

// runWorldDeferredCrashRecovery checks that world commits with deferred
// durability survive a crash only up to the last Sync. A commit advances only
// the in-memory root; Sync writes the blocks, then advances the durable head.
// After a teardown without a final Sync, a reopened engine lands on the last
// synced head: the head, not block absence, provides the rollback.
func runWorldDeferredCrashRecovery(ctx context.Context, c *config) error {
	// Commit obj-a, Sync, commit obj-b, and tear down without a final Sync.
	headA, err := writeDeferredCommits(ctx, c)
	if err != nil {
		return err
	}

	// Reopen at the durable head and check that it is the synced one.
	recovered, err := openDeferredWorldEngine(ctx, c)
	if err != nil {
		return err
	}
	defer recovered.release()
	if !objectRefsEqual(recovered.engine.GetRootRef(), headA) {
		return errors.Errorf("OPFS recovery root=%v want last Sync'd head=%v", recovered.engine.GetRootRef(), headA)
	}

	// Check that obj-a recovered and obj-b rolled back. Reading obj-a also
	// shows that the head names only durable blocks.
	tx, err := recovered.engine.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open OPFS recovery read transaction")
	}
	defer tx.Discard()
	if _, found, err := tx.GetObject(ctx, "opfs-deferred-obj-a"); err != nil {
		return errors.Wrap(err, "read obj-a after OPFS recovery")
	} else if !found {
		return errors.New("OPFS recovery must land on the last Sync'd head with obj-a present")
	}
	if _, found, err := tx.GetObject(ctx, "opfs-deferred-obj-b"); err != nil {
		return errors.Wrap(err, "read obj-b after OPFS recovery")
	} else if found {
		return errors.New("post-Sync OPFS commit must not survive a crash before the next Sync")
	}
	return nil
}

// writeDeferredCommits commits obj-a, syncs, and commits obj-b on a deferred
// world engine, checking the durable head after each step, then tears the
// engine down without a final Sync. It returns the synced head.
func writeDeferredCommits(ctx context.Context, c *config) (*bucket.ObjectRef, error) {
	// Open the engine and read the seed head.
	writer, err := openDeferredWorldEngine(ctx, c)
	if err != nil {
		return nil, err
	}
	defer writer.release()
	seedHead, err := writer.readHead(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read seed OPFS world head")
	}

	// Commit obj-a; the durable head must stay at the seed.
	if err := createWorldObject(ctx, writer, "opfs-deferred-obj-a"); err != nil {
		return nil, err
	}
	if err := checkHead(ctx, writer, seedHead, "deferred commit must not advance the durable OPFS head before Sync"); err != nil {
		return nil, err
	}
	rootAfterA := writer.engine.GetRootRef().Clone()

	// Sync; the durable head must advance to the in-memory root.
	if _, err := writer.engine.Sync(ctx); err != nil {
		return nil, errors.Wrap(err, "Sync OPFS deferred world engine")
	}
	if err := checkHead(ctx, writer, rootAfterA, "Sync must advance the durable OPFS head to the in-memory root"); err != nil {
		return nil, err
	}
	if objectRefsEqual(rootAfterA, seedHead) {
		return nil, errors.New("Sync'd OPFS head must differ from the seed head")
	}

	// Commit obj-b; the durable head must stay at obj-a.
	if err := createWorldObject(ctx, writer, "opfs-deferred-obj-b"); err != nil {
		return nil, err
	}
	if err := checkHead(ctx, writer, rootAfterA, "post-Sync deferred commit must not advance the durable OPFS head"); err != nil {
		return nil, err
	}
	return rootAfterA, nil
}

// checkHead returns an error with msg unless the durable head of h is want.
func checkHead(ctx context.Context, h *worldEngine, want *bucket.ObjectRef, msg string) error {
	head, err := h.readHead(ctx)
	if err != nil {
		return errors.Wrap(err, "read OPFS world head")
	}
	if !objectRefsEqual(head, want) {
		return errors.New(msg)
	}
	return nil
}

// createWorldObject commits one named object through the world transaction interface.
func createWorldObject(ctx context.Context, h *worldEngine, key string) error {
	// Create the object in a write transaction and commit it.
	tx, err := h.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrapf(err, "open OPFS writer for %q", key)
	}
	if _, err := tx.CreateObject(ctx, key, nil); err != nil {
		tx.Discard()
		return errors.Wrapf(err, "create OPFS world object %q", key)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrapf(err, "commit OPFS world object %q", key)
	}
	return nil
}

// worldEngine owns a volume, object store, cursor, and world engine.
type worldEngine struct {
	// vol owns the persisted volume.
	vol *volume_browser.Volume
	// store provides the durable world head transaction API.
	store object.ObjectStore
	// storeRelease releases the object-store reference.
	storeRelease func()
	// cursor pins the world root used to construct the engine.
	cursor *bucket_lookup.Cursor
	// engine owns the world transaction lifecycle.
	engine *world_block.Engine
}

// openDeferredWorldEngine mounts a single-writer world at the persisted head.
// Block writes and the durable head both batch until Sync, so a teardown
// without Sync rolls back to the last synced head.
func openDeferredWorldEngine(ctx context.Context, c *config) (*worldEngine, error) {
	// Open the volume and the object store holding the head.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return nil, err
	}
	bucketID := c.root + "/world-coord-bucket"
	kvStore := store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), vol.GetKvtxStore(), &store_kvtx.Config{})
	objStore, storeRelease, err := kvStore.AccessObjectStore(ctx, c.root+"/world-coord-store", nil)
	if err != nil {
		_ = vol.Close()
		return nil, errors.Wrap(err, "open OPFS world object store")
	}
	h := &worldEngine{vol: vol, store: objStore, storeRelease: storeRelease}

	// Read the durable head, seeding an empty one on first open.
	headRef, err := h.readHead(ctx)
	if err != nil {
		h.release()
		return nil, err
	}
	if headRef == nil {
		headRef = &bucket.ObjectRef{BucketId: bucketID}
		if err := h.writeHead(ctx, headRef); err != nil {
			h.release()
			return nil, errors.Wrap(err, "seed OPFS world head")
		}
	}

	// Open the engine on a cursor at the head, publishing heads through casHead.
	le := logrus.NewEntry(logrus.New())
	h.cursor = bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		vol,
		nil,
		headRef,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	engine, err := world_block.NewEngine(
		ctx,
		le,
		h.cursor,
		space_world_ops.LookupWorldOp,
		h.casHead,
		false,
		world_block.WithDeferredDurability(),
	)
	if err != nil {
		h.release()
		return nil, errors.Wrap(err, "open OPFS world engine")
	}
	h.engine = engine
	return h, nil
}

// readHead reads the durable world head.
func (h *worldEngine) readHead(ctx context.Context) (*bucket.ObjectRef, error) {
	// Read the head record, if any.
	tx, err := h.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte("world-head"))
	if err != nil || !found {
		return nil, err
	}

	// Decode the head reference.
	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return state.GetHeadRef().Clone(), nil
}

// writeHead commits a serialized world head through the object store.
func (h *worldEngine) writeHead(ctx context.Context, ref *bucket.ObjectRef) error {
	// Encode the head and commit it in a write transaction.
	tx, err := h.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	data, err := (&world_block_engine.HeadState{HeadRef: ref}).MarshalVT()
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, []byte("world-head"), data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// casHead rejects a changed base before publishing the next head.
func (h *worldEngine) casHead(ctx context.Context, baseRef, nextRef *bucket.ObjectRef) error {
	current, err := h.readHead(ctx)
	if err != nil {
		return err
	}
	if !objectRefsEqual(current, baseRef) {
		return coord.ErrStaleGeneration
	}
	return h.writeHead(ctx, nextRef)
}

// objectRefsEqual compares optional object references by value.
func objectRefsEqual(a, b *bucket.ObjectRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EqualsRef(b)
}

// release closes the world engine before releasing its cursor, store, and volume.
func (h *worldEngine) release() {
	if h.engine != nil {
		_ = h.engine.Close()
	}
	if h.cursor != nil {
		h.cursor.Release()
	}
	if h.storeRelease != nil {
		h.storeRelease()
	}
	if h.vol != nil {
		_ = h.vol.Close()
	}
}

// runWorldLargeUnixFSUpload writes and reads a large deterministic file through the world API.
func runWorldLargeUnixFSUpload(ctx context.Context, c *config) error {
	// Commit an empty UnixFS root in a world on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, vol)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Write the large file through a batch writer.
	totalSize := largeFileSize(c)
	b := unixfs_world.NewBatchFSWriter(
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		vol.GetPeerID(),
	)
	defer b.Release()
	if err := b.AddFile(
		ctx,
		nil,
		"large-video.mp4",
		unixfs_sdk.NewFSCursorNodeType_File(),
		int64(totalSize),
		newDeterministicLargeReader(totalSize, 0),
		0o644,
		time.Now(),
	); err != nil {
		return errors.Wrap(err, "add large unixfs file")
	}
	if err := b.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit large unixfs upload")
	}

	// Read the file back through a handle on the root.
	handle, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	largeFile, err := handle.Lookup(ctx, "large-video.mp4")
	if err != nil {
		return errors.Wrap(err, "lookup large file")
	}
	defer largeFile.Release()
	return verifyDeterministicFSFile(ctx, largeFile, totalSize, 0, c)
}

// runWorldResourceLargeUnixFSUpload checks the resource upload contract over a direct volume.
func runWorldResourceLargeUnixFSUpload(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, vol)
}

// runWorldResourceDirectUploadTreeLargeUnixFSUpload checks UploadTree without the RPC transport.
func runWorldResourceDirectUploadTreeLargeUnixFSUpload(ctx context.Context, c *config) error {
	// Commit an empty UnixFS root in a world on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, vol)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Serve the root as a UnixFS resource.
	handle, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	rootResource := resource_unixfs.NewFSHandleObjectResource(
		logrus.NewEntry(logrus.StandardLogger()),
		handle,
		nil,
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
	)

	// Upload the generated tree directly and check the written counts.
	totalSize := largeFileSize(c)
	postProgress(c, "direct-upload-tree-start", 0, totalSize)
	resp, err := rootResource.UploadTree(newGeneratedUploadTreeStream(ctx, c, "large-video.mp4", totalSize, 0))
	if err != nil {
		return errors.Wrap(err, "direct UploadTree")
	}
	postProgress(c, "direct-upload-tree-complete", int(resp.GetBytesWritten()), totalSize)
	if resp.GetBytesWritten() != int64(totalSize) {
		return errors.Errorf("UploadTree bytes_written=%d want=%d", resp.GetBytesWritten(), totalSize)
	}
	if resp.GetFilesWritten() != 1 {
		return errors.Errorf("UploadTree files_written=%d want=1", resp.GetFilesWritten())
	}

	// Read the file back through the resource's handle.
	largeFile, err := rootResource.GetHandle().Lookup(ctx, "large-video.mp4")
	if err != nil {
		return errors.Wrap(err, "lookup direct uploaded file")
	}
	defer largeFile.Release()
	return verifyDeterministicFSFile(ctx, largeFile, totalSize, 0, c)
}

// runWorldControllerResourceLargeUnixFSUpload checks resource upload through a live volume controller.
func runWorldControllerResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, bkt, cleanup, err := openControllerBucket(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, bkt)
}

// runWorldCloudOverlayResourceLargeUnixFSUpload checks resource upload through the dirty-tracking overlay.
func runWorldCloudOverlayResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, bkt, cleanup, err := openControllerCloudOverlayBucket(ctx, c, false)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, bkt)
}

// runWorldCloudSyncResourceLargeUnixFSUpload checks resource upload while dirty blocks are packed.
func runWorldCloudSyncResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, bkt, cleanup, err := openControllerCloudOverlayBucket(ctx, c, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, bkt)
}

// runWorldResourceLargeUnixFSUploadOnBucket checks streamed upload and readback through resource clients.
func runWorldResourceLargeUnixFSUploadOnBucket(
	ctx context.Context,
	c *config,
	vol volume.Volume,
	bkt bucket.BucketOps,
) (retErr error) {
	// Commit an empty UnixFS root in a world on the bucket.
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, bkt)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Serve the root as a UnixFS resource through a resource client.
	handle, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	rootResource := resource_unixfs.NewFSHandleObjectResource(
		logrus.NewEntry(logrus.StandardLogger()),
		handle,
		nil,
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
	)
	resClient, cleanup, err := openResourceClient(ctx, rootResource.GetMux())
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	// Open the root resource's service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get root resource client")
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Upload the file, stopping there for the write-only scenario.
	totalSize := largeFileSize(c)
	postProgress(c, "resource-upload-start", 0, totalSize)
	if err := uploadDeterministicResourceFile(ctx, rootSvc, "large-video.mp4", totalSize, 0, c); err != nil {
		return err
	}
	postProgress(c, "resource-upload-complete", totalSize, totalSize)
	if c.scenario == "world-resource-large-unixfs-write" {
		return nil
	}

	// Look up the uploaded file as a new resource.
	postProgress(c, "resource-lookup-start")
	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "large-video.mp4",
	})
	if err != nil {
		return errors.Wrap(err, "lookup uploaded resource file")
	}
	postProgress(c, "resource-lookup-complete")
	fileRef := resClient.CreateResourceReference(fileResp.GetResourceId())
	defer fileRef.Release()

	// Open the file resource's client.
	postProgress(c, "resource-client-start")
	fileClient, err := fileRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file client")
	}
	postProgress(c, "resource-client-complete")

	// Read the file back through its resource.
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	postProgress(c, "resource-readback-start", 0, totalSize)
	if err := verifyDeterministicResourceFile(ctx, fileSvc, totalSize, 0, c); err != nil {
		return err
	}
	postProgress(c, "resource-readback-complete", totalSize, totalSize)
	return nil
}

// openControllerBucket returns a running volume controller's bucket and ordered cleanup.
func openControllerBucket(
	ctx context.Context,
	c *config,
) (volume.Volume, bucket.BucketOps, func() error, error) {
	// Run a volume controller on the browser volume.
	le := logrus.NewEntry(logrus.New())
	ctrlCtx, cancelCtrl := context.WithCancel(ctx)
	ctrl := volume_controller.NewController(
		le,
		&volume_controller.Config{DisablePeer: true},
		nil,
		controller.NewInfo(
			volume_browser.ControllerID,
			volume_browser.Version,
			"opfs-chrometest@"+c.root,
		),
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return volume_browser.NewVolume(ctx, le, newVolumeConfig(c))
		},
	)
	ctrlErrCh := make(chan error, 1)
	go func() {
		ctrlErrCh <- ctrl.Execute(ctrlCtx)
	}()

	// Stop the controller on cleanup, ignoring the cancellation itself.
	cleanup := func() error {
		cancelCtrl()
		err := <-ctrlErrCh
		if err != nil && !stderrors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}

	// Wait for the volume and apply the world bucket's config.
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, err
	}
	bucketID := c.root + "/world"
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  bucketID,
		Rev: 1,
	}); err != nil {
		_ = cleanup()
		return nil, nil, nil, errors.Wrap(err, "apply controller bucket config")
	}

	// Build the bucket API, releasing it before the controller on cleanup.
	bktHandle, releaseBucket, err := ctrl.BuildBucketAPI(ctx, bucketID)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, errors.Wrap(err, "build controller bucket api")
	}
	bkt := bktHandle.GetBucket()
	if bkt == nil {
		releaseBucket()
		_ = cleanup()
		return nil, nil, nil, errors.New("controller bucket handle did not exist")
	}
	return vol, bkt, func() error {
		releaseBucket()
		return cleanup()
	}, nil
}

// runCopyWalkWrapperConcurrency probes whether the production
// AccessWorldState -> FollowRef -> lookup Handle -> CopyObjectToBucket ->
// WalkObjectBlocks wrapper deadlocks at a raised maxConcurrency on real OPFS
// under native Go-WASM, with the GoScript compiler held out of the loop. A prior
// bench drove the raw OPFS engine at concurrency 16 directly and stayed healthy;
// this exercises the full wrapper path the production download-manifest copy
// uses, including the real concurrent-lookup Handle that resolves the
// cross-bucket source ref.
//
// It stands up a real controllerbus over the OPFS volume with the
// concurrent-lookup controller (so cross-bucket FollowRef resolves through the
// production Handle), builds a wide source-object block DAG in a bucket distinct
// from the dest world root (CopyObjectToBucket no-ops when src and dest share a
// bucket), then runs the production nested-access copy pattern twice over fresh
// equivalent source objects: first at maxConcurrency=1 (control, must pass),
// then at c.batch (the suspect, default 16). c.iterations is the source input
// byte count; the JC chunker fans it into hundreds of leaf blocks.
func runCopyWalkWrapperConcurrency(ctx context.Context, c *config) error {
	// Default the source size and the suspect concurrency.
	inputBytes := c.iterations
	if inputBytes <= 0 {
		inputBytes = 64 * 1024
	}
	suspectConc := c.batch
	if suspectConc <= 0 {
		suspectConc = 16
	}

	// Build a real bus over the browser volume so cross-bucket FollowRef
	// resolves through the production concurrent-lookup Handle, matching
	// download-manifest, and run the configset controller on it.
	le := logrus.NewEntry(logrus.New())
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return errors.Wrap(err, "construct core bus")
	}
	sr.AddFactory(volume_browser.NewFactory(b))
	_, _, csRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load configset controller")
	}
	defer csRef.Release()

	// Node controller owns per-bucket lookup loading: it reacts to applied
	// bucket configs and loads the concurrent-lookup controller that resolves
	// BuildBucketLookup. Without it FollowRef waits forever for the lookup Handle.
	_, _, nodeRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&node_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load node controller")
	}
	defer nodeRef.Release()

	// Run the browser volume controller and wait for its volume.
	volDV, _, volRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(newVolumeConfig(c)),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load browser volume controller")
	}
	defer volRef.Release()
	vol, err := volDV.(volume.Controller).GetVolume(ctx)
	if err != nil {
		return errors.Wrap(err, "get browser volume")
	}
	volID := vol.GetID()

	// Bucket config carrying the concurrent-lookup controller so each bucket
	// resolves through the same Handle the production world path uses. Single
	// node, so the default NONE not-found behavior (no remote lookup wait).
	lookupConf := node_controller.BuildDefaultLookupConfig()
	lookupCC, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf), false)
	if err != nil {
		return errors.Wrap(err, "encode lookup controller config")
	}

	// Apply the world and source bucket configs.
	worldBucketID := c.root + "/world"
	sourceBucketID := c.root + "/source"
	for _, bucketID := range []string{worldBucketID, sourceBucketID} {
		if _, err := bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(
			&bucket.Config{
				Id:     bucketID,
				Rev:    1,
				Lookup: &bucket.LookupConfig{Controller: lookupCC},
			},
			volID,
		)); err != nil {
			return errors.Wrapf(err, "apply bucket config %s", bucketID)
		}
	}

	// Share a gzip transform so stored bytes hash consistently with their
	// object refs across both buckets; CopyObjectToBucket's forced-ref writes
	// require the source stored representation to match its ref.
	sfs := transform_all.BuildFactorySet()
	transformConf, err := block_transform.NewConfig([]cbconfig.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		return errors.Wrap(err, "build transform config")
	}

	// Build the destination world state on the world bucket.
	worldCursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, b, le, sfs, worldBucketID, volID, transformConf, nil)
	if err != nil {
		return errors.Wrap(err, "build world cursor")
	}
	defer worldCursor.Release()
	ws, err := world_block.BuildWorldStateFromCursor(
		ctx,
		le,
		true,
		worldCursor,
		world.NewWorldStorageFromCursor(worldCursor),
		space_world_ops.LookupWorldOp,
		false,
	)
	if err != nil {
		return errors.Wrap(err, "build world state")
	}
	defer ws.Discard()

	// Source cursor for building wide DAGs in a distinct bucket, bus-backed.
	sourceCursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, b, le, sfs, sourceBucketID, volID, transformConf, nil)
	if err != nil {
		return errors.Wrap(err, "build source cursor")
	}
	defer sourceCursor.Release()
	sourceAccess := world.NewAccessWorldStateFunc(sourceCursor)

	// Build sources with small chunks for a wide DAG: ChunkIndex -> many Chunk
	// -> many ByteSlice leaves, so WalkObjectBlocks has real fan-out to
	// schedule.
	blobOpts := &blob.BuildBlobOpts{
		RawHighWaterMark: 1,
		ChunkerArgs: &blob.ChunkerArgs{
			ChunkerType: blob.ChunkerType_ChunkerType_JC,
			JcArgs: &blob.JcArgs{
				ChunkingMinSize:    64,
				ChunkingTargetSize: 128,
				ChunkingMaxSize:    256,
			},
		},
	}
	buildSource := func(salt int) (*bucket.ObjectRef, error) {
		return world.AccessObject(ctx, sourceAccess, nil, func(bcs *block.Cursor) error {
			_, err := blob.BuildBlob(
				ctx,
				int64(inputBytes),
				newDeterministicLargeReader(inputBytes, salt),
				bcs,
				blobOpts,
			)
			return err
		})
	}

	// Copy a fresh source at the control concurrency, then at the suspect one.
	runs := []struct {
		label string
		conc  int
	}{
		{label: "control", conc: 1},
		{label: "suspect", conc: suspectConc},
	}
	for i, r := range runs {
		postProgress(c, "copy-walk-source-build-start", i, r.conc)
		srcObjRef, err := buildSource(i + 1)
		if err != nil {
			return errors.Wrapf(err, "build source DAG (%s)", r.label)
		}
		postProgress(c, "copy-walk-source-build-complete", i, r.conc)

		postProgress(c, "copy-walk-copy-start", i, r.conc)
		if err := runCopyWalkWrapperCopy(ctx, le, ws, srcObjRef, r.conc, r.label); err != nil {
			return errors.Wrapf(err, "%s copy at concurrency %d", r.label, r.conc)
		}
		postProgress(c, "copy-walk-copy-complete", i, r.conc)
	}

	// Report the probe's dimensions.
	c.extra = map[string]int64{
		"inputBytes":  int64(inputBytes),
		"controlConc": 1,
		"suspectConc": int64(suspectConc),
	}
	return nil
}

// runCopyWalkWrapperCopy runs one wrapper copy mirroring the production
// nested-access shape: dest world bucket, then source bucket via cross-bucket
// FollowRef, then CopyObjectToBucket + Sync. A concurrency regression in the
// wrapper surfaces as the copy never returning, which the chrome harness context
// deadline turns into a test failure.
func runCopyWalkWrapperCopy(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	srcObjRef *bucket.ObjectRef,
	maxConcurrency int,
	label string,
) error {
	return ws.AccessWorldState(ctx, nil, func(dest *bucket_lookup.Cursor) error {
		return ws.AccessWorldState(ctx, srcObjRef, func(src *bucket_lookup.Cursor) error {
			le.Infof(
				"copy-walk-wrapper %s: copying DAG bucket %s -> %s at concurrency %d",
				label,
				src.GetOpArgs().GetBucketId(),
				dest.GetOpArgs().GetBucketId(),
				maxConcurrency,
			)
			if _, err := bucket_lookup.CopyObjectToBucket(
				ctx,
				dest,
				src,
				blob.NewBlobBlock,
				maxConcurrency,
				false,
				nil,
			); err != nil {
				return errors.Wrap(err, "copy object to bucket")
			}
			if _, err := ws.Sync(ctx); err != nil {
				return errors.Wrap(err, "sync copied blocks")
			}
			return nil
		})
	})
}

// openControllerCloudOverlayBucket wraps a controller bucket with dirty tracking and optional packing.
func openControllerCloudOverlayBucket(
	ctx context.Context,
	c *config,
	syncDuringUpload bool,
) (volume.Volume, bucket.BucketOps, func() error, error) {
	// Open the controller bucket and the dirty index store.
	vol, upper, cleanupBucket, err := openControllerBucket(ctx, c)
	if err != nil {
		return nil, nil, nil, err
	}
	objStore, releaseObjStore, err := vol.AccessObjectStore(ctx, c.root+"/cloud-overlay-meta", func() {})
	if err != nil {
		_ = cleanupBucket()
		return nil, nil, nil, errors.Wrap(err, "open cloud overlay dirty store")
	}

	// Wrap the bucket in a dirty-tracking write cache, packing as it goes when
	// requested.
	var flusher *probeSyncFlusher
	if syncDuringUpload {
		flusher = newProbeSyncFlusher(upper, objStore)
	}
	dirtyUpper := &probeDirtyTrackingStore{store: upper, dirtyStore: objStore, flusher: flusher}
	overlay := block.NewOverlay(
		ctx,
		logrus.NewEntry(logrus.New()),
		block.NopStoreOps{},
		dirtyUpper,
		block.OverlayMode_UPPER_WRITE_CACHE,
		0,
		nil,
	)

	// Wait for the packing, then release the store and the bucket.
	return vol, overlay, func() error {
		// Join the packer, then release in order, keeping the first error.
		var err error
		if flusher != nil {
			err = flusher.wait()
		}
		releaseObjStore()
		if cleanupErr := cleanupBucket(); err == nil {
			err = cleanupErr
		}
		return err
	}, nil
}

// probeDirtyTrackingStore records newly written blocks for the concurrent packing probe.
type probeDirtyTrackingStore struct {
	// store provides the underlying block operations.
	store block.StoreOps
	// dirtyStore stores the dirty index.
	dirtyStore kvtx.Store
	// flusher optionally packs blocks after the dirty-byte threshold.
	flusher *probeSyncFlusher
}

// GetHashType returns the underlying store's content hash algorithm.
func (d *probeDirtyTrackingStore) GetHashType() hash.HashType {
	return d.store.GetHashType()
}

// GetSupportedFeatures returns the underlying store's advertised capabilities.
func (d *probeDirtyTrackingStore) GetSupportedFeatures() block.StoreFeature {
	return d.store.GetSupportedFeatures()
}

// BeginReadOperation retains the underlying read scope while preserving dirty tracking.
func (d *probeDirtyTrackingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := d.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &probeDirtyTrackingStore{store: store, dirtyStore: d.dirtyStore, flusher: d.flusher}, release, nil
}

// PutBlock writes content and records a newly admitted block in the dirty store.
func (d *probeDirtyTrackingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := d.store.PutBlock(ctx, data, opts)
	if err == nil && !existed {
		err = d.markDirty(ctx, ref.GetHash(), int64(len(data)))
	}
	return ref, existed, err
}

// PutBlockBatch writes the batch and records each previously absent live block.
func (d *probeDirtyTrackingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Write the batch, then mark the previously absent live blocks dirty.
	existed, err := d.store.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		if entry == nil || entry.Tombstone || entry.Ref == nil || entry.Ref.GetEmpty() || existed[i] {
			continue
		}
		if err := d.markDirty(ctx, entry.Ref.GetHash(), int64(len(entry.Data))); err != nil {
			return nil, err
		}
	}
	return existed, nil
}

// GetBlock reads content from the underlying store.
func (d *probeDirtyTrackingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return d.store.GetBlock(ctx, ref)
}

// GetStoredBlock forwards to the inner store.
func (d *probeDirtyTrackingStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return d.store.GetStoredBlock(ctx, ref)
}

// GetBlockExists delegates the underlying store's presence check.
func (d *probeDirtyTrackingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return d.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch delegates a batch of presence checks.
func (d *probeDirtyTrackingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return d.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock removes the block through the underlying store.
func (d *probeDirtyTrackingStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return d.store.RmBlock(ctx, ref)
}

// StatBlock returns the underlying store's block metadata.
func (d *probeDirtyTrackingStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return d.store.StatBlock(ctx, ref)
}

// Sync fences writes through the underlying store.
func (d *probeDirtyTrackingStore) Sync(ctx context.Context) (bool, error) {
	return d.store.Sync(ctx)
}

// BeginDeferFlush opens the underlying store's optional deferred flush scope.
func (d *probeDirtyTrackingStore) BeginDeferFlush() {
	block.BeginDeferFlush(d.store)
}

// EndDeferFlush closes the underlying store's deferred flush scope.
func (d *probeDirtyTrackingStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, d.store)
}

// markDirty commits the dirty entry before notifying the optional packer.
func (d *probeDirtyTrackingStore) markDirty(ctx context.Context, h *hash.Hash, size int64) error {
	// Record the block's size under its hash.
	tx, err := d.dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open dirty tx")
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte("dirty/"+h.MarshalString()), []byte(strconv.FormatInt(size, 10))); err != nil {
		return errors.Wrap(err, "set dirty key")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit dirty key")
	}

	// Account the bytes with the packer, if any.
	if d.flusher != nil {
		d.flusher.markDirty(ctx, size)
	}
	return nil
}

const (
	// probeSyncSizeThresholdBytes starts packing after enough dirty data accumulates.
	probeSyncSizeThresholdBytes int64 = 48 * 1024 * 1024
	// probeSyncFlushMaxPackBytes bounds each in-memory pack payload.
	probeSyncFlushMaxPackBytes int64 = 4 * 1024 * 1024
)

// probeSyncFlusher starts one packing pass after dirty bytes cross its threshold.
type probeSyncFlusher struct {
	// upper reads dirty block payloads.
	upper block.StoreOps
	// dirtyStore provides the dirty-index transaction API.
	dirtyStore kvtx.Store
	// done reports the single packing pass's result.
	done chan error

	// mtx protects dirtySize and started.
	mtx sync.Mutex
	// dirtySize counts admitted dirty payload bytes under mtx.
	dirtySize int64
	// started prevents a second packing pass under mtx.
	started bool
}

// newProbeSyncFlusher constructs the single-pass dirty block packer.
func newProbeSyncFlusher(upper block.StoreOps, dirtyStore kvtx.Store) *probeSyncFlusher {
	return &probeSyncFlusher{
		upper:      upper,
		dirtyStore: dirtyStore,
		done:       make(chan error, 1),
	}
}

// markDirty accounts bytes and starts at most one background packing pass.
func (f *probeSyncFlusher) markDirty(ctx context.Context, size int64) {
	// Add the bytes and claim the pass once the threshold is crossed.
	f.mtx.Lock()
	f.dirtySize += size
	if f.started || f.dirtySize < probeSyncSizeThresholdBytes {
		f.mtx.Unlock()
		return
	}
	f.started = true
	f.mtx.Unlock()

	// Pack in the background.
	go func() {
		f.done <- f.flush(ctx)
	}()
}

// wait joins the packing pass if the threshold started one.
func (f *probeSyncFlusher) wait() error {
	// Join the pass only if one started.
	f.mtx.Lock()
	started := f.started
	f.mtx.Unlock()
	if !started {
		return nil
	}
	return <-f.done
}

// probeDirtyCandidate identifies a stored dirty block and its indexed byte length.
type probeDirtyCandidate struct {
	// hash identifies the stored block.
	hash *hash.Hash
	// size is its indexed payload length.
	size int64
}

// probeDirtyBlock holds one loaded dirty block until its pack is encoded.
type probeDirtyBlock struct {
	// hash identifies the loaded content.
	hash *hash.Hash
	// data holds the content until the current pack is encoded.
	data []byte
}

// flush packs dirty blocks in bounded chunks through the production pack writer.
func (f *probeSyncFlusher) flush(ctx context.Context) error {
	// Read the dirty index.
	candidates, err := f.scanDirty(ctx)
	if err != nil {
		return err
	}

	// Load and pack each bounded chunk in turn.
	maxBlocks := int(packfile_writer.DefaultPolicy().MaxBlocksPerPack)
	for start := 0; start < len(candidates); {
		end, err := nextProbeDirtyChunk(candidates, start, probeSyncFlushMaxPackBytes, maxBlocks)
		if err != nil {
			return err
		}
		blocks, err := f.loadDirtyBlocks(ctx, candidates[start:end])
		if err != nil {
			return err
		}
		if err := packProbeDirtyBlocks(blocks); err != nil {
			return err
		}
		start = end
	}
	return nil
}

// scanDirty reads the probe's dirty index from one metadata transaction.
func (f *probeSyncFlusher) scanDirty(ctx context.Context) ([]probeDirtyCandidate, error) {
	// Open a read transaction on the index.
	tx, err := f.dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open dirty scan tx")
	}
	defer tx.Discard()

	// Decode each entry's hash and size; an unreadable size counts as zero.
	var out []probeDirtyCandidate
	prefix := []byte("dirty/")
	if err := tx.ScanPrefix(ctx, prefix, func(k, v []byte) error {
		// Parse the hash from the key.
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(k[len(prefix):])); err != nil {
			return err
		}

		// Read the size.
		size, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil || size < 0 {
			size = 0
		}
		out = append(out, probeDirtyCandidate{hash: h, size: size})
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "scan dirty keys")
	}
	return out, nil
}

// loadDirtyBlocks loads the present blocks for one bounded candidate chunk.
func (f *probeSyncFlusher) loadDirtyBlocks(ctx context.Context, candidates []probeDirtyCandidate) ([]probeDirtyBlock, error) {
	// Skip candidates the upper store no longer holds.
	blocks := make([]probeDirtyBlock, 0, len(candidates))
	for _, candidate := range candidates {
		data, found, err := f.upper.GetBlock(ctx, block.NewBlockRef(candidate.hash))
		if err != nil {
			return nil, errors.Wrap(err, "get dirty block")
		}
		if !found {
			continue
		}
		blocks = append(blocks, probeDirtyBlock{hash: candidate.hash, data: data})
	}
	return blocks, nil
}

// nextProbeDirtyChunk selects a nonempty chunk within the pack writer's limits.
func nextProbeDirtyChunk(blocks []probeDirtyCandidate, start int, maxChunkBytes int64, maxChunkBlocks int) (int, error) {
	// Extend the chunk until a limit; an unknown size fills a whole chunk.
	var chunkBytes int64
	end := start
	for end < len(blocks) {
		size := blocks[end].size
		if size <= 0 {
			size = maxChunkBytes
		}
		if size > packfile_writer.DefaultMaxPackBytes {
			return 0, errors.Errorf("dirty block %s exceeds max pack chunk size", blocks[end].hash.MarshalString())
		}
		if maxChunkBlocks > 0 && end-start >= maxChunkBlocks {
			break
		}
		if chunkBytes > 0 && chunkBytes+size > maxChunkBytes {
			break
		}
		chunkBytes += size
		end++
	}

	// Always take at least one block.
	if end == start {
		end++
	}
	return end, nil
}

// packProbeDirtyBlocks encodes a chunk with the production pack format.
func packProbeDirtyBlocks(blocks []probeDirtyBlock) error {
	// Feed the blocks to the writer in order and discard the encoding.
	var buf bytes.Buffer
	idx := 0
	_, err := packfile_writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		dirty := blocks[idx]
		idx++
		return dirty.hash, &block.StoredBlock{Data: dirty.data, RefsKnown: true}, nil
	})
	return errors.Wrap(err, "pack dirty blocks")
}

// openResourceClient returns a live resource client for rootMux and a cleanup
// that joins its server.
func openResourceClient(
	ctx context.Context,
	rootMux srpc.Mux,
) (*resource_client.Client, func() error, error) {
	// Serve the resource service over an in-memory connection.
	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(serverMux); err != nil {
		return nil, nil, errors.Wrap(err, "register resource server")
	}
	conn, closeConn, err := serveMuxedPipe(ctx, serverMux)
	if err != nil {
		return nil, nil, err
	}

	// Open the resource client on the connection.
	resClient, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(conn))
	if err != nil {
		_ = closeConn()
		return nil, nil, errors.Wrap(err, "open resource client")
	}
	return resClient, func() error {
		resClient.Release()
		return closeConn()
	}, nil
}

// isExpectedMuxCloseError recognizes the normal transport termination errors.
func isExpectedMuxCloseError(err error) bool {
	return stderrors.Is(err, context.Canceled) ||
		stderrors.Is(err, io.EOF) ||
		stderrors.Is(err, io.ErrClosedPipe) ||
		stderrors.Is(err, net.ErrClosed)
}

// uploadDeterministicResourceFile streams one deterministic file through UploadTree.
func uploadDeterministicResourceFile(
	ctx context.Context,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	name string,
	totalSize int,
	salt int,
	c *config,
) error {
	// Open the stream and send the file header.
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		return errors.Wrap(err, "open UploadTree stream")
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      name,
				TotalSize: int64(totalSize),
				Mode:      0o644,
			},
		},
	}); err != nil {
		return errors.Wrap(err, "send UploadTree file_start")
	}

	// Stream the payload in 64 KiB chunks.
	const chunkSize = 64 * 1024
	for offset := 0; offset < totalSize; offset += chunkSize {
		n := min(chunkSize, totalSize-offset)
		if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
				Data: deterministicLargeWindow(offset, n, salt),
			},
		}); err != nil {
			return errors.Wrapf(err, "send UploadTree data offset=%d", offset)
		}
		next := offset + n
		if next == totalSize || next%largeScenarioProgressEvery == 0 {
			postProgress(c, "resource-upload-stream", next, totalSize)
		}
	}

	// Close the stream and check the written totals.
	postProgress(c, "resource-upload-close-start", totalSize, totalSize)
	resp, err := strm.CloseAndRecv()
	if err != nil {
		return errors.Wrap(err, "close UploadTree stream")
	}
	postProgress(c, "resource-upload-close-complete", totalSize, totalSize)
	if resp.GetBytesWritten() != int64(totalSize) {
		return errors.Errorf("UploadTree bytes_written=%d want=%d", resp.GetBytesWritten(), totalSize)
	}
	if resp.GetFilesWritten() != 1 {
		return errors.Errorf("UploadTree files_written=%d want=1", resp.GetFilesWritten())
	}
	return nil
}

// verifyDeterministicResourceFile checks file size, sampled offsets, and full content over RPC.
func verifyDeterministicResourceFile(
	ctx context.Context,
	fileSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	totalSize int,
	salt int,
	c *config,
) error {
	// Check the size.
	postProgress(c, "resource-readback-size-start", 0, totalSize)
	sizeResp, err := fileSvc.GetSize(ctx, &s4wave_unixfs.HandleGetSizeRequest{})
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file size")
	}
	postProgress(c, "resource-readback-size-complete", int(sizeResp.GetSize()), totalSize)
	if sizeResp.GetSize() != uint64(totalSize) {
		return errors.Errorf("resource file size=%d want=%d", sizeResp.GetSize(), totalSize)
	}

	// Check sampled windows at the start, middle, and end.
	for _, offset := range []int{0, 4096, totalSize / 2, max(0, totalSize-4096)} {
		wantLen := min(4096, totalSize-offset)
		postProgress(c, "resource-readback-read-start", offset, totalSize)
		resp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(wantLen),
		})
		if err != nil {
			return errors.Wrapf(err, "read uploaded resource file offset=%d", offset)
		}
		postProgress(c, "resource-readback-read-complete", offset+len(resp.GetData()), totalSize)
		if len(resp.GetData()) != wantLen {
			return errors.Errorf("resource file offset=%d read=%d want=%d", offset, len(resp.GetData()), wantLen)
		}
		want := deterministicLargeWindow(offset, wantLen, salt)
		if !bytes.Equal(resp.GetData(), want) {
			return errors.Errorf("resource file offset=%d data mismatch", offset)
		}
	}

	// Read back the whole file in bounded windows.
	postProgress(c, "resource-readback-full-start", 0, totalSize)
	fullReadChunkSize := resourceFullReadChunkSize(c)
	fullReadProgressEvery := max(largeScenarioProgressEvery, fullReadChunkSize)
	for offset := 0; offset < totalSize; {
		wantLen := min(fullReadChunkSize, totalSize-offset)
		resp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(wantLen),
		})
		if err != nil {
			return errors.Wrapf(err, "full read uploaded resource file offset=%d", offset)
		}

		// Check the returned bytes and advance.
		got := resp.GetData()
		if len(got) == 0 {
			return errors.Errorf("full read resource file offset=%d read=0 want progress", offset)
		}
		if len(got) > wantLen {
			return errors.Errorf("full read resource file offset=%d read=%d max=%d", offset, len(got), wantLen)
		}
		want := deterministicLargeWindow(offset, len(got), salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("full read resource file offset=%d data mismatch", offset)
		}
		offset += len(got)
		if offset == totalSize || offset%fullReadProgressEvery == 0 {
			postProgress(c, "resource-readback-full-stream", offset, totalSize)
		}
	}
	postProgress(c, "resource-readback-full-complete", totalSize, totalSize)
	return nil
}

// resourceFullReadChunkSize returns the requested readback window or its bounded default.
func resourceFullReadChunkSize(c *config) int {
	if c != nil && c.batch > 0 {
		return c.batch
	}
	return 256 * 1024
}

// verifyDeterministicFSFile checks file size, sampled offsets, and full content through FSHandle.
func verifyDeterministicFSFile(
	ctx context.Context,
	handle *unixfs_sdk.FSHandle,
	totalSize int,
	salt int,
	c *config,
) error {
	// Check the size.
	postProgress(c, "fs-readback-size-start", 0, totalSize)
	size, err := handle.GetSize(ctx)
	if err != nil {
		return errors.Wrap(err, "get large file size")
	}
	postProgress(c, "fs-readback-size-complete", int(size), totalSize)
	if size != uint64(totalSize) {
		return errors.Errorf("large file size=%d want=%d", size, totalSize)
	}

	// Check sampled windows at the start, middle, and end.
	for _, offset := range []int{0, 4096, totalSize / 2, max(0, totalSize-4096)} {
		wantLen := min(4096, totalSize-offset)
		got := make([]byte, wantLen)
		postProgress(c, "fs-readback-read-start", offset, totalSize)
		n, err := handle.ReadAt(ctx, int64(offset), got)
		if err != nil && err != io.EOF {
			return errors.Wrapf(err, "read large file offset=%d", offset)
		}
		postProgress(c, "fs-readback-read-complete", offset+int(n), totalSize)
		if int(n) != wantLen {
			return errors.Errorf("large file offset=%d read=%d want=%d", offset, n, wantLen)
		}
		want := deterministicLargeWindow(offset, wantLen, salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("large file offset=%d data mismatch", offset)
		}
	}

	// Read back the whole file in bounded windows.
	postProgress(c, "fs-readback-full-start", 0, totalSize)
	fullReadChunkSize := resourceFullReadChunkSize(c)
	fullReadProgressEvery := max(largeScenarioProgressEvery, fullReadChunkSize)
	for offset := 0; offset < totalSize; {
		wantLen := min(fullReadChunkSize, totalSize-offset)
		got := make([]byte, wantLen)
		n, err := handle.ReadAt(ctx, int64(offset), got)
		if err != nil && err != io.EOF {
			return errors.Wrapf(err, "full read large file offset=%d", offset)
		}
		if n <= 0 {
			return errors.Errorf("full read large file offset=%d read=0 want progress", offset)
		}

		// Check the returned bytes and advance.
		got = got[:int(n)]
		want := deterministicLargeWindow(offset, len(got), salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("full read large file offset=%d data mismatch", offset)
		}
		offset += len(got)
		if offset == totalSize || offset%fullReadProgressEvery == 0 {
			postProgress(c, "fs-readback-full-stream", offset, totalSize)
		}
	}
	postProgress(c, "fs-readback-full-complete", totalSize, totalSize)
	return nil
}

// generatedUploadTreeStream generates one file stream without retaining its full payload.
type generatedUploadTreeStream struct {
	// ctx bounds the upload lifetime.
	ctx context.Context
	// c identifies the worker for progress reports.
	c *config
	// name names the uploaded file.
	name string
	// totalSize is the complete file length.
	totalSize int
	// salt selects the reproducible payload.
	salt int
	// offset is the next unread byte offset.
	offset int
	// startSent records whether the file header has been emitted.
	startSent bool
}

// newGeneratedUploadTreeStream constructs a deterministic UploadTree request source.
func newGeneratedUploadTreeStream(
	ctx context.Context,
	c *config,
	name string,
	totalSize int,
	salt int,
) *generatedUploadTreeStream {
	return &generatedUploadTreeStream{
		ctx:       ctx,
		c:         c,
		name:      name,
		totalSize: totalSize,
		salt:      salt,
	}
}

// Context returns the upload operation's cancellation context.
func (s *generatedUploadTreeStream) Context() context.Context {
	return s.ctx
}

// MsgSend accepts the unused response direction of the local upload stream.
func (s *generatedUploadTreeStream) MsgSend(srpc.Message) error {
	return nil
}

// MsgRecv fills a typed upload request from the next generated message.
func (s *generatedUploadTreeStream) MsgRecv(msg srpc.Message) error {
	req, ok := msg.(*s4wave_unixfs.HandleUploadTreeRequest)
	if !ok {
		return errors.Errorf("unexpected UploadTree stream recv target %T", msg)
	}
	return s.RecvTo(req)
}

// CloseSend accepts closure of the unused response direction.
func (s *generatedUploadTreeStream) CloseSend() error {
	return nil
}

// Close accepts closure of the generated stream, which owns no external handles.
func (s *generatedUploadTreeStream) Close() error {
	return nil
}

// Recv emits the file header, bounded data chunks, and then EOF.
func (s *generatedUploadTreeStream) Recv() (*s4wave_unixfs.HandleUploadTreeRequest, error) {
	// Emit the header first.
	if !s.startSent {
		s.startSent = true
		postProgress(s.c, "direct-upload-tree-file-start", 0, s.totalSize)
		return &s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
				FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
					Path:      s.name,
					TotalSize: int64(s.totalSize),
					Mode:      0o644,
				},
			},
		}, nil
	}

	// Emit EOF after the last chunk.
	if s.offset >= s.totalSize {
		postProgress(s.c, "direct-upload-tree-eof", s.totalSize, s.totalSize)
		return nil, io.EOF
	}

	// Emit the next 64 KiB chunk, reporting every 8 MiB.
	const chunkSize = 64 * 1024
	const progressEvery = 8 * 1024 * 1024
	n := min(chunkSize, s.totalSize-s.offset)
	offset := s.offset
	s.offset += n
	if s.offset == s.totalSize || s.offset%progressEvery == 0 {
		postProgress(s.c, "direct-upload-tree-stream", s.offset, s.totalSize)
	}
	return &s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
			Data: deterministicLargeWindow(offset, n, s.salt),
		},
	}, nil
}

// RecvTo copies the next generated message into the caller's request.
func (s *generatedUploadTreeStream) RecvTo(req *s4wave_unixfs.HandleUploadTreeRequest) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*req = *next
	return nil
}

// openVolume opens the browser volume under the scenario's isolated root.
func openVolume(ctx context.Context, c *config) (*volume_browser.Volume, error) {
	return volume_browser.NewVolume(ctx, logrus.NewEntry(logrus.New()), newVolumeConfig(c))
}

// newVolumeConfig names the volume by the scenario's isolated root.
func newVolumeConfig(c *config) *volume_browser.Config {
	return &volume_browser.Config{
		Name:        c.root + "/volume",
		StoreConfig: &store_kvtx.Config{},
	}
}

// runCounterInit creates and flushes the counter used by cross-worker lock probes.
func runCounterInit(c *config) error {
	// Lock the counter file.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return err
	}
	defer release()

	// Write and flush a zero value.
	var zero [8]byte
	if err := file.Truncate(int64(len(zero))); err != nil {
		return err
	}
	if _, err := file.WriteAt(zero[:], 0); err != nil {
		return err
	}
	return file.Flush()
}

// runCounterHold holds the counter's exclusive file lock until the harness releases it.
func runCounterHold(c *config) error {
	// Lock and read the counter, then hold it until released.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return errors.Wrap(err, "acquire held counter")
	}
	defer release()
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return errors.Wrap(err, "read held counter")
	}
	return waitCounterRelease(c)
}

// runCounterIncrement flushes each counter increment while holding its exclusive lock.
func runCounterIncrement(c *config) error {
	// Open the lock directory, then increment under the lock each iteration.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	for range c.iterations {
		if err := incrementCounter(dir, c); err != nil {
			return err
		}
	}
	return nil
}

// incrementCounter adds one to the counter in dir while holding its exclusive
// lock, flushing before the release.
func incrementCounter(dir js.Value, c *config) error {
	// Lock and read the counter.
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return errors.Wrap(err, "acquire counter")
	}
	defer release()
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return errors.Wrap(err, "read counter")
	}

	// Write and flush the incremented value.
	binary.LittleEndian.PutUint64(buf[:], binary.LittleEndian.Uint64(buf[:])+1)
	if _, err := file.WriteAt(buf[:], 0); err != nil {
		return errors.Wrap(err, "write counter")
	}
	return errors.Wrap(file.Flush(), "flush counter")
}

// runCounterTryLock checks the nonblocking Web Lock acquisition outcome.
func runCounterTryLock(c *config, want bool) error {
	// Try the lock and compare the outcome, releasing it if taken.
	release, acquired, err := filelock.AcquireWebLockIfAvailable(c.root+"/locks/counter", true)
	if err != nil {
		return err
	}
	if acquired != want {
		return errors.Errorf("try counter lock acquired=%v want %v", acquired, want)
	}
	if release != nil {
		release()
	}
	return nil
}

// runCounterTimeoutLock requires a queued Web Lock request to honor cancellation.
func runCounterTimeoutLock(ctx context.Context, c *config) error {
	// Request the held lock with a short deadline; it must end canceled.
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	result, err := opfs.DefaultDriver.AcquireWebLock(ctx, c.root+"/locks/counter", true)
	if err == nil {
		if result != nil && result.Release != nil {
			result.Release()
		}
		return errors.New("blocking WebLock unexpectedly acquired before timeout")
	}
	if result == nil || result.Outcome != opfs.WebLockOutcomeCanceled {
		return errors.Errorf("WebLock timeout outcome=%v err=%v", result, err)
	}
	return nil
}

// waitCounterRelease announces readiness and waits for the harness release message.
func waitCounterRelease(c *config) error {
	// Listen for the release message.
	ch := make(chan struct{}, 1)
	bc := js.Global().Get("BroadcastChannel").New(counterReleaseChannel(c.root))
	cb := js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data")
		if data.Get("type").String() == "release" {
			ch <- struct{}{}
		}
		return nil
	})
	defer cb.Release()
	defer bc.Call("close")
	bc.Set("onmessage", cb)

	// Announce readiness and wait.
	postReady(c)
	<-ch
	return nil
}

// runCounterVerify checks that serialized increments preserved every writer's update.
func runCounterVerify(c *config) error {
	// Lock the counter shared.
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", false)
	if err != nil {
		return err
	}
	defer release()

	// Check that every increment landed.
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return err
	}
	got := binary.LittleEndian.Uint64(buf[:])
	want := uint64(c.workers * c.iterations)
	if got != want {
		return errors.Errorf("counter=%d want=%d", got, want)
	}
	return nil
}

// openTestDirectory creates the requested descendant under the disposable test root.
func openTestDirectory(rootName string, parts []string) (js.Value, error) {
	// Resolve the path from the OPFS root.
	root, err := opfs.GetRoot()
	if err != nil {
		return js.Undefined(), err
	}
	path := append([]string{rootName}, parts...)
	return opfs.GetDirectoryPath(root, path, true)
}

// deterministicLargeBytes builds a reproducible payload from offset zero.
func deterministicLargeBytes(size int, salt int) []byte {
	buf := make([]byte, size)
	fillDeterministicLargeBytes(buf, 0, salt)
	return buf
}

// deterministicLargeWindow builds a reproducible window without earlier payload bytes.
func deterministicLargeWindow(offset, size int, salt int) []byte {
	buf := make([]byte, size)
	fillDeterministicLargeBytes(buf, offset, salt)
	return buf
}

// fillDeterministicLargeBytes fills a window using absolute offsets and a payload salt.
func fillDeterministicLargeBytes(buf []byte, offset int, salt int) {
	for i := range buf {
		buf[i] = deterministicLargeByte(offset+i, salt)
	}
}

// deterministicLargeByte mixes absolute offset and salt into a reproducible byte.
func deterministicLargeByte(offset int, salt int) byte {
	// Mix with a murmur-style finalizer.
	x := uint32(offset) + uint32(0x9e3779b9)
	x ^= uint32(salt) * uint32(0x85ebca6b)
	x ^= x >> 16
	x *= uint32(0x7feb352d)
	x ^= x >> 15
	x *= uint32(0x846ca68b)
	x ^= x >> 16
	return byte(x) + byte(offset)
}

// deterministicLargeReader streams deterministic content with bounded retained state.
type deterministicLargeReader struct {
	// remaining counts unread bytes.
	remaining int
	// offset is the absolute position of the next byte.
	offset int
	// salt selects the reproducible payload.
	salt int
}

// newDeterministicLargeReader constructs a finite deterministic payload stream.
func newDeterministicLargeReader(size int, salt int) *deterministicLargeReader {
	return &deterministicLargeReader{
		remaining: size,
		salt:      salt,
	}
}

// Read fills the caller's buffer until the deterministic stream reaches EOF.
func (r *deterministicLargeReader) Read(p []byte) (int, error) {
	// Report an empty read or EOF.
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}

	// Fill the next bytes.
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = deterministicLargeByte(r.offset, r.salt)
		r.offset++
	}
	r.remaining -= n
	return n, nil
}

// volumeMetaKey returns the runtime probe's metadata key.
func volumeMetaKey() []byte {
	return []byte("volume/runtime/meta")
}

// volumeMetaValue returns the runtime probe's expected metadata bytes.
func volumeMetaValue() []byte {
	return []byte("volume-runtime-meta-value")
}

// volumeRefKey returns the key holding the runtime probe's serialized block reference.
func volumeRefKey() []byte {
	return []byte("volume/runtime/block-ref")
}

// volumeBlockValue returns the runtime probe's expected block content.
func volumeBlockValue() []byte {
	return []byte("volume-runtime-block-value")
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

// counterReleaseChannel derives the lock-holder release channel from the test root.
func counterReleaseChannel(root string) string {
	return "opfs-chrometest-counter-release:" + root
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
