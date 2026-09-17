package world_block_engine_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/block"
	block_byteslice "github.com/s4wave/spacewave/db/block/byteslice"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// baselineSample records one measured workload sample.
type baselineSample struct {
	commits uint64
	elapsed time.Duration
}

// TestWorldCommitBatchingBaseline measures the matched complete-update and
// commit-only workloads through the native world engine testbed on a synced
// Bolt volume. Physical commit counts come from the vendored bbolt fork's
// CommitCounter: one increment per physical write transaction.
func TestWorldCommitBatchingBaseline(t *testing.T) {
	ctx := context.Background()
	eng, bdb, cleanup := setupWorldEngineBaseline(t, ctx)
	defer cleanup()

	sampleIdx := 0
	measure := func(name string, run func() error) []baselineSample {
		var out []baselineSample
		for range 10 {
			sampleIdx++
			before := bdb.CommitCounter()
			start := time.Now()
			if err := run(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			out = append(out, baselineSample{bdb.CommitCounter() - before, time.Since(start)})
		}
		for _, s := range out {
			t.Logf("%s: commits=%d elapsed=%s", name, s.commits, s.elapsed)
		}
		return out
	}

	// Seed 32 objects before any measured sample.
	seedTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 32 {
		key := "baseline/seed/" + strconv.Itoa(i)
		created, _, err := world.CreateWorldObject(ctx, seedTx, key, func(bcs *block.Cursor) error {
			body := []byte("seed-" + key)
			bcs.SetBlock(block_byteslice.NewByteSlice(&body), true)
			return nil
		})
		world.ReleaseObjectState(created)
		if err != nil {
			seedTx.Discard()
			t.Fatal(err)
		}
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Warm the commit-only path once so the first write transaction's lazy
	// initialization does not pollute the measured counts.
	warmMutateTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := world.AccessWorldObject(ctx, warmMutateTx, "baseline/seed/1", true, func(bcs *block.Cursor) error {
		body := []byte("warm-mutate")
		bcs.SetBlock(block_byteslice.NewByteSlice(&body), true)
		return nil
	}); err != nil {
		warmMutateTx.Discard()
		t.Fatal(err)
	}
	if err := warmMutateTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Commit-only: one transaction mutating one existing object body.
	commitOnly := measure("commit-only", func() error {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		if _, _, err := world.AccessWorldObject(ctx, tx, "baseline/seed/0", true, func(bcs *block.Cursor) error {
			body := []byte("mutated-" + strconv.Itoa(sampleIdx))
			bcs.SetBlock(block_byteslice.NewByteSlice(&body), true)
			return nil
		}); err != nil {
			tx.Discard()
			return err
		}
		return tx.Commit(ctx)
	})
	for _, s := range commitOnly {
		if s.commits != commitOnly[0].commits {
			t.Fatalf("commit-only commit count varied: %v", commitOnly)
		}
	}

	// Complete update: one transaction creating 32 objects.
	complete := measure("complete-update", func() error {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		for i := range 32 {
			key := "baseline/complete/" + strconv.Itoa(sampleIdx) + "/" + strconv.Itoa(i)
			created, _, err := world.CreateWorldObject(ctx, tx, key, func(bcs *block.Cursor) error {
				body := []byte("body-" + key)
				bcs.SetBlock(block_byteslice.NewByteSlice(&body), true)
				return nil
			})
			world.ReleaseObjectState(created)
			if err != nil {
				tx.Discard()
				return err
			}
		}
		return tx.Commit(ctx)
	})
	for _, s := range complete {
		if s.commits != complete[0].commits {
			t.Fatalf("complete-update commit count varied: %v", complete)
		}
	}

	t.Logf("commit-only: %d commits, median %v", commitOnly[0].commits, medianElapsed(commitOnly))
	t.Logf("complete-update: %d commits, median %v", complete[0].commits, medianElapsed(complete))

	// Parity hash over the complete- prefix keys.
	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	h := fnv.New32a()
	iter := readTx.IterateObjects(ctx, "baseline/complete/", false)
	defer iter.Close()
	for iter.Next() {
		key := iter.Key()
		obj, found, err := readTx.GetObject(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			continue
		}
		ref, rev, err := obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		refString := ""
		if ref != nil {
			refString = ref.MarshalString()
		}
		_, _ = fmt.Fprintf(h, "%s;%d;%s;", key, rev, refString)
	}
	if err := iter.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("parity_hash=%d", h.Sum32())
}

// setupWorldEngineBaseline constructs the synced native world engine testbed
// on a fresh Bolt volume and returns the engine, its bolt DB, and cleanup.
func setupWorldEngineBaseline(t testing.TB, ctx context.Context) (world.Engine, *bdb.DB, func()) {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	boltPath := filepath.Join(t.TempDir(), "world-commit-batching-baseline.bolt")
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_bolt.Config{
		Path: boltPath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	engineID := "world-commit-batching-baseline-engine"
	objectStoreID := "world-commit-batching-baseline-store"
	encKey := make([]byte, 32)
	blake3.DeriveKey("spacewave/test/world-commit-batching-baseline", []byte(objectStoreID), encKey)
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}
	initWorldRef := &bucket.ObjectRef{
		BucketId:      tb.BucketId,
		TransformConf: transformConf,
	}

	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			tb.Volume.GetID(),
			tb.BucketId,
			objectStoreID,
			initWorldRef,
			transformConf,
			true,
		),
	)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	opc := world.NewLookupOpController(engineID+"-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		worldCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		relOpc()
		worldCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	boltDB := volume_bolt.GetBoltDB(tb.Volume)
	if boltDB == nil {
		relOpc()
		worldCtrlRef.Release()
		tb.Release()
		t.Fatal("testbed volume is not bolt-backed")
	}

	return eng, boltDB, func() {
		relOpc()
		worldCtrlRef.Release()
		tb.Release()
	}
}

func medianElapsed(samples []baselineSample) time.Duration {
	times := make([]time.Duration, 0, len(samples))
	for _, s := range samples {
		times = append(times, s.elapsed)
	}
	slices.Sort(times)
	return times[len(times)/2]
}
