package plugin_host_scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestControllerEnsuresManifestStoreOnPluginDemand(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	const objectKey = "plugin-host"
	controller := NewController(logrus.NewEntry(logrus.New()), tb.Bus, NewConfig(
		"",
		tb.EngineID,
		objectKey,
		tb.EngineVolumeID,
		tb.Volume.GetPeerID().String(),
		true,
		false,
		false,
	))
	state := world.NewEngineWorldState(tb.BusEngine, false)
	_, exists, err := state.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("read manifest store before plugin demand: %v", err)
	}
	if exists {
		t.Fatal("manifest store exists before the first plugin demand")
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := controller.ensureManifestStore(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled manifest store initialization = %v, want context.Canceled", err)
	}

	const demandCount = 8
	start := make(chan struct{})
	errs := make(chan error, demandCount)
	var wg sync.WaitGroup
	for range demandCount {
		wg.Go(func() {
			<-start
			errs <- controller.ensureManifestStore(ctx)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ensure manifest store: %v", err)
		}
	}

	if err := bldr_manifest_world.CheckManifestStoreType(ctx, state, objectKey); err != nil {
		t.Fatalf("manifest store after first plugin demand: %v", err)
	}
}
