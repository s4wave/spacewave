package provider_local_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestNativeSpaceLeave observes committed removal at both ends of a real invitation transport.
func TestNativeSpaceLeave(t *testing.T) {
	// Native providers retain independent storage and device identities.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	_, _, reader, readerSession, releaseReader := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseReader)

	// The owner hosts a Space.
	ref, err := owner.CreateSharedObject(ctx, "leave-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	object := mounted.(*provider_local.SharedObject)
	id := object.GetSharedObjectID()

	// The owner serves a direct invitation over its session transport.
	invite, err := object.CreateSOInviteOp(ctx, object.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), object.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.StopSessionTransport)
	t.Cleanup(owner.StopP2PSync)

	// The reader joins through a reachable transport.
	if err := reader.EnsureConfiguredSessionTransport(ctx, readerSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.StopSessionTransport)
	t.Cleanup(reader.StopP2PSync)
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), reader.GetSessionTransport())
	if _, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	copy := mountJoined(ctx, t, reader, id)

	// The joined copy enrolled both reader identities.
	before, err := copy.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.GetConfig().GetParticipants()) != 3 {
		t.Fatalf("expected owner, device, and storage participants: %v", before.GetConfig().GetParticipants())
	}

	// A leave proof cannot be redirected to another object.
	consent, err := sobject.BuildSOLeaveRequest(id, before.GetConfig().GetConfigChainHash(), copy.GetPrivKey(), readerSession.GetPrivKey())
	if err != nil {
		t.Fatal(err)
	}
	redirected := consent.CloneVT()
	redirected.SharedObjectId = "another-object"
	if _, err := sobject.LeaveSOParticipants(ctx, object.GetSOHost(), object.GetPrivKey(), redirected); err == nil {
		t.Fatal("a leave proof was accepted for a different object")
	}

	// The owner acknowledges both removals, and the local provider adopts native revocation.
	if err := reader.LeaveSharedObject(ctx, readerSession.GetPrivKey(), id, ""); err != nil {
		t.Fatal(err)
	}
	after, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.GetConfig().GetParticipants()) != 1 || after.GetConfig().GetParticipants()[0].GetPeerId() != object.GetPeerID().String() {
		t.Fatalf("departure did not preserve only the remaining owner: %v", after.GetConfig().GetParticipants())
	}

	// The owner's retained history proves the departure.
	base, changes, err := object.ReadSharedObjectConfigHistory(ctx, after.GetConfig())
	if err != nil || len(changes) == 0 {
		t.Fatalf("owner lost native departure history: %v", err)
	}
	if err := sobject.VerifyConfigChainSuffix(id, base, after.GetConfig(), changes); err != nil {
		t.Fatalf("owner history does not prove current participation: %v", err)
	}

	// The departed copy holds the owner's configuration and no grants.
	retired, err := copy.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !retired.GetConfig().EqualVT(after.GetConfig()) || len(retired.GetRootGrants()) != 0 {
		t.Fatal("departing copy retained native authority after acknowledgment")
	}

	// The departed copy loses live participation and decryption.
	readable, releaseReadable, err := copy.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseReadable)
	if _, err := readable.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		_, err := snapshot.GetParticipantConfig(ctx)
		if !errors.Is(err, sobject.ErrNotParticipant) {
			return false, err
		}
		if _, err := snapshot.GetTransformer(ctx); err == nil {
			return false, errors.New("departed participant retained live decryption authority")
		}
		return true, nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	// The read checkpoint keeps the history the reader could read before leaving.
	checkpoint, err := copy.GetSharedObjectReadCheckpoint(ctx)
	if err != nil || checkpoint == nil {
		t.Fatalf("departed copy lost read checkpoint: %v", err)
	}
	if !checkpoint.Config.EqualVT(before.GetConfig()) {
		t.Fatal("checkpoint audience differs from the readable native snapshot")
	}
	if _, err := checkpoint.Snapshot.GetTransformer(ctx); err != nil {
		t.Fatalf("checkpoint lost its historical decryption grant: %v", err)
	}

	// Leaving again succeeds and keeps the local data.
	if err := reader.LeaveSharedObject(ctx, readerSession.GetPrivKey(), id, ""); err != nil {
		t.Fatalf("repeated departure failed: %v", err)
	}
	if !slices.ContainsFunc(reader.GetSOListCtr().GetValue().GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == id
	}) {
		t.Fatal("leave deleted retained local data")
	}

	// A fresh native invitation can grant access again.
	if _, err := reader.JoinViaInvite(ctx, readerSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	rejoined, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(rejoined.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool { return p.GetPeerId() == readerSession.GetPeerId().String() }) {
		t.Fatal("fresh invitation did not restore native device participation")
	}

	// A retried old departure acknowledges without undoing the new grant.
	acknowledged, err := sobject.LeaveSOParticipants(ctx, object.GetSOHost(), object.GetPrivKey(), consent)
	if err != nil {
		t.Fatalf("lost acknowledgment could not be recovered: %v", err)
	}
	if len(acknowledged.GetChanges()) != 1 {
		t.Fatal("departure retry exposed configuration changes after the acknowledged removal")
	}
	unchanged, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged.EqualVT(rejoined) {
		t.Fatal("an old departure removed newly granted participation")
	}
	if err := reader.LeaveSharedObject(ctx, readerSession.GetPrivKey(), id, ""); err != nil {
		t.Fatal(err)
	}

	// The sole remaining owner can leave a terminal signed configuration without deleting data.
	if err := owner.LeaveSharedObject(ctx, ownerSession.GetPrivKey(), id, ""); err != nil {
		t.Fatal(err)
	}
	closed, err := object.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(closed.GetConfig().GetParticipants()) != 0 || len(closed.GetRootGrants()) != 0 {
		t.Fatal("last departure retained native participant authority")
	}
	if err := closed.GetConfig().Validate(); err != nil {
		t.Fatalf("terminal signed configuration is invalid: %v", err)
	}
	if err := owner.LeaveSharedObject(ctx, ownerSession.GetPrivKey(), id, ""); err != nil {
		t.Fatal(err)
	}
}
