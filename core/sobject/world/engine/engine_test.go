package sobject_world_engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/zeebo/blake3"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/testbed"
)

// TestWorldEngineController tests constructing the engine controller, looking up
// the engine on the bus, & running some basic queries.
func TestWorldEngineController(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	le := tb.Logger
	vol := tb.Volume
	peerID := vol.GetPeerID()
	// volumeID := vol.GetID()
	// bucketID := tb.EngineBucketID
	// engineID := tb.EngineID

	// Create the provider controller
	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	// Check LookupProvider works.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	provInfo := prov.GetProviderInfo()
	provRef.Release()
	_ = provInfo

	// Acquire a provider account handle.
	accountID := "test-account"
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	// Get the provider account feature.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the shared object.
	sobjectID := "test-shared-object"
	createdSoRef, err := wsProv.CreateSharedObject(ctx, sobjectID, &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = createdSoRef

	tb.Logger.Infof(
		"created shared object with provider %s id %s",
		createdSoRef.GetProviderResourceRef().GetProviderId(),
		createdSoRef.GetProviderResourceRef().GetId(),
	)

	engineID := "test-world-engine"

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test/sobject: engine_test.go", []byte(sobjectID), encKey)

	// initialize world engine
	startEngine := func() (*sobject_world_engine.Controller, directive.Reference) {
		engineConf := sobject_world_engine.NewConfig(
			engineID,
			createdSoRef,
		)
		// engineConf.Verbose = true
		worldCtrl, _, worldCtrlRef, err := sobject_world_engine.StartEngineWithConfig(
			ctx,
			tb.Bus,
			engineConf,
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		return worldCtrl, worldCtrlRef
	}

	worldCtrl, worldCtrlRef := startEngine()
	defer worldCtrlRef.Release()

	// provide object op handlers to bus
	opc := world.NewLookupOpController("test-world-engine-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relOpc()

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// uses directive to look up the engine
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	err = world_mock.TestWorldEngine(ctx, le, busEngine)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		wi, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		worldState := wi.(*world_block.World)
		_ = worldState
		// le.Infof("world state after test suite: %s", worldState.String())
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// re-mount the world and make sure it still works.
	worldCtrlRef.Release()
	<-time.After(time.Millisecond * 100)

	worldCtrl, worldCtrlRef = startEngine()
	defer worldCtrlRef.Release()
	<-time.After(time.Millisecond * 100)

	eng, err = worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// second test pass
	engTx, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, found, err := engTx.GetObject(ctx, "test-object")
	if !found && err == nil {
		err = errors.New("object not found after remounting")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// success
}
