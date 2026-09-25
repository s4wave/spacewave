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
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/engine"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/hash"
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
}

// blockEvent identifies a published batch or completed writer.
type blockEvent struct {
	// typ identifies publication or writer completion.
	typ string
	// worker identifies the publisher.
	worker int
	// iteration identifies the published batch.
	iteration int
}

// blockEventSub owns a browser subscription and its buffered Go event queue.
type blockEventSub struct {
	// ch buffers all expected events so the browser callback never waits.
	ch chan blockEvent
	// bc owns the underlying BroadcastChannel.
	bc js.Value
	// cb retains the installed browser callback until Close.
	cb js.Func
}

// blockEventPub owns the send side of the workload's browser event channel.
type blockEventPub struct {
	// bc owns the send-only BroadcastChannel.
	bc js.Value
}

// largeScenarioProgressEvery bounds progress-report frequency during large transfers.
const largeScenarioProgressEvery = 8 * 1024 * 1024

// main runs one selected worker scenario and reports its terminal result.
func main() {
	start := time.Now()
	c, err := parseConfig(testArgs())
	if err == nil {
		err = run(context.Background(), c)
	}
	postResult(c, time.Since(start), err)
}

// testArgs returns process arguments or the browser harness fallback.
func testArgs() []string {
	if len(os.Args) >= 7 {
		return os.Args
	}
	val := js.Global().Get("__OPFS_CHROMETEST_ARGS")
	if val.IsUndefined() || val.IsNull() {
		return os.Args
	}
	n := val.Get("length").Int()
	args := make([]string, n)
	for i := range n {
		args[i] = val.Index(i).String()
	}
	return args
}

// parseConfig validates the scenario's numeric workload arguments.
func parseConfig(args []string) (*config, error) {
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

// run installs the browser driver and dispatches one selected scenario.
func run(ctx context.Context, c *config) error {
	opfs.InstallRemoteDriverFromGlobal()
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
	case "large-block-batch":
		return runLargeBlockBatch(ctx, c)
	case "large-block-verify":
		return runLargeBlockVerify(ctx, c)
	case "read-at-helper-loop":
		return runReadAtHelperLoop(c)
	case "engine-crash-before-root-block":
		return runEngineCrash(ctx, c, false, true)
	case "engine-crash-verify-block-clean":
		return verifyEngineCrash(ctx, c, true)
	case "engine-crash-before-root-meta":
		return runEngineCrash(ctx, c, false, false)
	case "engine-crash-after-root-meta":
		return runEngineCrash(ctx, c, true, false)
	case "engine-crash-verify-meta":
		return verifyEngineCrash(ctx, c, false)
	case "block-writer":
		return runBlockWriter(ctx, c)
	case "block-reader":
		return runBlockReader(ctx, c, false)
	case "block-reader-compact":
		return runBlockReader(ctx, c, true)
	case "block-verify":
		return runBlockVerify(ctx, c)
	case "remote-cache-lifecycle":
		return runRemoteCacheLifecycle(ctx, c)
	case "meta-writer":
		return runMetaWriter(ctx, c)
	case "meta-verify":
		return runMetaVerify(ctx, c)
	case "meta-mixed-writer":
		return runMetaMixedWriter(ctx, c)
	case "meta-mixed-verify":
		return runMetaMixedVerify(ctx, c)
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
	case "volume-runtime-seed-incompatible":
		return runVolumeRuntimeSeedIncompatible(c)
	case "volume-runtime-seed-unknown":
		return runVolumeRuntimeSeedUnknown(c)
	case "volume-runtime-verify-incompatible-recovered":
		return runVolumeRuntimeVerifyRecovered(ctx, c, "incompatible")
	case "volume-runtime-verify-unknown-recovered":
		return runVolumeRuntimeVerifyRecovered(ctx, c, "unknown")
	case "volume-runtime-delete-verify":
		return runVolumeRuntimeDeleteVerify(ctx, c)
	case "volume-kv-write-per-op":
		return runVolumeKVWritePerOp(ctx, c)
	case "volume-kv-write-single-tx":
		return runVolumeKVWriteSingleTx(ctx, c)
	case "volume-coord-local":
		return runVolumeCoordinatorLocal(ctx, c)
	case "volume-coord-watch":
		return runVolumeCoordinatorWatch(ctx, c)
	case "volume-coord-broadcast":
		return runVolumeCoordinatorBroadcast(ctx, c)
	case "world-init-unixfs":
		return runWorldInitUnixFS(ctx, c)
	case "world-coord-multi-writer":
		return runWorldCoordinatorMultiWriter(ctx, c)
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
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "open client muxed conn")
	}
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		clientMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "open server muxed conn")
	}

	serverMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(serverMux); err != nil {
		clientMp.Close()
		serverMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "register echo server")
	}

	serverCtx, cancelServer := context.WithCancel(ctx)
	serverErrCh := make(chan error, 1)
	server := srpc.NewServer(serverMux)
	go func() {
		serverErrCh <- server.AcceptMuxedConn(serverCtx, serverMp)
	}()
	defer func() {
		cancelServer()
		_ = clientMp.Close()
		_ = serverMp.Close()
		_ = clientPipe.Close()
		_ = serverPipe.Close()
		<-serverErrCh
	}()

	client := echo.NewSRPCEchoerClient(srpc.NewClientWithMuxedConn(clientMp))
	return runEchoClientLoop(ctx, c, client, "srpc-echo-loop")
}

// runSRPCRpcStreamEchoLoop checks echo calls through a nested RPC stream.
func runSRPCRpcStreamEchoLoop(ctx context.Context, c *config) error {
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "open client muxed conn")
	}
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		clientMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "open server muxed conn")
	}

	innerMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(innerMux); err != nil {
		clientMp.Close()
		serverMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "register inner echo server")
	}
	serverMux := srpc.NewMux()
	if err := echo.NewEchoServer(innerMux).Register(serverMux); err != nil {
		clientMp.Close()
		serverMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return errors.Wrap(err, "register outer echo server")
	}

	serverCtx, cancelServer := context.WithCancel(ctx)
	serverErrCh := make(chan error, 1)
	server := srpc.NewServer(serverMux)
	go func() {
		serverErrCh <- server.AcceptMuxedConn(serverCtx, serverMp)
	}()
	defer func() {
		cancelServer()
		_ = clientMp.Close()
		_ = serverMp.Close()
		_ = clientPipe.Close()
		_ = serverPipe.Close()
		<-serverErrCh
	}()

	outerClient := echo.NewSRPCEchoerClient(srpc.NewClientWithMuxedConn(clientMp))
	nestedClient := rpcstream.NewRpcStreamClient(
		func(ctx context.Context) (echo.SRPCEchoer_RpcStreamClient, error) {
			return outerClient.RpcStream(ctx)
		},
		"echo",
		true,
	)
	client := echo.NewSRPCEchoerClient(nestedClient)
	return runEchoClientLoop(ctx, c, client, "srpc-rpcstream-echo-loop")
}

