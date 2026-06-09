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
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

func TestWorldEngineStaleHeadPublicationRejectsOpenWriter(t *testing.T) {
	ctx := context.Background()
	eng, cleanup := setupWorldEngineCoordBaseline(t, ctx)
	defer cleanup()
	rootOwner, ok := eng.(interface {
		GetRootRef() *bucket.ObjectRef
		SetRootRef(context.Context, *bucket.ObjectRef) error
	})
	if !ok {
		t.Fatal("world engine does not expose root publication controls")
	}

	initialTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initialTx.CreateObject(ctx, "coord-baseline/stale-head/initial", &bucket.ObjectRef{BucketId: "coord-baseline-bucket"}); err != nil {
		initialTx.Discard()
		t.Fatal(err)
	}
	if err := initialTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	firstRoot := rootOwner.GetRootRef()

	freshTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := freshTx.CreateObject(ctx, "coord-baseline/stale-head/fresh", &bucket.ObjectRef{BucketId: "coord-baseline-bucket"}); err != nil {
		freshTx.Discard()
		t.Fatal(err)
	}
	if err := freshTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	staleTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer staleTx.Discard()
	if _, err := staleTx.CreateObject(ctx, "coord-baseline/stale-head/rejected", &bucket.ObjectRef{BucketId: "coord-baseline-bucket"}); err != nil {
		t.Fatal(err)
	}
	if err := rootOwner.SetRootRef(ctx, firstRoot); err != nil {
		t.Fatal(err)
	}
	if err := staleTx.Commit(ctx); err != tx.ErrDiscarded {
		t.Fatalf("expected stale writer to reject after root publication, got %v", err)
	}
}

func BenchmarkWorldEngineOneWriterBaseline(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng, cleanup := setupWorldEngineCoordBaseline(b, ctx)
	defer cleanup()

	var totalCommitLatency time.Duration
	var rootSeqno uint64
	var parityHash uint32
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		key := "coord-baseline/object/" + strconv.Itoa(i)
		if _, err := tx.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "coord-baseline-bucket"}); err != nil {
			tx.Discard()
			b.Fatal(err)
		}
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
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

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
		t.Fatal(err)
	}

	opc := world.NewLookupOpController("coord-baseline-world-engine-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err)
	}

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return eng, func() {
		relOpc()
		worldCtrlRef.Release()
		tb.Release()
	}
}

func worldEngineBaselineHash(ctx context.Context, ws world.WorldState, prefix string) (uint32, error) {
	h := fnv.New32a()
	iter := ws.IterateObjects(ctx, prefix, false)
	defer iter.Close()
	for iter.Next() {
		key := iter.Key()
		obj, found, err := ws.GetObject(ctx, key)
		if err != nil {
			return 0, err
		}
		if !found {
			continue
		}
		ref, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			return 0, err
		}
		refString := ""
		if ref != nil {
			refString = ref.MarshalString()
		}
		fmt.Fprintf(h, "%s;%d;%s;", key, rev, refString)
	}
	if err := iter.Err(); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}
