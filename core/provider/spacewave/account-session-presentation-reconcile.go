package provider_spacewave

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// sessionPresentationReconcileState is the desired authoritative reconcile target.
type sessionPresentationReconcileState struct {
	// accountSettingsSOID is the bound account-settings shared object ID.
	accountSettingsSOID string
	// liveSessionPeerIDs is the sorted authoritative live session peer set.
	liveSessionPeerIDs []string
}

func equalSessionPresentationReconcileState(
	v1, v2 *sessionPresentationReconcileState,
) bool {
	if v1 == nil || v2 == nil {
		return v1 == v2
	}
	return v1.accountSettingsSOID == v2.accountSettingsSOID &&
		slices.Equal(v1.liveSessionPeerIDs, v2.liveSessionPeerIDs)
}

// buildSessionPresentationReconcileStateLocked builds the desired reconcile target.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) buildSessionPresentationReconcileStateLocked() *sessionPresentationReconcileState {
	if !a.state.sessionsValid {
		return nil
	}

	info := a.state.info
	if info == nil {
		return nil
	}
	if !providerAccountStatusAllowsCloudMutation(a.state.status) ||
		!cloudMutationAllowed(info) {
		return nil
	}

	var soID string
	for _, binding := range info.GetAccountSobjectBindings() {
		if binding.GetPurpose() != account_settings.BindingPurpose {
			continue
		}
		if binding.GetState() != api.
			AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY {
			return nil
		}
		soID = binding.GetSoId()
		break
	}
	if soID == "" {
		return nil
	}

	peerIDs := make([]string, 0, len(a.state.sessions))
	for _, row := range a.state.sessions {
		peerID := row.GetPeerId()
		if peerID == "" {
			continue
		}
		peerIDs = append(peerIDs, peerID)
	}
	slices.Sort(peerIDs)
	return &sessionPresentationReconcileState{
		accountSettingsSOID: soID,
		liveSessionPeerIDs:  peerIDs,
	}
}

// setSessionPresentationReconcileState updates the reconcile target.
func (a *ProviderAccount) setSessionPresentationReconcileState(
	state *sessionPresentationReconcileState,
) {
	if a.sessionPresentationReconcile == nil {
		return
	}
	a.sessionPresentationReconcile.SetState(state)
}

// runSessionPresentationReconcile prunes orphaned mirrored session metadata.
func (a *ProviderAccount) runSessionPresentationReconcile(
	ctx context.Context,
	state *sessionPresentationReconcileState,
) error {
	if state == nil || state.accountSettingsSOID == "" {
		return nil
	}
	if !a.canMutateCloudObjects() {
		return nil
	}

	so, relSO, err := a.MountSharedObject(
		ctx,
		a.buildSharedObjectRef(state.accountSettingsSOID),
		nil,
	)
	if err != nil {
		if isTerminalSharedObjectMountError(err) {
			a.le.WithError(err).
				WithField("sobject-id", state.accountSettingsSOID).
				Warn("account settings shared object mount hit terminal error")
			return nil
		}
		return errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	if err := a.reconcileSessionPresentationState(ctx, so, state); err != nil {
		return errors.Wrap(err, "reconcile session presentation state")
	}
	return nil
}

// reconcileSessionPresentationState removes mirrored session rows missing from the
// authoritative live session set.
func (a *ProviderAccount) reconcileSessionPresentationState(
	ctx context.Context,
	so sobject.SharedObject,
	state *sessionPresentationReconcileState,
) error {
	presentationPeerIDs, err := getSessionPresentationPeerIDs(ctx, so)
	if err != nil {
		return err
	}

	orphanedPeerIDs := buildOrphanedSessionPresentationPeerIDs(
		presentationPeerIDs,
		state.liveSessionPeerIDs,
	)
	for _, peerID := range orphanedPeerIDs {
		if err := removeSessionPresentationFromSharedObject(ctx, so, peerID); err != nil {
			return errors.Wrapf(err, "remove orphaned session presentation %q", peerID)
		}
	}
	return nil
}

func getSessionPresentationPeerIDs(
	ctx context.Context,
	so sobject.SharedObject,
) ([]string, error) {
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access account settings state")
	}
	defer relStateCtr()

	snap, err := stateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "wait account settings state")
	}

	settings, err := decodeAccountSettingsSnapshot(ctx, snap)
	if err != nil {
		return nil, err
	}

	peerIDs := make([]string, 0, len(settings.GetSessionPresentations()))
	for _, pres := range settings.GetSessionPresentations() {
		peerID := pres.GetPeerId()
		if peerID == "" {
			continue
		}
		peerIDs = append(peerIDs, peerID)
	}
	return peerIDs, nil
}

func buildOrphanedSessionPresentationPeerIDs(
	presentationPeerIDs []string,
	liveSessionPeerIDs []string,
) []string {
	if len(presentationPeerIDs) == 0 {
		return nil
	}

	live := make(map[string]struct{}, len(liveSessionPeerIDs))
	for _, peerID := range liveSessionPeerIDs {
		if peerID == "" {
			continue
		}
		live[peerID] = struct{}{}
	}

	orphaned := make([]string, 0, len(presentationPeerIDs))
	for _, peerID := range presentationPeerIDs {
		if peerID == "" {
			continue
		}
		if _, ok := live[peerID]; ok {
			continue
		}
		orphaned = append(orphaned, peerID)
	}
	slices.Sort(orphaned)
	return orphaned
}

func decodeAccountSettingsSnapshot(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
) (*account_settings.AccountSettings, error) {
	settings := &account_settings.AccountSettings{}
	if snap == nil {
		return settings, nil
	}

	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get account settings root")
	}
	if rootInner == nil {
		return settings, nil
	}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil, errors.Wrap(err, "unmarshal account settings state")
		}
	}
	return settings, nil
}
