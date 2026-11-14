package resource_session

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// unlinkTimeout bounds each cloud unlink after an account's volume is gone.
const unlinkTimeout = 30 * time.Second

// legacyDelete selects the account delete that ran before the deleting mark
// and the checks around the volume delete, as a switch back while the new
// delete has not yet run in production: SPACEWAVE_ACCOUNT_DELETE=legacy in
// the process environment of a native build, or in a browser build's
// globalThis.BLDR_RUNTIME_WASM_ENV, which the web document hands the Go
// runtime as its environment before it starts. Account delete then removes
// the keys and the sessions, and logs a failed volume delete; the source
// delete of a Merge or Migrate removes only the volume, leaving the sessions
// listed.
var legacyDelete = os.Getenv("SPACEWAVE_ACCOUNT_DELETE") == "legacy"

// deleteTimeout bounds an account delete from its first session delete to its
// volume delete, a stage that ignores the caller's cancellation. A variable
// only so a test can shorten it.
var deleteTimeout = 2 * time.Minute

// deleteStep names a point in an account delete after which a stop leaves
// the account for the resume to finish.
type deleteStep int

const (
	// stepListed is after the entries are DELETING and the lifetimes stopped.
	stepListed deleteStep = iota
	// stepMarked is after the deleting mark is written.
	stepMarked
	// stepKeys is after the session keys are deleted.
	stepKeys
	// stepVolume is after the volume is deleted.
	stepVolume
)

// stopAfter, when set, runs after each step of an account delete; an error
// from it stops the delete there, as a crash would. Only tests set it.
var stopAfter func(deleteStep) error

// stopAt runs stopAfter for step, if set.
func stopAt(step deleteStep) error {
	if stopAfter == nil {
		return nil
	}
	return stopAfter(step)
}

// deleteProviderAccount deletes every session of a provider account and then
// the account's volume. Account delete and the source cleanup of a Merge or
// Migrate both remove an account through it, and a resume finishes one that
// stopped.
//
// The session list is the durable record of a delete in progress. In order:
//
//  1. keep, when set, must pass.
//  2. Every listed entry of the account turns DELETING, and its lifetimes
//     stop, which closes the volume's connections: IndexedDB's
//     deleteDatabase blocks while any stay open. A DELETING entry never
//     opens; the next mount of one, or the next startup, resumes the delete.
//  3. A local account's session object store gets the deleting mark, so no
//     mount by ref opens a session either.
//  4. keep runs again. Until here the delete can be undone: on a failure the
//     mark goes and the entries turn ACTIVE again.
//  5. The session keys go.
//  6. A fresh list must hold no entry of the account that is not DELETING.
//  7. The entries turn UNLINKING, then the volume goes.
//  8. Each cloud session's link to one of the account's indexes goes.
//  9. The entries go, last, so the resume always finds a stopped delete.
//
// Invariant: nothing on a resume creates a volume, a key or a session for an
// account with a DELETING or UNLINKING entry. A resume opens the account only
// while its entries are DELETING, which step 7 ends before the volume goes;
// from UNLINKING it removes the volume through its storage without opening it.
//
// Once step 2 starts, the rest ignores ctx's cancellation, since the caller
// may run under one of the stopped sessions, but not deleteTimeout. A failure
// after step 4 and before the volume goes is returned with the entries left
// for the resume. A failure in steps 8 or 9 is only logged: the account is
// gone, its entries stay UNLINKING, and the next startup finishes them.
//
// objectStoreID names the session object store on vol that holds the
// sessions' keys.
func deleteProviderAccount(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sessCtrl session.SessionController,
	providerID, accountID, objectStoreID string,
	vol volume.Volume,
	keep func(context.Context) error,
) error {
	// Run the legacy delete when selected.
	if legacyDelete {
		return deleteProviderAccountLegacy(ctx, le, b, sessCtrl, providerID, accountID, objectStoreID, vol, keep != nil)
	}

	// Keep the account whole when it holds something the caller must keep.
	if err := runKeep(ctx, keep); err != nil {
		return err
	}

	// Turn the account's entries DELETING, which stops their lifetimes.
	ctx = context.WithoutCancel(ctx)
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	refs, err := listAccountRefs(ctx, sessCtrl, providerID, accountID)
	if err != nil {
		return err
	}
	if err := sessCtrl.SetSessionState(ctx, refs, session.SessionListState_SESSION_LIST_STATE_DELETING); err != nil {
		return errors.Wrap(err, "mark the sessions deleting")
	}
	if err := stopAt(stepListed); err != nil {
		return err
	}

	// Mark, check, delete the keys and then the volume.
	kind := provider_local.DeletingAccount
	if keep != nil {
		kind = provider_local.DeletingTransferSource
	}
	if err := deleteAccountVolume(ctx, le, b, sessCtrl, providerID, accountID, objectStoreID, vol, kind, keep); err != nil {
		return err
	}

	// Past the volume delete the account is gone; what remains falls to the
	// resume when it fails here.
	if err := finishAccountDelete(ctx, le, b, sessCtrl, providerID, accountID); err != nil {
		le.WithError(err).Warn("account volume deleted; the next startup finishes the unlinks")
	}
	return nil
}

