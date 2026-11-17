//go:build unix

package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/s4wave/spacewave/db/packfile/writer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	recovery "github.com/s4wave/spacewave/db/world/recovery"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestRecoveryScale counts synthetic captures of n and 10n objects, each with
// two versions, spread over SQLite, a local pack log and retained remote
// packs, and logs one line per size. It runs only when RECOVERY_SCALE_N is
// set, keeping its stores under RECOVERY_SCALE_DIR. Each count runs in a
// child process, so its peak RSS excludes the capture's construction.
func TestRecoveryScale(t *testing.T) {
	// A child counts one capture and exits.
	if manifest := os.Getenv("RECOVERY_SCALE_CHILD"); manifest != "" {
		data, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		source := &recovery.Source{}
		if err := source.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		if err := runCount(t.Context(), io.Discard, source, os.Getenv("RECOVERY_SCALE_SCRATCH"), countOptions()); err != nil {
			t.Fatal(err)
		}
		return
	}
	n, _ := strconv.Atoi(os.Getenv("RECOVERY_SCALE_N"))
	if n <= 0 {
		t.Skip("set RECOVERY_SCALE_N to measure")
	}
	base := t.TempDir()
	if dir := os.Getenv("RECOVERY_SCALE_DIR"); dir != "" {
		var err error
		if base, err = os.MkdirTemp(dir, "scale-"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(base) })
	}
	t.Logf("scale stores in %s", base)

	for _, size := range []int{n, 10 * n} {
		// Build the capture, then count it in a child process.
		dir := filepath.Join(base, strconv.Itoa(size))
		source, sourceBytes := makeScaleCapture(t, dir, size)
		stamp := treeStamp(t, []string{source.GetPath(), source.GetPacks()[0].GetPath(), source.GetPacks()[1].GetPath()})
		scratch := filepath.Join(dir, "scratch.sqlite")
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRecoveryScale$", "-test.timeout=60m")
		cmd.Env = append(os.Environ(), "RECOVERY_SCALE_CHILD="+writeManifest(t, source), "RECOVERY_SCALE_SCRATCH="+scratch)
		start := time.Now()
		out, err := cmd.CombinedOutput()
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("count of %d objects: %v\n%s", size, err, out)
		}
		usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
		if !ok {
			t.Fatal("child resource usage is unavailable")
		}
		result, err := recovery.ReadResult(scratch)
		if err != nil {
			t.Fatal(err)
		}
		var scratchBytes int64
		for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
			if info, err := os.Stat(scratch + suffix); err == nil {
				scratchBytes += info.Size()
			}
		}
		count := result.GetCounts()
		writes := changedPaths(t, stamp)
		t.Logf("scale objects=%d phase=%s source_bytes=%d blocks=%d block_bytes=%d locations=%d versions=%d pack_files=%d peak_rss_kib=%d scratch_bytes=%d duration=%s source_writes=%d in_blocks=%d out_blocks=%d",
			size, result.GetPhase(), sourceBytes, count.GetPhysicalBlocks(), count.GetPhysicalBytes(), count.GetPhysicalLocations(), count.GetObjectVersions(), count.GetPackFiles(), maxRSSKiB(usage), scratchBytes, elapsed.Round(time.Millisecond), writes, usage.Inblock, usage.Oublock)
		if writes != 0 || result.GetPhase() != recovery.Phase_PHASE_COUNTED || count.GetObjectVersions() != uint64(2*size) { //nolint:gosec // positive.
			t.Fatalf("scale count of %d objects: phase %s, %d versions, %d source writes", size, result.GetPhase(), count.GetObjectVersions(), writes)
		}
	}
}

// maxRSSKiB returns the child's peak resident set in KiB; Darwin reports bytes.
func maxRSSKiB(usage *syscall.Rusage) int64 {
	if runtime.GOOS == "darwin" {
		return usage.Maxrss / 1024
	}
	return usage.Maxrss
}