// runEchoClientLoop verifies repeated echo responses and reports stream progress.
func runEchoClientLoop(
	ctx context.Context,
	c *config,
	client echo.SRPCEchoerClient,
	phase string,
) error {
	iterations := c.iterations
	if iterations <= 0 {
		iterations = 128
	}
	payloadSize := c.batch
	if payloadSize <= 0 {
		payloadSize = 4096
	}

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
	rootMux := srpc.NewMux()
	if err := echo.NewEchoServer(nil).Register(rootMux); err != nil {
		return errors.Wrap(err, "register root echo server")
	}
	resClient, cleanup, err := openResourceClient(ctx, rootMux)
	if err != nil {
		return err
	}
	defer cleanup()

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
	dir, err := openTestDirectory(c.root, []string{"read-helper"})
	if err != nil {
		return err
	}
	want := []byte("tinygo-opfs-read-file-helper")
	if err := opfs.WriteFile(dir, "manifest-a", want); err != nil {
		return err
	}
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
	dir, err := openTestDirectory(c.root, []string{"large-helper"})
	if err != nil {
		return err
	}
	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 64 * 1024 * 1024
	}
	files := c.batch
	if files <= 0 {
		files = 64
	}
	baseSize := totalSize / files
	remainder := totalSize % files
	for i := range files {
		size := baseSize
		if i < remainder {
			size++
		}
		name := "chunk-" + zeroPad(i, 3) + ".bin"
		if err := opfs.WriteFile(dir, name, deterministicLargeBytes(size, i)); err != nil {
			return errors.Wrapf(err, "write %s", name)
		}
	}

	for _, i := range []int{0, files / 2, files - 1} {
		size := baseSize
		if i < remainder {
			size++
		}
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

// runLargeBlockBatch checks a durable large batch before and after remount.
func runLargeBlockBatch(ctx context.Context, c *config) error {
	_, e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}

	estimate, err := opfs.EstimateStorage()
	if err != nil {
		release()
		return errors.Wrap(err, "estimate worker storage")
	}
	if estimate.Quota <= estimate.Usage {
		release()
		return errors.Errorf("worker storage estimate has no headroom: usage=%d quota=%d", estimate.Usage, estimate.Quota)
	}

	totalSize, entriesCount := largeBlockShape(c)
	baseSize := totalSize / entriesCount
	remainder := totalSize % entriesCount
	entries := make([]*block.PutBatchEntry, entriesCount)
	for i := range entries {
		size := baseSize
		if i < remainder {
			size++
		}
		data := deterministicLargeBytes(size, i)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			release()
			return err
		}
		entries[i] = &block.PutBatchEntry{Ref: ref, Data: data}
	}
	postProgress(c, "large-block-put-start", 0, totalSize)
	if err := e.PutBlockBatch(ctx, entries); err != nil {
		release()
		return errors.Wrap(err, "put large block batch")
	}
	if _, err := e.Sync(ctx); err != nil {
		release()
		return err
	}
	postProgress(c, "large-block-put-complete", totalSize, totalSize)
	postProgress(c, "large-block-readback-before-close-start", 0, totalSize)
	if err := verifyLargeBlocks(ctx, c, e, totalSize, entriesCount); err != nil {
		release()
		return err
	}
	postProgress(c, "large-block-readback-before-close-complete", totalSize, totalSize)
	release()

	_, e, release, err = openBlockEngine(ctx, c)
	if err != nil {
		return errors.Wrap(err, "reopen large block engine")
	}
	defer release()
	postProgress(c, "large-block-readback-after-reopen-start", 0, totalSize)
	if err := verifyLargeBlocks(ctx, c, e, totalSize, entriesCount); err != nil {
		return err
	}
	postProgress(c, "large-block-readback-after-reopen-complete", totalSize, totalSize)
	return nil
}

// runLargeBlockVerify checks a large batch from a fresh worker instance.
func runLargeBlockVerify(ctx context.Context, c *config) error {
	_, e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()

	totalSize, entriesCount := largeBlockShape(c)
	postProgress(c, "large-block-readback-fresh-worker-start", 0, totalSize)
	if err := verifyLargeBlocks(ctx, c, e, totalSize, entriesCount); err != nil {
		return err
	}
	postProgress(c, "large-block-readback-fresh-worker-complete", totalSize, totalSize)
	return nil
}

// largeBlockShape returns the requested total bytes and block count with probe defaults.
func largeBlockShape(c *config) (int, int) {
	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 64 * 1024 * 1024
	}
	entriesCount := c.batch
	if entriesCount <= 0 {
		entriesCount = 96
	}
	return totalSize, entriesCount
}

// verifyLargeBlocks compares every large block against its deterministic content.
func verifyLargeBlocks(ctx context.Context, c *config, e *engine.BlockStore, totalSize, entriesCount int) error {
	baseSize := totalSize / entriesCount
	remainder := totalSize % entriesCount
	for i := range entriesCount {
		size := baseSize
		if i < remainder {
			size++
		}
		want := deterministicLargeBytes(size, i)
		ref, err := block.BuildBlockRef(want, nil)
		if err != nil {
			return err
		}
		got, found, err := e.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "get large block %d", i)
		}
		if !found {
			return errors.Errorf("large block %d not found", i)
		}
		if len(got) != len(want) {
			return errors.Errorf("large block %d length=%d want=%d", i, len(got), len(want))
		}
		if !bytes.Equal(got, want) {
			return errors.Errorf("large block %d data mismatch", i)
		}
		if (i+1)%16 == 0 || i == entriesCount-1 {
			readBytes := baseSize*(i+1) + min(i+1, remainder)
			postProgress(c, "large-block-readback-stream", readBytes, totalSize)
		}
	}
	return nil
}

// runReadAtHelperLoop checks offset reads and exact EOF behavior.
func runReadAtHelperLoop(c *config) error {
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

// runBlockWriter publishes deterministic batches before announcing their availability.
func runBlockWriter(ctx context.Context, c *config) error {
	// Open one publishing engine and its cross-runtime event channel.
	_, blocks, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventPub(c.root)
	defer events.Close()

	// Publish deterministic batches and announce each visible generation.
	for i := range c.iterations {
		entries := make([]*block.PutBatchEntry, c.batch)
		for j := range entries {
			key := blockKey(c.worker, i, j)
			value := blockValue(key)
			ref, err := block.BuildBlockRef(value, nil)
			if err != nil {
				return errors.Wrap(err, "build concurrent block reference")
			}
			entries[j] = &block.PutBatchEntry{Ref: ref, Data: value}
		}
		if err := blocks.PutBlockBatch(ctx, entries); err != nil {
			return errors.Wrap(err, "write concurrent blocks")
		}
		if _, err := blocks.Sync(ctx); err != nil {
			return errors.Wrap(err, "sync concurrent blocks")
		}
		events.Post(blockEvent{typ: "block-written", worker: c.worker, iteration: i})
	}

	// Announce completion after every published batch is durable.
	events.Post(blockEvent{typ: "block-writer-done", worker: c.worker})
	return nil
}

// runBlockReader observes concurrent publishers and optionally verifies maintenance.
func runBlockReader(ctx context.Context, c *config, compact bool) error {
	// Open one reader before the publishers start.
	e, blocks, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventSub(c)
	defer events.Close()
	postReady(c)

	// Consume publication events until every writer completes.
	done := make([]bool, c.workers)
	var found int
	var doneCount int
	for doneCount < c.workers {
		event, err := events.Next(ctx)
		if err != nil {
			return err
		}
		switch event.typ {
		case "block-written":
			for j := range c.batch {
				key := blockKey(event.worker, event.iteration, j)
				value, ok, err := getBlock(ctx, blocks, key)
				if err != nil {
					return errors.Wrap(err, "read concurrent block")
				}
				if !ok {
					continue
				}
				if !bytes.Equal(value, blockValue(key)) {
					return errors.Errorf("block value mismatch key=%s", string(key))
				}
				found++
			}
		case "block-writer-done":
			if event.worker < 0 || event.worker >= len(done) {
				return errors.Errorf("invalid writer id %d", event.worker)
			}
			if !done[event.worker] {
				done[event.worker] = true
				doneCount++
			}
		}
	}
	if found == 0 {
		return errors.New("reader found no concurrently written blocks")
	}
	if !compact {
		return nil
	}

	// Run bounded maintenance through the live reader before checking all values.
	if err := e.Maintenance(ctx); err != nil {
		return errors.Wrap(err, "maintain shared block volume")
	}
	return verifyBlocks(ctx, c, blocks, "after maintenance")
}

// runBlockVerify checks every expected block after a fresh engine mount.
func runBlockVerify(ctx context.Context, c *config) error {
	_, blocks, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	return verifyBlocks(ctx, c, blocks, "after remount")
}

// runRemoteCacheLifecycle checks stale handle rejection and fresh reads after bridge replacement.
func runRemoteCacheLifecycle(ctx context.Context, c *config) error {
	// Install and require the bridge-backed driver.
	if !opfs.InstallRemoteDriverFromGlobal() {
		return errors.New("remote OPFS driver was not installed")
	}
	driver, ok := opfs.DefaultDriver.(*opfs.RemoteDriver)
	if !ok {
		return errors.Errorf("OPFS driver is %T, want *opfs.RemoteDriver", opfs.DefaultDriver)
	}

	// Populate the block cache through the first bridge.
	_, e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	value := []byte("remote-cache-value")
	ref, _, err := e.PutBlock(ctx, value, &block.PutOpts{Sync: true})
	if err != nil {
		return errors.Wrap(err, "write remote cache block")
	}
	got, found, err := e.GetBlock(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "read remote cache block")
	}
	if !found || !bytes.Equal(got, value) {
		return errors.Errorf("remote cache read returned found=%t value=%q", found, got)
	}

	// Retain one raw file token that must become stale on replacement.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	dir, err := opfs.GetDirectory(root, c.root, true)
	if err != nil {
		return err
	}
	const filename = "remote-stale-handle"
	if err := opfs.WriteFile(dir, filename, []byte("stale")); err != nil {
		return err
	}
	stale, err := opfs.OpenAsyncFile(dir, filename)
	if err != nil {
		return err
	}

	// Replace the bridge and reject every token from its prior id space.
	postReady(c)
	if err := driver.WaitSwap(ctx); err != nil {
		return err
	}
	if _, err := stale.Size(); err == nil {
		return errors.New("stale remote file handle remained usable after bridge swap")
	}
	if err := stale.Close(); err == nil {
		return errors.New("stale remote file close unexpectedly succeeded")
	}
	release()
	release = nil

	// Remount the block cache through fresh directory and file tokens.
	_, fresh, freshRelease, err := openBlockEngine(ctx, c)
	if err != nil {
		return errors.Wrap(err, "remount block engine after bridge swap")
	}
	defer freshRelease()
	got, found, err = fresh.GetBlock(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "read remote cache block after remount")
	}
	if !found || !bytes.Equal(got, value) {
		return errors.Errorf("remote cache remount returned found=%t value=%q", found, got)
	}

	// Verify remote deletion errors and explicit fresh-token release.
	root, err = opfs.GetRoot()
	if err != nil {
		return err
	}
	dir, err = opfs.GetDirectory(root, c.root, false)
	if err != nil {
		return err
	}
	if err := opfs.DeleteEntry(dir, "missing-entry", false); !opfs.IsNotFound(err) {
		return errors.Errorf("remote missing delete error=%v, want NotFoundError", err)
	}
	file, err := opfs.OpenAsyncFile(dir, filename)
	if err != nil {
		return err
	}
	buf := make([]byte, len("stale"))
	if _, err := file.ReadAt(buf, 0); err != nil {
		return err
	}
	if string(buf) != "stale" {
		return errors.Errorf("remote remount file value=%q", buf)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

// openBlockEngine returns the immutable engine, its block adapter, and their joint release.
func openBlockEngine(ctx context.Context, c *config) (*engine.Engine, *engine.BlockStore, func(), error) {
	dir, err := openTestDirectory(c.root, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	e, err := engine.Open(ctx, engine.NewBrowserBackend(opfs.DefaultDriver, dir, c.root))
	if err != nil {
		return nil, nil, nil, err
	}
	blocks := engine.NewBlockStore(ctx, e, block.DefaultHashType)
	release := func() {
		_ = blocks.Close()
		_ = e.Close()
	}
	return e, blocks, release, nil
}

// runMetaWriter commits one deterministic key per transaction while other workers write.
func runMetaWriter(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()
	for i := range c.iterations {
		key := metaKey(c.worker, i)
		if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, key, metaValue(key))
		}); err != nil {
			return err
		}
		if i%5 == 0 {
			if err := verifyMetaKey(ctx, store, key); err != nil {
				return err
			}
		}
	}
	return nil
}