// deleteAccountVolume runs steps 3 to 7 of deleteProviderAccount on an
// account whose entries are DELETING.
func deleteAccountVolume(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sessCtrl session.SessionController,
	providerID, accountID, objectStoreID string,
	vol volume.Volume,
	kind provider_local.Deleting,
	keep func(context.Context) error,
) error {
	// Open the session object store until the keys are gone.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, b, false, objectStoreID, vol.GetID(), nil)
	if err != nil {
		return errors.Wrap(err, "build object store for session cleanup")
	}
	objStore := handle.GetObjectStore()
	local := providerID == provider_local.ProviderID

	// Mark a local account, then check keep while the delete can be undone.
	if local {
		if err := provider_local.MarkAccountDeleting(ctx, objStore, kind); err != nil {
			ref.Release()
			return errors.Wrap(err, "mark the account deleting")
		}
	}
	if err := stopAt(stepMarked); err != nil {
		ref.Release()
		return err
	}
	if err := runKeep(ctx, keep); err != nil {
		err = errors.Wrap(err, "keep the account volume")
		if uerr := undoDelete(ctx, sessCtrl, objStore, providerID, accountID, local); uerr != nil {
			le.WithError(uerr).Warn("failed to undo the account delete")
		}
		ref.Release()
		return err
	}

	// Delete the keys of every listed session of the account.
	refs, err := listAccountRefs(ctx, sessCtrl, providerID, accountID)
	if err == nil {
		err = deleteSessionKeys(ctx, objStore, refs)
	}
	ref.Release()
	if err != nil {
		return err
	}
	if err := stopAt(stepKeys); err != nil {
		return err
	}

	// Delete the volume only while every entry of the account is DELETING,
	// turning them UNLINKING first.
	entries, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions after the keys")
	}
	refs = nil
	for _, entry := range filterAccountSessions(entries, providerID, accountID) {
		if entry.GetState() != session.SessionListState_SESSION_LIST_STATE_DELETING {
			return errors.Errorf("session %d of the account is not deleting", entry.GetSessionIndex())
		}
		refs = append(refs, entry.GetSessionRef())
	}
	if err := sessCtrl.SetSessionState(ctx, refs, session.SessionListState_SESSION_LIST_STATE_UNLINKING); err != nil {
		return errors.Wrap(err, "mark the sessions unlinking")
	}
	if err := vol.Delete(); err != nil {
		return errors.Wrap(err, "delete the account volume")
	}
	return stopAt(stepVolume)
}

// undoDelete returns an account whose delete has not removed a key to use:
// it removes the mark and turns the entries ACTIVE again.
func undoDelete(ctx context.Context, sessCtrl session.SessionController, objStore object.ObjectStore, providerID, accountID string, local bool) error {
	if local {
		if err := provider_local.ClearAccountDeleting(ctx, objStore); err != nil {
			return err
		}
	}
	refs, err := listAccountRefs(ctx, sessCtrl, providerID, accountID)
	if err != nil {
		return err
	}
	return sessCtrl.SetSessionState(ctx, refs, session.SessionListState_SESSION_LIST_STATE_ACTIVE)
}

