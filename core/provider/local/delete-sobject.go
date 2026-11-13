package provider_local

import (
	"bytes"
	"context"
	"os"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// DeleteStateEnv names the environment variable that, set to "off", turns off
// the removal of a deleted object's stored state and the paths that keep
// stored state in step with the list:
//   - delete unlists the object and releases its block roots, but keeps its
//     keys under so/<id>/ and does not restart its live mount;
//   - create writes only the list, without clearing the prefix or writing a
//     genesis;
//   - a mount of an unlisted id is not refused, and a mount that finds no
//     stored state writes a genesis without clearing the prefix.
//
// It does not turn off what loses no data and keeps handles honest: the host
// key checks on state and local state writes, the refusals of an ended mount's
// reads and writes, the released callback of a mount, the placeholder genesis
// of a listed object with no state, the transfer refusal of an unlisted id,
// and the catalog generations with their deleted sets.
const DeleteStateEnv = "SPACEWAVE_SOBJECT_DELETE_STATE"

// deleteStateOff selects DeleteStateEnv's old behavior. It is read once, here,
// at startup.
var deleteStateOff = os.Getenv(DeleteStateEnv) == "off"

// Test hooks that force an interleaving; nil outside tests.
var (
	// testHookDeletePublished runs after a delete publishes its catalog
	// entry, before it takes mtx for the local delete.
	testHookDeletePublished func(id string)
	// testHookStateListed runs after a mount reads that its listed object has
	// no stored state, before the transaction that stores a genesis.
	testHookStateListed func(id string)
	// testHookRunStarting runs as a tracker run starts, before it takes its
	// sobjectRun.
	testHookRunStarting func(id string)
	// testHookRunEnding runs as a ready tracker run exits, in place of
	// clearing its mount result; it must call end.
	testHookRunEnding func(id string, end func())
	// testHookDeleteCommitted runs after a delete commits, before it restarts
	// the live mount.
	testHookDeleteCommitted func(id string)
	// testHookStateRefused runs after a mount finds no state it may use,
	// before it publishes the refusal.
	testHookStateRefused func(id string)
)

// errCannotDeleteSettings is returned when a delete names the account's bound
// settings object.
var errCannotDeleteSettings = errors.New("cannot delete the account settings shared object")

// listedSharedObjectEntry returns the account list's entry for id, or nil.
func (a *ProviderAccount) listedSharedObjectEntry(id string) *sobject.SharedObjectListEntry {
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return entry
		}
	}
	return nil
}

// onListedGeneration calls fn under the account lock if the list entry for id
// is still of generation, and does nothing otherwise. An
// action planned against one lifetime of an id, such as a delete or a catalog
// metadata replay, decides through it, so it never reaches an object created
// later under the same id: a create holds the same lock and mints a new
// generation.
func (a *ProviderAccount) onListedGeneration(ctx context.Context, id, generation string, fn func() error) error {
	// Hold the lock a create holds while the generation decides.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer relMtx()
	entry := a.listedSharedObjectEntry(id)
	if entry == nil || entry.GetGeneration() != generation {
		return nil
	}
	return fn()
}

// deleteListedGeneration removes the local object id if it is still of
// generation. An object created since under the id stays.
func (a *ProviderAccount) deleteListedGeneration(ctx context.Context, id, generation string) error {
	return a.onListedGeneration(ctx, id, generation, func() error {
		err := a.deleteSharedObjectLocked(ctx, id)
		if errors.Is(err, sobject.ErrSharedObjectNotFound) {
			return nil
		}
		return err
	})
}

// DeleteSharedObject deletes the shared object with the given id.
func (a *ProviderAccount) DeleteSharedObject(ctx context.Context, id string) error {
	// Publish the deletion first so interrupted cleanup cannot resurrect it.
	entry := a.listedSharedObjectEntry(id)
	if entry == nil {
		return sobject.ErrSharedObjectNotFound
	}
	if err := a.publishAccountCatalogEntry(ctx, entry, true); err != nil {
		return err
	}
	if testHookDeletePublished != nil {
		testHookDeletePublished(id)
	}

	// Local cleanup follows the committed account decision, and removes only
	// the generation the tombstone names.
	return a.deleteListedGeneration(ctx, id, entry.GetGeneration())
}

