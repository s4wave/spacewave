package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	"github.com/s4wave/spacewave/core/bstore"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_engine "github.com/s4wave/spacewave/core/space/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
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
	if _, _, _, err := sobject.AddSOParticipant(ctx, host.GetSOHost(), object.GetSharedObjectID(), host.GetPrivKey(), object.GetPeerID().String(), externalID.String(), external, sobject.SOParticipantRole_SOParticipantRole_READER, "external-reader", ""); err != nil {
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
	if _, _, _, err := sobject.AddSOParticipant(ctx, host.GetSOHost(), object.GetSharedObjectID(), host.GetPrivKey(), object.GetPeerID().String(), externalID.String(), external, sobject.SOParticipantRole_SOParticipantRole_OWNER, "external-owner", ""); err != nil {
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

// TestAccountMergeRefusedOpAboveCheckpoint migrates a Space whose first
// operation above its checkpoint has a type nothing resolves or declares,
// followed by a create. The export skips the refused operation's payloads, and
// the target replays past it to the create.
func TestAccountMergeRefusedOpAboveCheckpoint(t *testing.T) {
	// Start a source and a target account running no plugin.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	sourceTB, source, sourceSession, _ := setupMigrationTestbed(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)

	// Write the unresolved operation, then the create, above the checkpoint.
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	so := pauseAboveCheckpoint(ctx, t, sourceTB, source, ref)
	missing := &payloadOp{MockWorldOp: world_mock.MockWorldOp{ObjectKey: "base", NextMsg: absentRef(t).MarshalString()}}
	after := putPayload(ctx, t, so, "after")
	queueTxs(ctx, t, so, applyWorldOpTx(t, so, missing), createTx(t, "after", after.ref))

	// The merge copies the Space without the refused operation's payload.
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatal(err)
	}

	// The target replays past the refused operation to the create.
	store, _ := openMigratedSpace(ctx, t, target, ref, "after")
	data, found, err := store.store.GetBlock(ctx, after.ref)
	if err != nil || !found || !bytes.Equal(data, after.data) {
		t.Fatalf("migration lost the create's payload: found=%v err=%v", found, err)
	}
}

// TestAccountMergeOpsAboveCheckpoint migrates a Space whose operations above
// its checkpoint are a create and then a running plugin's operation on the
// created object, onto a target whose Volume frees at once, collecting between
// the copy and the listing. The target replays both and reads every payload
// after more collection.
func TestAccountMergeOpsAboveCheckpoint(t *testing.T) {
	// Start a source and a target account running the plugin, the target's
	// Volume freeing at once.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	sourceTB, source, sourceSession, _ := setupMigrationTestbed(ctx, t, network)
	storeConf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	target, targetSession, _ := setupMigrationClient(ctx, t, network, testbed.WithStorages(storage_inmem.NewInmemStorageWithStoreConfig("default", storeConf)))
	defer startPayloadOpPlugin(ctx, t, sourceTB.Bus)()
	defer startPayloadOpPlugin(ctx, t, target.t.p.b)()

	// Write the create and the plugin's operation above the checkpoint.
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	so := pauseAboveCheckpoint(ctx, t, sourceTB, source, ref)
	payloads := writeCreateThenPluginOp(ctx, t, so)

	// Migrate, collecting twice while the copy's stage alone holds the
	// payloads.
	target.migrationHook = collectAt(target, migrationCopied)
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatal(err)
	}
	target.migrationHook = nil

	// Replay at the target until the plugin's operation applied, collect
	// again past the grace, then read every payload.
	store, _ := openMigratedSpace(ctx, t, target, ref, "")
	if err := collectTwice(ctx, target); err != nil {
		t.Fatal(err)
	}
	for name, payload := range payloads {
		data, found, err := store.store.GetBlock(ctx, payload.ref)
		if err != nil || !found || !bytes.Equal(data, payload.data) {
			t.Fatalf("migration lost the %s payload: found=%v err=%v", name, found, err)
		}
	}
}

