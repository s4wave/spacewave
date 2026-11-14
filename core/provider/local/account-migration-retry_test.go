package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	"github.com/s4wave/spacewave/core/bstore"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/s4wave/spacewave/testbed"
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

	// The source moves on with another write, then the retry merges it.
	laterLeaf, laterPayload := seedProviderReplicaPayload(ctx, t, source, source, ref, "written after the interrupted copy\n")
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
	data, found, err = copied.GetBlockStore().(*BlockStore).store.GetBlock(ctx, laterLeaf)
	if err != nil || !found || !bytes.Equal(data, laterPayload) {
		t.Fatalf("retry did not copy the write made after the interruption: found=%v err=%v", found, err)
	}

	// Check that the retry kept the listed generation and the catalog names it.
	targetSettings, err := target.readAccountSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := target.listedSharedObjectEntry(object.GetSharedObjectID())
	cataloged := targetSettings.FindCatalogEntry(object.GetSharedObjectID())
	if listed == nil || cataloged.GetDeleted() || cataloged.GetEntry().GetGeneration() != listed.GetGeneration() {
		t.Fatalf("retry left list generation %q and catalog record %v", listed.GetGeneration(), cataloged)
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
	change, err := sobject.BuildSOConfigChange(object.GetSharedObjectID(), state.GetConfig(), next, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, host.GetPrivKey(), nil)
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

// TestAccountMergeCopyHeldPastGrace checks, on a target Volume that frees
// what no root holds once its record is a nanosecond old, that a migrated
// Space nobody opens keeps its copied World through collection: the copy's
// stage holds the World until the import root does, and the import root
// holds it after. A block the Space does not hold is freed by the same
// collection, so the Volume does free.
func TestAccountMergeCopyHeldPastGrace(t *testing.T) {
	// Start a source account and a target whose Volume frees at once.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	source, sourceSession, _ := setupMigrationClient(ctx, t, network)
	storeConf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	target, targetSession, _ := setupMigrationClient(ctx, t, network, testbed.WithStorages(storage_inmem.NewInmemStorageWithStoreConfig("default", storeConf)))

	// Create a source Space holding one data block and migrate it.
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, source, ref)

	// Collect twice after the copy, while its stage alone holds the World,
	// so a copy under no stage, or a stage released before the hold, loses
	// it. StageCopy puts blocks only at a drain or its flush, so this is
	// also the state of a copy slower than the grace.
	target.migrationHook = collectAt(target, migrationCopied)
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatal(err)
	}
	target.migrationHook = nil

	// Mount the migrated object without opening its Space, and store a
	// block nothing holds beside its World.
	id := ref.GetProviderResourceRef().GetId()
	destinationRef := sobject.NewSharedObjectRef(target.GetProviderID(), target.GetAccountID(), id, SobjectBlockStoreID(id))
	copied, releaseCopied, err := target.MountSharedObject(ctx, destinationRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCopied()
	store := copied.GetBlockStore().(*BlockStore)
	unheld, _, err := store.store.PutBlock(ctx, []byte("unheld beside the migrated World"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}

	// Collect past the grace.
	for range 2 {
		target.triggerGCCleanup()
		if err := target.WaitGCCleanup(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// The unheld block is freed, the copied block reads, and the import root
	// holds the World.
	if _, found, err := store.store.GetBlock(ctx, unheld); err != nil || found {
		t.Fatalf("collection kept a block nothing holds: found=%v err=%v", found, err)
	}
	data, found, err := store.store.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("collection freed the migrated Space's block: found=%v err=%v", found, err)
	}
	held, err := store.GetRoot(ctx, space_world.RootImport)
	if err != nil || held.GetEmpty() {
		t.Fatalf("import root after the migration = %v, %v; want it to hold the World", held, err)
	}
}

// collectAt returns a migration hook that collects the account's Volume twice
// at step, so a record staged before the first is past its grace by the
// second.
func collectAt(account *ProviderAccount, at migrationStep) func(context.Context, migrationStep) error {
	return func(ctx context.Context, step migrationStep) error {
		if step != at {
			return nil
		}
		return collectTwice(ctx, account)
	}
}

// collectTwice collects the account's Volume twice.
func collectTwice(ctx context.Context, account *ProviderAccount) error {
	for range 2 {
		if _, err := account.collectGCRootlessBlocks(ctx); err != nil {
			return err
		}
	}
	return nil
}

// TestAccountMergeOpenAfterMigration checks, on a target Volume that frees at
// once, that opening a migrated Space moves the replay base onto the copied
// World and releases it from the import root, and that its data survives
// collection after.
func TestAccountMergeOpenAfterMigration(t *testing.T) {
	// Migrate a Space onto a target whose Volume frees at once.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	source, sourceSession, _ := setupMigrationClient(ctx, t, network)
	storeConf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	target, targetSession, _ := setupMigrationClient(ctx, t, network, testbed.WithStorages(storage_inmem.NewInmemStorageWithStoreConfig("default", storeConf)))
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, source, ref)
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatal(err)
	}

	// The import root holds the copied World.
	id := ref.GetProviderResourceRef().GetId()
	destinationRef := sobject.NewSharedObjectRef(target.GetProviderID(), target.GetAccountID(), id, SobjectBlockStoreID(id))
	copied, releaseCopied, err := target.MountSharedObject(ctx, destinationRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCopied()
	store := copied.GetBlockStore().(*BlockStore)
	worlds := readImportRoots(ctx, t, store)
	if len(worlds) != 1 {
		t.Fatalf("import root after the migration = %v, want the copied World", worlds)
	}
	migrated := worlds[0]

	// Open the Space and wait for its install to move the replay base onto
	// the copied World and release it from the import root.
	b := target.t.p.b
	_, engineRef, err := world.StartEngine(ctx, b, space_world.NewConfig("migrated-world", destinationRef), space_world.NewFactory(b, space_world.OpTypes{}))
	if err != nil {
		t.Fatal(err)
	}
	defer engineRef.Release()
	for {
		base, err := store.GetRoot(ctx, space_world.RootReplayBase)
		if err != nil {
			t.Fatal(err)
		}
		held := readImportRoots(ctx, t, store)
		if base.EqualsRef(migrated) && !slices.ContainsFunc(held, migrated.EqualsRef) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("engine left replay base %v and import root %v; want the base on the copied World %v", base, held, migrated)
		case <-time.After(20 * time.Millisecond):
		}
	}

	// The copied data survives collection under the replay base.
	if err := collectTwice(ctx, target); err != nil {
		t.Fatal(err)
	}
	data, found, err := store.store.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("collection freed the opened Space's block: found=%v err=%v", found, err)
	}
}