// finishAccountDelete runs steps 8 and 9 of deleteProviderAccount on an
// account whose volume is gone: it unlinks every cloud session from the
// account's indexes and then removes the account's entries. An unlink that
// fails keeps the entries, whose indexes the link names, for the next resume.
func finishAccountDelete(ctx context.Context, le *logrus.Entry, b bus.Bus, sessCtrl session.SessionController, providerID, accountID string) error {
	// Unlink the cloud sessions that name one of the account's indexes.
	entries, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions before the unlinks")
	}
	account := filterAccountSessions(entries, providerID, accountID)
	if providerID == provider_local.ProviderID {
		if err := unlinkCloudSessions(ctx, le, b, entries, account); err != nil {
			return err
		}
	}

	// Remove the entries.
	for _, entry := range account {
		if err := sessCtrl.DeleteSession(ctx, entry.GetSessionRef()); err != nil {
			return errors.Wrap(err, "remove a session of the deleted account")
		}
	}
	return nil
}

// listAccountRefs returns the refs of the listed entries of the account.
func listAccountRefs(ctx context.Context, sessCtrl session.SessionController, providerID, accountID string) ([]*session.SessionRef, error) {
	// List every entry and keep the account's refs.
	entries, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list sessions")
	}
	var refs []*session.SessionRef
	for _, entry := range filterAccountSessions(entries, providerID, accountID) {
		refs = append(refs, entry.GetSessionRef())
	}
	return refs, nil
}

// deleteProviderAccountLegacy is the delete legacyDelete selects. A transfer's
// source delete removes only the volume; an account delete removes the keys,
// the sessions and then the volume, and unlinks the cloud sessions. A failed
// volume delete is logged.
func deleteProviderAccountLegacy(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sessCtrl session.SessionController,
	providerID, accountID, objectStoreID string,
	vol volume.Volume,
	transfer bool,
) error {
	// A transfer deleted only the source volume.
	if transfer {
		if err := vol.Delete(); err != nil {
			le.WithError(err).Warn("failed to delete source volume")
		}
		return nil
	}

	// List the account's sessions.
	allSessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}
	accountSessions := filterAccountSessions(allSessions, providerID, accountID)

	// Delete the keys.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, b, false, objectStoreID, vol.GetID(), nil)
	if err != nil {
		return errors.Wrap(err, "build object store for session cleanup")
	}
	var refs []*session.SessionRef
	for _, entry := range accountSessions {
		refs = append(refs, entry.GetSessionRef())
	}
	err = deleteSessionKeys(ctx, handle.GetObjectStore(), refs)
	ref.Release()
	if err != nil {
		return err
	}

	// Then the sessions, which closes the volume's connections, then the
	// volume.
	for _, entry := range accountSessions {
		if err := sessCtrl.DeleteSession(ctx, entry.GetSessionRef()); err != nil {
			le.WithError(err).Warn("failed to delete session from list")
		}
	}
	if err := vol.Delete(); err != nil {
		le.WithError(err).Warn("failed to delete volume backing store")
	}

	// Unlink the cloud sessions, best effort; the unlink logs each failure.
	if providerID == provider_local.ProviderID {
		_ = unlinkCloudSessions(ctx, le, b, allSessions, accountSessions)
	}
	return nil
}

// runKeep runs keep, if set.
func runKeep(ctx context.Context, keep func(context.Context) error) error {
	if keep == nil {
		return nil
	}
	return keep(ctx)
}

// filterAccountSessions returns the entries of sessions naming the account.
func filterAccountSessions(entries []*session.SessionListEntry, providerID, accountID string) []*session.SessionListEntry {
	return slices.DeleteFunc(slices.Clone(entries), func(entry *session.SessionListEntry) bool {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		return ref.GetProviderId() != providerID || ref.GetProviderAccountId() != accountID
	})
}

// deleteSessionKeys deletes every key under each session's prefix in the
// session object store.
func deleteSessionKeys(ctx context.Context, objStore object.ObjectStore, refs []*session.SessionRef) error {
	for _, ref := range refs {
		prefix := []byte(ref.GetProviderResourceRef().GetId() + "/")
		err := kvtx.RunTransaction(ctx, true,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, true)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				return tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
					return tx.Delete(ctx, key)
				})
			},
		)
		if err != nil {
			return errors.Wrap(err, "scan and delete session keys")
		}
	}
	return nil
}