// deleteSharedObjectLocked deletes a shared object. Assumes mtx is locked.
//
// Invariants:
//   - It deletes only the object the caller named, and only while this
//     account lists it; an unlisted id returns ErrSharedObjectNotFound and
//     touches nothing. The bound account settings object is refused.
//   - It removes only keys under so/<id>/. Ids are DNS labels, so no other
//     object's keys share that prefix; the list, the bindings and every other
//     object's keys are outside it.
//   - Nothing another listed object references is removed. Objects refer to
//     each other only by ref inside their own state, never by a key under
//     another object's prefix, and blocks are released by this bucket's
//     named roots, so a block another bucket roots stays.
//   - The list entry and the stored state go in one transaction: after a
//     crash the object is either listed with all its state or unlisted with
//     none, never listed without state.
//   - No handle writes under the prefix after the commit. Write transactions
//     on the account store hold its writer from open to end, so each one
//     commits wholly before or wholly after the delete. The store is the
//     volume's object store, a kvtx prefixer over the volume's kvtx store;
//     each layer opens its base transaction in NewTransaction and adds no
//     buffer: the s4db DB on desktop, behind a logging wrapper when the
//     volume is verbose, and a paylog Domain in the browser. s4db's
//     optimistic mode, which takes the writer at commit, applies only to a
//     transaction RunOptimisticTransaction opens; none here does. One
//     before is removed with the prefix. One after finds no stored state and
//     fails: state writes check the host key in their transaction, and a
//     mount's other writes check it when their transaction opens. The live
//     mount is restarted after the commit, still under mtx, through
//     restartSharedObject, so a mount after the delete returns gets the
//     refusal rather than the ended handle; a handle of the ended mount fails
//     every write, so it cannot write into an object a later create lists
//     under the same id.
//   - No holder of an old handle reads the deleted state after the delete
//     returns. The restart retires the run, which clears the host's state
//     snapshot under its lock, and ends the mount's context, both before the
//     delete returns. From then on a state watchable taken earlier holds
//     nil, the ended handle's state and local state reads return
//     context.Canceled, and MountSharedObject calls each resolver's released
//     callback, so the bus drops the ended handle and resolves again to the
//     refusal. A read that took its snapshot before the commit keeps that
//     snapshot; nothing reads one after.
//   - Every caller removes only the generation it planned against, decided
//     under mtx through onListedGeneration: a delete removes the generation
//     its tombstone names, and the catalog reconcile removes a local object
//     only when its generation is in the catalog's deleted set. A create in
//     between mints a new generation, so it is never removed. See
//     reconcileAccountCatalog.
//   - Root release follows the commit and is not part of it. A crash after
//     the commit and before the release leaves the roots held, and a rerun
//     finds the object unlisted and returns ErrSharedObjectNotFound without
//     releasing them: those blocks leak until a later sweep; none is lost.
//
// Producers of keys under so/<id>/:
//   - host, config-history/ and read-checkpoint: the state lock of
//     NewObjectStoreSOStateFuncs; genesisSOStateWriter, used by create,
//     pairing, enrollment and a mount of a listed object with no state; and
//     ReplaceSharedObjectState for a transfer, which requires the listing.
//   - ls/<store>/ through SharedObject.AccessLocalStateStore, and
//     join-requests through join-requests.go: both through the mount's
//     mountStore.
//
// DeleteStateEnv set to off keeps the old delete: unlist and release roots,
// keep the stored state and the live mount.
func (a *ProviderAccount) deleteSharedObjectLocked(ctx context.Context, id string) error {
	// Read the provider and account that own the shared-object bucket.
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()

	// Get current list
	sharedObjectList := a.soListCtr.GetValue().CloneVT()
	if sharedObjectList == nil {
		return sobject.ErrSharedObjectNotFound
	}

	// Find and remove the shared object from the list
	idx := slices.IndexFunc(sharedObjectList.GetSharedObjects(), func(e *sobject.SharedObjectListEntry) bool {
		return e.GetRef().GetProviderResourceRef().GetId() == id
	})
	if idx == -1 {
		return sobject.ErrSharedObjectNotFound
	}

	// Refuse the bound settings object, which every account path mounts.
	settings, err := a.GetAccountSettingsRef(ctx)
	if err != nil && err != sobject.ErrSharedObjectNotFound {
		return err
	}
	if settings.GetProviderResourceRef().GetId() == id {
		return errCannotDeleteSettings
	}

	// Resolve the block store and bucket for the shared object being removed.
	soEntry := sharedObjectList.GetSharedObjects()[idx]
	blockStoreID := soEntry.GetRef().GetBlockStoreId()
	bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
	sharedObjectList.SharedObjects = slices.Delete(sharedObjectList.SharedObjects, idx, idx+1)

	// Unlist the object, and unless the old behavior is selected, drop its
	// live mount and remove its stored state in the same transaction. On the
	// desktop's on-disk volume, the delete of 5,000 config-history entries of
	// 38,557 bytes each held the writer 20 ms, and of 500 entries 6 ms.
	if deleteStateOff {
		err = a.writeSharedObjectList(ctx, sharedObjectList)
	} else {
		err = a.writeSharedObjectList(ctx, sharedObjectList, func(ctx context.Context, tx kvtx.Tx) error {
			return deleteKeysWithPrefix(ctx, tx, SobjectObjectStorePrefix(id))
		})
	}
	if err != nil {
		return err
	}
	a.soListCtr.SetValue(sharedObjectList)
	if testHookDeleteCommitted != nil {
		testHookDeleteCommitted(id)
	}

	// Restart the live mount after the commit. It reloads, finds no state and
	// refuses, so every handle ends, a mount that raced the delete ends too,
	// and each waiter gets ErrSharedObjectNotFound rather than hanging.
	if !deleteStateOff {
		a.restartSharedObject(id, nil)
	}

	// Drop the deleted bucket's GC refs and sweep newly rootless blocks.
	a.removeSharedObjectGCRefs(ctx, providerID, bucketID, a.le.WithField("sobject-id", id))
	a.triggerGCCleanup()
	return nil
}