// TestAccountMergeHoldBeforeListing checks that an import that fails after
// its hold and before its listing leaves the id unlisted with the copied
// World held through collection, and that a retry completes.
func TestAccountMergeHoldBeforeListing(t *testing.T) {
	// Start a source account and a target whose Volume frees at once.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	source, sourceSession, _ := setupMigrationClient(ctx, t, network)
	storeConf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	target, targetSession, _ := setupMigrationClient(ctx, t, network, testbed.WithStorages(storage_inmem.NewInmemStorageWithStoreConfig("default", storeConf)))
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, payload := seedAccountReplicaPayload(ctx, t, source, ref)

	// Fail the import at the listing, after the hold.
	errListing := errors.New("lost before the listing")
	target.migrationHook = func(_ context.Context, step migrationStep) error {
		if step == migrationHeld {
			return errListing
		}
		return nil
	}
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err == nil || !strings.Contains(err.Error(), errListing.Error()) {
		t.Fatalf("merge = %v, want the injected failure", err)
	}
	target.migrationHook = nil

	// The id is unlisted, and its bucket holds the World through
	// collection.
	id := ref.GetProviderResourceRef().GetId()
	if target.listedSharedObjectEntry(id) != nil {
		t.Fatal("a failed import listed the object")
	}
	destinationRef := sobject.NewSharedObjectRef(target.GetProviderID(), target.GetAccountID(), id, SobjectBlockStoreID(id))
	blocksRef := &bstore.BlockStoreRef{ProviderResourceRef: destinationRef.GetProviderResourceRef().CloneVT()}
	blocksRef.ProviderResourceRef.Id = SobjectBlockStoreID(id)
	blocks, releaseBlocks, err := target.MountBlockStore(ctx, blocksRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBlocks()
	if worlds := readImportRoots(ctx, t, blocks); len(worlds) != 1 {
		t.Fatalf("import root after the failed import = %v, want the copied World", worlds)
	}
	if err := collectTwice(ctx, target); err != nil {
		t.Fatal(err)
	}
	if _, found, err := blocks.GetBlock(ctx, leaf); err != nil || !found {
		t.Fatalf("collection freed the held World's block: found=%v err=%v", found, err)
	}

	// A retry completes, and the import root still holds one World.
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if target.listedSharedObjectEntry(id) == nil {
		t.Fatal("the retry did not list the object")
	}
	if worlds := readImportRoots(ctx, t, blocks); len(worlds) != 1 {
		t.Fatalf("import root after the retry = %v, want the one copied World", worlds)
	}
	data, found, err := blocks.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("retry lost the copied block: found=%v err=%v", found, err)
	}
}

// readImportRoots returns the roots the import root of store holds, newest
// first.
func readImportRoots(ctx context.Context, t *testing.T, store block.StoreOps) []*block.BlockRef {
	t.Helper()
	held, err := store.GetRoot(ctx, space_world.RootImport)
	if err != nil {
		t.Fatal(err)
	}
	var worlds []*block.BlockRef
	for !held.GetEmpty() {
		data, found, err := store.GetBlock(ctx, held)
		if err != nil || !found {
			t.Fatalf("read import root block: found=%v err=%v", found, err)
		}
		roots := &space_world.HeldRoots{}
		if err := roots.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		worlds = append(worlds, roots.GetRoots()...)
		held = roots.GetPrev()
	}
	return worlds
}
