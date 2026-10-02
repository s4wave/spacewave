//go:build !tinygo

package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestLocalProviderInvitationUsesNativeNetwork joins two accounts without signaling or manual transport wiring.
func TestLocalProviderInvitationUsesNativeNetwork(t *testing.T) {
	// Mount the owner account under the real native provider.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)

	// Look up the native provider that hosts both accounts.
	rawProvider, providerRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	local := rawProvider.(*provider_local.Provider)

	// Create and mount an independent reader account and session.
	readerRef, err := local.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	rawReader, releaseReader, err := local.AccessProviderAccount(ctx, readerRef.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseReader)

	// Mount the reader's session.
	reader := rawReader.(*provider_local.ProviderAccount)
	readerSession, releaseSession, err := reader.MountSession(ctx, readerRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)

	// Create and mount the owner's shared object.
	ref, err := owner.CreateSharedObject(ctx, "local-invitation", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)

	// Publish a signed targeted invitation through the existing owner API.
	host := object.(sobject.InviteHost)
	invite, err := host.CreateSOInviteOp(ctx, host.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, TargetPeerId: readerSession.GetPeerId().String(), MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), host.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}

	// Complete the unchanged authenticated invitation and grant installation protocol.
	joined, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatal(err)
	}
	if joined.SharedObjectID != ref.GetProviderResourceRef().GetId() || joined.Grant == nil {
		t.Fatal("native invitation did not grant the requested object")
	}
	found := false
	for _, entry := range reader.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == joined.SharedObjectID {
			found = true
		}
	}
	if !found {
		t.Fatal("joined object is absent from the reader's retained list")
	}

	// Native presence includes the local link and updates when its peer stops.
	ownerID := ownerSession.GetPeerId().String()
	for {
		online, waitChs := reader.GetOnlinePeerIDsWithWait([]string{ownerID})
		if len(online) == 1 {
			break
		}
		if err := broadcast.WaitAny(ctx, waitChs...); err != nil {
			t.Fatal(err)
		}
	}
	owner.StopP2PSync()
	owner.StopSessionTransport()
	for {
		online, waitChs := reader.GetOnlinePeerIDsWithWait([]string{ownerID})
		if len(online) == 0 {
			break
		}
		if err := broadcast.WaitAny(ctx, waitChs...); err != nil {
			t.Fatal(err)
		}
	}
}
