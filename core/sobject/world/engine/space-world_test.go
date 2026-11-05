package sobject_world_engine_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
)

// spaceWorld is a World engine on a local Space SharedObject.
type spaceWorld struct {
	bus      bus.Bus
	soRef    *sobject.SharedObjectRef
	engineID string
	sender   peer.ID
	eng      world.Engine
	vol      volume.Volume
}

// newSpaceWorld starts a World engine on a new local Space SharedObject.
func newSpaceWorld(ctx context.Context, t *testing.T) *spaceWorld {
	t.Helper()
	return newSpaceWorldWithConf(ctx, t, nil)
}

// newSpaceWorldWithConf starts a World engine on a new local Space
// SharedObject with no changelog, after edit adjusts its Config.
func newSpaceWorldWithConf(ctx context.Context, t *testing.T, edit func(*sobject_world_engine.Config)) *spaceWorld {
	// Start a testbed.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register the provider and World engine factories.
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	// Create a local Space SharedObject.
	providerID, accountID, sobjectID := "local", "test-account", "file-gc-space"
	_, provCtrlRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provCtrlRef.Release)

	// Create the Space through the provider account.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provAccRef.Release)
	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err)
	}
	soRef, err := soProv.CreateSharedObject(ctx, sobjectID, &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Register the mock and file operations.
	engineID := "file-gc-engine"
	lookupOp := world.NewLookupOpFromSlice(world.LookupOpSlice{world_mock.LookupMockOp, world.NewSchema(unixfs_world.UnixfsTypes).LookupOp})
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("file-gc-ops", engineID, lookupOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relOpc)

	// Start the World engine on the SharedObject without a changelog, as a
	// Space does, so replaced roots are unreachable from the head.
	conf := sobject_world_engine.NewConfig(engineID, soRef)
	conf.InitWorldOp = &sobject_world_engine.InitWorldOp{}
	if edit != nil {
		edit(conf)
	}
	conf.InitWorldOp.LastChangeDisable = true
	eng, engRef, err := world.StartEngine(ctx, tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engRef.Release)

	// Look up the volume that holds the Space bucket.
	vol, _, volRef, err := volume.ExLookupVolume(ctx, tb.Bus, provider_local.StorageVolumeID(providerID, accountID), "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(volRef.Release)
	return &spaceWorld{
		bus:      tb.Bus,
		soRef:    soRef,
		engineID: engineID,
		sender:   tb.Volume.GetPeerID(),
		eng:      eng,
		vol:      vol,
	}
}

// engineHead returns the root of the World eng installed.
func engineHead(ctx context.Context, t *testing.T, eng world.Engine) *block.BlockRef {
	// Ask the engine for its head.
	t.Helper()
	var root *block.BlockRef
	err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
		var err error
		root, _, err = eng.(sobject_world_engine.BackfillEngine).GetHead(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}
