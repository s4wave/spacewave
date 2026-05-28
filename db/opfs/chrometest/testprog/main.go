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
	"syscall/js"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	block_gc_wal "github.com/s4wave/spacewave/db/block/gc/wal"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/metashard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
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
	if len(args) < 8 {
		return nil, errors.Errorf("expected 7 args, got %d", len(args)-1)
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
	return &config{
		scenario:   args[1],
		root:       args[2],
		worker:     worker,
		workers:    workers,
		iterations: iterations,
		batch:      batch,
		shards:     shards,
	}, nil
}

func run(ctx context.Context, c *config) error {
	switch c.scenario {
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
	case "block-corrupt-compaction":
		return runBlockCorruptCompaction(ctx, c)
	case "block-zero-size-compaction":
		return runBlockZeroSizeCompaction(ctx, c)
	case "read-at-helper-loop":
		return runReadAtHelperLoop(c)
	case "gc-wal-write-loop":
		return runGCWalWriteLoop(ctx, c)
	case "block-writer":
		return runBlockWriter(ctx, c)
	case "block-reader":
		return runBlockReader(ctx, c)
	case "block-verify":
		return runBlockVerify(ctx, c)
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
	case "volume-runtime-seed-current-v1":
		return runVolumeRuntimeSeedCurrentV1(c)
	case "volume-runtime-seed-incompatible":
		return runVolumeRuntimeSeedIncompatible(c)
	case "volume-runtime-seed-unknown":
		return runVolumeRuntimeSeedUnknown(c)
	case "volume-runtime-verify-current-v1-reset":
		return runVolumeRuntimeVerifyReset(ctx, c, volume_opfs.ResetReasonCurrentV1)
	case "volume-runtime-verify-incompatible-reset":
		return runVolumeRuntimeVerifyReset(ctx, c, volume_opfs.ResetReasonIncompatible)
	case "volume-runtime-verify-unknown-reset":
		return runVolumeRuntimeVerifyReset(ctx, c, volume_opfs.ResetReasonUnknown)
	case "volume-runtime-delete-verify":
		return runVolumeRuntimeDeleteVerify(ctx, c)
	case "world-init-unixfs":
		return runWorldInitUnixFS(ctx, c)
	case "world-large-unixfs-upload":
		return runWorldLargeUnixFSUpload(ctx, c)
	case "world-resource-large-unixfs-upload":
		return runWorldResourceLargeUnixFSUpload(ctx, c)
	default:
		return errors.Errorf("unknown scenario %q", c.scenario)
	}
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

	totalSize := c.iterations
	if totalSize <= 0 {
		totalSize = 64 * 1024 * 1024
	}
	entriesCount := c.batch
	if entriesCount <= 0 {
		entriesCount = 96
	}
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
	if err := e.Put(ctx, entries); err != nil {
		release()
		return errors.Wrap(err, "put large block batch")
	}
	if err := verifyLargeBlockSamples(ctx, e, totalSize, entriesCount); err != nil {
		release()
		return err
	}
	release()

	e, release, err = openBlockEngine(ctx, c)
	if err != nil {
		return errors.Wrap(err, "reopen large block engine")
	}
	defer release()
	return verifyLargeBlockSamples(ctx, e, totalSize, entriesCount)
}

