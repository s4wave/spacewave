//go:build e2e

package onboarding_test

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ulid"
	core_provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestCloudOwnerFreshCache checks that cloud admission and nonce consumption
// survive an independently constructed provider cache with the registered key.
// The original Session remains alive; this does not establish Worker transport.
func TestCloudOwnerFreshCache(t *testing.T) {
	// Create the account and Space through the isolated Workers backend.
	ctx, cancel := context.WithTimeout(env.ctx, 90*time.Second)
	t.Cleanup(cancel)
	entry := createCloudSession(ctx, t)
	resource, original, release := mountSessionResource(ctx, t, entry)
	t.Cleanup(release)
	t.Cleanup(resource.Close)
	account := original.GetProviderAccount().(*provider_spacewave.ProviderAccount)
	setTestSubscriptionStatus(t, account.GetAccountID(), "active")
	setTestEmailVerified(t, ctx, account.GetAccountID(), "owner-cache-"+ulid.NewULID()+"@example.com")
	account.BumpLocalEpoch()
	if _, err := waitForSubscriptionStatus(ctx, account, "active"); err != nil {
		t.Fatal(err)
	}
	meta, err := space.NewSharedObjectMeta("Owner cache")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), meta, "", "")
	if err != nil {
		t.Fatal(err)
	}
	spaceID := ref.GetProviderResourceRef().GetId()

	// Consume a signed guest nonce before constructing the second cache.
	approval := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(resource.GetMux()))),
	)
	used := ownerCacheTicket(t, spaceID)
	if _, err := approval.ApproveGuestSpaceLink(ctx, used); err != nil {
		t.Fatal(err)
	}

	// Build an independent volume, Session inventory, and cloud provider.
	fresh, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Release)
	fresh.StaticResolver.AddFactory(session_controller.NewFactory(fresh.Bus))
	fresh.StaticResolver.AddFactory(provider_spacewave.NewFactory(fresh.Bus))
	for _, conf := range []config.Config{
		&session_controller.Config{VolumeId: fresh.Volume.GetID()},
		&provider_spacewave.Config{Endpoint: env.cloudURL},
	} {
		_, ref, err := fresh.Bus.AddDirective(resolver.NewLoadControllerWithConfig(conf), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ref.Release)
	}
	provider, providerRef, err := core_provider.ExLookupProvider(ctx, fresh.Bus, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	inventory, inventoryRef, err := session.ExLookupSessionController(ctx, fresh.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(inventoryRef.Release)

	// Restore the existing registered Session using its retained signing key.
	restored, err := provider.(*provider_spacewave.Provider).MountHandoffSession(
		ctx, account.GetAccountID(), "Owner cache", original.GetPrivKey(), inventory,
	)
	if err != nil {
		t.Fatal(err)
	}
	mounted, mountedRef, err := session.ExMountSession(ctx, fresh.Bus, restored.GetSessionRef(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mountedRef.Release)
	if mounted.GetPeerId() != original.GetPeerId() {
		t.Fatal("restored Session changed its registered identity")
	}
	freshResource := resource_session.NewSessionResource(logrus.NewEntry(logrus.New()), fresh.Bus, mounted)
	t.Cleanup(freshResource.Close)
	freshApproval := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(freshResource.GetMux()))),
	)

	// Cloud nonce state must reject replay independently of the original cache.
	if _, err := freshApproval.ApproveGuestSpaceLink(ctx, used); err == nil ||
		!strings.Contains(err.Error(), provider_spacewave.ErrSpaceLinkNonceConsumed.Error()) {
		t.Fatalf("fresh-cache replay = %v, want consumed nonce rejection", err)
	}
	invite, err := freshApproval.ApproveGuestSpaceLink(ctx, ownerCacheTicket(t, spaceID))
	if err != nil {
		t.Fatal(err)
	}
	if invite.GetSharedObjectId() != spaceID {
		t.Fatal("fresh cache did not recover the configured Space")
	}
}

// ownerCacheTicket signs a new Device writer request for the selected test Space.
func ownerCacheTicket(t *testing.T, spaceID string) *s4wave_provider_spacewave.ApproveSpaceLinkRequest {
	t.Helper()

	// Generate an independent guest identity and replay-defense nonce.
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}

	// Preserve the generated consent payload bytes as the signature input.
	payload, err := (&s4wave_provider_spacewave.SpaceLinkAuthRequest{
		Version:        1,
		SessionType:    session.SessionType_SESSION_TYPE_DEVICE,
		AgentPeerId:    []byte(pid),
		Label:          "Fresh-cache guest",
		RequestedRole:  sobject.SOParticipantRole_SOParticipantRole_WRITER,
		Nonce:          nonce,
		ExpiresAt:      time.Now().Add(5 * time.Minute).Unix(),
		CompletionMode: s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_CLI,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	signature, err := key.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := (&s4wave_provider_spacewave.SpaceLinkAuthTicket{Payload: payload, AgentSignature: signature}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return &s4wave_provider_spacewave.ApproveSpaceLinkRequest{Ticket: ticket, ResourceId: []byte(spaceID)}
}