// runMetaVerify checks all workers' metadata through a fresh volume.
func runMetaVerify(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()
	for w := range c.workers {
		for i := range c.iterations {
			if err := verifyMetaKey(ctx, store, metaKey(w, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// runMetaMixedWriter publishes alternating small and large metadata values.
func runMetaMixedWriter(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()
	for i := range c.iterations {
		key := metaKey(c.worker, i)
		if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, key, metaMixedValue(c.worker, key))
		}); err != nil {
			return err
		}
		if i%4 == 0 {
			if err := verifyMetaValue(ctx, store, key, metaMixedValue(c.worker, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// runMetaMixedVerify checks every small and large value after concurrent publication.
func runMetaMixedVerify(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()
	for w := range c.workers {
		for i := range c.iterations {
			key := metaKey(w, i)
			if err := verifyMetaValue(ctx, store, key, metaMixedValue(w, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// runVolumeRuntimeWrite persists a block and the metadata needed to find it after remount.
func runVolumeRuntimeWrite(ctx context.Context, c *config) error {
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

// openVolumeKVBench opens the product OPFS volume for the KV write benchmarks
// with driver_mode pinned to standard-wasm, the ABI the product ships.
func openVolumeKVBench(ctx context.Context, c *config) (*volume_opfs.Opfs, error) {
	conf := newOPFSConfig(c)
	conf.DriverMode = "standard-wasm"
	return volume_opfs.NewOpfs(ctx, logrus.NewEntry(logrus.New()), conf)
}

// runVolumeKVWritePerOp commits one key per write transaction. opNanos covers
// every iteration including transaction open and commit.
func runVolumeKVWritePerOp(ctx context.Context, c *config) error {
	vol, err := openVolumeKVBench(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()

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
	benchExtra = map[string]int64{
		"opNanos": time.Since(start).Nanoseconds(),
		"ops":     int64(c.iterations),
	}
	return nil
}

// runVolumeKVWriteSingleTx puts all values into one write transaction and
// commits once. opNanos covers every set plus the single commit.
func runVolumeKVWriteSingleTx(ctx context.Context, c *config) error {
	vol, err := openVolumeKVBench(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	store := vol.GetKvtxStore()

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open kv write tx")
	}
	defer tx.Discard()

	start := time.Now()
	for i := range c.iterations {
		if err := tx.Set(ctx, volumeKVKey(i), volumeKVValue(c)); err != nil {
			return errors.Wrap(err, "set kv")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit kv tx")
	}
	benchExtra = map[string]int64{
		"opNanos": time.Since(start).Nanoseconds(),
		"ops":     int64(c.iterations),
	}
	return nil
}

// runVolumeRuntimeVerify checks saved metadata, referenced block content, and storage totals.
func runVolumeRuntimeVerify(ctx context.Context, c *config) error {
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
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	if err := vol.Delete(); err != nil {
		return err
	}

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

// runVolumeCoordinatorLocal checks lease exclusion and authoritative local watch refresh.
func runVolumeCoordinatorLocal(ctx context.Context, c *config) error {
	reader, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer reader.Close()
	writer, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer writer.Close()

	readerScope := volumeCoordScope(reader, c)
	writerScope := volumeCoordScope(writer, c)
	capability, err := reader.Capability(ctx, readerScope)
	if err != nil {
		return errors.Wrap(err, "coordinator capability")
	}
	if !capability.Supported || capability.Backend != coord.BackendKindOPFS {
		return errors.Errorf("coordinator capability supported=%v backend=%s", capability.Supported, capability.Backend)
	}

	before, err := reader.Snapshot(ctx, readerScope)
	if err != nil {
		return errors.Wrap(err, "coordinator snapshot")
	}
	watch, err := reader.Watch(ctx, readerScope, before.Generation)
	if err != nil {
		return errors.Wrap(err, "coordinator watch")
	}
	defer watch.Close()

	lease, ok, err := writer.TryAcquireWriteLease(ctx, writerScope)
	if err != nil {
		return errors.Wrap(err, "acquire coordinator lease")
	}
	if !ok {
		return errors.New("coordinator lease unavailable")
	}
	if blocked, ok, err := reader.TryAcquireWriteLease(ctx, readerScope); err != nil {
		return errors.Wrap(err, "try blocked coordinator lease")
	} else if ok {
		_ = blocked.Release(ctx)
		return errors.New("second coordinator lease acquired while writer holds WebLock")
	}

	if err := advanceVolumeCoordinatorGeneration(ctx, writer, []byte("volume/coord/local")); err != nil {
		return err
	}
	ref := volumeCoordRoot(c, "local")
	if _, err := lease.Publish(ctx, coord.Event{
		RootChanged:      ref,
		KeyPrefixChanged: []byte("volume/coord/"),
	}); err != nil {
		return errors.Wrap(err, "publish coordinator event")
	}
	if err := lease.Release(ctx); err != nil {
		return errors.Wrap(err, "release coordinator lease")
	}

	// Storage invalidation hints may precede the richer logical lease event.
	var event coord.Event
	for event.RootChanged == nil {
		event, err = waitCoordEvent(ctx, watch.Events(), before.Generation)
		if err != nil {
			return err
		}
	}
	if !event.RootChanged.EqualsRef(ref) {
		return errors.Errorf("root event=%v want=%v", event.RootChanged, ref)
	}
	if !bytes.Equal(event.KeyPrefixChanged, []byte("volume/coord/")) {
		return errors.Errorf("prefix event=%q want volume/coord/", string(event.KeyPrefixChanged))
	}

	after, err := reader.Snapshot(ctx, readerScope)
	if err != nil {
		return errors.Wrap(err, "coordinator missed snapshot")
	}
	if after.Generation <= before.Generation {
		return errors.Errorf("snapshot generation=%d want > %d", after.Generation, before.Generation)
	}
	if after.Root == nil || !after.Root.EqualsRef(ref) {
		return errors.Errorf("snapshot root=%v want=%v", after.Root, ref)
	}
	return nil
}

// runVolumeCoordinatorWatch waits for another worker's durable generation to become visible.
func runVolumeCoordinatorWatch(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	scope := volumeCoordScope(vol, c)
	before, err := vol.Snapshot(ctx, scope)
	if err != nil {
		return errors.Wrap(err, "coordinator snapshot before watch")
	}
	watch, err := vol.Watch(ctx, scope, before.Generation)
	if err != nil {
		return errors.Wrap(err, "coordinator watch")
	}
	defer watch.Close()

	postReady(c)
	event, err := waitCoordEvent(ctx, watch.Events(), before.Generation)
	if err != nil {
		return err
	}
	if event.Generation <= before.Generation {
		return errors.Errorf("broadcast generation=%d want > %d", event.Generation, before.Generation)
	}
	after, err := vol.Snapshot(ctx, scope)
	if err != nil {
		return errors.Wrap(err, "coordinator snapshot after broadcast")
	}
	if after.Generation <= before.Generation {
		return errors.Errorf("snapshot generation after broadcast=%d want > %d", after.Generation, before.Generation)
	}
	return nil
}

// runVolumeCoordinatorBroadcast fences admitted writes before the cross-worker wakeup.
func runVolumeCoordinatorBroadcast(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	if err := advanceVolumeCoordinatorGeneration(ctx, vol, []byte("volume/coord/broadcast")); err != nil {
		return err
	}
	if _, _, err := vol.PutBlock(ctx, []byte("volume-coord-broadcast"), nil); err != nil {
		return errors.Wrap(err, "put broadcast block")
	}
	// PutBlock is async by default: its immutable pack publication, which fires the
	// cross-worker BroadcastChannel wakeup the watcher waits for, is deferred.
	// A cross-worker coordinator change is observable only once fenced at the
	// volume commit boundary, so Sync here as a production writer would before
	// the watcher can rely on seeing the advanced generation.
	if _, err := vol.Sync(ctx); err != nil {
		return errors.Wrap(err, "sync broadcast")
	}
	return nil
}

// volumeCoordScope identifies the shared object store and this worker's participant.
func volumeCoordScope(vol volume.Volume, c *config) coord.Scope {
	return coord.Scope{
		VolumeID:      vol.GetID(),
		ObjectStoreID: c.root + "/coord",
		ParticipantID: c.scenario + "-" + strconv.Itoa(c.worker),
	}
}

// volumeCoordRoot builds the distinct root marker used by a coordinator probe.
func volumeCoordRoot(c *config, suffix string) *bucket.ObjectRef {
	return &bucket.ObjectRef{BucketId: c.root + "/coord/" + suffix}
}

// advanceVolumeCoordinatorGeneration commits metadata to advance the durable revision.
func advanceVolumeCoordinatorGeneration(ctx context.Context, vol *volume_opfs.Opfs, key []byte) error {
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open coordinator generation tx")
	}
	defer tx.Discard()
	if err := tx.Set(ctx, key, []byte("generation")); err != nil {
		return errors.Wrap(err, "set coordinator generation key")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit coordinator generation tx")
	}
	return nil
}

// waitCoordEvent waits for a newer event within the probe's bounded deadline.
func waitCoordEvent(ctx context.Context, events <-chan coord.Event, afterGeneration uint64) (coord.Event, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return coord.Event{}, errors.New("coordinator watch closed")
			}
			if event.Generation > afterGeneration {
				return event, nil
			}
		case <-waitCtx.Done():
			return coord.Event{}, errors.Wrap(waitCtx.Err(), "wait coordinator event")
		}
	}
}

// runVolumeRuntimeSeedIncompatible writes an old format marker and sentinel saved bytes.
func runVolumeRuntimeSeedIncompatible(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"volume"})
	if err != nil {
		return err
	}
	if err := opfs.WriteFile(dir, ".spacewave-opfs-format.json", []byte(`{"kind":"spacewave-opfs-volume","version":1}`)); err != nil {
		return errors.Wrap(err, "write incompatible marker")
	}
	return opfs.WriteFile(dir, "legacy-only", []byte("incompatible"))
}

// runVolumeRuntimeSeedUnknown writes sentinel data without a recognized format marker.
func runVolumeRuntimeSeedUnknown(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"volume"})
	if err != nil {
		return err
	}
	return opfs.WriteFile(dir, "legacy-only", []byte("unknown"))
}

// runVolumeRuntimeVerifyRecovered requires an incompatible open to preserve sentinel bytes.
func runVolumeRuntimeVerifyRecovered(ctx context.Context, c *config, expected string) error {
	// A replacement must remain writable and readable through a remount.
	if err := runVolumeRuntimeWrite(ctx, c); err != nil {
		return err
	}
	if err := runVolumeRuntimeVerify(ctx, c); err != nil {
		return err
	}

	// Deleting the active replacement must not remove the legacy directory.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	if err := vol.Delete(); err != nil {
		return err
	}

	// Verify the original saved bytes after opening and deleting the replacement.
	dir, err := openTestDirectory(c.root, []string{"volume"})
	if err != nil {
		return err
	}
	data, err := opfs.ReadFile(dir, "legacy-only")
	if err != nil {
		return err
	}
	if string(data) != expected {
		return errors.New("incompatible saved volume changed")
	}
	return nil
}

// runWorldInitUnixFS initializes and reads an empty UnixFS root through the product volume.
func runWorldInitUnixFS(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	le := logrus.NewEntry(logrus.New())
	bucketID := c.root + "/world"
	ref := &bucket.ObjectRef{BucketId: bucketID}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		vol,
		nil,
		ref,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	defer cursor.Release()

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
		return errors.Wrap(err, "build world state")
	}
	defer ws.Discard()

	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, vol.GetPeerID(), "files", time.Now()); err != nil {
		return errors.Wrap(err, "init unixfs")
	}
	if err := ws.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit world state")
	}

	fsCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: "files",
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		vol.GetPeerID(),
		true,
	)
	if err != nil {
		return errors.Wrap(err, "follow unixfs")
	}
	defer fsCursor.Release()

	handle, err := unixfs_sdk.NewFSHandle(fsCursor)
	if err != nil {
		return errors.Wrap(err, "open fs handle")
	}
	defer handle.Release()

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

// runWorldCoordinatorMultiWriter checks stale-head rejection and serialized world commits.
func runWorldCoordinatorMultiWriter(ctx context.Context, c *config) error {
	writer, err := openCoordinatorWorldEngine(ctx, c, "writer")
	if err != nil {
		return err
	}
	defer writer.release()
	reader, err := openCoordinatorWorldEngine(ctx, c, "reader")
	if err != nil {
		return err
	}
	defer reader.release()

	initTx, err := writer.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open initial OPFS writer")
	}
	if _, err := initTx.CreateObject(ctx, "opfs-initial-head-object", nil); err != nil {
		initTx.Discard()
		return errors.Wrap(err, "create initial OPFS world object")
	}
	if err := initTx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit initial OPFS world object")
	}

	baseHead := writer.engine.GetRootRef()
	staleTx, err := writer.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open stale writer")
	}
	if _, err := staleTx.CreateObject(ctx, "opfs-stale-head-object", nil); err != nil {
		staleTx.Discard()
		return errors.Wrap(err, "create stale writer object")
	}
	if err := writer.writeHead(ctx, &bucket.ObjectRef{BucketId: writer.bucketID}); err != nil {
		staleTx.Discard()
		return errors.Wrap(err, "write stale durable head")
	}
	if err := staleTx.Commit(ctx); !stderrors.Is(err, coord.ErrStaleGeneration) {
		return errors.Errorf("stale OPFS writer commit error=%v want ErrStaleGeneration", err)
	}
	if err := writer.writeHead(ctx, baseHead); err != nil {
		return errors.Wrap(err, "restore durable head after stale proof")
	}

	watchScope := coord.Scope{
		VolumeID:      writer.vol.GetID(),
		ObjectStoreID: writer.objectStoreID,
		ParticipantID: "opfs-world-watch",
	}
	before, err := writer.vol.Snapshot(ctx, watchScope)
	if err != nil {
		return errors.Wrap(err, "snapshot before OPFS world watch")
	}
	watch, err := writer.vol.Watch(ctx, watchScope, before.Generation)
	if err != nil {
		return errors.Wrap(err, "open OPFS world watch")
	}
	defer watch.Close()

	firstTx, err := writer.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open first OPFS writer")
	}
	if _, err := firstTx.CreateObject(ctx, "opfs-serialized-writer-a", nil); err != nil {
		firstTx.Discard()
		return errors.Wrap(err, "create first OPFS writer object")
	}
	secondTxCh := make(chan world.Tx, 1)
	secondErrCh := make(chan error, 1)
	go func() {
		tx, err := reader.engine.NewTransaction(ctx, true)
		if err != nil {
			secondErrCh <- err
			return
		}
		secondTxCh <- tx
	}()
	select {
	case err := <-secondErrCh:
		firstTx.Discard()
		return errors.Wrap(err, "second OPFS writer failed while waiting")
	case tx := <-secondTxCh:
		tx.Discard()
		firstTx.Discard()
		return errors.New("second OPFS writer acquired while first writer held coordinator lease")
	case <-time.After(50 * time.Millisecond):
	}
	if err := firstTx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit first OPFS writer")
	}

	var secondTx world.Tx
	select {
	case err := <-secondErrCh:
		return errors.Wrap(err, "second OPFS writer failed after first commit")
	case secondTx = <-secondTxCh:
	case <-time.After(5 * time.Second):
		return errors.New("second OPFS writer did not acquire after first commit")
	}
	if _, err := secondTx.CreateObject(ctx, "opfs-serialized-writer-b", nil); err != nil {
		secondTx.Discard()
		return errors.Wrap(err, "create second OPFS writer object")
	}
	if err := secondTx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit second OPFS writer")
	}

	acceptedRoot := reader.engine.GetRootRef()
	after, err := writer.vol.Snapshot(ctx, watchScope)
	if err != nil {
		return errors.Wrap(err, "snapshot after OPFS world commits")
	}
	if after.Root == nil || !after.Root.EqualsRef(acceptedRoot) {
		return errors.Errorf("OPFS coordinator root=%v want=%v", after.Root, acceptedRoot)
	}
	if err := waitCoordinatorRootPrefix(ctx, watch.Events(), acceptedRoot, []byte("world-head")); err != nil {
		return err
	}
	if err := writer.refreshHead(ctx); err != nil {
		return errors.Wrap(err, "refresh OPFS reader head")
	}
	return writer.verifyObjects(ctx, "opfs-serialized-writer-a", "opfs-serialized-writer-b")
}