func verifyLargeBlockSamples(ctx context.Context, e *blockshard.Engine, totalSize, entriesCount int) error {
	baseSize := totalSize / entriesCount
	remainder := totalSize % entriesCount
	for _, i := range []int{0, entriesCount / 2, entriesCount - 1} {
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
		for _, idx := range []int{0, 1, 4095, 4096, size / 2, size - 2, size - 1} {
			if idx < 0 || idx >= len(want) {
				continue
			}
			if got[idx] != want[idx] {
				return errors.Errorf("large block %d byte[%d]=%d want=%d", i, idx, got[idx], want[idx])
			}
		}
	}
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
	err = blockshard.ExecuteCompaction(shard, plan)
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
	err = blockshard.ExecuteCompaction(shard, plan)
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
	for i := 0; i < blockshard.DefaultL0Trigger; i++ {
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
	for i := 0; i < blockshard.DefaultL0Trigger; i++ {
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

func runBlockReader(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventSub(c)
	defer events.Close()
	postReady(c)

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
			for j := 0; j < c.batch; j++ {
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
	if found == 0 {
		for w := 0; w < c.workers; w++ {
			for i := 0; i < c.iterations; i++ {
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
	if found > 0 {
		return nil
	}
	return errors.New("reader found no concurrently written blocks")
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

func openBlockEngine(ctx context.Context, c *config) (*blockshard.Engine, func(), error) {
	dir, err := openTestDirectory(c.root, []string{"blocks"})
	if err != nil {
		return nil, nil, err
	}
	settings := blockshard.DefaultSettings()
	settings.ShardCount = c.shards
	settings.AsyncIO = true
	e, err := blockshard.NewEngineWithSettings(ctx, dir, c.root+"/blocks", settings)
	if err != nil {
		return nil, nil, err
	}
	return e, e.Close, nil
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
	for i := 0; i < limit; i++ {
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
	if stats.GetTotalBytes() != uint64(len(data)) {
		return errors.Errorf("volume total bytes=%d want=%d", stats.GetTotalBytes(), len(data))
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

func runVolumeRuntimeSeedCurrentV1(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"volume"})
	if err != nil {
		return err
	}
	for _, name := range []string{"blocks", "meta", "gc"} {
		if _, err := opfs.GetDirectory(dir, name, true); err != nil {
			return errors.Wrapf(err, "create v1 %s directory", name)
		}
	}
	return opfs.WriteFile(dir, "legacy-only", []byte("current-v1"))
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
	for key, val := range entry.Data {
		data[key] = val
	}
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
	return verifyDeterministicFSFile(ctx, largeFile, totalSize, 0)
}

func runWorldResourceLargeUnixFSUpload(ctx context.Context, c *config) (retErr error) {
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

	resClient, cleanup, err := openFSHandleResourceClient(ctx, handle, ws, "files")
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
	if err := uploadDeterministicResourceFile(ctx, rootSvc, "large-video.mp4", totalSize, 0); err != nil {
		return err
	}

	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "large-video.mp4",
	})
	if err != nil {
		return errors.Wrap(err, "lookup uploaded resource file")
	}
	fileRef := resClient.CreateResourceReference(fileResp.GetResourceId())
	defer fileRef.Release()

	fileClient, err := fileRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file client")
	}
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	return verifyDeterministicResourceFile(ctx, fileSvc, totalSize, 0)
}

func openFSHandleResourceClient(
	ctx context.Context,
	handle *unixfs_sdk.FSHandle,
	ws world.WorldState,
	objKey string,
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

	rootResource := resource_unixfs.NewFSHandleObjectResource(
		handle,
		nil,
		ws,
		objKey,
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
	)
	resourceSrv := resource_server.NewResourceServer(rootResource.GetMux())
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
	}
	resp, err := strm.CloseAndRecv()
	if err != nil {
		return errors.Wrap(err, "close UploadTree stream")
	}
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
) error {
	sizeResp, err := fileSvc.GetSize(ctx, &s4wave_unixfs.HandleGetSizeRequest{})
	if err != nil {
		return errors.Wrap(err, "get uploaded resource file size")
	}
	if sizeResp.GetSize() != uint64(totalSize) {
		return errors.Errorf("resource file size=%d want=%d", sizeResp.GetSize(), totalSize)
	}
	for _, offset := range []int{0, totalSize / 2, max(0, totalSize-4096)} {
		wantLen := min(4096, totalSize-offset)
		resp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(wantLen),
		})
		if err != nil {
			return errors.Wrapf(err, "read uploaded resource file offset=%d", offset)
		}
		if len(resp.GetData()) != wantLen {
			return errors.Errorf("resource file offset=%d read=%d want=%d", offset, len(resp.GetData()), wantLen)
		}
		want := deterministicLargeWindow(offset, wantLen, salt)
		if !bytes.Equal(resp.GetData(), want) {
			return errors.Errorf("resource file offset=%d data mismatch", offset)
		}
	}
	return nil
}

func verifyDeterministicFSFile(
	ctx context.Context,
	handle *unixfs_sdk.FSHandle,
	totalSize int,
	salt int,
) error {
	size, err := handle.GetSize(ctx)
	if err != nil {
		return errors.Wrap(err, "get large file size")
	}
	if size != uint64(totalSize) {
		return errors.Errorf("large file size=%d want=%d", size, totalSize)
	}
	for _, offset := range []int{0, totalSize / 2, max(0, totalSize-4096)} {
		wantLen := min(4096, totalSize-offset)
		got := make([]byte, wantLen)
		n, err := handle.ReadAt(ctx, int64(offset), got)
		if err != nil && err != io.EOF {
			return errors.Wrapf(err, "read large file offset=%d", offset)
		}
		if int(n) != wantLen {
			return errors.Errorf("large file offset=%d read=%d want=%d", offset, n, wantLen)
		}
		want := deterministicLargeWindow(offset, wantLen, salt)
		if !bytes.Equal(got, want) {
			return errors.Errorf("large file offset=%d data mismatch", offset)
		}
	}
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
	dir, err := openTestDirectory(c.root, []string{"meta"})
	if err != nil {
		return nil, err
	}
	shard, err := metashard.NewMetaShard(dir, c.root+"/meta", 4096, nil)
	if err != nil {
		return nil, err
	}
	return metashard.NewMetaStore(shard), nil
}

func openVolume(ctx context.Context, c *config) (*volume_opfs.Opfs, error) {
	return openVolumeWithLogger(ctx, c, logrus.NewEntry(logrus.New()))
}

func openVolumeWithLogger(ctx context.Context, c *config, le *logrus.Entry) (*volume_opfs.Opfs, error) {
	return volume_opfs.NewOpfs(ctx, le, &volume_opfs.Config{
		RootPath:        c.root + "/volume",
		LockPrefix:      c.root + "/volume",
		StoreConfig:     &store_kvtx.Config{},
		BlockShardCount: uint32(c.shards),
		AsyncIo:         true,
		ResetPolicy:     "automatic",
	})
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
	r := newDeterministicLargeReader(offset+size, salt)
	var scratch [32 * 1024]byte
	for remaining := offset; remaining > 0; {
		n := min(remaining, len(scratch))
		if _, err := io.ReadFull(r, scratch[:n]); err != nil {
			return nil
		}
		remaining -= n
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil
	}
	return buf
}

func fillDeterministicLargeBytes(buf []byte, offset int, salt int) {
	x := uint32(0x9e3779b9) ^ (uint32(salt) * uint32(0x85ebca6b))
	for i := 0; i < offset; i++ {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
	}
	for i := range buf {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		buf[i] = byte(x) + byte(offset+i)
	}
}

type deterministicLargeReader struct {
	remaining int
	offset    int
	x         uint32
}

func newDeterministicLargeReader(size int, salt int) *deterministicLargeReader {
	return &deterministicLargeReader{
		remaining: size,
		x:         uint32(0x9e3779b9) ^ (uint32(salt) * uint32(0x85ebca6b)),
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
		r.x ^= r.x << 13
		r.x ^= r.x >> 17
		r.x ^= r.x << 5
		p[i] = byte(r.x) + byte(r.offset)
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

func postResult(c *config, dur time.Duration, err error) {
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "result")
	if c != nil {
		obj.Set("scenario", c.scenario)
		obj.Set("worker", c.worker)
	}
	obj.Set("durationMs", dur.Milliseconds())
	if err != nil {
		obj.Set("ok", false)
		obj.Set("error", err.Error())
	} else {
		obj.Set("ok", true)
	}
	js.Global().Call("postMessage", obj)
}

type neverReader struct{}

func (neverReader) Read(p []byte) (int, error) {
	ch := make(chan struct{})
	<-ch
	return 0, nil
}
