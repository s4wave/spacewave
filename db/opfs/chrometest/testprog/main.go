//go:build js

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	stderrors "errors"
	"io"
	"maps"
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
	packfile_writer "github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc_wal "github.com/s4wave/spacewave/db/block/gc/wal"
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
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/metashard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

type config struct {
	scenario   string
	root       string
	worker     int
	workers    int
	iterations int
	batch      int
	shards     int
	metrics    bool
}

type blockEvent struct {
	typ       string
	worker    int
	iteration int
}

type blockEventSub struct {
	ch chan blockEvent
	bc js.Value
	cb js.Func
}

type blockEventPub struct {
	bc js.Value
}

type manifestBloomCase struct {
	shard string
	pack  string
	size  int
}

type manifestSeedEntry struct {
	sub  string
	size int
}

var manifestSeedEntries = []manifestSeedEntry{
	{sub: "pack_bloom/00/pfv1_seed_left", size: 1700},
	{sub: "pack_bloom/zz/pfv1_seed_right", size: 1700},
}

const largeScenarioProgressEvery = 8 * 1024 * 1024

var manifestBloomCases = []manifestBloomCase{
	{shard: "mm", pack: "pfv1_manifest_middle_split", size: 2500},
	{shard: "2B", pack: "pfv1_manifest_2B", size: 1892},
	{shard: "4U", pack: "pfv1_manifest_4U", size: 2801},
	{shard: "Bn", pack: "pfv1_manifest_Bn", size: 2367},
	{shard: "pQ", pack: "pfv1_manifest_pQ", size: 2119},
	{shard: "z3", pack: "pfv1_manifest_z3", size: 1954},
}

func main() {
	start := time.Now()
	c, err := parseConfig(testArgs())
	if err == nil {
		err = run(context.Background(), c)
	}
	postResult(c, time.Since(start), err)
}

