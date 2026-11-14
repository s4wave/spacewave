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
//     stored state writes a genesis without clearing the prefix;
//   - delete writes no release mark, and neither the account start nor a
//     create or enrollment finishes one, so a mark an earlier run left stays
//     and releases nothing.
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
	// testHookReplaceCommitted runs after a replace commits, before it
	// restarts the live mount; an error it returns is the replace's, as from
	// a commit that stored the state and then failed.
	testHookReplaceCommitted func(id string) error
	// testHookStateWriting runs in a mounted state write's transaction,
	// after its mount check, before it reads the stored state.
	testHookStateWriting func(id string)
	// testHookStateCommitted runs after a mounted state write commits; an
	// error it returns is the write's, as from a commit that stored the
	// state and then failed.
	testHookStateCommitted func(id string) error
	// testHookStateRefused runs after a mount finds no state it may use,
	// before it publishes the refusal.
	testHookStateRefused func(id string)
	// testHookResumeMark runs as the start resume reaches a release mark,
	// before it takes mtx.
	testHookResumeMark func(id string)
	// testHookReleaseBucket runs before a bucket's roots are released; an
	// error it returns fails the release.
	testHookReleaseBucket func(bucketID string) error
	// testHookDeleteReleaseMark runs before a release mark is removed; an
	// error it returns fails the removal.
	testHookDeleteReleaseMark func(id string) error
	// testHookEnrollmentMounted runs after an enrollment mounts its
	// placeholder and before it takes mtx to install the state and list the
	// id.
	testHookEnrollmentMounted func(id string)
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

// resumeSharedObjectDeletes finishes, once per account start, the root
// release of each delete that a crash or a failed release left unfinished.
// Each such delete left its release mark, which the delete writes in the
// transaction that unlists the object and removes after the release, so the
// work follows the unfinished releases, not every delete ever made.
//
// Invariants:
//   - A mark exists only while its id is unlisted and holds no stored state.
//     Only a delete writes a mark, in the transaction that unlists the id
//     and removes its state, and every path that lists an id or gives it its
//     first state calls finishPendingReleaseLocked first, under mtx, so a new
//     lifetime never starts beside an earlier one's roots. One exception:
//     finishPendingReleaseLocked does nothing with DeleteStateEnv off, so a
//     mark written while on survives an off-mode create or enrollment that
//     lists the id; back on, the resume finds the mark beside the listing.
//     A mark beside a listing or stored state therefore keeps its roots: the
//     resume logs it, keeps the roots and removes the mark.
//   - An id that is neither listed nor holds stored state has no live roots
//     in its bucket. Only a mounted shared object's body moves its bucket's
//     roots, and a mount loads stored state first and refuses an id that has
//     neither. The check and the release both hold mtx, so neither a listing
//     nor first state appears in between. See MountBlockStore.
//   - Each mark is finished under its own hold of mtx. csync.Mutex hands a
//     released lock to its oldest waiter, so a create or a delete that waits
//     behind the resume waits through at most one release.
//   - A release that fails is logged and its mark kept for the next start;
//     the other marks still run. Releasing a bucket that holds no roots
//     writes nothing, so a mark left after a release only costs a recheck.
//   - Deletes committed before marks existed have none, so their leaked
//     roots stay held, as before. With DeleteStateEnv off no mark is written
//     or finished, and this does not run.
func (a *ProviderAccount) resumeSharedObjectDeletes(ctx context.Context) error {
	// Visit the marks in bounded batches.
	var after []byte
	var released bool
	for {
		// Read the next batch of marks.
		keys, err := a.scanReleaseMarks(ctx, after)
		if err != nil {
			return err
		}

		// Finish each, skipping one whose release fails.
		prefix := SobjectReleaseMarkPrefix()
		for _, key := range keys {
			id := string(key[len(prefix):])
			did, err := a.resumeSharedObjectRelease(ctx, id)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				a.le.WithError(err).WithField("sobject-id", id).Warn("failed to resume the deleted shared object's root release")
			}
			released = released || did
		}

		// Stop at the first short batch.
		if len(keys) < deleteBatchSize {
			break
		}
		after = keys[len(keys)-1]
	}

	// Sweep the blocks the released roots reached.
	if released {
		a.triggerGCCleanup()
	}
	return nil
}

