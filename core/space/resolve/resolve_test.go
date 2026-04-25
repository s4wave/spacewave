package space_resolve_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

// TestResolveSpaceReturnsEngine tests that ResolveSpace resolves a session
// index and shared object ID to a running world engine using the full
// mounting chain with a real in-memory testbed.
func TestResolveSpaceReturnsEngine(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	peerID := tb.Volume.GetPeerID()

	// Register factories.
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	// Start session controller.
	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer sessCtrlRef.Release()

	// Start local provider controller.
	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	// Look up the provider and create a local account + session.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provRef.Release()

	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the session with the session controller.
	sessCtrl, sessCtrlLookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer sessCtrlLookupRef.Release()

	entry, err := sessCtrl.RegisterSession(ctx, sessRef, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	sessionIdx := entry.GetSessionIndex()

	// Access the provider account and create a shared object.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	sharedObjectID := "test-space"
	soRef, err := wsProv.CreateSharedObject(ctx, sharedObjectID, &sobject.SharedObjectMeta{
		BodyType: "space",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the sobject world engine for this shared object.
	engineID := space.SpaceEngineId(soRef)
	engineConf := sobject_world_engine.NewConfig(engineID, soRef)
	_, _, worldCtrlRef, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus, engineConf, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer worldCtrlRef.Release()

	// Provide mock op handlers so the engine can process operations.
	opc := world.NewLookupOpController("test-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relOpc()

	// Resolve the space.
	resolved, cleanup, err := space_resolve.ResolveSpace(ctx, tb.Bus, sessionIdx, sharedObjectID)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cleanup()

	if resolved.Engine == nil {
		t.Fatal("resolved engine is nil")
	}
	if resolved.EngineID != engineID {
		t.Fatalf("expected engine ID %q, got %q", engineID, resolved.EngineID)
	}
	if resolved.Ref == nil {
		t.Fatal("resolved ref is nil")
	}

	// Verify the engine is functional by creating a transaction.
	tx, err := resolved.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()
}