// deleteBatchSize bounds the keys deleteKeysWithPrefix holds at once.
const deleteBatchSize = 256

// deleteKeysWithPrefix deletes every key under prefix in tx. It reads sorted
// keys in bounded batches and resumes after the last key of each batch, so
// memory stays constant however many keys the prefix holds, and it does not
// rely on a deleted key leaving a later scan in the same transaction.
func deleteKeysWithPrefix(ctx context.Context, tx kvtx.Tx, prefix []byte) error {
	var after []byte
	for {
		// Read the next batch of keys past the previous one.
		keys, err := scanKeysAfter(ctx, tx, prefix, after)
		if err != nil {
			return err
		}

		// Delete them and stop at the first short batch.
		for _, key := range keys {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		if len(keys) < deleteBatchSize {
			return nil
		}
		after = keys[len(keys)-1]
	}
}

// scanKeysAfter returns up to deleteBatchSize sorted keys under prefix that
// sort after after, or from the start of prefix when after is nil.
func scanKeysAfter(ctx context.Context, tx kvtx.Tx, prefix, after []byte) ([][]byte, error) {
	// Position the iterator at the first key past after.
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if after == nil {
		it.Next()
	} else if err := it.Seek(append(bytes.Clone(after), 0)); err != nil {
		return nil, err
	}

	// Copy keys out, since the iterator may reuse them.
	keys := make([][]byte, 0, deleteBatchSize)
	for ; it.Valid() && len(keys) < deleteBatchSize; it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	return keys, it.Err()
}
