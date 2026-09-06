//go:build e2e

package onboarding_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ulid"
	core_provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_provider "github.com/s4wave/spacewave/core/resource/provider"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	transport_dialer "github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_inproc "github.com/s4wave/spacewave/net/transport/inproc"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	"github.com/sirupsen/logrus"
)

// TestCloudSpaceGuestDevice joins a cloud-owned Space through a targeted
// invitation using an independent local session, without cloud owner credentials.
func TestCloudSpaceGuestDevice(t *testing.T) {
	// Mount the real cloud account against the isolated Workers backend.
	ctx, cancel := context.WithTimeout(env.ctx, 90*time.Second)
	t.Cleanup(cancel)
	env.tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(env.tb.Bus))
	env.tb.StaticResolver.AddFactory(space_sobject.NewFactory(env.tb.Bus))
	_, controller, err := env.tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Release)

	// Keep the authenticated owner session mounted through guest synchronization.
	ownerEntry := createCloudSession(ctx, t)
	_, owner, releaseOwner := mountSessionResource(ctx, t, ownerEntry)
	t.Cleanup(releaseOwner)
	account := owner.GetProviderAccount().(*provider_spacewave.ProviderAccount)
	if err := account.ConfigureSessionTransport(ctx, ownerEntry.GetSessionRef().GetProviderResourceRef().GetId(), owner.GetPrivKey(), env.cloudURL, true); err != nil {
		t.Fatal(err)
	}

	// Only the isolated owner account receives the cloud creation prerequisites.
	setTestSubscriptionStatus(t, account.GetAccountID(), "active")
	setTestEmailVerified(t, ctx, account.GetAccountID(), "guest-owner-"+ulid.NewULID()+"@example.com")
	account.BumpLocalEpoch()
	if _, err := waitForSubscriptionStatus(ctx, account, "active"); err != nil {
		t.Fatal(err)
	}

	// Create the Space through the same provider used by a subscribed owner.
	meta, err := space.NewSharedObjectMeta("Guest enrollment")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), meta, "", "")
	if err != nil {
		t.Fatal(err)
	}
	shared, releaseShared, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseShared)
	ownerSpace, releaseOwnerSpace, err := space.ExMountSpaceSoBody(ctx, owner.GetBus(), ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOwnerSpace.Release)
	ownerWorld := world.NewEngineWorldState(ownerSpace.GetSharedObjectBody().GetWorldEngine(), true)
	if _, _, err := space_world_ops.SetSpaceSettings(ctx, ownerWorld, "", "", &space_world.SpaceSettings{IndexPath: "/guest-enrollment"}, true, time.Now()); err != nil {
		t.Fatal(err)
	}

	// The guest owns its key and account independently of the approving owner.
	guestEntry, _ := createLocalSession(ctx, t, "")
	_, guest, releaseGuest := mountSessionResource(ctx, t, guestEntry)
	t.Cleanup(releaseGuest)
	guestAccount := guest.GetProviderAccount().(*provider_local.ProviderAccount)
	if err := guestAccount.EnsureConfiguredSessionTransport(ctx, guest.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	guestBus := guestAccount.GetSessionTransport().GetChildBus()
	connectGuestDevice(t, ctx, owner.GetBus(), owner.GetPeerId(), guestBus, guest.GetPeerId())
	if guest.GetPeerId() == owner.GetPeerId() {
		t.Fatal("guest inherited the owner identity")
	}

	// Approval creates a one-use capability restricted to this guest key.
	invite, err := shared.(*provider_spacewave.SharedObject).CreateSOInviteOp(
		ctx, owner.GetPrivKey(), sobject.SOParticipantRole_SOParticipantRole_WRITER,
		"spacewave", guest.GetPeerId().String(), 1,
		timestamppb.New(time.Now().Add(time.Minute)),
	)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := keypem.MarshalPrivKeyPem(guest.GetPrivKey())
	if err != nil {
		t.Fatal(err)
	}
	localProvider, releaseProvider, err := core_provider.ExLookupProvider(ctx, env.tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseProvider.Release)
	provider := resource_provider.NewLocalProviderResource(
		nil, logrus.NewEntry(logrus.New()), env.tb.Bus,
		localProvider.(*provider_local.Provider),
	)
	t.Cleanup(provider.Release)
	result, err := provider.CompleteSpaceLinkEnrollment(ctx, &s4wave_provider_local.CompleteSpaceLinkEnrollmentRequest{
		SessionPemPrivateKey: pem,
		SessionPeerId:        guest.GetPeerId().String(),
		Invite:               invite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetSessionListEntry().GetSessionRef().GetProviderResourceRef().GetProviderAccountId() != guestAccount.GetAccountID() {
		t.Fatal("enrollment replaced the guest account")
	}

	// The invited Space must be readable through its normal local mount.
	guestRef := sobject.NewSharedObjectRef("local", guestAccount.GetAccountID(), invite.GetSharedObjectId(), provider_local.SobjectBlockStoreID(invite.GetSharedObjectId()))
	mounted, releaseMount, err := space.ExMountSpaceSoBody(ctx, guestBus, guestRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseMount.Release)
	guestWorld := world.NewEngineWorldState(mounted.GetSharedObjectBody().GetWorldEngine(), false)
	settings, _, err := space_world.LookupSpaceSettings(ctx, guestWorld)
	if err != nil {
		t.Fatal(err)
	}
	if settings.GetIndexPath() != "/guest-enrollment" {
		t.Fatal("guest did not read the owner's Space settings")
	}
	// Closing enrollment drops its retained Session and rejects stale client calls.
	provider.Release()
	if _, err := provider.CompleteSpaceLinkEnrollment(ctx, &s4wave_provider_local.CompleteSpaceLinkEnrollmentRequest{
		SessionPemPrivateKey: pem,
		SessionPeerId:        guest.GetPeerId().String(),
		Invite:               invite,
	}); err == nil || !strings.Contains(err.Error(), "resource is released") {
		t.Fatalf("enrollment after resource release: %v", err)
	}
	t.Log("cloud-owned Space mounted by independent local guest through targeted invitation")
}

// connectGuestDevice supplies in-process transport while retaining real signed
// peer negotiation and invitation RPCs on both session buses.
func connectGuestDevice(t *testing.T, ctx context.Context, ownerBus bus.Bus, ownerPeer peer.ID, guestBus bus.Bus, guestPeer peer.ID) {
	t.Helper()

	// Each transport resolves its own session peer from its owning bus.
	var transports []*transport_inproc.Inproc
	for _, target := range []struct {
		// bus resolves this endpoint's retained Session identity.
		bus bus.Bus
		// remote selects the other endpoint for the directed dial.
		remote peer.ID
	}{{ownerBus, guestPeer}, {guestBus, ownerPeer}} {
		controller := transport_inproc.BuildInprocController(logrus.NewEntry(logrus.New()), target.bus, "", &transport_inproc.Config{
			Dialers: map[string]*transport_dialer.DialerOpts{
				target.remote.String(): {Address: transport_inproc.NewAddr(target.remote).String()},
			},
		})
		release, err := target.bus.AddController(ctx, controller, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		transport, err := controller.GetTransport(ctx)
		if err != nil {
			t.Fatal(err)
		}
		transports = append(transports, transport.(*transport_inproc.Inproc))
	}

	// One directed dial avoids racing duplicate link establishment.
	transports[0].ConnectToInproc(ctx, transports[1])
	transports[1].ConnectToInproc(ctx, transports[0])
	_, release, err := link.EstablishLinkWithPeerEx(ctx, ownerBus, ownerPeer, guestPeer, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}