// TestAccountMergeUnloadedOpPlugin checks that a Space with a create and then
// a plugin's operation above its checkpoint, whose plugin is not running, fails
// to migrate before the target lists it, and the source keeps its Session. The
// World before the plugin's operation is one no replay recorded, so the export
// cannot tell whether a plugin the create installed would resolve it.
func TestAccountMergeUnloadedOpPlugin(t *testing.T) {
	// Write the operations with no plugin running.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	sourceTB, source, sourceSession, _ := setupMigrationTestbed(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	writeCreateThenPluginOp(ctx, t, pauseAboveCheckpoint(ctx, t, sourceTB, source, ref))

	// The merge fails naming the unresolved operation, before any listing.
	_, err = source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef())
	if !errors.Is(err, space_world.ErrUnresolvedOp) {
		t.Fatalf("merge without the plugin = %v, want %v", err, space_world.ErrUnresolvedOp)
	}
	if listed := target.listedSharedObjectEntry(ref.GetProviderResourceRef().GetId()); listed != nil {
		t.Fatalf("target listed the Space it could not copy: %v", listed)
	}
	settings, err := source.readAccountSettings(ctx)
	if err != nil || settings.GetTransition() != nil {
		t.Fatalf("failed migration redirected the source: %v, %v", settings.GetTransition(), err)
	}
}

// TestAccountMergeSavedRefusal migrates a Space whose source replay saved a
// cursor recording a create and then a refused operation of a type nothing
// resolves or declares, whose payload no store holds, batched with a create
// whose root the source holds. The export follows the cursor past the create,
// copies the refused transaction's object root but not the payload it cannot
// name, and the target replays it as refused.
func TestAccountMergeSavedRefusal(t *testing.T) {
	// Start a source and a target account running no plugin.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	_, source, sourceSession, _ := setupMigrationTestbed(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Replay the operations at the source until it saves a cursor recording
	// the refusal.
	cursor, _, root := saveRefusalCursor(ctx, t, source, ref, false)
	if cursor.GetOutcomes()[0].GetReason() != "" || cursor.GetOutcomes()[1].GetReason() == "" {
		t.Fatalf("saved outcomes %v, want the create applied and the operation refused", cursor.GetOutcomes()[:2])
	}

	// The merge copies the Space without the refused operation's payload.
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatal(err)
	}

	// The target replays past the refused operation without applying it, and
	// holds the refused transaction's object root.
	store, eng := openMigratedSpace(ctx, t, target, ref, refusalFillerKey(refusalFillers-1))
	if replayedTo(ctx, t, eng, "") || replayedTo(ctx, t, eng, "y") {
		t.Fatal("the target applied the refused operation")
	}
	data, found, err := store.store.GetBlock(ctx, root.ref)
	if err != nil || !found || !bytes.Equal(data, root.data) {
		t.Fatalf("migration lost the refused transaction's object root: found=%v err=%v", found, err)
	}
}

// TestAccountMergeReorderedCursor checks that a saved cursor whose outcomes no
// longer match the operations by position and hash, as after a reorder, is
// ignored: the refused operation after the create falls to the export's own
// rule, which cannot know the World before it, and the merge fails with
// ErrUnresolvedOp before the target lists anything.
func TestAccountMergeReorderedCursor(t *testing.T) {
	// Save the cursor recording the refusal at the source.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	_, source, sourceSession, _ := setupMigrationTestbed(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, _ := saveRefusalCursor(ctx, t, source, ref, false)

	// Swap the first two outcomes' operations, as a reorder would place them.
	outcomes := cursor.GetOutcomes()
	outcomes[0].Hash, outcomes[1].Hash = outcomes[1].GetHash(), outcomes[0].GetHash()
	writeSavedCursor(ctx, t, source, ref, cursor)

	// The merge fails naming the unresolved operation, before any listing.
	_, err = source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef())
	if !errors.Is(err, space_world.ErrUnresolvedOp) {
		t.Fatalf("merge with a reordered cursor = %v, want %v", err, space_world.ErrUnresolvedOp)
	}
	if listed := target.listedSharedObjectEntry(ref.GetProviderResourceRef().GetId()); listed != nil {
		t.Fatalf("target listed the Space it could not copy: %v", listed)
	}
}