// scanReleaseMarks returns up to deleteBatchSize release mark keys that sort
// after after, or from the first when after is nil.
func (a *ProviderAccount) scanReleaseMarks(ctx context.Context, after []byte) ([][]byte, error) {
	var keys [][]byte
	err := a.runSoObjectStoreTx(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		keys, err = scanKeysAfter(ctx, tx, SobjectReleaseMarkPrefix(), after)
		return err
	})
	return keys, err
}

// resumeSharedObjectRelease finishes id's pending release under mtx, and
// reports whether it released the bucket's roots. See
// resumeSharedObjectDeletes.
func (a *ProviderAccount) resumeSharedObjectRelease(ctx context.Context, id string) (bool, error) {
	// Let a test act before the lock is taken.
	if testHookResumeMark != nil {
		testHookResumeMark(id)
	}

	// Hold the lock a create, a delete and every list change hold.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return false, err
	}
	defer relMtx()

	// Keep the roots of a mark that should not exist: one beside a listing
	// or stored state.
	_, found, stored, err := a.readReleaseMark(ctx, id)
	if err != nil || !found {
		return false, err
	}
	if stored || a.listedSharedObjectEntry(id) != nil {
		a.le.WithField("sobject-id", id).Error("release mark beside a listed or stored shared object; keeping its roots")
		return false, a.deleteReleaseMark(ctx, id)
	}

	// Release the id's roots, then drop its mark.
	return a.releaseMarkedBucketLocked(ctx, id)
}

// finishPendingReleaseLocked completes id's pending release, if a delete left
// one: it releases the roots of the bucket the mark names and removes the
// mark. Every path that lists an id or gives it its first state calls it
// before that write, so the new lifetime starts with an empty bucket rather
// than adopting the earlier one's roots. A failure leaves the mark, and the
// caller fails its write, so a retry or the next start finishes it. With
// DeleteStateEnv off it does nothing. Assumes mtx is locked.
func (a *ProviderAccount) finishPendingReleaseLocked(ctx context.Context, id string) error {
	if deleteStateOff {
		return nil
	}
	released, err := a.releaseMarkedBucketLocked(ctx, id)
	if released {
		a.triggerGCCleanup()
	}
	return errors.Wrapf(err, "finish the pending release of shared object %s", id)
}

// releaseMarkedBucketLocked releases the bucket id's mark names, then removes
// the mark, and reports whether it released the bucket. A mark that names no
// block store is removed and logged. Assumes mtx is locked.
func (a *ProviderAccount) releaseMarkedBucketLocked(ctx context.Context, id string) (bool, error) {
	// Read the mark.
	blockStoreID, found, _, err := a.readReleaseMark(ctx, id)
	if err != nil || !found {
		return false, err
	}

	// Drop a mark that names no block store.
	if blockStoreID == "" {
		a.le.WithField("sobject-id", id).Warn("removing a release mark that names no block store")
		return false, a.deleteReleaseMark(ctx, id)
	}

	// Release the bucket's roots, then drop the mark.
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()
	bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
	if err := a.releaseBucketRoots(ctx, bucketID); err != nil {
		return false, errors.Wrap(err, "release roots")
	}
	if err := a.deleteReleaseMark(ctx, id); err != nil {
		return true, errors.Wrap(err, "remove release mark")
	}
	return true, nil
}

// readReleaseMark reads id's release mark: the block store id it names,
// whether it exists, and whether id holds stored state.
func (a *ProviderAccount) readReleaseMark(ctx context.Context, id string) (blockStoreID string, found, stored bool, err error) {
	err = a.runSoObjectStoreTx(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the mark.
		data, ok, err := tx.Get(ctx, SobjectReleaseMarkKey(id))
		if err != nil || !ok {
			return err
		}
		blockStoreID, found = string(data), true

		// Read whether id holds stored state.
		stored, err = tx.Exists(ctx, SobjectObjectStoreHostStateKey(id))
		return err
	})
	return blockStoreID, found, stored, err
}

