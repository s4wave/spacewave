package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// interruptedMigrationAccount loses its caller after one durable resource copy.
// All authorization, storage, and retry operations use the native provider.
type interruptedMigrationAccount struct {
	*ProviderAccount
	interrupt bool
}

func (a *interruptedMigrationAccount) ImportMigrationObject(ctx context.Context, source provider_migration.Account, object sobject.SharedObject, entry *sobject.SharedObjectListEntry, state *sobject.SOState) error {
	if err := a.ProviderAccount.ImportMigrationObject(ctx, source, object, entry, state); err != nil {
		return err
	}
	if a.interrupt {
		a.interrupt = false
		return context.Canceled
	}
	return nil
}

// TestAccountMergeInterruptedCopy preserves the source through a lost receipt,
// then retries the committed resource without changing external reader access.
func TestAccountMergeInterruptedCopy(t *testing.T) {
	// Start source and target accounts on one in-process network.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	source, sourceSession, _ := setupMigrationClient(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)

	// Create a source Space holding one data block.
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, source, ref)
	object, release, err := source.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Add an external reader to the Space.
	host := object.(sobject.InviteHost)
	_, external, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	externalID, err := peer.IDFromPublicKey(external)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sobject.AddSOParticipant(ctx, host.GetSOHost(), object.GetSharedObjectID(), host.GetPrivKey(), object.GetPeerID().String(), externalID.String(), external, sobject.SOParticipantRole_SOParticipantRole_READER, "external-reader", ""); err != nil {
		t.Fatal(err)
	}

	// Interrupt the first merge and check that the source keeps its data.
	interrupted := &interruptedMigrationAccount{ProviderAccount: target, interrupt: true}
	if _, err := source.MergePairingAccount(ctx, sourceSession, interrupted, targetSession.GetSessionRef()); err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("copy did not report interruption: %v", err)
	}
	settings, err := source.readAccountSettings(ctx)
	if err != nil || settings.GetTransition() != nil {
		t.Fatalf("incomplete copy redirected the source: %v, %v", settings.GetTransition(), err)
	}
	data, found, err := object.GetBlockStore().(*BlockStore).store.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("interrupted copy lost source data: found=%v err=%v", found, err)
	}

	// Retry the merge and check the copied data on the target.
	if _, err := source.MergePairingAccount(ctx, sourceSession, interrupted, targetSession.GetSessionRef()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	destinationRef := sobject.NewSharedObjectRef(target.GetProviderID(), target.GetAccountID(), object.GetSharedObjectID(), SobjectBlockStoreID(object.GetSharedObjectID()))
	copied, releaseCopied, err := target.MountSharedObject(ctx, destinationRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCopied()
	data, found, err = copied.GetBlockStore().(*BlockStore).store.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("retry did not retain destination data: found=%v err=%v", found, err)
	}

	// Check that the copy kept the external reader's access.
	state, err := copied.(sobject.InviteHost).GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() == externalID.String() {
			if participant.GetRole() != sobject.SOParticipantRole_SOParticipantRole_READER || participant.GetEntityId() != "external-reader" {
				t.Fatalf("external reader permission changed: %v", participant)
			}
			return
		}
	}
	t.Fatal("migration removed the external reader")
}

// TestAccountMergeExternalOwnerBlocker uses a signed ownership change to prove
// that a reader cannot silently omit or grant a third party's Space.
func TestAccountMergeExternalOwnerBlocker(t *testing.T) {
	// Start source and target accounts on one in-process network.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	source, sourceSession, _ := setupMigrationClient(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)

	// Create and mount a source Space.
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	object, release, err := source.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Add an external owner to the Space.
	host := object.(sobject.InviteHost)
	_, external, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	externalID, err := peer.IDFromPublicKey(external)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sobject.AddSOParticipant(ctx, host.GetSOHost(), object.GetSharedObjectID(), host.GetPrivKey(), object.GetPeerID().String(), externalID.String(), external, sobject.SOParticipantRole_SOParticipantRole_OWNER, "external-owner", ""); err != nil {
		t.Fatal(err)
	}

	// Demote every other participant to reader in a signed config change.
	state, err := host.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := state.GetConfig().CloneVT()
	for _, participant := range next.GetParticipants() {
		if participant.GetPeerId() != externalID.String() {
			participant.Role = sobject.SOParticipantRole_SOParticipantRole_READER
		}
	}
	change, err := sobject.BuildSOConfigChange(state.GetConfig(), next, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, host.GetPrivKey(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.GetSOHost().ApplyConfigChange(ctx, change, nil); err != nil {
		t.Fatal(err)
	}

	// Check that the merge stops on the external owner and changes nothing.
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err == nil || !strings.Contains(err.Error(), "owner must authorize") || !strings.Contains(err.Error(), object.GetSharedObjectID()) {
		t.Fatalf("migration did not identify the externally owned Space: %v", err)
	}
	settings, err := source.readAccountSettings(ctx)
	if err != nil || settings.GetTransition() != nil {
		t.Fatalf("blocked merge redirected the source: %v, %v", settings.GetTransition(), err)
	}
	for _, entry := range target.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == object.GetSharedObjectID() {
			t.Fatal("blocked merge imported the externally owned Space")
		}
	}
}
