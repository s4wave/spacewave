package space_resolve_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	"github.com/s4wave/spacewave/testbed"
)

// TestResolveSpaceAbsentEngineReturnsUnavailable ensures an HTTP-style resolve
// does not wait forever when no controller owns the space engine.
func TestResolveSpaceAbsentEngineReturnsUnavailable(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
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

	resolveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resultCh := make(chan error, 1)
	go func() {
		resolved, cleanup, err := space_resolve.ResolveSpace(
			resolveCtx,
			tb.Bus,
			entry.GetSessionIndex(),
			sharedObjectID,
		)
		if cleanup != nil {
			cleanup()
		}
		if resolved != nil {
			resultCh <- errors.New("resolved an absent world engine")
			return
		}
		resultCh <- err
	}()

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("expected unavailable world engine error")
		}
		if errors.Is(err, context.Canceled) {
			t.Fatalf("expected defined unavailable outcome, got context cancellation: %v", err)
		}
	case <-timer.C:
		cancel()
		<-resultCh
		t.Fatal("ResolveSpace remained pending without an engine owner")
	}
}
