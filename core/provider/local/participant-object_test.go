package provider_local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

// TestEnrollParticipantObject proves that a Session in another account reads a
// Secret only after its creator grants it, and that later payloads reach it.
func TestEnrollParticipantObject(t *testing.T) {
	// Start an owner and a member provider.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	ownerTb, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	defer releaseOwner()
	memberTb, _, member, memberSession, releaseMember := setupProviderAndSession(ctx, t)
	defer releaseMember()

	// The owner creates a Space-like object and the member joins it as a writer.
	spaceRef, err := owner.CreateSharedObject(ctx, "enroll-space", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseSpace, err := owner.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSpace()

	// The owner prepares a writer invite on its Session transport.
	ownerSpace := mounted.(*provider_local.SharedObject)
	invite, err := ownerSpace.CreateSOInviteOp(ctx, ownerSpace.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), ownerSpace.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionTransport()
	defer owner.StopP2PSync()

	// The member links its Session transport to the owner and joins.
	if err := member.EnsureConfiguredSessionTransport(ctx, memberSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	defer member.StopSessionTransport()
	defer member.StopP2PSync()
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), member.GetSessionTransport())
	if _, err := member.JoinViaInvite(ctx, memberSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}

	// The owner creates a Secret object holding the first payload.
	secretRef, err := owner.CreateSharedObject(ctx, "secret-enroll", s4wave_secret.NewSharedObjectMeta(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s4wave_secret.StoreSecretPayload(ctx, ownerTb.Bus, secretRef, &s4wave_secret.SecretPayload{Value: []byte("first"), Version: 1}); err != nil {
		t.Fatal(err)
	}
	meta := s4wave_secret.NewSharedObjectMeta()

	// An ungranted Session gets no checkpoint.
	if _, err := member.EnrollParticipantObject(ctx, "secret-enroll", meta, "enroll-space"); err == nil || !strings.Contains(err.Error(), "not a reader") {
		t.Fatalf("ungranted Session enrolled or failed otherwise: %v", err)
	}

	// The owner grants the member Session.
	mounted, releaseSecret, err := owner.MountSharedObject(ctx, secretRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecret()
	ownerSecret := mounted.(*provider_local.SharedObject)
	memberPeer := member.GetSessionTransport().GetPeerID()
	memberPub, err := memberPeer.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sobject.AddSOParticipant(ctx, ownerSecret.GetSOHost(), "secret-enroll", ownerSecret.GetPrivKey(), ownerSecret.GetPeerID().String(), memberPeer.String(), memberPub, sobject.SOParticipantRole_SOParticipantRole_READER, "", ""); err != nil {
		t.Fatal(err)
	}

	// A Space the owner did not write cannot vouch for the Secret.
	if _, err := member.CreateSharedObject(ctx, "member-own", &sobject.SharedObjectMeta{BodyType: "test"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := member.EnrollParticipantObject(ctx, "secret-enroll", meta, "member-own"); err == nil || !strings.Contains(err.Error(), "not created by a writer") {
		t.Fatalf("enrolled through an object the Secret's creator does not write, or failed otherwise: %v", err)
	}

	// The granted member enrolls under its own account and reads the payload.
	ref, err := member.EnrollParticipantObject(ctx, "secret-enroll", meta, "enroll-space")
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetProviderResourceRef().GetProviderAccountId() != member.GetAccountID() {
		t.Fatalf("enrolled ref names account %s", ref.GetProviderResourceRef().GetProviderAccountId())
	}
	payload, err := s4wave_secret.ReadSecretPayload(ctx, memberTb.Bus, &s4wave_secret.Secret{Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload.GetValue()) != "first" {
		t.Fatalf("member read %q", payload.GetValue())
	}

	// A later payload reaches the member through sync.
	if err := s4wave_secret.StoreSecretPayload(ctx, ownerTb.Bus, secretRef, &s4wave_secret.SecretPayload{Value: []byte("second"), Version: 2}); err != nil {
		t.Fatal(err)
	}
	mounted, releaseMemberSecret, err := member.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseMemberSecret()
	states, releaseStates, err := mounted.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		payload, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, snapshot)
		if err != nil {
			return false, nil
		}
		return string(payload.GetValue()) == "second", nil
	}, nil); err != nil {
		t.Fatalf("member did not receive the second payload: %v", err)
	}
}