// TestAccountMergeSavedRefusalResolved migrates a Space whose source replay
// saved a cursor recording an operation as refused for want of a handler, and
// whose plugin runs on both accounts by the merge. The export copies the
// operation's payload, and the target, which has the handler, applies it.
func TestAccountMergeSavedRefusalResolved(t *testing.T) {
	// Save the cursor recording the refusal with no plugin running.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	network := inproc.NewNetwork()
	sourceTB, source, sourceSession, _ := setupMigrationTestbed(ctx, t, network)
	target, targetSession, _ := setupMigrationClient(ctx, t, network)
	ref, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cursor, payload, _ := saveRefusalCursor(ctx, t, source, ref, true)
	if cursor.GetOutcomes()[1].GetReason() == "" {
		t.Fatalf("saved outcome %v, want the operation refused", cursor.GetOutcomes()[1])
	}

	// Start the plugin on both accounts and migrate.
	defer startPayloadOpPlugin(ctx, t, sourceTB.Bus)()
	defer startPayloadOpPlugin(ctx, t, target.t.p.b)()
	if _, err := source.MergePairingAccount(ctx, sourceSession, target, targetSession.GetSessionRef()); err != nil {
		t.Fatal(err)
	}

	// The target applies the operation and holds its payload.
	store, _ := openMigratedSpace(ctx, t, target, ref, "")
	data, found, err := store.store.GetBlock(ctx, payload.ref)
	if err != nil || !found || !bytes.Equal(data, payload.data) {
		t.Fatalf("migration lost the refused operation's payload: found=%v err=%v", found, err)
	}
}

// refusalFillers is how many creates saveRefusalCursor writes after the
// refused operation, enough to drive the replay past the cursor's save bound.
const refusalFillers = 2 * sobject.MinCheckpointOperations

// refusalFillerKey returns the key of the i-th create after the refused
// operation.
func refusalFillerKey(i int) string {
	return "filler-" + strconv.Itoa(i)
}

