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
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer sessCtrlRef.Release()
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

	_, spaceSobjectCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spaceSobjectCtrlRef.Release()

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

	sessCtrl, sessCtrlLookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer sessCtrlLookupRef.Release()

	entry, err := sessCtrl.RegisterSession(ctx, sessRef, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

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

	const sharedObjectID = "absent-engine-space"
	_, err = wsProv.CreateSharedObject(ctx, sharedObjectID, &sobject.SharedObjectMeta{
		BodyType: "space",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

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

	if resolved.Engine == nil {
		t.Fatal("resolved engine is nil")
	}
	tx, err := resolved.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()
}