func testArgs() []string {
	if len(os.Args) >= 8 {
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

func parseConfig(args []string) (*config, error) {
	if len(args) < 9 {
		return nil, errors.Errorf("expected 8 args, got %d", len(args)-1)
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
	shards, err := strconv.Atoi(args[7])
	if err != nil {
		return nil, errors.Wrap(err, "parse shards")
	}
	metrics, err := strconv.ParseBool(args[8])
	if err != nil {
		return nil, errors.Wrap(err, "parse metrics")
	}
	return &config{
		scenario:   args[1],
		root:       args[2],
		worker:     worker,
		workers:    workers,
		iterations: iterations,
		batch:      batch,
		shards:     shards,
		metrics:    metrics,
	}, nil
}

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
	case "materialize-fanout-serial":
		return runMaterializeFanout(ctx, c, fanoutSerial)
	case "materialize-fanout-concurrent":
		return runMaterializeFanout(ctx, c, fanoutConcurrent)
	case "materialize-fanout-batched":
		return runMaterializeFanout(ctx, c, fanoutBatched)
	case "materialize-fanout-async-serial":
		return runMaterializeFanout(ctx, c, fanoutAsyncSerial)
	case "block-corrupt-compaction":
		return runBlockCorruptCompaction(ctx, c)
	case "block-zero-size-compaction":
		return runBlockZeroSizeCompaction(ctx, c)
	case "read-at-helper-loop":
		return runReadAtHelperLoop(c)
	case "gc-wal-write-loop":
		return runGCWalWriteLoop(ctx, c)
	case "gc-wal-verify":
		return verifyGCWal(c)
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
	case "block-orphan-segment":
		return runBlockOrphanSegment(c)
	case "block-orphan-verify-clean":
		return runBlockOrphanVerifyClean(c)
	case "meta-writer":
		return runMetaWriter(ctx, c)
	case "meta-verify":
		return runMetaVerify(ctx, c)
	case "meta-mixed-writer":
		return runMetaMixedWriter(ctx, c)
	case "meta-mixed-verify":
		return runMetaMixedVerify(ctx, c)
	case "meta-manifest-bloom-split":
		return runMetaManifestBloomSplit(ctx, c)
	case "meta-manifest-bloom-verify":
		return runMetaManifestBloomVerify(ctx, c)
	case "meta-crash-before-superblock":
		return runMetaCrashWrite(c, false)
	case "meta-crash-after-superblock":
		return runMetaCrashWrite(c, true)
	case "meta-crash-verify":
		return runMetaCrashVerify(ctx, c)
	case "meta-read-isolation":
		return runMetaReadIsolation(ctx, c)
	case "meta-reset-identity":
		return runMetaResetIdentity(ctx, c)
	case "meta-fallback-shortcut":
		return runMetaFallbackShortcut(ctx, c)
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
	case "volume-runtime-verify-incompatible-reset":
		return runVolumeRuntimeVerifyReset(ctx, c, volume_opfs.ResetReasonIncompatible)
	case "volume-runtime-verify-unknown-reset":
		return runVolumeRuntimeVerifyReset(ctx, c, volume_opfs.ResetReasonUnknown)
	case "volume-runtime-delete-verify":
		return runVolumeRuntimeDeleteVerify(ctx, c)
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

type pipeReadResult struct {
	n   int
	err error
}

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

func runReadFileHelperLoop(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"read-helper"})
	if err != nil {
		return err
	}
	want := []byte("tinygo-opfs-read-file-helper")
	if err := opfs.WriteFile(dir, "manifest-a", want); err != nil {
		return err
	}
	for i := 0; i < c.iterations; i++ {
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
	for i := 0; i < files; i++ {
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
	for i := 0; i < files; i++ {
		name := "chunk-" + zeroPad(i, 3) + ".bin"
		if !seen[name] {
			return errors.Errorf("%s missing from list directory result", name)
		}
	}
	return nil
}

func runLargeBlockBatch(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
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
	entries := make([]segment.Entry, entriesCount)
	for i := range entries {
		size := baseSize
		if i < remainder {
			size++
		}
		key := largeBlockKey(i)
		entries[i] = segment.Entry{
			Key:   key,
			Value: deterministicLargeBytes(size, i),
		}
	}
	postProgress(c, "large-block-put-start", 0, totalSize)
	if err := e.Put(ctx, entries); err != nil {
		release()
		return errors.Wrap(err, "put large block batch")
	}
	postProgress(c, "large-block-put-complete", totalSize, totalSize)
	postProgress(c, "large-block-readback-before-close-start", 0, totalSize)
	if err := verifyLargeBlocks(ctx, c, e, totalSize, entriesCount); err != nil {
		release()
		return err
	}
	postProgress(c, "large-block-readback-before-close-complete", totalSize, totalSize)
	if err := verifyLargeBlockPublishLocks(ctx, c); err != nil {
		release()
		return err
	}
	release()

	e, release, err = openBlockEngine(ctx, c)
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

func runLargeBlockVerify(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
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

func verifyLargeBlocks(ctx context.Context, c *config, e *blockshard.Engine, totalSize, entriesCount int) error {
	baseSize := totalSize / entriesCount
	remainder := totalSize % entriesCount
	for i := range entriesCount {
		size := baseSize
		if i < remainder {
			size++
		}
		key := largeBlockKey(i)
		got, found, err := e.GetContext(ctx, key)
		if err != nil {
			return errors.Wrapf(err, "get large block %d", i)
		}
		if !found {
			return errors.Errorf("large block %d not found", i)
		}
		want := deterministicLargeBytes(size, i)
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

func verifyLargeBlockPublishLocks(ctx context.Context, c *config) error {
	postProgress(c, "large-block-publish-lock-start")
	shardCount := c.shards
	if shardCount <= 0 {
		shardCount = blockshard.DefaultShardCount
	}
	settings := blockshard.DefaultSettings()
	settings.ShardCount = shardCount
	for shardID := 0; shardID < shardCount; shardID++ {
		dir, err := openTestDirectory(c.root, []string{"blocks", "shard-" + zeroPad(shardID, 2)})
		if err != nil {
			return err
		}
		shard, err := blockshard.NewShard(shardID, dir, c.root+"/blocks", settings)
		if err != nil {
			return errors.Wrapf(err, "open large block shard %d for lock liveness", shardID)
		}
		lockCtx, cancel := context.WithTimeout(ctx, time.Second)
		release, err := shard.AcquirePublishLockContext(lockCtx)
		cancel()
		if err != nil {
			return errors.Wrapf(err, "acquire large block shard %d publish lock after publish", shardID)
		}
		release()
	}
	postProgress(c, "large-block-publish-lock-complete")
	return nil
}

func runBlockCorruptCompaction(ctx context.Context, c *config) error {
	dir, err := openTestDirectory(c.root, []string{"block-corrupt-compaction"})
	if err != nil {
		return err
	}

	shard, before, err := publishCompactionInputSegments(ctx, c, dir, "corrupt-compaction")
	if err != nil {
		return err
	}
	corruptName := before.Segments[0].Filename
	data, err := opfs.ReadFile(dir, corruptName)
	if err != nil {
		return errors.Wrap(err, "read segment to corrupt")
	}
	if len(data) < segment.HeaderSize {
		return errors.New("segment too small to corrupt")
	}
	hdr, err := segment.DecodeHeader(data[:segment.HeaderSize])
	if err != nil {
		return errors.Wrap(err, "decode corrupt target header")
	}
	if int(hdr.DataOffset) >= len(data)-4 {
		return errors.New("segment has no data byte to corrupt")
	}
	data[hdr.DataOffset] ^= 0xff
	if err := opfs.WriteFile(dir, corruptName, data); err != nil {
		return errors.Wrap(err, "write corrupt segment")
	}

	plan := blockshard.PlanCompaction(shard, blockshard.DefaultL0Trigger)
	if plan == nil {
		return errors.New("expected compaction plan")
	}
	release, err := shard.AcquirePublishLock()
	if err != nil {
		return err
	}
	err = blockshard.ExecuteCompaction(ctx, shard, plan)
	release()
	if err == nil {
		return errors.New("expected corrupt compaction input to fail")
	}
	if !strings.Contains(err.Error(), "CRC32 mismatch") {
		return errors.Wrap(err, "expected CRC32 mismatch")
	}

	after := shard.Manifest()
	if after.Generation != before.Generation {
		return errors.Errorf("generation mutated after failed compaction: got %d want %d", after.Generation, before.Generation)
	}
	if len(after.Segments) != len(before.Segments) {
		return errors.Errorf("segments mutated after failed compaction: got %d want %d", len(after.Segments), len(before.Segments))
	}
	if len(after.PendingDelete) != len(before.PendingDelete) {
		return errors.Errorf("pending deletes mutated after failed compaction: got %d want %d", len(after.PendingDelete), len(before.PendingDelete))
	}
	return nil
}

func runBlockZeroSizeCompaction(ctx context.Context, c *config) error {
	dir, err := openTestDirectory(c.root, []string{"block-zero-size-compaction"})
	if err != nil {
		return err
	}

	_, before, err := publishCompactionInputSegments(ctx, c, dir, "zero-size-compaction")
	if err != nil {
		return err
	}
	zeroSizeManifest := before.Clone()
	zeroSizeManifest.Generation = before.Generation + 1
	for i := range zeroSizeManifest.Segments {
		zeroSizeManifest.Segments[i].Size = 0
	}
	if err := writeTestBlockshardManifest(dir, zeroSizeManifest); err != nil {
		return err
	}

	settings := blockshard.DefaultSettings()
	settings.MaxSegmentDataBytes = 128
	shard, err := blockshard.NewShard(0, dir, c.root+"/zero-size-compaction", settings)
	if err != nil {
		return err
	}
	plan := blockshard.PlanCompaction(shard, blockshard.DefaultL0Trigger)
	if plan == nil {
		return errors.New("expected compaction plan")
	}
	for i := range plan.InputSegs {
		if plan.InputSegs[i].Size != 0 {
			return errors.Errorf("plan input size=%d want zero", plan.InputSegs[i].Size)
		}
	}

	release, err := shard.AcquirePublishLock()
	if err != nil {
		return err
	}
	err = blockshard.ExecuteCompaction(ctx, shard, plan)
	release()
	if err != nil {
		return errors.Wrap(err, "execute compaction with zero-size inputs")
	}
	after := shard.Manifest()
	if after.Generation <= zeroSizeManifest.Generation {
		return errors.Errorf("generation after compaction=%d want > %d", after.Generation, zeroSizeManifest.Generation)
	}
	if len(after.Segments) == 0 {
		return errors.New("compaction produced no output segments")
	}
	for i := range after.Segments {
		if after.Segments[i].Size == 0 {
			return errors.Errorf("output segment %s has zero size", after.Segments[i].Filename)
		}
	}
	if err := verifyCompactionInputValues(dir, after, "zero-size-compaction"); err != nil {
		return err
	}
	return nil
}

func publishCompactionInputSegments(
	ctx context.Context,
	c *config,
	dir js.Value,
	lockSuffix string,
) (*blockshard.Shard, *blockshard.Manifest, error) {
	settings := blockshard.DefaultSettings()
	settings.MaxSegmentDataBytes = 128
	shard, err := blockshard.NewShard(0, dir, c.root+"/"+lockSuffix, settings)
	if err != nil {
		return nil, nil, err
	}

	release, err := shard.AcquirePublishLock()
	if err != nil {
		return nil, nil, err
	}
	for i := range blockshard.DefaultL0Trigger {
		key := []byte(lockSuffix + "-key-" + strconv.Itoa(i))
		value := bytes.Repeat([]byte{byte(i + 1)}, 48)
		if err := shard.Publish(ctx, []segment.Entry{{Key: key, Value: value}}); err != nil {
			release()
			return nil, nil, err
		}
	}
	release()

	manifest := shard.Manifest()
	if len(manifest.Segments) != blockshard.DefaultL0Trigger {
		return nil, nil, errors.Errorf("segments before compaction=%d want=%d", len(manifest.Segments), blockshard.DefaultL0Trigger)
	}
	return shard, manifest, nil
}

func verifyCompactionInputValues(dir js.Value, m *blockshard.Manifest, keyPrefix string) error {
	for i := range blockshard.DefaultL0Trigger {
		key := []byte(keyPrefix + "-key-" + strconv.Itoa(i))
		want := bytes.Repeat([]byte{byte(i + 1)}, 48)
		found := false
		for j := range m.Segments {
			sr, err := blockshard.OpenSegment(dir, m.Segments[j].Filename)
			if err != nil {
				return err
			}
			got, ok, err := sr.Get(key)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			found = true
			if !bytes.Equal(got, want) {
				return errors.Errorf("compacted value mismatch for %s", key)
			}
			break
		}
		if !found {
			return errors.Errorf("compacted key %s not found", key)
		}
	}
	return nil
}

func writeTestBlockshardManifest(dir js.Value, m *blockshard.Manifest) error {
	slot := "manifest-a"
	if m.Generation%2 == 0 {
		slot = "manifest-b"
	}
	return opfs.WriteFile(dir, slot, m.Encode())
}

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
	for i := 0; i < c.iterations; i++ {
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

func runGCWalWriteLoop(ctx context.Context, c *config) error {
	dir, err := openTestDirectory(c.root, []string{"gc", "wal"})
	if err != nil {
		return err
	}
	writer := block_gc_wal.NewWriter(
		dir,
		c.root+"/gc/wal",
		c.root+"|gc-wal-order",
		c.root+"|gc-stw",
	)
	for i := 0; i < c.iterations; i++ {
		edge := &block_gc_wal.RefEdge{
			Subject: "subject/" + zeroPad(c.worker, 2) + "/" + zeroPad(i, 5),
			Object:  "object/" + zeroPad(c.worker, 2) + "/" + zeroPad(i, 5),
		}
		if err := writer.Append(ctx, []*block_gc_wal.RefEdge{edge}, nil); err != nil {
			return errors.Wrapf(err, "append wal %d", i)
		}
	}
	return verifyGCWal(c)
}

func verifyGCWal(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"gc", "wal"})
	if err != nil {
		return err
	}
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return errors.Wrap(err, "list wal directory")
	}
	var walFiles int
	var maxSeq uint64
	for _, name := range names {
		if !strings.HasSuffix(name, ".wal") {
			continue
		}
		walFiles++
		data, err := opfs.ReadFile(dir, name)
		if err != nil {
			return errors.Wrapf(err, "read wal file %s", name)
		}
		var entry block_gc_wal.WALEntry
		if err := entry.UnmarshalVT(data); err != nil {
			return errors.Wrapf(err, "unmarshal wal file %s", name)
		}
		if entry.Sequence == 0 || len(entry.Adds) != 1 {
			return errors.Errorf("invalid wal entry %s sequence=%d adds=%d", name, entry.Sequence, len(entry.Adds))
		}
		if entry.Sequence > maxSeq {
			maxSeq = entry.Sequence
		}
	}
	if walFiles != c.iterations {
		return errors.Errorf("wal files=%d want=%d", walFiles, c.iterations)
	}
	if maxSeq != uint64(c.iterations) {
		return errors.Errorf("wal max sequence=%d want=%d", maxSeq, c.iterations)
	}
	return nil
}

func runBlockWriter(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventPub(c.root)
	defer events.Close()
	defer events.Post(blockEvent{
		typ:    "block-writer-done",
		worker: c.worker,
	})

	for i := 0; i < c.iterations; i++ {
		entries := make([]segment.Entry, c.batch)
		for j := range entries {
			key := blockKey(c.worker, i, j)
			entries[j] = segment.Entry{
				Key:   key,
				Value: blockValue(key),
			}
		}
		if err := e.Put(ctx, entries); err != nil {
			return errors.Wrap(err, "put block batch")
		}
		events.Post(blockEvent{
			typ:       "block-written",
			worker:    c.worker,
			iteration: i,
		})
		if i%4 == 0 {
			key := blockKey(c.worker, i, 0)
			val, found, err := e.GetContext(ctx, key)
			if err != nil {
				return errors.Wrap(err, "read own block")
			}
			if !found || string(val) != string(blockValue(key)) {
				return errors.Errorf("own block mismatch worker=%d iteration=%d found=%v", c.worker, i, found)
			}
		}
	}
	return nil
}

func runBlockReader(ctx context.Context, c *config, compact bool) error {
	// Open one reader runtime before the writers start.
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventSub(c)
	defer events.Close()
	postReady(c)

	// Read each published batch until every writer reports completion.
	done := make([]bool, c.workers)
	var found int
	var doneCount int
	for doneCount < c.workers {
		ev, err := events.Next(ctx)
		if err != nil {
			return err
		}
		switch ev.typ {
		case "block-written":
			for j := range c.batch {
				key := blockKey(ev.worker, ev.iteration, j)
				val, ok, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read concurrent block")
				}
				if !ok {
					continue
				}
				if string(val) != string(blockValue(key)) {
					return errors.Errorf("block value mismatch key=%s", string(key))
				}
				found++
			}
		case "block-writer-done":
			if ev.worker < 0 || ev.worker >= len(done) {
				return errors.Errorf("invalid writer id %d", ev.worker)
			}
			if !done[ev.worker] {
				done[ev.worker] = true
				doneCount++
			}
		default:
			continue
		}
	}

	// Confirm final visibility when publication events raced manifest refresh.
	if found == 0 {
		for w := range c.workers {
			for i := range c.iterations {
				key := blockKey(w, i, 0)
				val, ok, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read final concurrent block")
				}
				if ok {
					found++
					if string(val) != string(blockValue(key)) {
						return errors.Errorf("block value mismatch key=%s", string(key))
					}
				}
			}
		}
	}
	if found == 0 {
		return errors.New("reader found no concurrently written blocks")
	}
	if !compact {
		return nil
	}

	// Compact through the live reader and verify every retained value.
	if err := e.CompactOnce(ctx); err != nil {
		return errors.Wrap(err, "compact shared block volume")
	}
	for w := range c.workers {
		for i := range c.iterations {
			for j := range c.batch {
				key := blockKey(w, i, j)
				val, ok, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read block after compaction")
				}
				if !ok {
					return errors.Errorf("missing block after compaction key=%s", string(key))
				}
				if string(val) != string(blockValue(key)) {
					return errors.Errorf("bad block after compaction key=%s", string(key))
				}
			}
		}
	}
	return nil
}

func runBlockVerify(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()

	for w := 0; w < c.workers; w++ {
		for i := 0; i < c.iterations; i++ {
			for j := 0; j < c.batch; j++ {
				key := blockKey(w, i, j)
				val, found, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "verify block")
				}
				if !found {
					return errors.Errorf("missing block key=%s %s", string(key), describeBlockShard(c, e.ShardForKey(key)))
				}
				if string(val) != string(blockValue(key)) {
					return errors.Errorf("bad block value key=%s", string(key))
				}
			}
		}
	}
	return nil
}

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
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	key := []byte("remote-cache-key")
	value := []byte("remote-cache-value")
	if err := e.Put(ctx, []segment.Entry{{Key: key, Value: value}}); err != nil {
		return errors.Wrap(err, "write remote cache block")
	}
	got, found, err := e.GetContext(ctx, key)
	if err != nil {
		return errors.Wrap(err, "read remote cache block")
	}
	if !found || string(got) != string(value) {
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
	fresh, freshRelease, err := openBlockEngine(ctx, c)
	if err != nil {
		return errors.Wrap(err, "remount block engine after bridge swap")
	}
	defer freshRelease()
	got, found, err = fresh.GetContext(ctx, key)
	if err != nil {
		return errors.Wrap(err, "read remote cache block after remount")
	}
	if !found || string(got) != string(value) {
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

func openBlockEngine(ctx context.Context, c *config) (*blockshard.Engine, func(), error) {
	return openBlockEngineWithMetrics(ctx, c, nil)
}

func openBlockEngineWithMetrics(
	ctx context.Context,
	c *config,
	metrics *blockshard.BenchmarkMetrics,
) (*blockshard.Engine, func(), error) {
	dir, err := openTestDirectory(c.root, []string{"blocks"})
	if err != nil {
		return nil, nil, err
	}
	settings := blockshard.DefaultSettings()
	settings.ShardCount = c.shards
	settings.BenchmarkMetrics = metrics
	e, err := blockshard.NewEngineWithSettings(ctx, dir, c.root+"/blocks", settings)
	if err != nil {
		return nil, nil, err
	}
	return e, e.Close, nil
}

// materializeBlockBytes is the per-block payload for the manifest materialization
// fanout benchmark. Small blocks make the per-Publish fixed cost (Web Lock
// acquire, sync access handle open/flush/close, double-buffered manifest write)
// dominate, which is the regime first-run plugin manifest materialization hits:
// the world Manifest DAG is hundreds of small UnixFS and metadata blocks.
const materializeBlockBytes = 16 * 1024

type fanoutMode int

const (
	fanoutSerial fanoutMode = iota
	fanoutConcurrent
	fanoutBatched
	fanoutAsyncSerial
)

// runMaterializeFanout writes c.iterations content-addressed blocks into the
// production OPFS blockshard engine and records how the block feed pattern
// changes total write time and the number of OPFS Publish cycles.
//
// fanoutSerial mirrors the pre-refactor manifest copy loop: a foreground-awaited
// Put per block, so every block becomes its own Publish and pays the full fixed
// tax. fanoutConcurrent issues single-block puts from c.batch goroutines so the
// shard write actor coalesces concurrently queued puts into far fewer Publishes.
// fanoutBatched groups c.batch blocks per Put. fanoutAsyncSerial mirrors the
// landed async-default BlockStore.PutBlock path: a strictly serial one-block-at-
// a-time PutBackground feed (no caller concurrency or batching) fenced by a
// single Sync, so the actor coalesces whatever piles up behind the in-flight
// publish. All four write identical blocks; only the feed pattern differs.
func runMaterializeFanout(ctx context.Context, c *config, mode fanoutMode) error {
	blocks := c.iterations
	if blocks <= 0 {
		blocks = 512
	}
	entries := make([]segment.Entry, blocks)
	for i := range entries {
		entries[i] = segment.Entry{
			Key:   materializeBlockKey(i),
			Value: deterministicLargeBytes(materializeBlockBytes, i),
		}
	}

	var metrics *blockshard.BenchmarkMetrics
	if c.metrics {
		metrics = &blockshard.BenchmarkMetrics{}
	}
	e, release, err := openBlockEngineWithMetrics(ctx, c, metrics)
	if err != nil {
		return err
	}
	defer release()
	if metrics != nil {
		metrics.Reset()
	}
	generationStart := sumManifestGenerations(c)

	postProgress(c, "materialize-write-start", 0, blocks)
	start := time.Now()
	switch mode {
	case fanoutSerial:
		for i := range entries {
			if err := e.Put(ctx, entries[i:i+1]); err != nil {
				return errors.Wrapf(err, "serial put block %d", i)
			}
		}
	case fanoutConcurrent:
		if err := putEntriesConcurrent(ctx, e, entries, c.batch); err != nil {
			return err
		}
	case fanoutBatched:
		if err := putEntriesBatched(ctx, e, entries, c.batch); err != nil {
			return err
		}
	case fanoutAsyncSerial:
		if err := putEntriesAsyncSerial(ctx, e, entries); err != nil {
			return err
		}
	}
	writeDur := time.Since(start)
	postProgress(c, "materialize-write-complete", blocks, blocks)

	generationEnd := sumManifestGenerations(c)
	// Sync above is the durability barrier. No writer remains active in this
	// worker, so PendingEntries is stable for the result snapshot.
	benchExtra = map[string]int64{
		"writeMs":         writeDur.Milliseconds(),
		"blocks":          int64(blocks),
		"publishGen":      generationEnd,
		"generationDelta": generationEnd - generationStart,
		"pendingEntries":  int64(e.PendingEntries()),
	}
	if metrics != nil {
		m := metrics.Snapshot()
		benchExtra["acceptedRequests"] = m.AcceptedRequests
		benchExtra["acceptedEntries"] = m.AcceptedEntries
		benchExtra["acceptedBytes"] = m.AcceptedBytes
		benchExtra["actorCycles"] = m.ActorCycles
		benchExtra["drainRounds"] = m.DrainRounds
		benchExtra["publishAttempts"] = m.PublishAttempts
		benchExtra["publishSuccesses"] = m.PublishSuccesses
		benchExtra["publishErrors"] = m.PublishErrors
		benchExtra["publishErrorEntries"] = m.PublishErrorEntries
		benchExtra["publishedEntries"] = m.PublishedEntries
		benchExtra["publishedBytes"] = m.PublishedBytes
		benchExtra["publishedSegments"] = m.PublishedSegments
		benchExtra["manifestSlotReads"] = m.ManifestSlotReads
		benchExtra["manifestWrites"] = m.ManifestWrites
		benchExtra["reclaimCalls"] = m.ReclaimCalls
		benchExtra["reclaimHits"] = m.ReclaimHits
		benchExtra["reclaimDeletes"] = m.ReclaimDeletes
		benchExtra["reclaimNs"] = m.ReclaimNanoseconds
	}
	return nil
}

// putEntriesConcurrent writes single-block puts from concurrency goroutines.
// Each goroutine blocks on its own OPFS publish promise; while one awaits, the
// Go scheduler runs the others, so multiple blocks queue into a shard before its
// actor publishes and the actor coalesces them into one Publish.
func putEntriesConcurrent(ctx context.Context, e *blockshard.Engine, entries []segment.Entry, concurrency int) error {
	if concurrency <= 0 {
		concurrency = 16
	}
	idx := make(chan int, len(entries))
	for i := range entries {
		idx <- i
	}
	close(idx)

	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Go(func() {
			for i := range idx {
				if err := e.Put(ctx, entries[i:i+1]); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = errors.Wrapf(err, "concurrent put block %d", i)
					}
					mu.Unlock()
					return
				}
			}
		})
	}
	wg.Wait()
	return firstErr
}