// runWorldDeferredCrashRecovery is the wasm/OPFS port of the host
// TestEngineDeferredDurabilityCrashRecovery. It proves world commits over the
// immutable OPFS volume defer durability to Sync: a per-commit write advances
// only the in-memory root, Sync runs the block barrier then advances the durable
// head, and a crash (engine + volume teardown WITHOUT a final Sync) recovers to
// the last Sync'd world head. The rollback invariant holds via the durable HEAD
// (advanced only at Sync through commitFn), not block absence.
func runWorldDeferredCrashRecovery(ctx context.Context, c *config) error {
	writer, err := openDeferredWorldEngine(ctx, c, "writer")
	if err != nil {
		return err
	}

	seedHead, err := writer.readHead(ctx)
	if err != nil {
		writer.release()
		return errors.Wrap(err, "read seed OPFS world head")
	}

	// Tick 1: a deferred commit advances only the in-memory root; the durable
	// head must still lag at the seed.
	if err := createWorldObject(ctx, writer, "opfs-deferred-obj-a"); err != nil {
		writer.release()
		return err
	}
	if lagHead, err := writer.readHead(ctx); err != nil {
		writer.release()
		return errors.Wrap(err, "read OPFS head after deferred obj-a")
	} else if !objectRefsEqual(lagHead, seedHead) {
		writer.release()
		return errors.New("deferred commit must not advance the durable OPFS head before Sync")
	}
	rootAfterA := writer.engine.GetRootRef().Clone()

	// Sync fences the block barrier then advances the durable head to obj-a.
	if _, err := writer.engine.Sync(ctx); err != nil {
		writer.release()
		return errors.Wrap(err, "Sync OPFS deferred world engine")
	}
	headA, err := writer.readHead(ctx)
	if err != nil {
		writer.release()
		return errors.Wrap(err, "read OPFS head after Sync")
	}
	if !objectRefsEqual(headA, rootAfterA) {
		writer.release()
		return errors.New("Sync must advance the durable OPFS head to the in-memory root")
	}
	if objectRefsEqual(headA, seedHead) {
		writer.release()
		return errors.New("Sync'd OPFS head must differ from the seed head")
	}

	// Tick 2: another deferred commit; the durable head must still lag at obj-a.
	if err := createWorldObject(ctx, writer, "opfs-deferred-obj-b"); err != nil {
		writer.release()
		return err
	}
	if lagHead, err := writer.readHead(ctx); err != nil {
		writer.release()
		return errors.Wrap(err, "read OPFS head after deferred obj-b")
	} else if !objectRefsEqual(lagHead, headA) {
		writer.release()
		return errors.New("post-Sync deferred commit must not advance the durable OPFS head")
	}

	// Crash: tear down the engine and volume WITHOUT a final Sync. obj-b lives
	// only in the in-memory buffer; the durable head still names obj-a.
	writer.release()

	// Recover: reopen a deferred world engine over the same OPFS origin storage.
	// The cursor builds at the persisted durable head (obj-a).
	recovered, err := openDeferredWorldEngine(ctx, c, "writer")
	if err != nil {
		return err
	}
	defer recovered.release()

	if !objectRefsEqual(recovered.engine.GetRootRef(), headA) {
		return errors.Errorf("OPFS recovery root=%v want last Sync'd head=%v", recovered.engine.GetRootRef(), headA)
	}

	tx, err := recovered.engine.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open OPFS recovery read transaction")
	}
	defer tx.Discard()
	// obj-a's blocks were fenced durable by Sync, so it recovers (this read also
	// proves block-before-head ordering: the head names only durable blocks).
	if _, found, err := tx.GetObject(ctx, "opfs-deferred-obj-a"); err != nil {
		return errors.Wrap(err, "read obj-a after OPFS recovery")
	} else if !found {
		return errors.New("OPFS recovery must land on the last Sync'd head with obj-a present")
	}
	// obj-b, committed after the last Sync, is rolled back: the durable head never
	// referenced its tree.
	if _, found, err := tx.GetObject(ctx, "opfs-deferred-obj-b"); err != nil {
		return errors.Wrap(err, "read obj-b after OPFS recovery")
	} else if found {
		return errors.New("post-Sync OPFS commit must not survive a crash before the next Sync")
	}
	return nil
}

