package sobject_world_engine_test

import (
	"context"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

// TestWorldEngineController starts the engine on a local SharedObject, runs the
// World engine suite through the bus, checkpoints the owner's stable history,
// and checks a remounted engine still holds every object.
func TestWorldEngineController(t *testing.T) {
	// Bound the test, allowing more time under js.
	timeout := 10 * time.Second
	if runtime.GOOS == "js" {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	// Build the testbed with the engine and local provider factories.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	le := tb.Logger

	// Start the local provider.
	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	// Access a provider account and its SharedObject feature.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, "test-account", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the SharedObject.
	createdSoRef, err := wsProv.CreateSharedObject(ctx, "test-shared-object", &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the engine, and serve the mock operations to it.
	engineID := "test-world-engine"
	startEngine := func() (*sobject_world_engine.Controller, directive.Reference) {
		engineConf := sobject_world_engine.NewConfig(engineID, createdSoRef)
		worldCtrl, _, worldCtrlRef, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus, engineConf, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		return worldCtrl, worldCtrlRef
	}
	worldCtrl, worldCtrlRef := startEngine()
	defer worldCtrlRef.Release()
	opc := world.NewLookupOpController("test-world-engine-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relOpc()

	// Open and discard a write transaction.
	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// Run the World engine suite through the bus.
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	if err := world_mock.TestWorldEngine(ctx, le, busEngine); err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	// Read the World block.
	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		_, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write past the checkpoint threshold. As the only roster member, the
	// owner checkpoints its stable history and trims the operation set.
	for i := range sobject.MinCheckpointOperations {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		objectState, err := tx.CreateObject(ctx, "checkpoint-object-"+strconv.Itoa(i), nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		world.ReleaseObjectState(objectState)
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Wait for the checkpoint to trim the operation set.
	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, createdSoRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer soRef.Release()
	soState, relSoState, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relSoState()
	_, err = soState.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		return checkpointTrimmed(ctx, snap)
	}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Remount the engine.
	worldCtrlRef.Release()
	worldCtrl, worldCtrlRef = startEngine()
	defer worldCtrlRef.Release()
	eng, err = worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The suite's object and the objects below the checkpoint remain.
	engTx, err = eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer engTx.Discard()
	last := "checkpoint-object-" + strconv.Itoa(sobject.MinCheckpointOperations-1)
	for _, key := range []string{"test-object", last} {
		objectState, found, err := engTx.GetObject(ctx, key)
		world.ReleaseObjectState(objectState)
		if err != nil || !found {
			t.Fatalf("object %s after remounting: found %v, %v", key, found, err)
		}
	}
}

// checkpointTrimmed reports whether snap holds a checkpoint above genesis and
// fewer operations than one checkpoint covers.
func checkpointTrimmed(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (bool, error) {
	// Wait for a state.
	if snap == nil {
		return false, nil
	}

	// Compare the checkpoint height and the held operations.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return false, err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return false, err
	}
	return checkpoint.GetHeight() != 0 && set.Len() < sobject.MinCheckpointOperations, nil
}
