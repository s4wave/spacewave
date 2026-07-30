package space_resolve_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	"github.com/s4wave/spacewave/testbed"
)

// TestResolveSpaceRetainsEngineMount tests that ResolveSpace keeps the mounted
// world engine alive until its cleanup function releases the mount reference.
func TestResolveSpaceRetainsEngineMount(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()

	// Register factories.
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))

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

	// Start the space shared object controller.
	_, spaceSobjectCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spaceSobjectCtrlRef.Release()

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

	engineID := space.SpaceEngineId(soRef)
	attachedMount, mountInstance, mountRef, err := bus.ExecOneOffTyped[space.MountSharedObjectBodyValue](
		ctx,
		tb.Bus,
		sobject.NewMountSharedObjectBody(soRef, space.SpaceBodyType),
		bus.ReturnIfIdle(false),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	mounted := attachedMount.GetValue()
	if mounted.GetSharedObjectBody().GetWorldEngineID() != engineID {
		t.Fatalf("expected mounted engine ID %q, got %q", engineID, mounted.GetSharedObjectBody().GetWorldEngineID())
	}

	resolved, cleanup, err := space_resolve.ResolveSpace(
		ctx,
		tb.Bus,
		sessionIdx,
		sharedObjectID,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cleanup()

	// ResolveSpace's mount keeps the engine alive after the pre-existing mount releases.
	mountRef.Release()

	if resolved.Engine == nil {
		t.Fatal("resolved engine is nil")
	}
	if resolved.EngineID != engineID {
		t.Fatalf("expected engine ID %q, got %q", engineID, resolved.EngineID)
	}
	if resolved.Ref == nil {
		t.Fatal("resolved ref is nil")
	}

	tx, err := resolved.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("use engine after unrelated mount release: %v", err)
	}
	tx.Discard()

	cleanup()
	if !mountInstance.CloseIfUnreferenced(true) {
		t.Fatal("ResolveSpace cleanup did not release the space body mount")
	}
}
