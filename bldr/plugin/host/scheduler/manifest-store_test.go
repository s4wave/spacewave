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
	// Open a World testbed for the scheduler controller.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Read the plugin-host object before any plugin demands it.
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

	// The manifest store must not exist yet.
	objectState, exists, err := state.GetObject(ctx, objectKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatalf("read manifest store before plugin demand: %v", err)
	}
	if exists {
		t.Fatal("manifest store exists before the first plugin demand")
	}

	// A canceled context fails the store initialization.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := controller.ensureManifestStore(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled manifest store initialization = %v, want context.Canceled", err)
	}

	// Eight concurrent demands all succeed on the shared store.
	const demandCount = 8
	start := make(chan struct{})
	errs := make(chan error, demandCount)
	var wg sync.WaitGroup

	// Start every demand behind the same gate.
	for range demandCount {
		wg.Go(func() {
			<-start
			errs <- controller.ensureManifestStore(ctx)
		})
	}

	// Release the gate and collect every demand result.
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ensure manifest store: %v", err)
		}
	}

	// The store exists with the manifest store type after the demands.
	if err := bldr_manifest_world.CheckManifestStoreType(ctx, state, objectKey); err != nil {
		t.Fatalf("manifest store after first plugin demand: %v", err)
	}
}
