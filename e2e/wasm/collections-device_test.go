//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	resolver_static "github.com/aperturerobotics/controllerbus/controller/resolver/static"
	plugin_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	worldop_registry "github.com/s4wave/spacewave/core/resource/worldop/registry"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	space_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/s4wave/spacewave/testbed"
)

// mountPluginBuildDevice joins a disposable native participant to the browser's
// Space. Its own World engine signs worker operations and retains local blocks.
func mountPluginBuildDevice(t *testing.T, ctx context.Context, author *s4wave_session.Session, spaceID string) (bus.Bus, *resolver_static.Resolver, world.Engine, peer.ID) {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	// Root retention traverses every built-in Space object on this device.
	releaseBlockTypes, err := tb.Bus.AddController(ctx, blocktype_controller.NewController(space_world.LookupBlockType), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseBlockTypes)
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, sessionsRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{VolumeId: tb.EngineVolumeID}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessionsRef.Release)
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	_, providerRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local", PeerId: tb.Volume.GetPeerID().String(), StorageId: tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	raw, lookupRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lookupRef.Release)
	local := raw.(*provider_local.Provider)
	sessionRef, err := local.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	rawAccount, releaseAccount, err := local.AccessProviderAccount(ctx, sessionRef.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAccount)
	account := rawAccount.(*provider_local.ProviderAccount)
	mounted, releaseSession, err := account.MountSession(ctx, sessionRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)
	native := resource_session.NewSessionResource(tb.Logger, tb.Bus, mounted)
	t.Cleanup(native.Close)

	// Pair two disposable devices through the normal bilateral account enrollment.
	offer, err := author.CreateLocalPairingOffer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := native.AcceptLocalPairingOffer(ctx, &s4wave_session.AcceptLocalPairingOfferRequest{OfferPayload: offer.GetOfferPayload()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := author.AcceptLocalPairingAnswer(ctx, answer.GetAnswerPayload()); err != nil {
		t.Fatal(err)
	}
	info, err := author.GetSessionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	authorPeer, err := peer.IDB58Decode(info.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	enginePairing, err := mounted.(*provider_local.Session).GetPairingEngine()
	if err != nil {
		t.Fatal(err)
	}
	waitPairing := func(want pairing.Status) {
		t.Helper()
		err := enginePairing.Watch(ctx, func(state pairing.Snapshot) error {
			if state.Status == want {
				return io.EOF
			}
			if state.Status == pairing.StatusFailed {
				t.Fatalf("native pairing: %s", state.ErrMsg)
			}
			return nil
		})
		if err != io.EOF {
			t.Fatal(err)
		}
	}
	waitPairing(pairing.StatusVerifyingEmoji)
	watch, err := author.WatchPairingStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	waitForPairingStatus(t, "author", watch, s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI)
	enginePairing.ConfirmSAS(true)
	if err := author.ConfirmSASMatch(ctx, true); err != nil {
		t.Fatal(err)
	}
	waitPairing(pairing.StatusBothConfirmed)
	waitForPairingStatus(t, "author", watch, s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED)
	pairedRef, err := enginePairing.Result(authorPeer)
	if err != nil {
		t.Fatal(err)
	}
	rawPaired, releasePaired, err := local.AccessProviderAccount(ctx, pairedRef.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releasePaired)
	account = rawPaired.(*provider_local.ProviderAccount)
	paired, releasePairedSession, err := account.MountSession(ctx, pairedRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releasePairedSession)
	ref := sobject.NewSharedObjectRef("local", pairedRef.GetProviderResourceRef().GetProviderAccountId(), spaceID, provider_local.SobjectBlockStoreID(spaceID))
	engine, _, engineRef, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus,
		sobject_world_engine.NewConfig("plugin-device-space", ref), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engineRef.Release)
	engine.SetStaticLookupOp(space_optypes.LookupWorldOp)
	if _, err := engine.GetWorldEngine(ctx); err != nil {
		t.Fatal(err)
	}
	worldEngine := world.NewBusEngine(ctx, tb.Bus, "plugin-device-space")
	t.Cleanup(worldEngine.ClearContext)
	localPeer, err := peer.NewPeer(paired.GetPrivKey())
	if err != nil {
		t.Fatal(err)
	}
	releasePeer, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, localPeer), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releasePeer)
	startBuildDevicePlugins(t, ctx, tb, paired.GetPeerId())
	return tb.Bus, tb.StaticResolver, worldEngine, paired.GetPeerId()
}

// startBuildDevicePlugins gives the native validator the same immutable app
// implementations as its browser peer. It executes retained operations through
// QuickJS without installing UI registrations on this headless build device.
func startBuildDevicePlugins(t *testing.T, ctx context.Context, tb *testbed.Testbed, sender peer.ID) {
	t.Helper()
	b := tb.Bus
	tb.StaticResolver.AddFactory(plugin_quickjs.NewFactory(b))
	tb.StaticResolver.AddFactory(plugin_scheduler.NewFactory(b))
	_, _, hostRef, err := loader.WaitExecControllerRunning(ctx, b,
		resolver.NewLoadControllerWithConfig(plugin_quickjs.NewConfig()), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hostRef.Release)
	conf := plugin_scheduler.NewConfig("", "plugin-device-space", "projects/colors/builds",
		tb.EngineVolumeID, sender.String(), false, true, true)
	_, _, schedulerRef, err := loader.WaitExecControllerRunning(ctx, b,
		resolver.NewLoadControllerWithConfig(conf), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(schedulerRef.Release)
	releaseTypes, err := b.AddController(ctx, objecttype_controller.NewController(space_objecttypes.LookupObjectType), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseTypes)
	registry := worldop_registry.NewWorldOpRegistryResource(nil)
	releaseOps, err := b.AddController(ctx, worldop_registry.NewWorldOpRegistryBridgeController(tb.Logger, b, registry), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOps)
}
