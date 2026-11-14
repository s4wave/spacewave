package resource_session

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// unlinkTimeout bounds each cloud unlink after an account's volume is gone.
const unlinkTimeout = 30 * time.Second

// deleteProviderAccount deletes every session of a provider account and then
// the account's volume. Account delete and the source cleanup of a Merge or
// Migrate both remove an account through it.
//
// The order is the contract. No listed session may name a volume that is gone:
// the account's next mount would open an empty volume under a fresh peer key,
// and a Migrate from it would copy that key over the moved one. So the volume
// goes only after a fresh session list shows none of the account's sessions,
// and an error before that point leaves the volume and returns, so the caller
// can retry. Removing the sessions also stops their lifetimes, which closes
// the volume's connections; IndexedDB's deleteDatabase blocks while any stay
// open. A stop between the sessions and the volume leaves an unlisted volume
// on disk, which loses nothing the person kept.
//
// keep, when set, runs before the first change and again just before the
// volume delete; an error from it keeps the volume and returns. A transfer
// passes a check that the source lists no Space the run did not move.
//
// Once the first session delete starts, the rest runs to its end even if ctx
// is cancelled. A failed volume delete is returned. The cloud unlinks that
// follow it are best effort: a failure is logged, because the account is
// already gone.
//
// objectStoreID names the session object store on vol that holds the
// sessions' keys and, for a local account, their linked-cloud account ids.
func deleteProviderAccount(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sessCtrl session.SessionController,
	providerID, accountID, objectStoreID string,
	vol volume.Volume,
	keep func(context.Context) error,
) error {
	// List the account's sessions, and keep the full list to find the cloud
	// sessions to unlink after the volume is gone.
	allSessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}
	accountSessions := filterAccountSessions(allSessions, providerID, accountID)

	// Keep the account whole when it holds something the caller must keep.
	if err := runKeep(ctx, keep); err != nil {
		return err
	}

	// Read the linked-cloud ids and delete the session keys while the object
	// store is open; both live on the volume.
	linkedCloudIDs, err := clearSessionKeys(ctx, le, b, objectStoreID, vol, providerID, accountSessions)
	if err != nil {
		return err
	}

	// Delete every session, then delete the volume only once none is listed.
	// The caller may run under one of these sessions, whose stop cancels ctx,
	// so the rest of the delete ignores cancellation and runs to its end.
	ctx = context.WithoutCancel(ctx)
	for _, entry := range accountSessions {
		if err := sessCtrl.DeleteSession(ctx, entry.GetSessionRef()); err != nil {
			le.WithError(err).Warn("failed to delete session from list")
		}
	}
	remaining, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions after delete")
	}
	if n := len(filterAccountSessions(remaining, providerID, accountID)); n != 0 {
		return errors.Errorf("%d sessions of the account are still listed", n)
	}
	if err := runKeep(ctx, keep); err != nil {
		return errors.Wrap(err, "keep the account volume")
	}
	if err := vol.Delete(); err != nil {
		return errors.Wrap(err, "delete the account volume")
	}

	// Unlink the cloud side's linked-local keys, each within a deadline since
	// ctx no longer ends.
	for _, cloudAccountID := range linkedCloudIDs {
		uctx, cancel := context.WithTimeout(ctx, unlinkTimeout)
		err := unlinkCloudSession(uctx, b, allSessions, accountSessions, cloudAccountID)
		cancel()
		if err != nil {
			le.WithError(err).WithField("cloud-account-id", cloudAccountID).
				Warn("failed to unlink cloud-side linked-local key")
		}
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

// clearSessionKeys returns the linked-cloud account ids of a local account's
// sessions, then deletes every key under each session's prefix in the session
// object store. Deleting the keys first means a volume a stop leaves behind
// holds no session secrets. A failed id read is logged and skips the unlink.
func clearSessionKeys(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	objectStoreID string,
	vol volume.Volume,
	providerID string,
	sessions []*session.SessionListEntry,
) ([]string, error) {
	// Open the session object store until both steps finish.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, b, false, objectStoreID, vol.GetID(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "build object store for session cleanup")
	}
	defer ref.Release()
	objStore := handle.GetObjectStore()

	// Read the linked-cloud ids, which only local sessions store.
	var linked []string
	if providerID == provider_local.ProviderID {
		err := kvtx.RunTransaction(ctx, false,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, false)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				linked = nil
				for _, entry := range sessions {
					sid := entry.GetSessionRef().GetProviderResourceRef().GetId()
					data, found, err := tx.Get(ctx, provider_local.LinkedCloudKey(sid))
					if err != nil {
						return err
					}
					if found && len(data) != 0 {
						linked = append(linked, string(data))
					}
				}
				return nil
			},
		)
		if err != nil {
			le.WithError(err).Warn("failed to read linked-cloud account ids")
			linked = nil
		}
	}

	// Delete each session's keys.
	for _, entry := range sessions {
		prefix := []byte(entry.GetSessionRef().GetProviderResourceRef().GetId() + "/")
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
			return nil, errors.Wrap(err, "scan and delete session keys")
		}
	}
	return linked, nil
}

// unlinkCloudSession deletes the linked-local key that each cloud session of
// cloudAccountID keeps, when the local session index it stores is one of
// deleted. A key naming another local session, such as one the cloud account
// was linked to since, stays. It does nothing when no listed cloud session
// names the account.
func unlinkCloudSession(ctx context.Context, b bus.Bus, sessions []*session.SessionListEntry, deleted []*session.SessionListEntry, cloudAccountID string) error {
	// Find the cloud sessions that name the account.
	cloud := filterAccountSessions(sessions, provider_spacewave.ProviderID, cloudAccountID)
	if len(cloud) == 0 {
		return nil
	}

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

	// Delete each linked-local key that names a deleted session. Reading a
	// key whose index is no longer listed already removes it.
	gone := make(map[uint32]struct{}, len(deleted))
	for _, e := range deleted {
		gone[e.GetSessionIndex()] = struct{}{}
	}
	for _, entry := range cloud {
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
