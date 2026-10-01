package space_sobject

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/testbed"
)

func TestNewSpaceWorldEngineConfigDisablesChangelog(t *testing.T) {
	// Build a shared object reference pointing at the test space.
	sharedObjectRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-space",
			ProviderId:        "local",
			ProviderAccountId: "test-account",
		},
	}

	// Build the engine config and assert its engine id and changelog.
	conf := newSpaceWorldEngineConfig(sharedObjectRef, &Config{})
	if conf.GetEngineId() != space.SpaceEngineId(sharedObjectRef) {
		t.Fatalf("unexpected engine id: %q", conf.GetEngineId())
	}
	if !conf.GetInitWorldOp().GetLastChangeDisable() {
		t.Fatal("expected Space world init to disable changelog")
	}
}

func TestMountSpaceBodyProvidesSpaceWorldOps(t *testing.T) {
	// Build a background context for the test.
	ctx := context.Background()

	// Start the default testbed.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Register the provider, engine, and space controllers.
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// Load the local provider controller.
	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	// Load the space sobject controller.
	_, spaceSobjectCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spaceSobjectCtrlRef.Release()

	// Access the test provider account.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, "test-account", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	// Fetch the shared-object feature from the provider account.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the test space shared object.
	spaceMeta, err := space.NewSharedObjectMeta("Test Space")
	if err != nil {
		t.Fatal(err.Error())
	}
	soRef, err := wsProv.CreateSharedObject(ctx, "test-space", spaceMeta, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Mount the space body from the shared object.
	mounted, mountedRef, err := space.ExMountSpaceSoBody(ctx, tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer mountedRef.Release()

	// Write test space settings through the world state.
	spaceBody := mounted.GetSharedObjectBody()
	ws := world.NewEngineWorldState(spaceBody.GetWorldEngine(), true)
	settings := &space_world.SpaceSettings{
		IndexPath: "/test",
		PluginIds: []string{
			"spacewave-test",
		},
	}

	// Apply the settings operation.
	if _, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", settings, true, time.Now()); err != nil {
		t.Fatal(err.Error())
	}

	// Look up the settings and assert the index path persisted.
	gotSettings, objectState, err := space_world.LookupSpaceSettings(ctx, ws)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}
	if gotSettings.GetIndexPath() != "/test" {
		t.Fatalf("unexpected index path: %q", gotSettings.GetIndexPath())
	}
}