// saveRefusalCursor runs a Space world Adapter on the Space at ref with
// account's owner as its sequencer until its World is checkpointed, then holds
// every later checkpoint and writes a create of the object "z", a payloadOp on
// it whose type nothing resolves or declares, batched with a create of the
// object "y", and refusalFillers creates. The payloadOp's payload is stored in
// the Space's block store when stored is set, and in no store otherwise; the
// root of "y" is always stored. It waits for the Adapter to save a replay
// cursor recording the first two transactions, stops the Adapter, and returns
// the cursor, the payload and the root of "y".
func saveRefusalCursor(ctx context.Context, t *testing.T, account *ProviderAccount, ref *sobject.SharedObjectRef, stored bool) (*space_world.ReplayCursor, seededPayload, seededPayload) {
	// Hold the Space as its sequencer and run the Adapter as the Space
	// controller does.
	t.Helper()
	mounted, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSO)
	so := mounted.(*SharedObject)
	if _, err := so.SetSequencer(ctx, so.GetPeerID().String()); err != nil {
		t.Fatal(err)
	}
	held := new(atomic.Bool)
	a, err := space_world.NewAdapter(logrus.NewEntry(logrus.New()), heldRootSO{SharedObject: so, held: held}, space_world.AdapterOptions{
		Bus:            account.t.p.b,
		StepFactorySet: account.t.p.sfs,
		EngineID:       space.SpaceEngineId(ref),
		OpTypes:        space_world_engine.OpTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- a.Execute(runCtx) }()
	go func() { _ = sobject.Sequence(runCtx, so, so.SequenceOperations) }()
	defer func() {
		stop()
		<-done
	}()

	// Wait for the checkpoint over the initialized World, then hold the
	// rest.
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		cp, err := snapshot.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		head := &space_world.InnerState{}
		if err := head.UnmarshalVT(cp.GetStateData()); err != nil {
			return false, err
		}
		return !head.GetHeadRef().GetEmpty(), nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	held.Store(true)

	// Write the create, the refused operation and the creates after it.
	z := putPayload(ctx, t, so, "z")
	filler := putPayload(ctx, t, so, "filler")
	payload := seededPayload{ref: absentRef(t)}
	if stored {
		payload = putPayload(ctx, t, so, "refused")
	}
	refused := &payloadOp{MockWorldOp: world_mock.MockWorldOp{ObjectKey: "z", NextMsg: payload.ref.MarshalString()}}
	root := putPayload(ctx, t, so, "y")
	batch, err := world_block_tx.NewTxBatch(&world_block_tx.TxBatch{Txs: []*world_block_tx.Tx{applyWorldOpTx(t, so, refused), createTx(t, "y", root.ref)}})
	if err != nil {
		t.Fatal(err)
	}
	txs := []*world_block_tx.Tx{createTx(t, "z", z.ref), batch}
	for i := range refusalFillers {
		txs = append(txs, createTx(t, refusalFillerKey(i), filler.ref))
	}
	queueTxs(ctx, t, so, txs...)

	// Wait for the saved cursor recording the first two.
	for {
		cursor := readSavedCursor(ctx, t, so)
		if len(cursor.GetOutcomes()) >= 2 {
			return cursor, payload, root
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the source saved no replay cursor: %v", context.Cause(ctx))
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// heldRootSO is a SharedObject whose bodies return no checkpoint state while
// held is set, so the sequencer signs no checkpoint then.
type heldRootSO struct {
	*SharedObject
	// held holds the checkpoints while set.
	held *atomic.Bool
}

// SetBody attaches body through heldRootBody.
func (s heldRootSO) SetBody(ctx context.Context, body sobject.Body) (func(), error) {
	return s.SharedObject.SetBody(ctx, heldRootBody{Body: body, held: s.held})
}

// heldRootBody is a body returning no checkpoint state while held is set.
type heldRootBody struct {
	sobject.Body
	// held holds the checkpoints while set.
	held *atomic.Bool
}

// Root returns the body's state, or none while held.
func (b heldRootBody) Root(ctx context.Context, n int) ([]byte, func(context.Context) error, error) {
	if b.held.Load() {
		return nil, nil, nil
	}
	return b.Body.Root(ctx, n)
}

// savedCursorStore and savedCursorKey locate the replay cursor the Space
// world Adapter saves in a SharedObject's local state.
const savedCursorStore, savedCursorKey = "space-world-replay", "cursor"

// readSavedCursor returns the replay cursor the Adapter of so saved, or nil.
func readSavedCursor(ctx context.Context, t *testing.T, so sobject.SharedObject) *space_world.ReplayCursor {
	t.Helper()
	store, release, err := so.AccessLocalStateStore(ctx, savedCursorStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(savedCursorKey))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		return nil
	}
	cursor := &space_world.ReplayCursor{}
	if err := cursor.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	return cursor
}

// writeSavedCursor replaces the replay cursor saved for the Space at ref.
func writeSavedCursor(ctx context.Context, t *testing.T, account *ProviderAccount, ref *sobject.SharedObjectRef, cursor *space_world.ReplayCursor) {
	t.Helper()
	data, err := cursor.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	so, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()
	store, release, err := so.AccessLocalStateStore(ctx, savedCursorStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(savedCursorKey), data); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// seededPayload is a payload an operation names and its bytes.
type seededPayload struct {
	// ref is the payload's root.
	ref *block.BlockRef
	// data is the root's bytes.
	data []byte
}

// writeCreateThenPluginOp writes to so a create of the object "z" and then a
// payloadOp on it, each naming a payload, and returns each payload by the
// operation that names it.
func writeCreateThenPluginOp(ctx context.Context, t *testing.T, so *SharedObject) map[string]seededPayload {
	t.Helper()
	z := putPayload(ctx, t, so, "z")
	plugin := putPayload(ctx, t, so, "plugin")
	op := &payloadOp{MockWorldOp: world_mock.MockWorldOp{ObjectKey: "z", NextMsg: plugin.ref.MarshalString()}}
	queueTxs(ctx, t, so, createTx(t, "z", z.ref), applyWorldOpTx(t, so, op))
	return map[string]seededPayload{"z": z, "plugin": plugin}
}

// pausedBody is a Space body that never finishes applying an operation, so
// the sequencer signs no checkpoint over operations written while it is
// attached.
type pausedBody struct{}

// Root waits for ctx.
func (pausedBody) Root(ctx context.Context, _ int) ([]byte, func(context.Context) error, error) {
	<-ctx.Done()
	return nil, nil, context.Cause(ctx)
}

// Apply waits for ctx.
func (pausedBody) Apply(ctx context.Context, _ int, _ *sobject.OrderedOperation) (string, error) {
	<-ctx.Done()
	return "", context.Cause(ctx)
}

// Install keeps no checkpoint.
func (pausedBody) Install(context.Context, *sobject.SOCheckpointInner, int) (bool, error) {
	return false, nil
}

// Hold holds nothing.
func (pausedBody) Hold(context.Context, *sobject.OrderEvent) error {
	return nil
}

// pauseAboveCheckpoint mounts the Space at ref for the rest of the test with
// account's owner as its sequencer, creates the object "base" through the
// Space body, waits for the checkpoint over it, then swaps the body for a
// pausedBody. Operations written after it stay above the checkpoint.
func pauseAboveCheckpoint(ctx context.Context, t *testing.T, tb *testbed.Testbed, account *ProviderAccount, ref *sobject.SharedObjectRef) *SharedObject {
	// Hold the Space and checkpoint as its owner.
	t.Helper()
	mounted, releaseSO, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSO)
	so := mounted.(*SharedObject)
	if _, err := so.SetSequencer(ctx, so.GetPeerID().String()); err != nil {
		t.Fatal(err)
	}

	// Create the object through the Space body.
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	_, controller, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Release()
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()
	engine := body.GetSharedObjectBody().GetWorldEngine()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, "base", &bucket.ObjectRef{})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var derived *block.BlockRef
	if err := engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		derived = cursor.GetRef().GetRootRef().CloneVT()
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for the checkpoint over the create.
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		cp, err := snapshot.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		head := &space_world.InnerState{}
		if err := head.UnmarshalVT(cp.GetStateData()); err != nil {
			return false, err
		}
		return !derived.GetEmpty() && head.GetHeadRef().GetRootRef().EqualVT(derived), nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Swap the Space body for the paused one once it detaches.
	bodyRef.Release()
	controller.Release()
	releaseBody, err := so.SetBody(ctx, pausedBody{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseBody)
	return so
}

// putPayload stores bytes named by key outside the World of so.
func putPayload(ctx context.Context, t *testing.T, so sobject.SharedObject, key string) seededPayload {
	t.Helper()
	data := bytes.Repeat([]byte("payload "+key+"\n"), 4096)
	ref, _, err := so.GetBlockStore().PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	return seededPayload{ref: ref, data: data}
}

// absentRef returns the ref of a block no store holds.
func absentRef(t *testing.T) *block.BlockRef {
	t.Helper()
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// createTx returns the transaction creating the object key with root ref.
func createTx(t *testing.T, key string, ref *block.BlockRef) *world_block_tx.Tx {
	t.Helper()
	tx, err := world_block_tx.NewTxCreateObject(key, &bucket.ObjectRef{RootRef: ref})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// applyWorldOpTx returns the transaction applying op as the owner of so.
func applyWorldOpTx(t *testing.T, so sobject.SharedObject, op world.Operation) *world_block_tx.Tx {
	t.Helper()
	tx, err := world_block_tx.NewTxApplyWorldOp(op, so.GetPeerID())
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// queueTxs queues each transaction as a World operation of so, in order.
func queueTxs(ctx context.Context, t *testing.T, so *SharedObject, txs ...*world_block_tx.Tx) {
	t.Helper()
	for _, tx := range txs {
		data, err := (&space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{Tx: tx}}}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := so.QueueOperation(ctx, data); err != nil {
			t.Fatal(err)
		}
	}
}

// openMigratedSpace mounts the Space ref migrated to target for the rest of
// the test, starts its World engine, and waits until its World holds the
// object key, or for an empty key until the object "z" carries payloadOp's
// link. It returns the Space's block store and its World engine.
func openMigratedSpace(ctx context.Context, t *testing.T, target *ProviderAccount, ref *sobject.SharedObjectRef, key string) (*BlockStore, world.Engine) {
	// Mount the copy and open its World.
	t.Helper()
	id := ref.GetProviderResourceRef().GetId()
	destinationRef := sobject.NewSharedObjectRef(target.GetProviderID(), target.GetAccountID(), id, SobjectBlockStoreID(id))
	copied, releaseCopied, err := target.MountSharedObject(ctx, destinationRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseCopied)
	b := target.t.p.b
	eng, engineRef, err := world.StartEngine(ctx, b, space_world.NewConfig("migrated-world", destinationRef), space_world.NewFactory(b, space_world_engine.OpTypes))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engineRef.Release)

	// Wait for the replay to reach the operation.
	for !replayedTo(ctx, t, eng, key) {
		select {
		case <-ctx.Done():
			t.Fatalf("target replay did not reach %q: %v", key, context.Cause(ctx))
		case <-time.After(20 * time.Millisecond):
		}
	}
	return copied.GetBlockStore().(*BlockStore), eng
}

// replayedTo reports whether the World of eng holds the object key, or for an
// empty key whether the object "z" carries payloadOp's link.
func replayedTo(ctx context.Context, t *testing.T, eng world.Engine, key string) bool {
	t.Helper()
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if key != "" {
		obj, found, err := tx.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("z", payloadOpTypeID, "z", ""), 1)
	if err != nil {
		t.Fatal(err)
	}
	return len(quads) != 0
}

// payloadOpTypeID is the type of payloadOp, which only its running plugin
// resolves.
const payloadOpTypeID = "test/migration-payload-op"

// payloadOp is a plugin's World operation naming a payload written outside
// the World, whose ref its message encodes. Its apply links its object to
// itself, since a transaction that leaves the World unchanged proposes no
// operation.
type payloadOp struct {
	world_mock.MockWorldOp
}

// TypeID returns the operation type identifier.
func (*payloadOp) TypeID() string {
	return payloadOpTypeID
}

// ApplyWorldOp links the operation's object to itself.
func (o *payloadOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	key := o.GetObjectKey()
	return false, ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, payloadOpTypeID, key, ""))
}

// GetPayloadRefs returns the payload the message encodes.
func (o *payloadOp) GetPayloadRefs() []*block.BlockRef {
	ref, err := block.UnmarshalBlockRefB58(o.GetNextMsg())
	if err != nil || ref == nil {
		return nil
	}
	return []*block.BlockRef{ref}
}

// startPayloadOpPlugin registers payloadOp's handler on b for every engine, as
// its plugin does while it runs, and returns the release that stops it.
func startPayloadOpPlugin(ctx context.Context, t *testing.T, b bus.Bus) func() {
	t.Helper()
	lookup := func(_ context.Context, opTypeID string) (world.Operation, error) {
		if opTypeID != payloadOpTypeID {
			return nil, nil
		}
		return &payloadOp{}, nil
	}
	release, err := b.AddController(ctx, world.NewLookupOpController("test-payload-op-plugin", "", lookup), nil)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// _ is a type assertion
var _ world.PayloadOperation = (*payloadOp)(nil)
