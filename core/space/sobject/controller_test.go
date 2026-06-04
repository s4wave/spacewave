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
	sharedObjectRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-space",
			ProviderId:        "local",
			ProviderAccountId: "test-account",
		},
	}

	conf := newSpaceWorldEngineConfig(sharedObjectRef, &Config{})
	if conf.GetEngineId() != space.SpaceEngineId(sharedObjectRef) {
		t.Fatalf("unexpected engine id: %q", conf.GetEngineId())
	}
	if !conf.GetInitWorldOp().GetLastChangeDisable() {
		t.Fatal("expected Space world init to disable changelog")
	}
}

func TestMountSpaceBodyProvidesSpaceWorldOps(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

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

	_, spaceSobjectCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer spaceSobjectCtrlRef.Release()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, "test-account", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	spaceMeta, err := space.NewSharedObjectMeta("Test Space")
	if err != nil {
		t.Fatal(err.Error())
	}
	soRef, err := wsProv.CreateSharedObject(ctx, "test-space", spaceMeta, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	mounted, mountedRef, err := space.ExMountSpaceSoBody(ctx, tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer mountedRef.Release()

	spaceBody := mounted.GetSharedObjectBody()
	ws := world.NewEngineWorldState(spaceBody.GetWorldEngine(), true)
	settings := &space_world.SpaceSettings{
		IndexPath: "/test",
		PluginIds: []string{
			"spacewave-test",
		},
	}

	if _, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", settings, true, time.Now()); err != nil {
		t.Fatal(err.Error())
	}

	gotSettings, _, err := space_world.LookupSpaceSettings(ctx, ws)
	if err != nil {
		t.Fatal(err.Error())
	}
	if gotSettings.GetIndexPath() != "/test" {
		t.Fatalf("unexpected index path: %q", gotSettings.GetIndexPath())
	}
}
