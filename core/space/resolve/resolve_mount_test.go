package space_resolve_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	"github.com/s4wave/spacewave/testbed"
)

// TestResolveSpaceWithoutConcurrentMounter ensures ResolveSpace mounts the
// space body needed to start and retain the returned world engine.
func TestResolveSpaceWithoutConcurrentMounter(t *testing.T) {
	// Give the resolver test an independent context.
	ctx := context.Background()

	// Start the testbed and retain its cleanup.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Register the controllers needed to resolve a mounted Space.
	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	// Load the session controller from the testbed bus.
	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer sessCtrlRef.Release()

	// Load a local provider backed by the testbed volume.
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

	// Load the Space SharedObject controller.
	_, spaceSobjectCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spaceSobjectCtrlRef.Release()

	// Resolve the local provider account.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provRef.Release()

	// Create a local account and session for the resolver.
	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the session controller for the new session.
	sessCtrl, sessCtrlLookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer sessCtrlLookupRef.Release()

	// Register the session and retain its index.
	entry, err := sessCtrl.RegisterSession(ctx, sessRef, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Access the provider account that owns the target Space.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	// Resolve SharedObject creation through the account feature.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a Space body that has no existing engine mount.
	const sharedObjectID = "absent-engine-space"
	_, err = wsProv.CreateSharedObject(ctx, sharedObjectID, &sobject.SharedObjectMeta{
		BodyType: "space",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the Space and keep its mounted resources alive.
	resolved, cleanup, err := space_resolve.ResolveSpace(
		ctx,
		tb.Bus,
		entry.GetSessionIndex(),
		sharedObjectID,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cleanup()

	// Verify the resolved engine remains usable until cleanup.
	if resolved.Engine == nil {
		t.Fatal("resolved engine is nil")
	}
	tx, err := resolved.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()
}