// putEntriesBatched writes batch blocks per Put call, the explicit-batching
// alternative to raising walk concurrency.
func putEntriesBatched(ctx context.Context, e *blockshard.Engine, entries []segment.Entry, batch int) error {
	if batch <= 0 {
		batch = 64
	}
	for start := 0; start < len(entries); start += batch {
		end := min(start+batch, len(entries))
		if err := e.Put(ctx, entries[start:end]); err != nil {
			return errors.Wrapf(err, "batched put blocks %d..%d", start, end)
		}
	}
	return nil
}

// putEntriesAsyncSerial writes single-block PutBackground enqueues one at a time
// with no caller concurrency or batching, then fences once with Sync. This is
// the landed async-default BlockStore.PutBlock feed: the enqueue returns before
// the publish, so blocks pile up behind the shard's in-flight publish and the
// write actor coalesces whatever accumulated into one Publish. A single serial
// producer fills the pending buffer only shallowly, so the coalescing is partial
// (measured ~1.1-1.8x fewer Publishes than the serial Put path, weaker as the
// block count grows) rather than the order-of-magnitude collapse of a batched or
// concurrent feed; the trailing Sync still removes the per-block durability fence
// the serial Put path pays, which is the larger share of the write-time win.
func putEntriesAsyncSerial(ctx context.Context, e *blockshard.Engine, entries []segment.Entry) error {
	for i := range entries {
		if err := e.PutBackground(ctx, entries[i:i+1]); err != nil {
			return errors.Wrapf(err, "async-serial put block %d", i)
		}
	}
	if err := e.Sync(ctx); err != nil {
		return errors.Wrap(err, "async-serial sync")
	}
	return nil
}

