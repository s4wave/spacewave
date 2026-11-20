package provider_local_test

import (
	"context"
	"sync"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// TestSpaceMemberLinkAdmission checks the account's UDP listener against the
// members of its Spaces: a reader of a listed Space links without gaining
// account authority, a peer no Space names is refused, and a member whose
// participation is removed loses its link.
func TestSpaceMemberLinkAdmission(t *testing.T) {
	// Bound the test and start the owner, a member and a stranger.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	_, _, owner, ownerSess, release := setupProviderAndSession(ctx, t)
	defer release()
	_, _, memberAcc, memberSess, releaseMember := setupProviderAndSession(ctx, t)
	defer releaseMember()
	_, _, strangerAcc, strangerSess, releaseStranger := setupProviderAndSession(ctx, t)
	defer releaseStranger()
	memberID := memberSess.GetPeerId()
	strangerID := strangerSess.GetPeerId()

	// Create a Space whose config names the member as a reader.
	spaceRef, err := owner.CreateSharedObject(ctx, "member-link-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, releaseObject, err := owner.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseObject()
	host := object.(sobject.InviteHost)
	if _, _, _, err := sobject.AddSOParticipant(ctx, host.GetSOHost(), object.GetSharedObjectID(), host.GetPrivKey(), object.GetPeerID().String(), memberID.String(), memberSess.GetPrivKey().GetPublic(), sobject.SOParticipantRole_SOParticipantRole_READER, "", ""); err != nil {
		t.Fatal(err)
	}

	// Start each Session's transport and listen on the owner's.
	transports := make(map[peer.ID]*transport.SessionTransport, 3)
	for _, s := range []struct {
		acc  *provider_local.ProviderAccount
		sess *provider_local.Session
	}{{owner, ownerSess}, {memberAcc, memberSess}, {strangerAcc, strangerSess}} {
		if err := s.acc.CreateSessionTransport(ctx, s.sess.GetPrivKey(), ""); err != nil {
			t.Fatal(err)
		}
		defer s.acc.StopSessionTransport()
		st := s.acc.GetSessionTransport()
		if err := st.AwaitReady(ctx); err != nil {
			t.Fatal(err)
		}
		transports[s.sess.GetPeerId()] = st
	}
	listener := transports[ownerSess.GetPeerId()]
	addr, err := listener.StartUDP(ctx, transport.UDPConfig{ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}

	// The stranger dials first, then the member.
	for _, id := range []peer.ID{strangerID, memberID} {
		st := transports[id]
		if _, err := st.StartUDP(ctx, transport.UDPConfig{ListenAddr: "127.0.0.1:0", Peers: map[peer.ID]string{
			listener.GetPeerID(): addr.String(),
		}}); err != nil {
			t.Fatal(err)
		}
		_, elRef, err := st.GetChildBus().AddDirective(ctx, link.NewEstablishLinkWithPeer(id, listener.GetPeerID()), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer elRef.Release()
	}

	// The member links; the stranger does not, and no rule admits it.
	ids := []peer.ID{strangerID, memberID}
	waitLinked(ctx, t, listener, ids, func(linked map[peer.ID]struct{}) bool {
		_, ok := linked[memberID]
		return ok
	}, "member did not link")
	linked, _ := listener.GetLinkedPeerIDsSnapshotWithWait(ids)
	if _, ok := linked[strangerID]; ok {
		t.Fatal("listener linked a peer no Space names")
	}
	if err := listener.AuthorizeLink(ctx, strangerID); err == nil {
		t.Fatal("link rule admitted a peer no Space names")
	}

	// Membership admits the link only, never account authority.
	if err := listener.AuthorizePeer(ctx, memberID); err == nil {
		t.Fatal("Space member gained account authority")
	}

	// Removing the member closes its link and refuses it again.
	if _, err := sobject.RemoveSOParticipant(ctx, host.GetSOHost(), memberID.String(), host.GetPrivKey(), nil); err != nil {
		t.Fatal(err)
	}
	waitLinked(ctx, t, listener, ids, func(linked map[peer.ID]struct{}) bool {
		_, ok := linked[memberID]
		return !ok
	}, "removed member kept its link")
	if err := listener.AuthorizeLink(ctx, memberID); err == nil {
		t.Fatal("link rule admitted a removed member")
	}
}

// waitLinked waits until done accepts the listener's links to ids.
func waitLinked(ctx context.Context, t *testing.T, listener *transport.SessionTransport, ids []peer.ID, done func(map[peer.ID]struct{}) bool, fail string) {
	t.Helper()
	for {
		linked, waitChs := listener.GetLinkedPeerIDsSnapshotWithWait(ids)
		if done(linked) {
			return
		}
		woke := make(chan struct{})
		var once sync.Once
		for _, ch := range waitChs {
			go func() {
				select {
				case <-ch:
					once.Do(func() { close(woke) })
				case <-ctx.Done():
				}
			}()
		}
		select {
		case <-woke:
		case <-ctx.Done():
			t.Fatal(fail)
		}
	}
}
