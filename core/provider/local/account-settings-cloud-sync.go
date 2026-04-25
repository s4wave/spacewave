package provider_local

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/volume"
)

type accountSettingsSyncSource int

const (
	accountSettingsSyncSourceLocal accountSettingsSyncSource = iota + 1
	accountSettingsSyncSourceCloud
)

type accountSettingsSyncEvent struct {
	source   accountSettingsSyncSource
	settings *account_settings.AccountSettings
	seqno    uint64
}

type accountSettingsSyncTarget interface {
	QueueOperation(ctx context.Context, op []byte) (string, error)
	WaitOperation(ctx context.Context, localID string) (uint64, bool, error)
	ClearOperationResult(ctx context.Context, localID string) error
}

func (a *ProviderAccount) setLinkedCloudAccountID(cloudAccountID string) {
	if a.accountSettingsCloudSync == nil {
		return
	}
	a.accountSettingsCloudSync.SetState(cloudAccountID)
}

func (a *ProviderAccount) loadLinkedCloudAccountID(ctx context.Context) (string, error) {
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		a.t.p.b,
		false,
		SessionObjectStoreID(a.GetProviderID(), a.GetAccountID()),
		a.vol.GetID(),
		nil,
	)
	if err != nil {
		return "", errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "open session object store transaction")
	}
	defer otx.Discard()

	var linkedCloudAccountID string
	stopErr := errors.New("linked cloud account id found")
	err = otx.ScanPrefix(ctx, []byte{}, func(key, value []byte) error {
		if !bytes.HasSuffix(key, []byte("/linked-cloud")) {
			return nil
		}
		if len(value) == 0 {
			return nil
		}
		linkedCloudAccountID = string(value)
		return stopErr
	})
	if err != nil && !errors.Is(err, stopErr) {
		return "", errors.Wrap(err, "scan session object store")
	}
	return linkedCloudAccountID, nil
}

func (a *ProviderAccount) runAccountSettingsCloudSync(
	ctx context.Context,
	cloudAccountID string,
) error {
	localRef, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get local account settings ref")
	}
	localSO, relLocalSO, err := a.MountSharedObject(ctx, localRef, nil)
	if err != nil {
		return errors.Wrap(err, "mount local account settings")
	}
	defer relLocalSO()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx,
		a.t.p.b,
		"spacewave",
		cloudAccountID,
		false,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "access cloud provider account")
	}
	if provAcc == nil {
		return errors.New("cloud provider account not available")
	}
	defer provAccRef.Release()

	swAcc, ok := provAcc.(*provider_spacewave.ProviderAccount)
	if !ok {
		return errors.New("unexpected cloud provider account type")
	}

	cloudRef, err := waitForCloudAccountSettingsRef(ctx, swAcc)
	if err != nil {
		return err
	}
	cloudSO, relCloudSO, err := swAcc.MountSharedObject(ctx, cloudRef, nil)
	if err != nil {
		return errors.Wrap(err, "mount cloud account settings")
	}
	defer relCloudSO()

	localCtr, relLocalCtr, err := localSO.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "access local account settings state")
	}
	defer relLocalCtr()
	cloudCtr, relCloudCtr, err := cloudSO.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "access cloud account settings state")
	}
	defer relCloudCtr()

	evCh := make(chan accountSettingsSyncEvent)
	errCh := make(chan error, 2)
	go watchAccountSettingsSyncState(
		ctx,
		localCtr,
		accountSettingsSyncSourceLocal,
		evCh,
		errCh,
	)
	go watchAccountSettingsSyncState(
		ctx,
		cloudCtr,
		accountSettingsSyncSourceCloud,
		evCh,
		errCh,
	)

	var (
		localSettings *account_settings.AccountSettings
		localSeqno    uint64
		localReady    bool

		cloudSettings *account_settings.AccountSettings
		cloudSeqno    uint64
		cloudReady    bool
	)

	writeAllowed, accountCh := loadCloudAccountSettingsSyncState(swAcc)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case <-accountCh:
			writeAllowed, accountCh = loadCloudAccountSettingsSyncState(swAcc)
			if writeAllowed && localReady && cloudReady {
				if err := syncAccountSettingsState(
					ctx,
					localSettings,
					cloudSettings,
					cloudSO,
				); err != nil {
					return err
				}
			}
		case ev := <-evCh:
			switch ev.source {
			case accountSettingsSyncSourceLocal:
				localSettings = ev.settings
				localSeqno = ev.seqno
				localReady = true
			case accountSettingsSyncSourceCloud:
				cloudSettings = ev.settings
				cloudSeqno = ev.seqno
				cloudReady = true
			}
			if !localReady || !cloudReady {
				continue
			}
			if localSeqno >= cloudSeqno {
				if !writeAllowed {
					continue
				}
				if err := syncAccountSettingsState(
					ctx,
					localSettings,
					cloudSettings,
					cloudSO,
				); err != nil {
					return err
				}
				continue
			}
			if err := syncAccountSettingsState(
				ctx,
				cloudSettings,
				localSettings,
				localSO,
			); err != nil {
				return err
			}
		}
	}
}