// createWorldObject commits one named object through the world transaction interface.
func createWorldObject(ctx context.Context, h *coordinatorWorldEngine, key string) error {
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

// coordinatorWorldEngine owns a volume, object store, cursor, and world engine for one participant.
type coordinatorWorldEngine struct {
	// vol owns the persisted volume.
	vol *volume_opfs.Opfs
	// objectStoreID identifies the shared object store.
	objectStoreID string
	// bucketID identifies the shared world bucket.
	bucketID string
	// store provides the durable world head transaction API.
	store object.ObjectStore
	// storeRelease releases the object-store reference.
	storeRelease func()
	// cursor pins the world root used to construct the engine.
	cursor *bucket_lookup.Cursor
	// engine owns the participant's world transaction lifecycle.
	engine *world_block.Engine
}

// openCoordinatorWorldEngine mounts a world engine using the volume's write coordinator.
func openCoordinatorWorldEngine(ctx context.Context, c *config, participant string) (*coordinatorWorldEngine, error) {
	return openWorldEngine(ctx, c, participant, false)
}

// openDeferredWorldEngine mounts a single-writer world whose durable head advances at Sync.
func openDeferredWorldEngine(ctx context.Context, c *config, participant string) (*coordinatorWorldEngine, error) {
	return openWorldEngine(ctx, c, participant, true)
}

// openWorldEngine mounts the requested world durability mode at the persisted head.
func openWorldEngine(ctx context.Context, c *config, participant string, deferred bool) (*coordinatorWorldEngine, error) {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return nil, err
	}
	objectStoreID := c.root + "/world-coord-store"
	bucketID := c.root + "/world-coord-bucket"
	kvkey := store_kvkey.NewDefaultKVKey()
	hydraStore := store_kvtx.NewKVTx(kvkey, vol.GetKvtxStore(), &store_kvtx.Config{})
	objStore, storeRelease, err := hydraStore.AccessObjectStore(ctx, objectStoreID, nil)
	if err != nil {
		_ = vol.Close()
		return nil, errors.Wrap(err, "open OPFS world object store")
	}
	h := &coordinatorWorldEngine{
		vol:           vol,
		objectStoreID: objectStoreID,
		bucketID:      bucketID,
		store:         objStore,
		storeRelease:  storeRelease,
	}

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
	commitFn := func(ctx context.Context, baseRef, nref *bucket.ObjectRef) error {
		return h.casHead(ctx, baseRef, nref)
	}
	var engineOpt world_block.EngineOption
	if deferred {
		// Single-writer deferred durability: block writes and the durable head
		// advance both batch until Sync. The crash-recovery scenario fences
		// explicitly and a teardown without Sync rolls back to the last head.
		engineOpt = world_block.WithDeferredDurability()
	} else {
		scope := coord.Scope{
			VolumeID:      vol.GetID(),
			ObjectStoreID: objectStoreID,
			ParticipantID: participant,
		}
		engineOpt = world_block.WithWriteCoordinator(vol, scope, []byte("world-head"), h.readHead)
	}
	engine, err := world_block.NewEngine(
		ctx,
		le,
		h.cursor,
		space_world_ops.LookupWorldOp,
		commitFn,
		false,
		engineOpt,
	)
	if err != nil {
		h.release()
		return nil, errors.Wrap(err, "open OPFS world engine")
	}
	h.engine = engine
	return h, nil
}