// makeScaleCapture builds size typed objects, replaces each object's version
// once, and writes every block to one of three tiers in turn. It returns the
// declared capture and the bytes its sources hold.
func makeScaleCapture(t *testing.T, dir string, size int) (*recovery.Source, int64) {
	t.Helper()
	ctx := t.Context()
	store := store_inmem.NewStore()
	keygen := store_kvkey.NewDefaultKVKey()
	blocks, err := volume_kvtx.NewVolume(ctx, "synthetic-scale", keygen, store, nil, false, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocks.Close()
	put := func(value block.Block) *block.BlockRef {
		t.Helper()
		data, err := value.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		ref, _, err := blocks.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	body := func(label string) *bucket.ObjectRef {
		value := &blob.Blob{TotalSize: uint64(len(label)), RawData: []byte(label)}
		return &bucket.ObjectRef{RootRef: put(&kvtx_block.KeyValueStore{ImplType: kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL, IavlRoot: &iavl.Node{Key: []byte("item"), Size: 1, ValueBlob: value}})}
	}

	// Write the first version of every object, then replace each once.
	initial := world_block.NewWorld(world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV, false)
	initial.ObjectKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL)
	initial.GraphKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL)
	engine, err := world_block.OpenSnapshot(ctx, logrus.NewEntry(logrus.New()), blocks, &bucket.ObjectRef{RootRef: put(initial), BucketId: "synthetic-scale"})
	if err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(engine, true)
	for i := range size {
		key := "object-" + strconv.Itoa(i)
		object, err := ws.CreateObject(ctx, key, body(fmt.Sprintf("first version of %s", key)))
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
		if err := world_types.SetObjectType(ctx, ws, key, "kv/store"); err != nil {
			t.Fatal(err)
		}
	}
	first := engine.GetRootRef().CloneVT()
	for i := range size {
		key := "object-" + strconv.Itoa(i)
		object, _, err := ws.GetObject(ctx, key)
		if err != nil || object == nil {
			t.Fatal("scale object missing", err)
		}
		_, err = object.SetRootRef(ctx, body(fmt.Sprintf("second version of %s", key)))
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
	}
	current := engine.GetRootRef().CloneVT()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Spread the blocks over SQLite, the local log and remote packs in turn.
	local, remote := filepath.Join(dir, "packahead"), filepath.Join(dir, "remote")
	if err := os.MkdirAll(remote, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "capture.sqlite")
	db, err := sql.Open(sqliteDriver(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE volume (key BLOB PRIMARY KEY,value BLOB NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	sqlTx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlTx.Exec("INSERT INTO volume VALUES (?,?)", keygen.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(9)); err != nil {
		t.Fatal(err)
	}
	log, err := packahead.Open(ctx, local, packahead.Options{})
	if err != nil {
		t.Fatal(err)
	}
	type remoteBlock struct {
		h    *hash.Hash
		data []byte
	}
	var batch []packahead.Block
	var pending []remoteBlock
	packs := 0
	flushRemote := func() {
		file, err := os.Create(filepath.Join(remote, fmt.Sprintf("%06d.kvf", packs)))
		if err != nil {
			t.Fatal(err)
		}
		next := pending
		if _, err := writer.PackBlocks(file, func() (*hash.Hash, *block.StoredBlock, error) {
			if len(next) == 0 {
				return nil, nil, nil
			}
			b := next[0]
			next = next[1:]
			return b.h, &block.StoredBlock{Data: b.data}, nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		packs++
		pending = nil
	}
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	i := 0
	err = read.ScanPrefix(ctx, keygen.GetBlockFullPrefix(), func(key, value []byte) error {
		defer func() { i++ }()
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(key[len(keygen.GetBlockFullPrefix()):]); err != nil {
			return err
		}
		switch i % 3 {
		case 0:
			_, err := sqlTx.Exec("INSERT INTO volume VALUES (?,?)", key, value)
			return err
		case 1:
			encoded, err := block.EncodeBlockObject(value, nil, false)
			if err != nil {
				return err
			}
			batch = append(batch, packahead.Block{Key: packfile.BlockKey(ref.GetHash()), Value: encoded})
			if len(batch) == 256 {
				err = log.Append(ctx, batch)
				batch = nil
			}
			return err
		default:
			pending = append(pending, remoteBlock{ref.GetHash(), append([]byte(nil), value...)})
			if uint64(len(pending)) == writer.DefaultMaxBlocksPerPack {
				flushRemote()
			}
			return nil
		}
	})
	read.Discard()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		if err := log.Append(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	log.Close()
	if len(pending) != 0 {
		flushRemote()
	}

	// Name the current World's root, as the Volume's root table does.
	rootKey := append([]byte{2, 'R'}, binary.AppendUvarint(nil, uint64(len("synthetic-scale")))...)
	rootKey = append(rootKey, []byte("synthetic-scale/current")...)
	rootValue, err := (&block_gc.RootEntry{Ref: block_gc.BlockKey(current.GetRootRef())}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlTx.Exec("INSERT INTO volume VALUES (?,?)", rootKey, rootValue); err != nil {
		t.Fatal(err)
	}
	if err := sqlTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Sum the source bytes the count reads.
	var total int64
	for _, p := range []string{path, local, remote} {
		err := filepath.WalkDir(p, func(_ string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			info, err := entry.Info()
			if err == nil {
				total += info.Size()
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	source := &recovery.Source{CaptureId: "synthetic-scale", Path: path, Tables: []string{"volume"}, Worlds: []*recovery.WorldInput{
		{SpaceId: "synthetic-scale", Ref: first}, {SpaceId: "synthetic-scale", Ref: current, Current: true},
	}, Packs: []*recovery.PackSource{{Path: local, Tier: recovery.PackTier_PACK_TIER_LOCAL}, {Path: remote, Tier: recovery.PackTier_PACK_TIER_REMOTE}}}
	return source, total
}