// unlinkCloudSessions deletes the linked-local key of each listed cloud
// session whose key names the index of one of deleted. A key naming another
// local session, such as one the cloud account was linked to since, stays.
func unlinkCloudSessions(ctx context.Context, le *logrus.Entry, b bus.Bus, sessions, deleted []*session.SessionListEntry) error {
	// Collect the deleted indexes and the cloud accounts to open.
	gone := make(map[uint32]struct{}, len(deleted))
	for _, e := range deleted {
		gone[e.GetSessionIndex()] = struct{}{}
	}
	cloud := make(map[string][]*session.SessionListEntry)
	for _, e := range sessions {
		provRef := e.GetSessionRef().GetProviderResourceRef()
		if provRef.GetProviderId() == provider_spacewave.ProviderID {
			cloud[provRef.GetProviderAccountId()] = append(cloud[provRef.GetProviderAccountId()], e)
		}
	}

	// Unlink each cloud account's sessions within a deadline.
	var failed error
	for cloudAccountID, entries := range cloud {
		uctx, cancel := context.WithTimeout(ctx, unlinkTimeout)
		err := unlinkCloudAccount(uctx, b, cloudAccountID, entries, gone)
		cancel()
		if err != nil {
			le.WithError(err).WithField("cloud-account-id", cloudAccountID).Warn("failed to unlink cloud-side linked-local key")
			failed = err
		}
	}
	return failed
}

// unlinkCloudAccount deletes the linked-local key of each of the cloud
// account's sessions in entries whose key names an index in gone.
func unlinkCloudAccount(ctx context.Context, b bus.Bus, cloudAccountID string, entries []*session.SessionListEntry, gone map[uint32]struct{}) error {
	// Open the cloud account, if a loaded provider serves it.
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, b, provider_spacewave.ProviderID, cloudAccountID, true, nil)
	if err != nil {
		return errors.Wrap(err, "access cloud account")
	}
	if accRef == nil {
		return errors.New("no loaded provider serves the cloud account")
	}
	defer accRef.Release()
	swAcc, ok := acc.(*provider_spacewave.ProviderAccount)
	if !ok {
		return errors.New("cloud account is not a spacewave account")
	}

	// Delete each key that names a deleted index.
	for _, entry := range entries {
		sid := entry.GetSessionRef().GetProviderResourceRef().GetId()
		found, idx, err := swAcc.GetLinkedLocalSession(ctx, sid)
		if err != nil {
			return err
		}
		if _, ok := gone[idx]; !found || !ok {
			continue
		}
		if err := swAcc.DeleteLinkedLocalSession(ctx, sid); err != nil {
			return err
		}
	}
	return nil
}

// ResumeAccountDelete finishes the delete of the account ref names, whose
// entry is DELETING or UNLINKING or whose mount returned
// session.ErrAccountDeleting. It decides where to resume from the
// session list, without opening the account first: from UNLINKING the volume
// delete started, so it removes the volume through its storage and finishes
// the unlinks; otherwise the volume is whole, so it opens the account and runs
// the delete from step 2, where each step it already took is a no-op. A
// transfer source's resume checks again that the account lists no Space the
// run did not move.
func ResumeAccountDelete(ctx context.Context, le *logrus.Entry, b bus.Bus, ref *session.SessionRef) error {
	// Read the account's entries.
	sessCtrl, sessCtrlRef, err := session.ExLookupSessionController(ctx, b, "", false, nil)
	if err != nil {
		return errors.Wrap(err, "lookup session controller")
	}
	defer sessCtrlRef.Release()
	provRef := ref.GetProviderResourceRef()
	providerID, accountID := provRef.GetProviderId(), provRef.GetProviderAccountId()
	entries, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}
	account := filterAccountSessions(entries, providerID, accountID)

	// Past the volume delete, finish it without opening the account.
	if slices.ContainsFunc(account, unlinking) {
		ctx = context.WithoutCancel(ctx)
		ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
		defer cancel()
		if err := deleteVolumeUnopened(ctx, b, providerID, accountID); err != nil {
			return errors.Wrap(err, "delete the account volume")
		}
		return finishAccountDelete(ctx, le, b, sessCtrl, providerID, accountID)
	}

	// Before it, open the account and run the delete again.
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, b, providerID, accountID, false, nil)
	if err != nil {
		return errors.Wrap(err, "access the account")
	}
	defer accRef.Release()
	vol, objectStoreID, err := accountVolume(acc, providerID, accountID)
	if err != nil {
		return err
	}
	keep, err := resumeKeep(ctx, b, acc, objectStoreID, vol)
	if err != nil {
		return err
	}
	return deleteProviderAccount(ctx, le, b, sessCtrl, providerID, accountID, objectStoreID, vol, keep)
}