// readHead reads the participant's shared durable world head.
func (h *coordinatorWorldEngine) readHead(ctx context.Context) (*bucket.ObjectRef, error) {
	tx, err := h.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte("world-head"))
	if err != nil || !found {
		return nil, err
	}
	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return state.GetHeadRef().Clone(), nil
}

// writeHead commits a serialized world head through the object store.
func (h *coordinatorWorldEngine) writeHead(ctx context.Context, ref *bucket.ObjectRef) error {
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

// casHead rejects a changed base before publishing under the caller's coordinator lease.
func (h *coordinatorWorldEngine) casHead(ctx context.Context, baseRef, nextRef *bucket.ObjectRef) error {
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

// refreshHead updates the world engine from its nonempty durable head.
func (h *coordinatorWorldEngine) refreshHead(ctx context.Context) error {
	headRef, err := h.readHead(ctx)
	if err != nil || headRef == nil || headRef.GetRootRef().GetEmpty() {
		return err
	}
	return h.engine.SetRootRef(ctx, headRef)
}

// verifyObjects requires all named objects to exist in one read transaction.
func (h *coordinatorWorldEngine) verifyObjects(ctx context.Context, keys ...string) error {
	tx, err := h.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, key := range keys {
		if _, found, err := tx.GetObject(ctx, key); err != nil {
			return err
		} else if !found {
			return errors.Errorf("OPFS world object %q not found after refresh", key)
		}
	}
	return nil
}

// release closes the world engine before releasing its cursor, store, and volume.
func (h *coordinatorWorldEngine) release() {
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

// waitCoordinatorRootPrefix waits for the expected root and changed-key prefix.
func waitCoordinatorRootPrefix(
	ctx context.Context,
	events <-chan coord.Event,
	root *bucket.ObjectRef,
	prefix []byte,
) error {
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return errors.New("OPFS world coordinator watch closed")
			}
			if event.RootChanged != nil &&
				event.RootChanged.EqualsRef(root) &&
				bytes.Equal(event.KeyPrefixChanged, prefix) {
				return nil
			}
		case <-waitCtx.Done():
			return errors.Wrap(waitCtx.Err(), "wait OPFS world coordinator root/prefix event")
		}
	}
}

// runWorldLargeUnixFSUpload writes and reads a large deterministic file through the world API.
func runWorldLargeUnixFSUpload(ctx context.Context, c *config) error {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	le := logrus.NewEntry(logrus.New())
	bucketID := c.root + "/world"
	ref := &bucket.ObjectRef{BucketId: bucketID}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		vol,
		nil,
		ref,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	defer cursor.Release()

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
		return errors.Wrap(err, "build world state")
	}
	defer ws.Discard()

	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, vol.GetPeerID(), "files", time.Now()); err != nil {
		return errors.Wrap(err, "init unixfs")
	}
	if err := ws.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit initial world state")
	}

	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 64 * 1024 * 1024
	}
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

	fsCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: "files",
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		vol.GetPeerID(),
		true,
	)
	if err != nil {
		return errors.Wrap(err, "follow unixfs")
	}
	defer fsCursor.Release()

	handle, err := unixfs_sdk.NewFSHandle(fsCursor)
	if err != nil {
		return errors.Wrap(err, "open fs handle")
	}
	defer handle.Release()

	largeFile, err := handle.Lookup(ctx, "large-video.mp4")
	if err != nil {
		return errors.Wrap(err, "lookup large file")
	}
	defer largeFile.Release()
	return verifyDeterministicFSFile(ctx, largeFile, totalSize, 0, c)
}

// runWorldResourceLargeUnixFSUpload checks the resource upload contract over a direct volume.
func runWorldResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, vol)
}