func materializeBlockKey(entry int) []byte {
	return []byte("materialize/" + zeroPad(entry, 6))
}

// sumManifestGenerations sums the blockshard manifest generation across shards
// as a proxy for the number of OPFS Publish cycles performed: each publish
// advances the shard's manifest generation.
func sumManifestGenerations(c *config) int64 {
	shardCount := c.shards
	if shardCount <= 0 {
		shardCount = blockshard.DefaultShardCount
	}
	var total int64
	for shardID := 0; shardID < shardCount; shardID++ {
		dir, err := openTestDirectory(c.root, []string{"blocks", "shard-" + zeroPad(shardID, 2)})
		if err != nil {
			continue
		}
		a, err := opfs.ReadFile(dir, "manifest-a")
		if err != nil && !opfs.IsNotFound(err) {
			continue
		}
		b, err := opfs.ReadFile(dir, "manifest-b")
		if err != nil && !opfs.IsNotFound(err) {
			continue
		}
		if m := blockshard.PickManifest(a, b); m != nil {
			total += int64(m.Generation)
		}
	}
	return total
}

func describeBlockShard(c *config, shard int) string {
	dir, err := openTestDirectory(c.root, []string{"blocks", "shard-" + zeroPad(shard, 2)})
	if err != nil {
		return "describe-shard-error=" + err.Error()
	}
	a, err := opfs.ReadFile(dir, "manifest-a")
	if err != nil && !opfs.IsNotFound(err) {
		return "read-manifest-a-error=" + err.Error()
	}
	b, err := opfs.ReadFile(dir, "manifest-b")
	if err != nil && !opfs.IsNotFound(err) {
		return "read-manifest-b-error=" + err.Error()
	}
	m := blockshard.PickManifest(a, b)
	if m == nil {
		return "manifest=nil"
	}
	var sb strings.Builder
	sb.WriteString("shard=")
	sb.WriteString(strconv.Itoa(shard))
	sb.WriteString(" gen=")
	sb.WriteString(strconv.FormatUint(m.Generation, 10))
	sb.WriteString(" segments=")
	sb.WriteString(strconv.Itoa(len(m.Segments)))
	limit := min(len(m.Segments), 8)
	for i := range limit {
		seg := m.Segments[i]
		sb.WriteString(" ")
		sb.WriteString(seg.Filename)
		sb.WriteString("[")
		sb.Write(seg.MinKey)
		sb.WriteString("..")
		sb.Write(seg.MaxKey)
		sb.WriteString("]")
	}
	return sb.String()
}

