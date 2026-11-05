package world_block_engine_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

func BenchmarkWorldEngineOneWriterBaseline(b *testing.B) {
	// Open the baseline World engine for the timed writer.
	ctx := b.Context()
	eng, cleanup := setupWorldEngineCoordBaseline(b, ctx)
	defer cleanup()

	// Initialize the World commit measurements before timing writes.
	var totalCommitLatency time.Duration
	var rootSeqno uint64
	var parityHash uint32
	b.ReportAllocs()
	b.ResetTimer()

	// Measure each World object commit and its resulting sequence.
	for i := 0; i < b.N; i++ {
		// Prepare one World object in a write transaction.
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		key := "coord-baseline/object/" + strconv.Itoa(i)
		{
			createdObject, err := tx.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "coord-baseline-bucket"})
			world.ReleaseObjectState(createdObject)
			if err != nil {
				tx.Discard()
				b.Fatal(err)
			}
		}

		// Measure the World commit latency and capture the published sequence.
		start := time.Now()
		if err := tx.Commit(ctx); err != nil {
			b.Fatal(err)
		}
		totalCommitLatency += time.Since(start)
		rootSeqno, err = eng.GetSeqno(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Read the completed World contents outside the timed writer loop.
	b.StopTimer()
	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	parityHash, err = worldEngineBaselineHash(ctx, readTx, "coord-baseline/object/")
	readTx.Discard()
	if err != nil {
		b.Fatal(err)
	}

	// Report World publication counts, commit latency, and content parity.
	b.ReportMetric(1, "writers/op")
	b.ReportMetric(1, "world_head_writes/op")
	b.ReportMetric(1, "root_publications/op")
	if b.N != 0 {
		b.ReportMetric(float64(totalCommitLatency.Nanoseconds())/float64(b.N), "write_commit_latency_ns/op")
	}
	b.ReportMetric(float64(rootSeqno), "root_seqno")
	b.ReportMetric(float64(parityHash), "parity_hash")
}

func setupWorldEngineCoordBaseline(t testing.TB, ctx context.Context) (world_block_engine.Engine, func()) {
	// Build the storage testbed and register the World engine factory.
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Configure encryption for the baseline World and its saved head.
	engineID := "coord-baseline-world-engine"
	objectStoreID := "coord-baseline-world-store"
	encKey := make([]byte, 32)
	blake3.DeriveKey("spacewave/test/world-coord-baseline", []byte(objectStoreID), encKey)
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	initWorldRef := &bucket.ObjectRef{
		BucketId:      tb.BucketId,
		TransformConf: transformConf,
	}

	// Start the World engine with its encrypted storage configuration.
	eng, worldCtrlRef, err := world.StartEngine(
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
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Register the mock World operations on the controller bus.
	opc := world.NewLookupOpController("coord-baseline-world-engine-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Return the World engine with cleanup for its retained resources.
	return eng, func() {
		relOpc()
		worldCtrlRef.Release()
		tb.Release()
	}
}

func worldEngineBaselineHash(ctx context.Context, ws world.WorldState, prefix string) (uint32, error) {
	// Open the World object iterator and initialize the parity hash.
	h := fnv.New32a()
	iter := ws.IterateObjects(ctx, prefix, false)
	defer iter.Close()

	// Hash the stored World objects and their revisions.
	for iter.Next() {
		key := iter.Key()
		obj, found, err := ws.GetObject(ctx, key)
		if err != nil {
			world.ReleaseObjectState(obj)
			return 0, err
		}
		if !found {
			continue
		}
		ref, rev, err := obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			return 0, err
		}
		refString := ""
		if ref != nil {
			refString = ref.MarshalString()
		}
		fmt.Fprintf(h, "%s;%d;%s;", key, rev, refString)
	}

	// Require the World object iterator to finish without an error.
	if err := iter.Err(); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}