// runWorldResourceDirectUploadTreeLargeUnixFSUpload checks UploadTree without the RPC transport.
func runWorldResourceDirectUploadTreeLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	le := logrus.NewEntry(logrus.New())
	bucketID := c.root + "/world"
	ref := &bucket.ObjectRef{BucketId: bucketID}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		vol,
		nil,
		ref,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	defer cursor.Release()

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
		return errors.Wrap(err, "build world state")
	}
	defer ws.Discard()

	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, vol.GetPeerID(), "files", time.Now()); err != nil {
		return errors.Wrap(err, "init unixfs")
	}
	if err := ws.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit initial world state")
	}

	fsCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: "files",
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		vol.GetPeerID(),
		true,
	)
	if err != nil {
		return errors.Wrap(err, "follow unixfs")
	}
	defer fsCursor.Release()

	handle, err := unixfs_sdk.NewFSHandle(fsCursor)
	if err != nil {
		return errors.Wrap(err, "open fs handle")
	}
	defer handle.Release()

	rootResource := resource_unixfs.NewFSHandleObjectResource(
		logrus.NewEntry(logrus.StandardLogger()),
		handle,
		nil,
		ws,
		"files",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
	)
	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 64 * 1024 * 1024
	}
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
	le := logrus.NewEntry(logrus.New())
	bucketID := c.root + "/world"
	ref := &bucket.ObjectRef{BucketId: bucketID}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		bkt,
		nil,
		ref,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	defer cursor.Release()

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
		return errors.Wrap(err, "build world state")
	}
	defer ws.Discard()

	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, vol.GetPeerID(), "files", time.Now()); err != nil {
		return errors.Wrap(err, "init unixfs")
	}
	if err := ws.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit initial world state")
	}

	fsCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: "files",
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		vol.GetPeerID(),
		true,
	)
	if err != nil {
		return errors.Wrap(err, "follow unixfs")
	}
	defer fsCursor.Release()

	handle, err := unixfs_sdk.NewFSHandle(fsCursor)
	if err != nil {
		return errors.Wrap(err, "open fs handle")
	}
	defer handle.Release()

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

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get root resource client")
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 64 * 1024 * 1024
	}
	postProgress(c, "resource-upload-start", 0, totalSize)
	if err := uploadDeterministicResourceFile(ctx, rootSvc, "large-video.mp4", totalSize, 0, c); err != nil {
		return err
	}
	postProgress(c, "resource-upload-complete", totalSize, totalSize)
	if c.scenario == "world-resource-large-unixfs-write" {
		return nil
	}

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

	postProgress(c, "resource-client-start")
	fileClient, err := fileRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file client")
	}
	postProgress(c, "resource-client-complete")
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
	le := logrus.NewEntry(logrus.New())
	ctrlCtx, cancelCtrl := context.WithCancel(ctx)
	ctrl := volume_controller.NewController(
		le,
		&volume_controller.Config{DisablePeer: true},
		nil,
		controller.NewInfo(
			volume_opfs.ControllerID,
			volume_opfs.Version,
			"opfs-chrometest@"+c.root,
		),
		func(ctx context.Context, le *logrus.Entry) (volume.Volume, error) {
			return volume_opfs.NewOpfs(ctx, le, newOPFSConfig(c))
		},
	)

	ctrlErrCh := make(chan error, 1)
	go func() {
		ctrlErrCh <- ctrl.Execute(ctrlCtx)
	}()

	cleanup := func() error {
		cancelCtrl()
		err := <-ctrlErrCh
		if err != nil && !stderrors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}

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
func runCopyWalkWrapperConcurrency(ctx context.Context, c *config) (retErr error) {
	inputBytes := c.iterations
	if inputBytes <= 0 {
		inputBytes = 64 * 1024
	}
	suspectConc := c.batch
	if suspectConc <= 0 {
		suspectConc = 16
	}

	le := logrus.NewEntry(logrus.New())

	// Real bus stack over the OPFS volume so cross-bucket FollowRef resolves
	// through the production concurrent-lookup Handle, matching download-manifest.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return errors.Wrap(err, "construct core bus")
	}
	sr.AddFactory(volume_opfs.NewFactory(b))

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

	volDV, _, volRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(newOPFSConfig(c)),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load opfs volume controller")
	}
	defer volRef.Release()
	vol, err := volDV.(volume.Controller).GetVolume(ctx)
	if err != nil {
		return errors.Wrap(err, "get opfs volume")
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

	sfs := transform_all.BuildFactorySet()

	// Shared gzip transform so stored bytes hash consistently with
	// their object refs across both buckets; CopyObjectToBucket's forced-ref
	// writes require the source stored representation to match its ref.
	transformConf, err := block_transform.NewConfig([]cbconfig.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		return errors.Wrap(err, "build transform config")
	}

	// Dest world cursor + world state (root bucket), bus-backed.
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

	// Small-chunk recipe forces a wide chunked DAG: ChunkIndex -> many Chunk ->
	// many ByteSlice leaves, so WalkObjectBlocks has real fan-out to schedule.
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

	benchExtra = map[string]int64{
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
	vol, upper, cleanupBucket, err := openControllerBucket(ctx, c)
	if err != nil {
		return nil, nil, nil, err
	}

	objStore, releaseObjStore, err := vol.AccessObjectStore(ctx, c.root+"/cloud-overlay-meta", func() {})
	if err != nil {
		_ = cleanupBucket()
		return nil, nil, nil, errors.Wrap(err, "open cloud overlay dirty store")
	}

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

	return vol, overlay, func() error {
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

var _ block.StoreOps = (*probeDirtyTrackingStore)(nil)

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
func (d *probeDirtyTrackingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	var refs []*block.BlockRef
	var valid []int
	for i, entry := range entries {
		if entry == nil || entry.Tombstone || entry.Ref == nil || entry.Ref.GetEmpty() {
			continue
		}
		valid = append(valid, i)
		refs = append(refs, entry.Ref)
	}
	exists, err := d.store.GetBlockExistsBatch(ctx, refs)
	if err != nil || len(exists) != len(refs) {
		exists = nil
	}

	if err := d.store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}

	for j, i := range valid {
		if exists != nil && exists[j] {
			continue
		}
		entry := entries[i]
		if err := d.markDirty(ctx, entry.Ref.GetHash(), int64(len(entry.Data))); err != nil {
			return err
		}
	}
	return nil
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
	f.mtx.Lock()
	f.dirtySize += size
	if f.started || f.dirtySize < probeSyncSizeThresholdBytes {
		f.mtx.Unlock()
		return
	}
	f.started = true
	f.mtx.Unlock()

	go func() {
		f.done <- f.flush(ctx)
	}()
}

// wait joins the packing pass if the threshold started one.
func (f *probeSyncFlusher) wait() error {
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
	candidates, err := f.scanDirty(ctx)
	if err != nil {
		return err
	}

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
		blocks = nil
		start = end
	}
	return nil
}

// scanDirty reads the probe's dirty index from one metadata transaction.
func (f *probeSyncFlusher) scanDirty(ctx context.Context) ([]probeDirtyCandidate, error) {
	tx, err := f.dirtyStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open dirty scan tx")
	}
	defer tx.Discard()

	var out []probeDirtyCandidate
	prefix := []byte("dirty/")
	if err := tx.ScanPrefix(ctx, prefix, func(k, v []byte) error {
		h := &hash.Hash{}
		if err := h.ParseFromB58(string(k[len(prefix):])); err != nil {
			return err
		}
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
	if end == start {
		end++
	}
	return end, nil
}

// packProbeDirtyBlocks encodes a chunk with the production pack format.
func packProbeDirtyBlocks(blocks []probeDirtyBlock) error {
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

// openResourceClient returns a live resource client and cleanup that joins its server.
func openResourceClient(
	ctx context.Context,
	rootMux srpc.Mux,
) (*resource_client.Client, func() error, error) {
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open client muxed conn")
	}

	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		clientMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open server muxed conn")
	}

	resourceSrv := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		clientMp.Close()
		serverMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "register resource server")
	}

	serverCtx, cancelServer := context.WithCancel(ctx)
	serverErrCh := make(chan error, 1)
	server := srpc.NewServer(serverMux)
	go func() {
		serverErrCh <- server.AcceptMuxedConn(serverCtx, serverMp)
	}()

	srpcClient := srpc.NewClientWithMuxedConn(clientMp)
	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		cancelServer()
		clientMp.Close()
		serverMp.Close()
		clientPipe.Close()
		serverPipe.Close()
		return nil, nil, errors.Wrap(err, "open resource client")
	}

	cleanup := func() error {
		resClient.Release()
		cancelServer()
		_ = clientMp.Close()
		_ = serverMp.Close()
		_ = clientPipe.Close()
		_ = serverPipe.Close()
		if err := <-serverErrCh; err != nil && !isExpectedMuxCloseError(err) {
			return errors.Wrap(err, "resource server mux")
		}
		return nil
	}
	return resClient, cleanup, nil
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
	postProgress(c, "resource-readback-size-start", 0, totalSize)
	sizeResp, err := fileSvc.GetSize(ctx, &s4wave_unixfs.HandleGetSizeRequest{})
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file size")
	}
	postProgress(c, "resource-readback-size-complete", int(sizeResp.GetSize()), totalSize)
	if sizeResp.GetSize() != uint64(totalSize) {
		return errors.Errorf("resource file size=%d want=%d", sizeResp.GetSize(), totalSize)
	}
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
	postProgress(c, "fs-readback-size-start", 0, totalSize)
	size, err := handle.GetSize(ctx)
	if err != nil {
		return errors.Wrap(err, "get large file size")
	}
	postProgress(c, "fs-readback-size-complete", int(size), totalSize)
	if size != uint64(totalSize) {
		return errors.Errorf("large file size=%d want=%d", size, totalSize)
	}
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
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*req = *next
	return nil
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
	if s.offset >= s.totalSize {
		postProgress(s.c, "direct-upload-tree-eof", s.totalSize, s.totalSize)
		return nil, io.EOF
	}
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

// openVolume mounts the product volume under the scenario's isolated root.
func openVolume(ctx context.Context, c *config) (*volume_opfs.Opfs, error) {
	return openVolumeWithLogger(ctx, c, logrus.NewEntry(logrus.New()))
}

// openVolumeWithLogger mounts the product volume using the supplied probe logger.
func openVolumeWithLogger(ctx context.Context, c *config, le *logrus.Entry) (*volume_opfs.Opfs, error) {
	return volume_opfs.NewOpfs(ctx, le, newOPFSConfig(c))
}