// deleteReleaseMark removes id's release mark. Assumes mtx is locked.
func (a *ProviderAccount) deleteReleaseMark(ctx context.Context, id string) error {
	if testHookDeleteReleaseMark != nil {
		if err := testHookDeleteReleaseMark(id); err != nil {
			return err
		}
	}
	return a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Delete(ctx, SobjectReleaseMarkKey(id))
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
//   - Root release follows the commit and is not part of it: the roots are
//     the volume's liveness records, which the account store's transaction
//     cannot reach. The commit writes the id's release mark, and the mark is
//     removed only after the release, so a crash or a failed release leaves
//     the mark, and the next account start's resumeSharedObjectDeletes, or
//     the next create or enrollment of the id, finishes the release.
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
			if err := deleteKeysWithPrefix(ctx, tx, SobjectObjectStorePrefix(id)); err != nil {
				return err
			}
			return tx.Set(ctx, SobjectReleaseMarkKey(id), []byte(blockStoreID))
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

	// Drop the deleted bucket's GC refs, then the mark, and sweep newly
	// rootless blocks. A failed release keeps the mark for the next account
	// start.
	if deleteStateOff {
		err = a.releaseBucketRoots(ctx, bucketID)
	} else {
		_, err = a.releaseMarkedBucketLocked(ctx, id)
	}
	if err != nil {
		a.le.WithError(err).WithField("sobject-id", id).Warn("failed to finish the deleted shared object's root release")
	}
	a.triggerGCCleanup()
	return nil
}

// errStaleSOKeys is returned by a write of an id's first state that finds
// keys under its prefix. withClearedSOKeys clears them and retries the write
// once; a second refusal means a writer keeps adding such keys, and the
// caller fails.
var errStaleSOKeys = errors.New("shared object prefix holds keys of an earlier lifetime")

// errSOEnrolling is returned by a create of an id an enrollment is writing.
var errSOEnrolling = errors.New("an enrollment of this shared object is in progress; retry after it finishes")

// beginEnrollmentLocked registers an enrollment of id, which writes state
// under id before it lists it, until the returned end runs. While it is
// registered a create of id fails with errSOEnrolling rather than clear that
// state. Every writer of state under an unlisted id registers before its
// first write; mountEnrolledSO does, for every enrollment. Assumes mtx is
// locked; end takes it.
func (a *ProviderAccount) beginEnrollmentLocked(id string) (end func()) {
	a.enrolling[id]++
	return func() {
		relMtx, _ := a.mtx.Lock(context.Background())
		defer relMtx()
		if a.enrolling[id]--; a.enrolling[id] == 0 {
			delete(a.enrolling, id)
		}
	}
}

// withClearedSOKeys clears the keys under id's prefix that clearKeys proves
// unowned, then runs write, which gives id its first state and refuses
// leftover keys with errStaleSOKeys. If keys appeared between the clear and
// the write, it clears and runs write once more. Assumes mtx is not locked.
func (a *ProviderAccount) withClearedSOKeys(ctx context.Context, id string, clearKeys func(context.Context, string) error, write func() error) error {
	for retried := false; ; retried = true {
		if err := clearKeys(ctx, id); err != nil {
			return err
		}
		err := write()
		if retried || !errors.Is(err, errStaleSOKeys) {
			return err
		}
	}
}

// clearStaleSOKeys deletes the keys under id's prefix while id holds no
// stored state, for a path that gives a listed or enrolling id its first
// state. Such keys come from a clear a crash interrupted or a write that
// failed partway; a delete that kept state kept the host key too. Each batch
// checks the host state key in its deleting transaction, so state another
// writer stores is never deleted. It does not take mtx: a mount's state load
// runs it, and bindPairingSettings mounts while holding mtx.
func (a *ProviderAccount) clearStaleSOKeys(ctx context.Context, id string) error {
	return a.clearSOKeys(ctx, id, false, func(ctx context.Context, tx kvtx.Tx) (bool, error) {
		return tx.Exists(ctx, SobjectObjectStoreHostStateKey(id))
	})
}

