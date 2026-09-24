package provider_local

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// TestAccountMergeReturningSession keeps a source replica offline during merge,
// then removes the merging machine before that replica discovers the redirect.
func TestAccountMergeReturningSession(t *testing.T) {
	// Seed independent accounts with identical initial heads and distinct file payloads.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	a, aSession, _ := setupMigrationClient(ctx, t, network)
	b, bSession, _ := setupMigrationClient(ctx, t, network)
	c, cSession, cController := setupMigrationClient(ctx, t, network)
	var spaces []*sobject.SharedObjectRef
	var leaves []*block.BlockRef
	var payloads [][]byte
	for _, account := range []*ProviderAccount{a, b} {
		ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
		leaf, payload := seedAccountReplicaPayload(ctx, t, account, ref)
		spaces = append(spaces, ref)
		leaves = append(leaves, leaf)
		payloads = append(payloads, payload)
	}

	// C is an independent source-account Session with its own storage key.
	cSource, cSourceSession := enrollMeshReplica(ctx, t, b, bSession, c, cSession)
	cEntry, err := cController.RegisterSession(ctx, cSourceSession.GetSessionRef(), nil)
	if err != nil {
		t.Fatal(err)
	}
	waitReplicaCopy(ctx, t, cSource, spaces[1])
	cSource.StopP2PSync()
	cSource.StopSessionTransport()

	// Run the approved exchange so its owner controls the active Session's
	// attachment while the recovery watcher handles the offline replica.
	engines := []*pairing.Engine{pairingEngineForTest(t, aSession), pairingEngineForTest(t, bSession)}
	left, right := net.Pipe()
	t.Cleanup(func() {
		_ = left.Close()
		_ = right.Close()
	})
	go engines[0].StartOnStream(ctx, left, bSession.GetPeerId(), true, true, "")
	go engines[1].StartOnStream(ctx, right, aSession.GetPeerId(), false, true, "")
	for _, engine := range engines {
		waitForPairingStatus(ctx, t, engine, pairing.StatusSelectingAccount)
	}
	if err := engines[1].SelectAccount(pairing.AccountOutcome_AccountOutcome_MERGE_INTO_OFFERED); err != nil {
		t.Fatal(err)
	}
	for _, engine := range engines {
		waitForPairingStatus(ctx, t, engine, pairing.StatusVerifyingEmoji)
		engine.ConfirmSAS(true)
	}
	for _, engine := range engines {
		waitForPairingStatus(ctx, t, engine, pairing.StatusBothConfirmed)
	}

	// Retain the destination using the merging machine's original Session identity.
	mergedRef, err := engines[1].Result(aSession.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	merged, releaseDestination, err := b.t.p.AccessProviderAccount(ctx, a.GetAccountID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseDestination)
	destination := merged.(*ProviderAccount)
	destinationSession, releaseSession, err := destination.MountSession(ctx, mergedRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)
	if active := destination.GetSessionTransport(); active == nil || active.GetPeerID() != bSession.GetPeerId() {
		t.Fatal("merged account still connects with its temporary enrollment key")
	}

	// A second move must preserve the destination known to still-offline C.
	if _, err := destination.MergePairingAccount(ctx, destinationSession, c, cSession.GetSessionRef()); err == nil || !strings.Contains(err.Error(), "previous account merge") {
		t.Fatalf("second merge did not retain the offline Session recovery destination: %v", err)
	}

	// Retain the moved Space and signed recovery state before removing B.
	movedSpace := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), spaces[1].GetProviderResourceRef().GetId(), spaces[1].GetBlockStoreId())
	waitReplicaObject(ctx, t, a, movedSpace)
	waitReplicaCopy(ctx, t, a, movedSpace)
	sourceSettings, err := b.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recovery := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), sourceSettings.GetProviderResourceRef().GetId(), sourceSettings.GetBlockStoreId())
	waitReplicaObject(ctx, t, a, recovery)
	waitReplicaMigrationAdmission(ctx, t, a, sourceSettings)
	b.StopP2PSync()
	b.StopSessionTransport()
	destination.StopP2PSync()
	destination.StopSessionTransport()

	// Only A can now provide the source's signed recovery state and destination.
	if err := cSource.EnsureConfiguredSessionTransport(ctx, cSourceSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if err := cSource.StartPersistentP2PSync(ctx, cSource.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	var returned *session.SessionListEntry
	for {
		var changed <-chan struct{}
		cController.GetSessionBroadcast().HoldLock(func(_ func(), getWait func() <-chan struct{}) { changed = getWait() })
		returned, err = cController.GetSessionByIdx(ctx, cEntry.GetSessionIndex())
		if err != nil {
			t.Fatal(err)
		}
		if returned.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() == a.GetAccountID() {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("returning Session did not follow the committed account: %v", returned)
		case <-changed:
		}
	}

	// Complete both copies on the returning machine before removing its source.
	account, releaseAccount, err := c.t.p.AccessProviderAccount(ctx, a.GetAccountID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAccount)
	replica := account.(*ProviderAccount)
	for _, original := range spaces {
		ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), original.GetProviderResourceRef().GetId(), original.GetBlockStoreId())
		waitReplicaObject(ctx, t, replica, ref)
		waitReplicaCopy(ctx, t, replica, ref)
	}

	// Read each payload with every other replica disconnected.
	a.StopP2PSync()
	a.StopSessionTransport()
	for i, original := range spaces {
		ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), original.GetProviderResourceRef().GetId(), original.GetBlockStoreId())
		object, releaseObject, err := replica.MountSharedObject(ctx, ref, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, found, err := object.GetBlockStore().(*BlockStore).store.GetBlock(ctx, leaves[i])
		releaseObject()
		if err != nil || !found || !bytes.Equal(data, payloads[i]) {
			t.Fatalf("returning Session lacks an independent file copy: found=%v err=%v", found, err)
		}
	}
}