// newOPFSConfig isolates the volume directory and lock namespace by test root.
func newOPFSConfig(c *config) *volume_opfs.Config {
	return &volume_opfs.Config{
		RootPath:    c.root + "/volume",
		LockPrefix:  c.root + "/volume",
		StoreConfig: &store_kvtx.Config{},
	}
}

// verifyMetaKey checks the deterministic value associated with a metadata key.
func verifyMetaKey(ctx context.Context, store kvtx.Store, key []byte) error {
	return verifyMetaValue(ctx, store, key, metaValue(key))
}

// verifyMetaValue checks one expected metadata value in a fresh read transaction.
func verifyMetaValue(ctx context.Context, store kvtx.Store, key, want []byte) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open meta read tx")
	}
	defer tx.Discard()
	val, found, err := tx.Get(ctx, key)
	if err != nil {
		return errors.Wrap(err, "get meta")
	}
	if !found {
		return errors.Errorf("missing meta key=%s", string(key))
	}
	if !bytes.Equal(val, want) {
		return errors.Errorf("bad meta value key=%s", string(key))
	}
	return nil
}

// runCounterInit creates and flushes the counter used by cross-worker lock probes.
func runCounterInit(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return err
	}
	defer release()
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
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	for range c.iterations {
		file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
		if err != nil {
			return errors.Wrap(err, "acquire counter")
		}
		var buf [8]byte
		if _, err := file.ReadAt(buf[:], 0); err != nil {
			release()
			return errors.Wrap(err, "read counter")
		}
		val := binary.LittleEndian.Uint64(buf[:])
		binary.LittleEndian.PutUint64(buf[:], val+1)
		if _, err := file.WriteAt(buf[:], 0); err != nil {
			release()
			return errors.Wrap(err, "write counter")
		}
		if err := file.Flush(); err != nil {
			release()
			return errors.Wrap(err, "flush counter")
		}
		release()
	}
	return nil
}

// runCounterTryLock checks the nonblocking Web Lock acquisition outcome.
func runCounterTryLock(c *config, want bool) error {
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
	postReady(c)
	<-ch
	return nil
}

// runCounterVerify checks that serialized increments preserved every writer's update.
func runCounterVerify(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", false)
	if err != nil {
		return err
	}
	defer release()
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
	root, err := opfs.GetRoot()
	if err != nil {
		return js.Undefined(), err
	}
	path := append([]string{rootName}, parts...)
	return opfs.GetDirectoryPath(root, path, true)
}

// blockKey encodes one worker, batch, and entry in lexical order.
func blockKey(worker, iteration, entry int) []byte {
	return []byte("b/" + strconv.Itoa(worker) + "/" + zeroPad(iteration, 5) + "/" + zeroPad(entry, 3))
}

// blockValue derives deterministic content from the workload key.
func blockValue(key []byte) []byte {
	return []byte("value:" + string(key))
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
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = deterministicLargeByte(r.offset, r.salt)
		r.offset++
	}
	r.remaining -= n
	return n, nil
}

// metaKey encodes a worker and iteration in lexical order.
func metaKey(worker, iteration int) []byte {
	return []byte("m/" + strconv.Itoa(worker) + "/" + zeroPad(iteration, 5))
}

// metaValue derives the small metadata value from its key.
func metaValue(key []byte) []byte {
	return []byte("value:" + string(key))
}

// metaMixedValue alternates small and multi-page values between writers.
func metaMixedValue(worker int, key []byte) []byte {
	if worker%2 != 0 {
		return metaValue(key)
	}
	seed := []byte("overflow:" + string(key) + ":")
	size := 4096 + 2048
	out := bytes.Repeat(seed, size/len(seed)+1)
	return out[:size]
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
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// newBlockEventSub subscribes with room for every expected publisher event.
func newBlockEventSub(c *config) *blockEventSub {
	ch := make(chan blockEvent, c.workers*c.iterations+c.workers+8)
	bc := js.Global().Get("BroadcastChannel").New(blockEventChannel(c.root))
	cb := js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data")
		ch <- blockEvent{
			typ:       data.Get("type").String(),
			worker:    data.Get("worker").Int(),
			iteration: data.Get("iteration").Int(),
		}
		return nil
	})
	bc.Set("onmessage", cb)
	return &blockEventSub{
		ch: ch,
		bc: bc,
		cb: cb,
	}
}

// Next waits for a publisher event or caller cancellation.
func (s *blockEventSub) Next(ctx context.Context) (blockEvent, error) {
	select {
	case ev := <-s.ch:
		return ev, nil
	case <-ctx.Done():
		return blockEvent{}, ctx.Err()
	}
}

// Close releases the subscription and its browser callback.
func (s *blockEventSub) Close() {
	s.bc.Set("onmessage", js.Null())
	s.bc.Call("close")
	s.cb.Release()
}

// newBlockEventPub opens the send side of the workload's broadcast channel.
func newBlockEventPub(root string) *blockEventPub {
	return &blockEventPub{
		bc: js.Global().Get("BroadcastChannel").New(blockEventChannel(root)),
	}
}

// Post broadcasts one publication or completion event.
func (p *blockEventPub) Post(ev blockEvent) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", ev.typ)
	obj.Set("worker", ev.worker)
	obj.Set("iteration", ev.iteration)
	p.bc.Call("postMessage", obj)
}

// Close closes the publication channel.
func (p *blockEventPub) Close() {
	p.bc.Call("close")
}

// blockEventChannel derives the shared publication channel from the test root.
func blockEventChannel(root string) string {
	return "opfs-chrometest:" + root
}

// counterReleaseChannel derives the lock-holder release channel from the test root.
func counterReleaseChannel(root string) string {
	return "opfs-chrometest-counter-release:" + root
}

// postReady announces that the worker reached the harness's synchronization point.
func postReady(c *config) {
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "ready")
	obj.Set("scenario", c.scenario)
	obj.Set("worker", c.worker)
	js.Global().Call("postMessage", obj)
}

// postProgress reports the current phase and optional byte or operation counts.
func postProgress(c *config, phase string, values ...int) {
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

// benchExtra carries operation counts and durations for the active probe
// from a scenario into the single worker result object.
var benchExtra map[string]int64

// postResult reports the terminal result and optional operation measurements.
func postResult(c *config, dur time.Duration, err error) {
	// Build the common result and optional benchmark fields.
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "result")
	if c != nil {
		obj.Set("scenario", c.scenario)
		obj.Set("worker", c.worker)
	}
	obj.Set("durationMs", dur.Milliseconds())
	for k, v := range benchExtra {
		obj.Set(k, v)
	}

	// Attach the current bridge handle count when this worker is remote.
	remote := js.Global().Get("__spacewaveOpfsBridgePort")
	if remote.Type() == js.TypeObject {
		handles := remote.Get("liveHandles")
		if handles.Type() == js.TypeNumber {
			obj.Set("remoteHandles", handles.Int())
		}
	}

	// Attach the terminal status and publish the result.
	obj.Set("ok", true)
	if err != nil {
		obj.Set("ok", false)
		obj.Set("error", err.Error())
	}
	js.Global().Call("postMessage", obj)
}

// verifyBlocks requires all expected content to survive publication and maintenance.
func verifyBlocks(ctx context.Context, c *config, blocks *engine.BlockStore, phase string) error {
	for w := range c.workers {
		for i := range c.iterations {
			for j := range c.batch {
				key := blockKey(w, i, j)
				value, found, err := getBlock(ctx, blocks, key)
				if err != nil {
					return errors.Wrap(err, "read block "+phase)
				}
				if !found {
					return errors.Errorf("missing block %s key=%s", phase, string(key))
				}
				if !bytes.Equal(value, blockValue(key)) {
					return errors.Errorf("bad block %s key=%s", phase, string(key))
				}
			}
		}
	}
	return nil
}

// getBlock reconstructs a deterministic block reference and reads it through StoreOps.
func getBlock(ctx context.Context, blocks *engine.BlockStore, key []byte) ([]byte, bool, error) {
	ref, err := block.BuildBlockRef(blockValue(key), nil)
	if err != nil {
		return nil, false, err
	}
	return blocks.GetBlock(ctx, ref)
}
