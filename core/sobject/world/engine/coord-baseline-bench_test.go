package sobject_world_engine_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

func BenchmarkSharedObjectWorldEngineFinalizationBaseline(b *testing.B) {
	ctx := b.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		b.Fatal(err)
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer provCtrlRef.Release()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, "coord-baseline-account", false, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer provAccRef.Release()

	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		b.Fatal(err)
	}
	createdSoRef, err := soProv.CreateSharedObject(ctx, "coord-baseline-shared-object", &sobject.SharedObjectMeta{
		BodyType: "coord-baseline",
	}, "", "")
	if err != nil {
		b.Fatal(err)
	}

	engineID := "coord-baseline-shared-object-engine"
	worldCtrl, _, worldCtrlRef, err := sobject_world_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		sobject_world_engine.NewConfig(engineID, createdSoRef),
		nil,
	)
	if err != nil {
		b.Fatal(err)
	}
	defer worldCtrlRef.Release()

	opc := world.NewLookupOpController("coord-baseline-shared-object-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer relOpc()

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		b.Fatal(err)
	}

	var totalFinalizationLatency time.Duration
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
		totalFinalizationLatency += time.Since(start)
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
	parityHash, err = sharedObjectBaselineHash(ctx, readTx, "coord-baseline/object/")
	readTx.Discard()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(1, "writers/op")
	b.ReportMetric(1, "shared_object_ops/op")
	b.ReportMetric(1, "local_finalization_waits/op")
	b.ReportMetric(1, "root_publications/op")
	if b.N != 0 {
		b.ReportMetric(float64(totalFinalizationLatency.Nanoseconds())/float64(b.N), "finalization_latency_ns/op")
	}
	b.ReportMetric(float64(rootSeqno), "root_seqno")
	b.ReportMetric(float64(parityHash), "parity_hash")
}

func sharedObjectBaselineHash(ctx context.Context, ws world.WorldState, prefix string) (uint32, error) {
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