func runBlockOrphanSegment(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"blocks", "shard-00"})
	if err != nil {
		return err
	}
	w := segment.NewWriter()
	key := []byte("orphan/terminated")
	w.Add(key, blockValue(key))
	var buf bytes.Buffer
	if _, err := w.Build(&buf); err != nil {
		return errors.Wrap(err, "build orphan segment")
	}
	if err := opfs.WriteFile(dir, orphanSegmentFilename(), buf.Bytes()); err != nil {
		return errors.Wrap(err, "write orphan segment")
	}
	postReady(c)
	_, err = io.Copy(io.Discard, neverReader{})
	return err
}

func runBlockOrphanVerifyClean(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"blocks", "shard-00"})
	if err != nil {
		return err
	}
	exists, err := opfs.FileExists(dir, orphanSegmentFilename())
	if err != nil {
		return err
	}
	if exists {
		return errors.Errorf("orphan segment %s still exists", orphanSegmentFilename())
	}
	return nil
}

func runMetaWriter(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for i := 0; i < c.iterations; i++ {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open meta write tx")
		}
		key := metaKey(c.worker, i)
		if err := tx.Set(ctx, key, metaValue(key)); err != nil {
			tx.Discard()
			return errors.Wrap(err, "set meta")
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit meta")
		}
		if i%5 == 0 {
			if err := verifyMetaKey(ctx, store, key); err != nil {
				return err
			}
		}
	}
	return nil
}

func runMetaVerify(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for w := 0; w < c.workers; w++ {
		for i := 0; i < c.iterations; i++ {
			if err := verifyMetaKey(ctx, store, metaKey(w, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// runMetaReadIsolation checks that a read transaction serves one committed
// generation. Reading several keys is how a caller assembles one object, so a
// transaction that answered from two generations would hand back a combination
// the store never held.
func runMetaReadIsolation(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}

	if err := putMetaPair(ctx, store, "1"); err != nil {
		return err
	}

	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open meta read tx")
	}
	defer readTx.Discard()

	val, found, err := readTx.Get(ctx, []byte("iso-a"))
	if err != nil {
		return errors.Wrap(err, "read iso-a")
	}
	if !found || string(val) != "1" {
		return errors.Errorf("iso-a = %q found=%v, want 1", val, found)
	}

	// Repeated reads with nothing committed in between stay on one generation
	// and must not fail.
	for i := 0; i < 8; i++ {
		if _, _, err := readTx.Get(ctx, []byte("iso-b")); err != nil {
			return errors.Wrapf(err, "repeat read %d", i)
		}
	}

	if err := putMetaPair(ctx, store, "2"); err != nil {
		return err
	}

	readErr := func() error {
		_, _, err := readTx.Get(ctx, []byte("iso-b"))
		return err
	}()
	if !errors.Is(readErr, metashard.ErrGenerationChanged) {
		return errors.Errorf("read after foreign commit err = %v, want ErrGenerationChanged", readErr)
	}
	// The callers that reopen a transaction at a fresh generation recognize the
	// store-level snapshot error, not this store's private sentinel, so a
	// generation change has to reach them as one.
	if !errors.Is(readErr, kvtx.ErrInvalidSnapshot) {
		return errors.Errorf("read after foreign commit err = %v, want kvtx.ErrInvalidSnapshot", readErr)
	}

	nextTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open fresh meta read tx")
	}
	defer nextTx.Discard()
	for _, key := range []string{"iso-a", "iso-b"} {
		val, found, err := nextTx.Get(ctx, []byte(key))
		if err != nil {
			return errors.Wrapf(err, "fresh read %s", key)
		}
		if !found || string(val) != "2" {
			return errors.Errorf("fresh %s = %q found=%v, want 2", key, val, found)
		}
	}
	return nil
}

func putMetaPair(ctx context.Context, store *metashard.MetaStore, value string) error {
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open meta write tx")
	}
	for _, key := range []string{"iso-a", "iso-b"} {
		if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
			tx.Discard()
			return errors.Wrapf(err, "set %s", key)
		}
	}
	return errors.Wrap(tx.Commit(ctx), "commit meta pair")
}

func putMetaValue(ctx context.Context, store *metashard.MetaStore, key, value string) error {
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open meta write tx")
	}
	if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
		tx.Discard()
		return errors.Wrapf(err, "set %s", key)
	}
	return errors.Wrapf(tx.Commit(ctx), "commit %s", key)
}

// runMetaResetIdentity checks that a shard holding cached committed state
// notices when another agent replaces the database underneath it.
//
// Corruption recovery deletes the page file and builds a new database in its
// place. A shard decides whether to revalidate by comparing the state it
// loaded against what is on disk, so a replacement that reached the same
// commit count with the same tree shape must still be distinguishable from the
// database it replaced. Otherwise the cached shard serves the old root page
// over the new file and returns metadata that no longer exists.
func runMetaResetIdentity(ctx context.Context, c *config) error {
	cached, err := openMetaStore(c)
	if err != nil {
		return err
	}
	if err := putMetaValue(ctx, cached, "reset-key", "before"); err != nil {
		return err
	}
	// Read once so this shard caches the state it just committed.
	if err := verifyMetaValue(ctx, cached, []byte("reset-key"), []byte("before")); err != nil {
		return errors.Wrap(err, "read before replacement")
	}

	dir, err := openTestDirectory(c.root, []string{"meta"})
	if err != nil {
		return err
	}
	for _, name := range []string{"pages.dat", "super-a", "super-b"} {
		if err := opfs.DeleteFile(dir, name); err != nil && !opfs.IsNotFound(err) {
			return errors.Wrapf(err, "delete %s", name)
		}
	}

	// Build the replacement in one commit, so it reaches the same commit count
	// as the database that was deleted while holding a different root page and
	// page count. That is the state a shard comparing commit counts alone would
	// mistake for the one it cached.
	replacement, err := openMetaStore(c)
	if err != nil {
		return err
	}
	tx, err := replacement.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open replacement write tx")
	}
	for i := 0; i < 512; i++ {
		if err := tx.Set(ctx, metaKey(0, i), metaValue(metaKey(0, i))); err != nil {
			tx.Discard()
			return errors.Wrap(err, "set replacement key")
		}
	}
	if err := tx.Set(ctx, []byte("reset-key"), []byte("after!")); err != nil {
		tx.Discard()
		return errors.Wrap(err, "set replacement reset-key")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit replacement")
	}

	return errors.Wrap(
		verifyMetaValue(ctx, cached, []byte("reset-key"), []byte("after!")),
		"read after replacement",
	)
}