func waitForCloudAccountSettingsRef(
	ctx context.Context,
	swAcc *provider_spacewave.ProviderAccount,
) (*sobject.SharedObjectRef, error) {
	for {
		state, err := swAcc.GetAccountState(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "get cloud account state")
		}
		if ref := buildCloudAccountSettingsRefFromState(swAcc, state); ref != nil {
			return ref, nil
		}
		if state.GetSubscriptionStatus().IsWriteAllowed() {
			ref, err := swAcc.GetAccountSettingsRef(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "ensure cloud account settings")
			}
			return ref, nil
		}

		var ch <-chan struct{}
		swAcc.GetAccountBroadcast().HoldLock(func(
			_ func(),
			getWaitCh func() <-chan struct{},
		) {
			ch = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

func buildCloudAccountSettingsRefFromState(
	swAcc *provider_spacewave.ProviderAccount,
	state *api.AccountStateResponse,
) *sobject.SharedObjectRef {
	if state == nil {
		return nil
	}
	for _, binding := range state.GetAccountSobjectBindings() {
		if binding.GetPurpose() != account_settings.BindingPurpose {
			continue
		}
		if binding.GetState() != api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_READY {
			return nil
		}
		return sobject.NewSharedObjectRef(
			swAcc.GetProviderID(),
			swAcc.GetAccountID(),
			binding.GetSoId(),
			provider_spacewave.SobjectBlockStoreID(binding.GetSoId()),
		)
	}
	return nil
}

func loadCloudAccountSettingsSyncState(
	swAcc *provider_spacewave.ProviderAccount,
) (bool, <-chan struct{}) {
	var (
		writeAllowed bool
		ch           <-chan struct{}
	)
	swAcc.GetAccountBroadcast().HoldLock(func(
		_ func(),
		getWaitCh func() <-chan struct{},
	) {
		if state := swAcc.AccountStateSnapshot(); state != nil {
			writeAllowed = state.GetSubscriptionStatus().IsWriteAllowed()
		}
		ch = getWaitCh()
	})
	return writeAllowed, ch
}

func watchAccountSettingsSyncState(
	ctx context.Context,
	ctr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	source accountSettingsSyncSource,
	evCh chan<- accountSettingsSyncEvent,
	errCh chan<- error,
) {
	var prev sobject.SharedObjectStateSnapshot
	for {
		next, err := ctr.WaitValueChange(ctx, prev, nil)
		if err != nil {
			if ctx.Err() == nil {
				errCh <- err
			}
			return
		}
		prev = next
		settings, seqno, err := decodeAccountSettingsSnapshot(ctx, next)
		if err != nil {
			errCh <- err
			return
		}
		select {
		case <-ctx.Done():
			return
		case evCh <- accountSettingsSyncEvent{
			source:   source,
			settings: settings,
			seqno:    seqno,
		}:
		}
	}
}

func decodeAccountSettingsSnapshot(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
) (*account_settings.AccountSettings, uint64, error) {
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, 0, err
	}
	settings := &account_settings.AccountSettings{}
	if rootInner == nil {
		return settings, 0, nil
	}
	if len(rootInner.GetStateData()) != 0 {
		if err := settings.UnmarshalVT(rootInner.GetStateData()); err != nil {
			return nil, 0, errors.Wrap(err, "unmarshal account settings")
		}
	}
	return settings, rootInner.GetSeqno(), nil
}

func syncAccountSettingsState(
	ctx context.Context,
	source *account_settings.AccountSettings,
	target *account_settings.AccountSettings,
	targetSO accountSettingsSyncTarget,
) error {
	ops, err := buildAccountSettingsSyncOps(source, target)
	if err != nil {
		return err
	}
	for _, opData := range ops {
		localID, err := targetSO.QueueOperation(ctx, opData)
		if err != nil {
			return errors.Wrap(err, "queue sync op")
		}
		if _, rejected, err := targetSO.WaitOperation(ctx, localID); err != nil {
			if rejected {
				_ = targetSO.ClearOperationResult(ctx, localID)
			}
			return errors.Wrap(err, "wait for sync op")
		}
	}
	return nil
}

func buildAccountSettingsSyncOps(
	source *account_settings.AccountSettings,
	target *account_settings.AccountSettings,
) ([][]byte, error) {
	if source == nil {
		source = &account_settings.AccountSettings{}
	}
	if target == nil {
		target = &account_settings.AccountSettings{}
	}

	ops := make([][]byte, 0)
	if source.GetDisplayName() != target.GetDisplayName() {
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_UpdateDisplayName{
				UpdateDisplayName: &account_settings.UpdateDisplayNameOp{
					DisplayName: source.GetDisplayName(),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}

	sourceDevices := make(map[string]*account_settings.PairedDevice, len(source.GetPairedDevices()))
	targetDevices := make(map[string]*account_settings.PairedDevice, len(target.GetPairedDevices()))
	for _, dev := range source.GetPairedDevices() {
		sourceDevices[dev.GetPeerId()] = dev
	}
	for _, dev := range target.GetPairedDevices() {
		targetDevices[dev.GetPeerId()] = dev
	}
	for _, peerID := range sortedStringKeys(sourceDevices) {
		src := sourceDevices[peerID]
		if dst, ok := targetDevices[peerID]; ok && src.EqualVT(dst) {
			continue
		}
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_AddPairedDevice{
				AddPairedDevice: src.CloneVT(),
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}
	for _, peerID := range sortedStringKeys(targetDevices) {
		if _, ok := sourceDevices[peerID]; ok {
			continue
		}
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_RemovePairedDevice{
				RemovePairedDevice: &account_settings.RemovePairedDeviceOp{
					PeerId: peerID,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}

	sourceKeypairs := make(map[string]*session.EntityKeypair, len(source.GetEntityKeypairs()))
	targetKeypairs := make(map[string]*session.EntityKeypair, len(target.GetEntityKeypairs()))
	for _, kp := range source.GetEntityKeypairs() {
		sourceKeypairs[kp.GetPeerId()] = kp
	}
	for _, kp := range target.GetEntityKeypairs() {
		targetKeypairs[kp.GetPeerId()] = kp
	}
	for _, peerID := range sortedStringKeys(sourceKeypairs) {
		src := sourceKeypairs[peerID]
		if dst, ok := targetKeypairs[peerID]; ok && src.EqualVT(dst) {
			continue
		}
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_AddEntityKeypair{
				AddEntityKeypair: src.CloneVT(),
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}
	for _, peerID := range sortedStringKeys(targetKeypairs) {
		if _, ok := sourceKeypairs[peerID]; ok {
			continue
		}
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_RemoveEntityKeypair{
				RemoveEntityKeypair: &account_settings.RemoveEntityKeypairOp{
					PeerId: peerID,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}

	sourcePresentations := make(map[string]*account_settings.SessionPresentation, len(source.GetSessionPresentations()))
	targetPresentations := make(map[string]*account_settings.SessionPresentation, len(target.GetSessionPresentations()))
	for _, pres := range source.GetSessionPresentations() {
		sourcePresentations[pres.GetPeerId()] = pres
	}
	for _, pres := range target.GetSessionPresentations() {
		targetPresentations[pres.GetPeerId()] = pres
	}
	for _, peerID := range sortedStringKeys(sourcePresentations) {
		src := sourcePresentations[peerID]
		if dst, ok := targetPresentations[peerID]; ok && src.EqualVT(dst) {
			continue
		}
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
				UpsertSessionPresentation: src.CloneVT(),
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}
	for _, peerID := range sortedStringKeys(targetPresentations) {
		if _, ok := sourcePresentations[peerID]; ok {
			continue
		}
		opData, err := marshalAccountSettingsSyncOp(&account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_RemoveSessionPresentation{
				RemoveSessionPresentation: &account_settings.RemoveSessionPresentationOp{
					PeerId: peerID,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		ops = append(ops, opData)
	}

	return ops, nil
}

func sortedStringKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func marshalAccountSettingsSyncOp(
	op *account_settings.AccountSettingsOp,
) ([]byte, error) {
	data, err := op.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal account settings sync op")
	}
	return data, nil
}