// waitReplicaMigrationAdmission waits for the authorization published after the recovery object.
func waitReplicaMigrationAdmission(ctx context.Context, t *testing.T, account *ProviderAccount, source *sobject.SharedObjectRef) {
	t.Helper()
	ref, err := account.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	object, release, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Object discovery precedes final admission; both must survive the source leaving.
	states, releaseStates, err := object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		settings, _, err := decodeAccountSettingsSnapshot(ctx, snapshot)
		if err != nil {
			return false, err
		}
		for _, migration := range settings.GetAcceptedMigrations() {
			if migration.GetSource().EqualVT(source.GetProviderResourceRef()) {
				return true, nil
			}
		}
		return false, nil
	}, nil); err != nil {
		t.Fatalf("replica did not retain migration admission: %v", err)
	}
}

// setupMigrationClient starts an independent account with retained Session routing.
func setupMigrationClient(ctx context.Context, t *testing.T, network *inproc.Network) (*ProviderAccount, *Session, session.SessionController) {
	t.Helper()

	// Connect the provider to the test network under its original Session key.
	tb, _, account, mounted, release := setupProviderAndSessionInternal(ctx, t, func(p *Provider) {
		p.localNetwork = transport.WithInprocNetwork(network)
	})
	t.Cleanup(release)
	if err := account.EnsureConfiguredSessionTransport(ctx, mounted.GetPrivKey()); err != nil {
		t.Fatal(err)
	}

	// Decode the file DAG used to prove that replicas retain independent copies.
	decoder := blocktype_controller.NewController(func(_ context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == "test/replica-payload" {
			return blocktype.NewBlockType(typeID, block_mock.NewRootBlock), nil
		}
		return nil, nil
	})
	releaseDecoder, err := tb.Bus.AddController(ctx, decoder, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseDecoder)

	// Register the mounted Session so production recovery can reattach its entry.
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, reference, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{VolumeId: tb.EngineVolumeID}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reference.Release)
	controller, lookup, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lookup.Release)
	if _, err := controller.RegisterSession(ctx, mounted.GetSessionRef(), nil); err != nil {
		t.Fatal(err)
	}
	return account, mounted, controller
}
