//go:build !tinygo

package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/peer"
)

// TestJoinViaInviteStaticUDP joins a Space whose owner is reachable only at a
// static UDP address without replicating it, relinks from the recorded address after a restart, and
// stops dialing the owner after leaving.
func TestJoinViaInviteStaticUDP(t *testing.T) {
	// Run the owner and the reader on separate testbeds, so UDP is the only
	// path between them.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	_, _, reader, readerSession, releaseReader := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseReader)

	// The owner creates the Space it invites the reader's Session to.
	ref, err := owner.CreateSharedObject(ctx, "static-udp", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)

	// Sign the invitation and start the owner's invitation service.
	host := object.(sobject.InviteHost)
	readerPeer := readerSession.GetPeerId()
	invite, err := host.CreateSOInviteOp(ctx, host.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_READER, TargetPeerId: readerPeer.String(), MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), host.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.StopSessionTransport)
	t.Cleanup(owner.StopP2PSync)

	// The owner listens on loopback for the reader alone.
	addr, err := owner.GetSessionTransport().StartUDP(ctx, transport.UDPConfig{
		ListenAddr: "127.0.0.1:0",
		Peers:      map[peer.ID]string{readerPeer: ""},
		NamedOnly:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The reader joins over the owner's address and records it on the listed
	// object, which stays lazy: the account fetches only the blocks it reads.
	t.Cleanup(reader.StopSessionTransport)
	t.Cleanup(reader.StopP2PSync)
	joined, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, addr.String())
	if err != nil {
		t.Fatal(err)
	}
	ownerID := ownerSession.GetPeerId().String()
	var recorded string
	for _, entry := range reader.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == joined.SharedObjectID {
			recorded = entry.GetUdpPeerAddrs()[ownerID]
			if provider_local.ReplicatesSpace(entry) {
				t.Fatal("reader replicates the joined Space")
			}
		}
	}
	if recorded != addr.String() {
		t.Fatalf("listed object records owner address %q, want %q", recorded, addr.String())
	}

	// The P2P sync generation keeps the owner linked after the join.
	waitOnline := func(want int) {
		t.Helper()
		for {
			online, waitChs := reader.GetOnlinePeerIDsWithWait([]string{ownerID})
			if len(online) == want {
				return
			}
			if err := broadcast.WaitAny(ctx, waitChs...); err != nil {
				t.Fatal(err)
			}
		}
	}
	waitOnline(1)

	// A new transport and sync generation link from the recorded address alone.
	reader.StopP2PSync()
	reader.StopSessionTransport()
	waitOnline(0)
	if err := reader.EnsureConfiguredSessionTransport(ctx, readerSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if err := reader.AutoStartP2PSyncIfNeeded(ctx, reader.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	waitOnline(1)

	// The owner commits the reader's departure over the restored link, and the
	// departed reader drops the address and with it its only path to the owner.
	if err := reader.LeaveSharedObject(ctx, readerSession.GetPrivKey(), joined.SharedObjectID, ""); err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.GetSOListCtr().GetValue().GetSharedObjects() {
		if len(entry.GetUdpPeerAddrs()) != 0 {
			t.Fatalf("departed reader kept udp peers %v", entry.GetUdpPeerAddrs())
		}
	}
	waitOnline(0)
}
