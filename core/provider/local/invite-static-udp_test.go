//go:build !tinygo

package provider_local_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// TestJoinViaInviteStaticUDP joins a Space whose owner is reachable only at a
// static UDP address without replicating it, refuses the reader's writes to its
// World, relinks from the recorded address after a restart, and stops dialing
// the owner after leaving.
func TestJoinViaInviteStaticUDP(t *testing.T) {
	// Bound the run.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Run the owner and the reader on separate testbeds, so UDP is the only
	// path between them, each with a World engine.
	ownerTb, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	ownerTb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(ownerTb.Bus))
	readerTb, _, reader, readerSession, releaseReader := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseReader)
	readerTb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(readerTb.Bus))

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

	// The owner initializes its World with one object.
	ownerWorld := &offlineMember{t: t, name: "owner", tb: ownerTb, account: owner, object: object.(*provider_local.SharedObject), ref: ref}
	ownerWorld.start(ctx)
	t.Cleanup(ownerWorld.stop)
	ownerWorld.create(ctx, "owner-object")

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

	// Find the listed object and the owner address it records.
	ownerID := ownerSession.GetPeerId().String()
	var recorded string
	var joinedRef *sobject.SharedObjectRef
	for _, entry := range reader.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == joined.SharedObjectID {
			recorded = entry.GetUdpPeerAddrs()[ownerID]
			joinedRef = entry.GetRef()
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

	// The reader's World engine refuses a write transaction, since every
	// replay would reject the operation it commits.
	eng, engRef, err := world.StartEngine(ctx, readerTb.Bus, sobject_world_engine.NewConfig("static-udp-world", joinedRef), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.NewTransaction(ctx, true); !errors.Is(err, tx.ErrNotWrite) {
		t.Fatalf("reader opened a write transaction: %v", err)
	}
	engRef.Release()

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