// clearAbandonedSOKeys releases the roots in the bucket of an id that is
// neither listed nor being enrolled, then deletes every key under its prefix,
// for a create. Such state is abandoned: it was left by a delete that kept
// state (released versions, or DeleteStateEnv off) or by an enrollment that
// returned without listing the id, such as an interrupted import whose hold
// still names the copied World. The release and each batch hold mtx and fail
// with sobject.ErrSharedObjectExists or errSOEnrolling once the id is listed
// or an enrollment registers. An enrollment registers under mtx before its
// first write, and writes its placeholder, holds and installs after, so the
// release and each batch either run before the registration, when the
// enrollment has written nothing, or see it and stop. After a restart no
// enrollment is registered, and one that starts again writes its own
// placeholder and holds after registering. With DeleteStateEnv off it does
// nothing.
func (a *ProviderAccount) clearAbandonedSOKeys(ctx context.Context, id string) error {
	// Keep every root and key in the old behavior.
	if deleteStateOff {
		return nil
	}

	// Release the bucket's roots, so the new lifetime does not adopt them.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	err = a.checkAbandonedLocked(id)
	if err == nil {
		providerID := a.t.accountInfo.GetProviderId()
		accountID := a.t.accountInfo.GetProviderAccountId()
		bucketID := BlockStoreBucketID(providerID, accountID, SobjectBlockStoreID(id))
		err = errors.Wrap(a.releaseBucketRoots(ctx, bucketID), "release abandoned roots")
	}
	relMtx()
	if err != nil {
		return err
	}

	// Delete the keys.
	return a.clearSOKeys(ctx, id, true, func(context.Context, kvtx.Tx) (bool, error) {
		return false, a.checkAbandonedLocked(id)
	})
}

// checkAbandonedLocked fails with sobject.ErrSharedObjectExists if id is
// listed, or errSOEnrolling if an enrollment of id is registered. Assumes mtx
// is locked.
func (a *ProviderAccount) checkAbandonedLocked(id string) error {
	if a.listedSharedObjectEntry(id) != nil {
		return sobject.ErrSharedObjectExists
	}
	if a.enrolling[id] != 0 {
		return errors.Wrapf(errSOEnrolling, "shared object %s", id)
	}
	return nil
}

// clearSOKeys deletes the keys under id's prefix in write transactions of at
// most deleteBatchSize keys, plus the host state key, which the first batch
// deletes so no mount writes to the id meanwhile. Each batch first asks keep
// within its transaction, and stops if it reports the keys owned or fails.
// With locked set each batch holds mtx, one bounded hold per batch. A read
// transaction probes first, so an empty prefix takes no writer. With
// DeleteStateEnv off it does nothing.
func (a *ProviderAccount) clearSOKeys(ctx context.Context, id string, locked bool, keep func(context.Context, kvtx.Tx) (bool, error)) error {
	// Keep every key in the old behavior.
	if deleteStateOff {
		return nil
	}

	// Probe without the writer.
	prefix := SobjectObjectStorePrefix(id)
	var keys [][]byte
	err := a.runSoObjectStoreTx(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		keys, err = scanKeysAfter(ctx, tx, prefix, nil)
		return err
	})

	// Delete batch by batch until a short one.
	batch := func(ctx context.Context, tx kvtx.Tx) error {
		// Keep owned keys.
		keys = nil
		owned, err := keep(ctx, tx)
		if err != nil || owned {
			return err
		}

		// Delete the host state key, then the batch.
		keys, err = scanKeysAfter(ctx, tx, prefix, nil)
		if err != nil || len(keys) == 0 {
			return err
		}
		if err := tx.Delete(ctx, SobjectObjectStoreHostStateKey(id)); err != nil {
			return err
		}
		for _, key := range keys {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	}
	for err == nil && len(keys) != 0 {
		err = a.runSOKeyBatch(ctx, locked, batch)
		if len(keys) < deleteBatchSize {
			break
		}
	}
	return err
}

// runSOKeyBatch runs batch in one write transaction, holding mtx if locked.
func (a *ProviderAccount) runSOKeyBatch(ctx context.Context, locked bool, batch func(context.Context, kvtx.Tx) error) error {
	// Hold the account lock for this batch only.
	if locked {
		relMtx, err := a.mtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer relMtx()
	}
	return a.runSoObjectStoreTx(ctx, true, batch)
}

// refuseStaleSOKeys is a transaction step that fails with errStaleSOKeys if
// any key remains under id's prefix.
func refuseStaleSOKeys(id string) func(context.Context, kvtx.Tx) error {
	return func(ctx context.Context, tx kvtx.Tx) error {
		// Look for the first key under the prefix.
		it := tx.Iterate(ctx, SobjectObjectStorePrefix(id), true, false)
		defer it.Close()
		it.Next()
		if it.Valid() {
			return errStaleSOKeys
		}
		return it.Err()
	}
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