// unlinking reports whether entry is UNLINKING.
func unlinking(entry *session.SessionListEntry) bool {
	return entry.GetState() == session.SessionListState_SESSION_LIST_STATE_UNLINKING
}

// resumeKeep returns the keep a resumed delete runs: for a local transfer
// source, a check that the account lists only account-private Spaces.
func resumeKeep(ctx context.Context, b bus.Bus, acc provider.ProviderAccount, objectStoreID string, vol volume.Volume) (func(context.Context) error, error) {
	// Only a local account carries the mark that names a transfer.
	local, ok := acc.(*provider_local.ProviderAccount)
	if !ok {
		return nil, nil
	}
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, b, false, objectStoreID, vol.GetID(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "build session object store")
	}
	kind, err := provider_local.AccountDeleting(ctx, handle.GetObjectStore())
	ref.Release()
	if err != nil || kind != provider_local.DeletingTransferSource {
		return nil, err
	}

	// Check the account's Space list.
	return func(ctx context.Context) error {
		// Wait for the list, then refuse any Space not account-private.
		ctr, rel, err := local.AccessSharedObjectList(ctx, nil)
		if err != nil {
			return err
		}
		defer rel()
		list, err := ctr.WaitValue(ctx, nil)
		if err != nil {
			return err
		}
		return provider_transfer.ListsOnlyPrivate(list)
	}, nil
}

// accountVolumeDeleter is a provider that removes an account's volume without
// opening it.
type accountVolumeDeleter interface {
	// DeleteAccountVolume removes the account's volume without opening it.
	DeleteAccountVolume(ctx context.Context, accountID string) error
}

// deleteVolumeUnopened removes the account's volume through its provider,
// which creates nothing.
func deleteVolumeUnopened(ctx context.Context, b bus.Bus, providerID, accountID string) error {
	// Look up the provider and have it remove the volume.
	prov, provRef, err := provider.ExLookupProvider(ctx, b, providerID, false, nil)
	if err != nil {
		return err
	}
	defer provRef.Release()
	deleter, ok := prov.(accountVolumeDeleter)
	if !ok {
		return errors.Errorf("provider %s cannot delete an account volume", providerID)
	}
	return deleter.DeleteAccountVolume(ctx, accountID)
}

// accountVolume returns the volume of a local or cloud account and the id of
// the session object store on it.
func accountVolume(acc provider.ProviderAccount, providerID, accountID string) (volume.Volume, string, error) {
	switch acc := acc.(type) {
	case *provider_local.ProviderAccount:
		return acc.GetVolume(), provider_local.SessionObjectStoreID(providerID, accountID), nil
	case *provider_spacewave.ProviderAccount:
		return acc.GetVolume(), provider_spacewave.SessionObjectStoreID(accountID), nil
	default:
		return nil, "", errors.New("unsupported provider account type for delete")
	}
}

// ResumeAccountDeletes finishes, through ResumeAccountDelete, the delete of
// every account with a DELETING or UNLINKING entry. The resource root runs it
// once at startup; it reads only the session list, and opens no account whose
// entries are all ACTIVE. A failure on one account is logged and the walk goes
// on to the next.
func ResumeAccountDeletes(ctx context.Context, le *logrus.Entry, b bus.Bus) error {
	// List the sessions.
	sessCtrl, sessCtrlRef, err := session.ExLookupSessionController(ctx, b, "", false, nil)
	if err != nil {
		return errors.Wrap(err, "lookup session controller")
	}
	entries, err := sessCtrl.ListSessions(ctx)
	sessCtrlRef.Release()
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}

	// Resume each account with an entry past ACTIVE, once.
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.GetState() == session.SessionListState_SESSION_LIST_STATE_ACTIVE {
			continue
		}
		provRef := entry.GetSessionRef().GetProviderResourceRef()
		key := provRef.GetProviderId() + "/" + provRef.GetProviderAccountId()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ale := le.WithField("account-id", provRef.GetProviderAccountId())
		if err := ResumeAccountDelete(ctx, ale, b, entry.GetSessionRef()); err != nil {
			ale.WithError(err).Warn("failed to finish the account delete")
		}
	}
	return ctx.Err()
}