// runMetaFallbackShortcut checks that a shard serving the older superblock
// still recognizes its own loaded state on the next read.
//
// A superblock whose header decodes can still fail tree validation, and the
// shard then falls back to the other slot. The rejected slot stays on disk, so
// a shard that decided whether to revalidate by looking at the newest slot
// alone would find a stranger there on every read and walk the whole tree
// again, under the shared metadata lock, for each point read.
func runMetaFallbackShortcut(ctx context.Context, c *config) error {
	shard, store, err := openMetaShard(c)
	if err != nil {
		return err
	}
	if err := putMetaValue(ctx, store, "fallback-key", "value!"); err != nil {
		return err
	}
	if err := shard.Close(); err != nil {
		return errors.Wrap(err, "close writer shard")
	}

	dir, err := openTestDirectory(c.root, []string{"meta"})
	if err != nil {
		return err
	}
	committed, err := readCurrentMetaSuperblock(dir)
	if err != nil {
		return err
	}
	if committed == nil {
		return errors.New("no committed meta superblock after write")
	}

	// Publish a newer superblock whose root page lies outside the page file.
	// Its header decodes, so the shard has to walk the tree to reject it, and
	// the slot it lands in is the one the committed superblock does not hold.
	poisoned := pagestore.Superblock{
		Magic:        pagestore.SuperblockMagic,
		Version:      1,
		Generation:   committed.Generation + 1,
		RootPage:     pagestore.PageID(committed.PageCount),
		FreelistPage: committed.FreelistPage,
		PageCount:    committed.PageCount,
	}
	slot := "super-a"
	if poisoned.Generation%2 == 0 {
		slot = "super-b"
	}
	var poisonedBuf [pagestore.SuperblockSize]byte
	pagestore.EncodeSuperblock(poisonedBuf[:], &poisoned)
	if err := opfs.WriteFile(dir, slot, poisonedBuf[:]); err != nil {
		return errors.Wrap(err, "write poisoned superblock")
	}

	reader, readerStore, err := openMetaShard(c)
	if err != nil {
		return err
	}
	if err := verifyMetaValue(
		ctx,
		readerStore,
		[]byte("fallback-key"),
		[]byte("value!"),
	); err != nil {
		return errors.Wrap(err, "read through fallback superblock")
	}

	before := reader.Revalidations()
	for i := 0; i < 4; i++ {
		if err := verifyMetaValue(
			ctx,
			readerStore,
			[]byte("fallback-key"),
			[]byte("value!"),
		); err != nil {
			return errors.Wrap(err, "repeat read through fallback superblock")
		}
	}
	if after := reader.Revalidations(); after != before {
		return errors.Errorf(
			"reads over an unchanged fallback shard revalidated %d times, want 0",
			after-before,
		)
	}
	return nil
}

