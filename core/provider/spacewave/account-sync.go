package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
)

// pendingParticipantSyncKey identifies a pending participant reconciliation task.
type pendingParticipantSyncKey struct {
	// soID is the shared object id.
	soID string
	// accountID is the target account to enroll.
	accountID string
}

// memberSessionSyncKey identifies a member session reconciliation task.
type memberSessionSyncKey struct {
	// soID is the shared object id.
	soID string
	// sessionPeerID is the member session peer id.
	sessionPeerID string
	// accountID is the target account id for add events.
	accountID string
	// added is true for session_added and false for session_removed.
	added bool
}

// buildOrgSyncRoutine constructs the keyed org sync routine.
func (a *ProviderAccount) buildOrgSyncRoutine(orgID string) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		if !a.canMutateCloudObjects() {
			return nil
		}
		a.fetchAndUpdateOrgList(ctx)
		if err := a.RefreshSharedObjectList(ctx); err != nil {
			return errors.Wrap(err, "refresh shared object list")
		}
		if err := a.reconcileOwnedOrganizationSpaces(ctx, orgID); err != nil {
			return errors.Wrap(err, "reconcile owned organization spaces")
		}
		a.bootstrapOrgSharedObjects(ctx)
		return nil
	}, struct{}{}
}

// buildPendingParticipantSyncRoutine constructs the keyed pending participant routine.
func (a *ProviderAccount) buildPendingParticipantSyncRoutine(
	key pendingParticipantSyncKey,
) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		if !a.canMutateCloudObjects() {
			return nil
		}
		return a.reconcilePendingParticipant(ctx, key.soID, key.accountID)
	}, struct{}{}
}

// buildMemberSessionSyncRoutine constructs the keyed member session routine.
func (a *ProviderAccount) buildMemberSessionSyncRoutine(
	key memberSessionSyncKey,
) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		if !a.canMutateCloudObjects() {
			return nil
		}
		var err error
		if key.added {
			err = a.reconcileMemberSession(ctx, key.soID, key.accountID)
		} else {
			err = a.revokeMemberSession(ctx, key.soID, key.sessionPeerID)
		}
		if isTerminalSharedObjectMountError(err) {
			a.le.WithError(err).
				WithField("sobject-id", key.soID).
				WithField("session-peer-id", key.sessionPeerID).
				Warn("member session sync hit terminal shared object mount error")
			return nil
		}
		return err
	}, struct{}{}
}
