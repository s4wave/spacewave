package provider_spacewave

import (
	"context"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/accountstatus"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// accountWSEvents applies the Session websocket's events to its account.
type accountWSEvents struct {
	// acc is the account the events describe.
	acc *ProviderAccount
	// ctx is the account tracker's run context, which bounds the work the
	// events start.
	ctx context.Context
}

// accountChanged refetches account state at the event's epoch, or bumps the
// local epoch when the event carries none.
func (e accountWSEvents) accountChanged(epoch uint64) {
	e.acc.le.WithField("epoch", epoch).Debug("account changed via ws notify")
	if epoch > 0 {
		e.acc.setEpoch(epoch)
		return
	}
	e.acc.BumpLocalEpoch()
}

// orgChanged drops the billing snapshot and resyncs the organization.
func (e accountWSEvents) orgChanged(orgID string) {
	e.acc.le.WithField("org-id", orgID).Debug("org changed via ws notify")
	e.acc.InvalidateBillingSnapshot("")
	e.acc.QueueOrganizationSync(orgID)
}

// pendingParticipant queues a sync of the shared object's pending participant.
func (e accountWSEvents) pendingParticipant(soID, accountID string) {
	e.acc.le.WithField("sobject-id", soID).
		WithField("target-account-id", accountID).
		Debug("pending participant via ws notify")
	e.acc.pendingParticipantSyncs.SetKey(pendingParticipantSyncKey{
		soID:      soID,
		accountID: accountID,
	}, true)
}

// memberSessionChanged queues a sync of the added or removed member Session.
func (e accountWSEvents) memberSessionChanged(soID, sessionPeerID, accountID string, added bool) {
	e.acc.le.WithField("sobject-id", soID).
		WithField("session-peer-id", sessionPeerID).
		WithField("account-id", accountID).
		WithField("added", added).
		Debug("member session changed via ws notify")
	e.acc.memberSessionSyncs.SetKey(memberSessionSyncKey{
		soID:          soID,
		sessionPeerID: sessionPeerID,
		accountID:     accountID,
		added:         added,
	}, true)
}

// soNotify applies a shared object notification to the account.
func (e accountWSEvents) soNotify(soID string, payload *api.SONotifyEventPayload) {
	e.acc.handleAccountSONotify(e.ctx, soID, payload)
}

// soListUpdate publishes the new shared object list and refreshes
// Self-Enrollment against it.
func (e accountWSEvents) soListUpdate(list *sobject.SharedObjectList) {
	e.acc.soListCtr.SetValue(list)
	e.acc.selfEnrollmentRefresh.Trigger()
}

// connected refetches the account state and the caches that events would
// have updated while no socket was subscribed.
func (e accountWSEvents) connected() {
	// Refetch account state and the caches that events would update.
	acc := e.acc
	acc.le.Debug("session ws connected; invalidating event-driven caches")
	acc.BumpLocalEpoch()
	acc.InvalidatePendingMailboxEntries()
	acc.InvalidateTargetedInvitations()
	acc.orgListJobs.Trigger()

	// The list refetch reconciles cached metadata by revision; without
	// one, every entry is stale.
	if !acc.invalidateSharedObjectList() {
		acc.InvalidateSharedObjectMetadataCache()
	}

	// Re-evaluate every mounted SO once per connection: the cold-start
	// gate short-circuits warm SOs and the cache-aware classifier fetches
	// only what is missing on the rest, so a long disconnect window cannot
	// leave caches stale across all SOs without producing per-mount rejoin
	// storms.
	acc.bumpSelfRejoinSweepGeneration()
}

// inviteMailbox applies a received mailbox entry, then processes it.
func (e accountWSEvents) inviteMailbox(soID string, entry *api.MailboxEntry, updatedAt int64) {
	// Apply a received entry, then process it.
	if soID == "" || entry == nil {
		return
	}
	e.acc.le.WithField("sobject-id", soID).
		WithField("entry-id", entry.GetId()).
		Debug("received mailbox add via ws notify")
	e.acc.ApplyMailboxEntryEvent(soID, entry, updatedAt)
	e.acc.triggerMailboxEntryAutoProcess(e.ctx, soID, entry)
}

// inviteMailboxUpdate applies an updated mailbox entry, refreshes the shared
// object list once the entry is accepted, then processes it.
func (e accountWSEvents) inviteMailboxUpdate(soID string, entry *api.MailboxEntry, updatedAt int64) {
	// Apply an updated entry, refresh the list once it is accepted, then
	// process it.
	if soID == "" || entry == nil {
		return
	}
	e.acc.le.WithField("sobject-id", soID).
		WithField("entry-id", entry.GetId()).
		WithField("status", entry.GetStatus()).
		Debug("received mailbox update via ws notify")
	e.acc.ApplyMailboxEntryEvent(soID, entry, updatedAt)
	if entry.GetStatus() == "accepted" {
		e.acc.invalidateSharedObjectList()
	}
	e.acc.triggerMailboxEntryAutoProcess(e.ctx, soID, entry)
}

// targetedInvitationsChanged reloads the targeted invitation inbox.
func (e accountWSEvents) targetedInvitationsChanged() {
	e.acc.InvalidateTargetedInvitations()
}

// updateAvailable rechecks the launcher's dist config.
func (e accountWSEvents) updateAvailable() {
	e.acc.le.Debug("dispatching launcher recheck after update_available notify")
	e.acc.launcherRecheckJobs.Trigger()
}

// cdnRootChanged tells the CDN root subscribers that the Space's root changed.
func (e accountWSEvents) cdnRootChanged(spaceID string) {
	e.acc.le.WithField("space-id", spaceID).
		Debug("dispatching cdn-root-changed invalidate to subscribers")
	e.acc.fireCdnRootChanged(spaceID)
}

// dormantChanged publishes the account's READY or DORMANT status as cloud
// access closes or reopens.
func (e accountWSEvents) dormantChanged(dormant bool) {
	// Publish the status and re-evaluate the rejoin sweep against it.
	acc := e.acc
	acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		status := provider.ProviderAccountStatus_ProviderAccountStatus_READY
		if dormant {
			status = provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT
		}
		acc.state.status = status
		broadcast()
	})
	acc.refreshSelfRejoinSweepState()
	if dormant {
		acc.syncSharedObjectListAccess(
			s4wave_provider_spacewave.BillingStatus_BillingStatus_UNKNOWN,
		)
		return
	}

	// Exiting dormant means cloud access reopened; force a fresh account
	// state fetch so onboarding reflects the restored subscription.
	acc.BumpLocalEpoch()
}

// sessionUnauthenticated publishes the unauthenticated status for a revoked
// or stale Session.
func (e accountWSEvents) sessionUnauthenticated() {
	acc := e.acc
	acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		acc.state.status = accountstatus.Unauthenticated(acc.state.info)
		broadcast()
	})
	acc.refreshSelfRejoinSweepState()
}

// accountWasDeleted releases the account's roots, cleans up its storage,
// publishes the DELETED status and restarts the account tracker.
func (e accountWSEvents) accountWasDeleted() {
	// Release the account's roots and clean up its storage.
	acc := e.acc
	acc.releaseAccountRoots(e.ctx, acc.le)
	acc.triggerGCCleanup()

	// Set account status to DELETED and broadcast so Watch loops see it.
	acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
		broadcast()
	})
	acc.refreshSelfRejoinSweepState()
	acc.p.accountRc.RestartRoutine(acc.accountID)
}

// _ is a type assertion.
var _ wsEvents = accountWSEvents{}