func runMetaMixedWriter(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for i := 0; i < c.iterations; i++ {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open mixed meta write tx")
		}
		key := metaKey(c.worker, i)
		if err := tx.Set(ctx, key, metaMixedValue(c.worker, key)); err != nil {
			tx.Discard()
			return errors.Wrap(err, "set mixed meta")
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit mixed meta")
		}
		if i%4 == 0 {
			if err := verifyMetaValue(ctx, store, key, metaMixedValue(c.worker, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func runMetaMixedVerify(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for w := 0; w < c.workers; w++ {
		for i := 0; i < c.iterations; i++ {
			key := metaKey(w, i)
			if err := verifyMetaValue(ctx, store, key, metaMixedValue(w, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func runMetaManifestBloomSplit(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open manifest seed tx")
	}
	defer tx.Discard()
	for _, entry := range manifestSeedEntries {
		if err := tx.Set(ctx, manifestKey(entry.sub), manifestSizedValue(entry.sub, entry.size)); err != nil {
			return errors.Wrap(err, "set manifest seed")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit manifest seed")
	}

	tx, err = store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open manifest delta tx")
	}
	defer tx.Discard()
	if err := tx.Set(ctx, manifestKey("meta/lastPullSequence"), []byte("42")); err != nil {
		return errors.Wrap(err, "set manifest sequence")
	}
	for _, entry := range manifestBloomCases {
		key := manifestKey("pack_bloom/" + entry.shard + "/" + entry.pack)
		if err := tx.Set(ctx, key, manifestBloomValue(entry)); err != nil {
			return errors.Wrap(err, "set manifest bloom")
		}
		packKey := manifestKey("packs/" + entry.shard + "/" + entry.pack)
		if err := tx.Set(ctx, packKey, manifestPackValue(entry)); err != nil {
			return errors.Wrap(err, "set manifest pack")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit manifest delta")
	}
	return nil
}

func runMetaManifestBloomVerify(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for _, entry := range manifestSeedEntries {
		if err := verifyMetaValue(ctx, store, manifestKey(entry.sub), manifestSizedValue(entry.sub, entry.size)); err != nil {
			return err
		}
	}
	if err := verifyMetaValue(ctx, store, manifestKey("meta/lastPullSequence"), []byte("42")); err != nil {
		return err
	}
	for _, entry := range manifestBloomCases {
		key := manifestKey("pack_bloom/" + entry.shard + "/" + entry.pack)
		if err := verifyMetaValue(ctx, store, key, manifestBloomValue(entry)); err != nil {
			return err
		}
		packKey := manifestKey("packs/" + entry.shard + "/" + entry.pack)
		if err := verifyMetaValue(ctx, store, packKey, manifestPackValue(entry)); err != nil {
			return err
		}
	}
	return nil
}

func runMetaCrashWrite(c *config, flipSuperblock bool) error {
	dir, err := openTestDirectory(c.root, []string{"meta"})
	if err != nil {
		return err
	}
	sb, err := readCurrentMetaSuperblock(dir)
	if err != nil {
		return err
	}
	pager := metashard.NewOpfsPager(dir, "pages.dat", pagestore.DefaultPageSize)
	if sb != nil {
		pager.SetPageCount(sb.PageCount)
		if err := pager.LoadFreelist(sb.FreelistPage); err != nil {
			return errors.Wrap(err, "load freelist")
		}
	}
	tree := pagestore.NewTree(pager)
	if sb != nil {
		tree = pagestore.OpenTree(pager, sb.RootPage)
	}
	key := metaKey(0, 0)
	if err := tree.Put(key, metaCrashValue(key)); err != nil {
		return errors.Wrap(err, "put crash meta")
	}
	freelistPage, err := pager.PersistFreelist()
	if err != nil {
		return errors.Wrap(err, "persist crash freelist")
	}
	pager.Flush()
	if err := pager.Close(); err != nil {
		return errors.Wrap(err, "close crash pager")
	}
	if flipSuperblock {
		gen := uint64(1)
		if sb != nil {
			gen = sb.Generation + 1
		}
		next := pagestore.Superblock{
			Magic:        pagestore.SuperblockMagic,
			Version:      1,
			Generation:   gen,
			RootPage:     tree.RootID(),
			FreelistPage: freelistPage,
			PageCount:    pager.PageCount(),
		}
		slot := "super-a"
		if gen%2 == 0 {
			slot = "super-b"
		}
		var sbBuf [pagestore.SuperblockSize]byte
		pagestore.EncodeSuperblock(sbBuf[:], &next)
		if err := opfs.WriteFile(dir, slot, sbBuf[:]); err != nil {
			return errors.Wrap(err, "write crash superblock")
		}
	}
	postReady(c)
	_, err = io.Copy(io.Discard, neverReader{})
	return err
}

func runMetaCrashVerify(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	key := metaKey(0, 0)
	return verifyMetaValue(ctx, store, key, metaCrashValue(key))
}

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
	return nil
}

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

	event, err := waitCoordEvent(ctx, watch.Events(), before.Generation)
	if err != nil {
		return err
	}
	if event.RootChanged == nil || !event.RootChanged.EqualsRef(ref) {
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
	// PutBlock is async by default: its blockshard publish, which fires the
	// cross-worker BroadcastChannel wakeup the watcher waits for, is deferred.
	// A cross-worker coordinator change is observable only once fenced at the
	// volume commit boundary, so Sync here as a production writer would before
	// the watcher can rely on seeing the advanced generation.
	if _, err := vol.Sync(ctx); err != nil {
		return errors.Wrap(err, "sync broadcast")
	}
	return nil
}

func volumeCoordScope(vol volume.Volume, c *config) coord.Scope {
	return coord.Scope{
		VolumeID:      vol.GetID(),
		ObjectStoreID: c.root + "/coord",
		ParticipantID: c.scenario + "-" + strconv.Itoa(c.worker),
	}
}

func volumeCoordRoot(c *config, suffix string) *bucket.ObjectRef {
	return &bucket.ObjectRef{BucketId: c.root + "/coord/" + suffix}
}

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

func runVolumeRuntimeSeedUnknown(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"volume"})
	if err != nil {
		return err
	}
	return opfs.WriteFile(dir, "legacy-only", []byte("unknown"))
}

func runVolumeRuntimeVerifyReset(ctx context.Context, c *config, reason volume_opfs.ResetReason) error {
	logger := logrus.New()
	hook := &captureLogHook{}
	logger.AddHook(hook)
	le := logrus.NewEntry(logger)
	before := volume_opfs.RuntimeResetCount(reason)
	vol, err := openVolumeWithLogger(ctx, c, le)
	if err != nil {
		return err
	}
	if err := vol.Close(); err != nil {
		return err
	}
	after := volume_opfs.RuntimeResetCount(reason)
	if after != before+1 {
		return errors.Errorf("runtime reset count for %s = %d, want %d", reason, after, before+1)
	}
	if !hook.hasVolumeResetLog(c.root+"/volume", reason) {
		return errors.Errorf("missing reset log for reason %s logs=%v", reason, hook.entries)
	}

	dir, err := openTestDirectory(c.root, []string{"volume"})
	if err != nil {
		return err
	}
	exists, err := opfs.FileExists(dir, "legacy-only")
	if err != nil {
		return err
	}
	if exists {
		return errors.New("legacy-only file survived v2 reset")
	}
	marker, err := opfs.ReadFile(dir, ".spacewave-opfs-format.json")
	if err != nil {
		return errors.Wrap(err, "read v2 marker")
	}
	if !strings.Contains(string(marker), `"version":2`) {
		return errors.Errorf("marker = %s, want version 2", string(marker))
	}
	for _, name := range []string{"blocks", "meta", "gc"} {
		if _, err := opfs.GetDirectory(dir, name, false); err != nil {
			return errors.Wrapf(err, "open v2 %s directory", name)
		}
	}
	return nil
}

type captureLogHook struct {
	entries []capturedLogEntry
}

type capturedLogEntry struct {
	level   logrus.Level
	message string
	data    logrus.Fields
}

func (h *captureLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *captureLogHook) Fire(entry *logrus.Entry) error {
	data := make(logrus.Fields, len(entry.Data))
	maps.Copy(data, entry.Data)
	h.entries = append(h.entries, capturedLogEntry{
		level:   entry.Level,
		message: entry.Message,
		data:    data,
	})
	return nil
}

func (h *captureLogHook) hasVolumeResetLog(rootPath string, reason volume_opfs.ResetReason) bool {
	for _, entry := range h.entries {
		if entry.level != logrus.WarnLevel {
			continue
		}
		if entry.message != "reset opfs volume root for v2 format" {
			continue
		}
		if entry.data["root_path"] != rootPath {
			continue
		}
		if entry.data["reason"] != string(reason) {
			continue
		}
		if entry.data["format_version"] != uint32(2) {
			continue
		}
		if reason == volume_opfs.ResetReasonIncompatible && entry.data["previous_format_version"] != uint32(1) {
			continue
		}
		return true
	}
	return false
}

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
// blockshard OPFS volume defer durability to Sync: a per-commit write advances
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

type coordinatorWorldEngine struct {
	vol           *volume_opfs.Opfs
	objectStoreID string
	bucketID      string
	store         object.ObjectStore
	storeRelease  func()
	cursor        *bucket_lookup.Cursor
	engine        *world_block.Engine
}

func openCoordinatorWorldEngine(ctx context.Context, c *config, participant string) (*coordinatorWorldEngine, error) {
	return openWorldEngine(ctx, c, participant, false)
}

func openDeferredWorldEngine(ctx context.Context, c *config, participant string) (*coordinatorWorldEngine, error) {
	return openWorldEngine(ctx, c, participant, true)
}

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

func objectRefsEqual(a, b *bucket.ObjectRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EqualsRef(b)
}

func (h *coordinatorWorldEngine) refreshHead(ctx context.Context) error {
	headRef, err := h.readHead(ctx)
	if err != nil || headRef == nil || headRef.GetRootRef().GetEmpty() {
		return err
	}
	return h.engine.SetRootRef(ctx, headRef)
}

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

func runWorldResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()

	return runWorldResourceLargeUnixFSUploadOnBucket(ctx, c, vol, vol)
}

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

type probeDirtyTrackingStore struct {
	store      block.StoreOps
	dirtyStore kvtx.Store
	flusher    *probeSyncFlusher
}

var _ block.StoreOps = (*probeDirtyTrackingStore)(nil)

func (d *probeDirtyTrackingStore) GetHashType() hash.HashType {
	return d.store.GetHashType()
}

func (d *probeDirtyTrackingStore) GetSupportedFeatures() block.StoreFeature {
	return d.store.GetSupportedFeatures()
}

func (d *probeDirtyTrackingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := d.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &probeDirtyTrackingStore{store: store, dirtyStore: d.dirtyStore, flusher: d.flusher}, release, nil
}

func (d *probeDirtyTrackingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := d.store.PutBlock(ctx, data, opts)
	if err == nil && !existed {
		err = d.markDirty(ctx, ref.GetHash(), int64(len(data)))
	}
	return ref, existed, err
}

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

func (d *probeDirtyTrackingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return d.store.GetBlock(ctx, ref)
}

func (d *probeDirtyTrackingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return d.store.GetBlockExists(ctx, ref)
}

func (d *probeDirtyTrackingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return d.store.GetBlockExistsBatch(ctx, refs)
}

func (d *probeDirtyTrackingStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return d.store.RmBlock(ctx, ref)
}

func (d *probeDirtyTrackingStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return d.store.StatBlock(ctx, ref)
}

func (d *probeDirtyTrackingStore) Sync(ctx context.Context) (bool, error) {
	return d.store.Sync(ctx)
}

func (d *probeDirtyTrackingStore) BeginDeferFlush() {
	block.BeginDeferFlush(d.store)
}

func (d *probeDirtyTrackingStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, d.store)
}

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
	probeSyncSizeThresholdBytes int64 = 48 * 1024 * 1024
	probeSyncFlushMaxPackBytes  int64 = 4 * 1024 * 1024
)

type probeSyncFlusher struct {
	upper      block.StoreOps
	dirtyStore kvtx.Store
	done       chan error

	mtx       sync.Mutex
	dirtySize int64
	started   bool
}

func newProbeSyncFlusher(upper block.StoreOps, dirtyStore kvtx.Store) *probeSyncFlusher {
	return &probeSyncFlusher{
		upper:      upper,
		dirtyStore: dirtyStore,
		done:       make(chan error, 1),
	}
}

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

func (f *probeSyncFlusher) wait() error {
	f.mtx.Lock()
	started := f.started
	f.mtx.Unlock()
	if !started {
		return nil
	}
	return <-f.done
}

type probeDirtyCandidate struct {
	hash *hash.Hash
	size int64
}

type probeDirtyBlock struct {
	hash *hash.Hash
	data []byte
}

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

func packProbeDirtyBlocks(blocks []probeDirtyBlock) error {
	var buf bytes.Buffer
	idx := 0
	_, err := packfile_writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		block := blocks[idx]
		idx++
		return block.hash, block.data, nil
	})
	return errors.Wrap(err, "pack dirty blocks")
}

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

func isExpectedMuxCloseError(err error) bool {
	return stderrors.Is(err, context.Canceled) ||
		stderrors.Is(err, io.EOF) ||
		stderrors.Is(err, io.ErrClosedPipe) ||
		stderrors.Is(err, net.ErrClosed)
}

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

func resourceFullReadChunkSize(c *config) int {
	if c != nil && c.batch > 0 {
		return c.batch
	}
	return 256 * 1024
}

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

type generatedUploadTreeStream struct {
	ctx       context.Context
	c         *config
	name      string
	totalSize int
	salt      int
	offset    int
	startSent bool
}

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

func (s *generatedUploadTreeStream) Context() context.Context {
	return s.ctx
}

func (s *generatedUploadTreeStream) MsgSend(srpc.Message) error {
	return nil
}

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

func (s *generatedUploadTreeStream) CloseSend() error {
	return nil
}

func (s *generatedUploadTreeStream) Close() error {
	return nil
}

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

func (s *generatedUploadTreeStream) RecvTo(req *s4wave_unixfs.HandleUploadTreeRequest) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*req = *next
	return nil
}

func readCurrentMetaSuperblock(dir js.Value) (*pagestore.Superblock, error) {
	a, err := opfs.ReadFile(dir, "super-a")
	if err != nil && !opfs.IsNotFound(err) {
		return nil, errors.Wrap(err, "read super-a")
	}
	b, err := opfs.ReadFile(dir, "super-b")
	if err != nil && !opfs.IsNotFound(err) {
		return nil, errors.Wrap(err, "read super-b")
	}
	sb := pagestore.PickSuperblock(a, b)
	if sb == nil && (len(a) != 0 || len(b) != 0) {
		return nil, errors.New("no valid meta superblock")
	}
	return sb, nil
}

func openMetaStore(c *config) (*metashard.MetaStore, error) {
	_, store, err := openMetaShard(c)
	return store, err
}

// openMetaShard returns the shard alongside its store, for scenarios that
// assert on shard-level counters rather than only on stored values.
func openMetaShard(c *config) (*metashard.MetaShard, *metashard.MetaStore, error) {
	dir, err := openTestDirectory(c.root, []string{"meta"})
	if err != nil {
		return nil, nil, err
	}
	shard, err := metashard.NewMetaShard(dir, c.root+"/meta", 4096, nil)
	if err != nil {
		return nil, nil, err
	}
	return shard, metashard.NewMetaStore(shard), nil
}

func openVolume(ctx context.Context, c *config) (*volume_opfs.Opfs, error) {
	return openVolumeWithLogger(ctx, c, logrus.NewEntry(logrus.New()))
}

func openVolumeWithLogger(ctx context.Context, c *config, le *logrus.Entry) (*volume_opfs.Opfs, error) {
	return volume_opfs.NewOpfs(ctx, le, newOPFSConfig(c))
}

func newOPFSConfig(c *config) *volume_opfs.Config {
	return &volume_opfs.Config{
		RootPath:        c.root + "/volume",
		LockPrefix:      c.root + "/volume",
		StoreConfig:     &store_kvtx.Config{},
		BlockShardCount: uint32(c.shards),
		ResetPolicy:     "automatic",
	}
}

func verifyMetaKey(ctx context.Context, store *metashard.MetaStore, key []byte) error {
	return verifyMetaValue(ctx, store, key, metaValue(key))
}

func verifyMetaValue(ctx context.Context, store *metashard.MetaStore, key, want []byte) error {
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

func runCounterIncrement(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	for i := 0; i < c.iterations; i++ {
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

func openTestDirectory(rootName string, parts []string) (js.Value, error) {
	root, err := opfs.GetRoot()
	if err != nil {
		return js.Undefined(), err
	}
	path := append([]string{rootName}, parts...)
	return opfs.GetDirectoryPath(root, path, true)
}

func blockKey(worker, iteration, entry int) []byte {
	return []byte("b/" + strconv.Itoa(worker) + "/" + zeroPad(iteration, 5) + "/" + zeroPad(entry, 3))
}

func largeBlockKey(entry int) []byte {
	return []byte("large/" + zeroPad(entry, 5))
}

func blockValue(key []byte) []byte {
	return []byte("value:" + string(key))
}

func deterministicLargeBytes(size int, salt int) []byte {
	buf := make([]byte, size)
	fillDeterministicLargeBytes(buf, 0, salt)
	return buf
}

func deterministicLargeWindow(offset, size int, salt int) []byte {
	buf := make([]byte, size)
	fillDeterministicLargeBytes(buf, offset, salt)
	return buf
}

func fillDeterministicLargeBytes(buf []byte, offset int, salt int) {
	for i := range buf {
		buf[i] = deterministicLargeByte(offset+i, salt)
	}
}

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

type deterministicLargeReader struct {
	remaining int
	offset    int
	salt      int
}

func newDeterministicLargeReader(size int, salt int) *deterministicLargeReader {
	return &deterministicLargeReader{
		remaining: size,
		salt:      salt,
	}
}

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

func metaKey(worker, iteration int) []byte {
	return []byte("m/" + strconv.Itoa(worker) + "/" + zeroPad(iteration, 5))
}

func metaValue(key []byte) []byte {
	return []byte("value:" + string(key))
}

func metaMixedValue(worker int, key []byte) []byte {
	if worker%2 != 0 {
		return metaValue(key)
	}
	seed := []byte("overflow:" + string(key) + ":")
	size := pagestore.DefaultPageSize + 2048
	out := bytes.Repeat(seed, size/len(seed)+1)
	return out[:size]
}

func manifestKey(sub string) []byte {
	return []byte("h/objs/p/spacewave/test-account/bstore/test-bstore/meta/" + sub)
}

func manifestBloomValue(entry manifestBloomCase) []byte {
	return manifestSizedValue(entry.pack, entry.size)
}

func manifestPackValue(entry manifestBloomCase) []byte {
	return []byte("pack:" + entry.shard + "/" + entry.pack)
}

func manifestSizedValue(seed string, size int) []byte {
	prefix := []byte("manifest:" + seed + ":")
	out := bytes.Repeat(prefix, size/len(prefix)+1)
	return out[:size]
}

func metaCrashValue(key []byte) []byte {
	return []byte("crash:" + string(key))
}

func volumeMetaKey() []byte {
	return []byte("volume/runtime/meta")
}

func volumeMetaValue() []byte {
	return []byte("volume-runtime-meta-value")
}

func volumeRefKey() []byte {
	return []byte("volume/runtime/block-ref")
}

func volumeBlockValue() []byte {
	return []byte("volume-runtime-block-value")
}

func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

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

func (s *blockEventSub) Next(ctx context.Context) (blockEvent, error) {
	select {
	case ev := <-s.ch:
		return ev, nil
	case <-ctx.Done():
		return blockEvent{}, ctx.Err()
	}
}

func (s *blockEventSub) Close() {
	s.bc.Set("onmessage", js.Null())
	s.bc.Call("close")
	s.cb.Release()
}

func newBlockEventPub(root string) *blockEventPub {
	return &blockEventPub{
		bc: js.Global().Get("BroadcastChannel").New(blockEventChannel(root)),
	}
}

func (p *blockEventPub) Post(ev blockEvent) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", ev.typ)
	obj.Set("worker", ev.worker)
	obj.Set("iteration", ev.iteration)
	p.bc.Call("postMessage", obj)
}

func (p *blockEventPub) Close() {
	p.bc.Call("close")
}

func blockEventChannel(root string) string {
	return "opfs-chrometest:" + root
}

func orphanSegmentFilename() string {
	return "seg-999999.sst"
}

func counterReleaseChannel(root string) string {
	return "opfs-chrometest-counter-release:" + root
}

func postReady(c *config) {
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "ready")
	obj.Set("scenario", c.scenario)
	obj.Set("worker", c.worker)
	js.Global().Call("postMessage", obj)
}

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

// benchExtra carries optional benchmark metrics (writeMs, blocks, publishGen)
// from a scenario into the single worker result object.
var benchExtra map[string]int64

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

type neverReader struct{}

func (neverReader) Read(p []byte) (int, error) {
	ch := make(chan struct{})
	<-ch
	return 0, nil
}
