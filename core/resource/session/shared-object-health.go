package resource_session

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/resource/session/sharedobjecthealth"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchSharedObjectHealth streams SharedObject health by SharedObject ID.
func (r *SessionResource) WatchSharedObjectHealth(
	req *s4wave_session.WatchSharedObjectHealthRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchSharedObjectHealthStream,
) error {
	ctx := strm.Context()
	sharedObjectID := req.GetSharedObjectId()
	if sharedObjectID == "" {
		return errors.New("shared_object_id is required")
	}
	sender := sharedObjectHealthStreamSender{strm: strm}

	if r.cdnLookup != nil {
		if cdnSO, _ := r.cdnLookup(sharedObjectID); cdnSO != nil {
			return r.watchMountedSharedObjectHealth(ctx, cdnSO, sender)
		}
	}

	providerAcc := r.session.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return err
	}
	soListCtr, relSoListCtr, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return err
	}
	defer relSoListCtr()

	soListEntry, err := r.lookupSharedObjectListEntry(
		ctx,
		soProvider,
		soListCtr,
		sharedObjectID,
	)
	if err != nil {
		return err
	}
	if soListEntry == nil {
		return sharedobjecthealth.Wait(
			ctx,
			sender,
			sharedobjecthealth.Error(sobject.ErrSharedObjectNotFound),
		)
	}

	sessionProviderResourceRef := r.session.GetSessionRef().GetProviderResourceRef().CloneVT()
	sessionProviderResourceRef.Id = sharedObjectID
	if err := sessionProviderResourceRef.Validate(); err != nil {
		return err
	}
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: sessionProviderResourceRef,
		BlockStoreId:        soListEntry.GetRef().GetBlockStoreId(),
	}
	if err := soRef.Validate(); err != nil {
		return err
	}
	if healthProvider, ok := sobject.GetSharedObjectHealthProvider(providerAcc); ok {
		healthCtr, relHealthCtr, err := healthProvider.AccessSharedObjectHealth(
			ctx,
			soRef,
			nil,
		)
		if err != nil {
			return err
		}
		defer relHealthCtr()
		return sharedobjecthealth.StreamWatchable(ctx, sender, healthCtr)
	}

	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(
		ctx,
		r.session.GetBus(),
		soRef,
		false,
		nil,
	)
	if err != nil {
		return sharedobjecthealth.Wait(
			ctx,
			sender,
			sharedobjecthealth.Error(err),
		)
	}
	defer mountedSoRef.Release()

	return r.watchMountedSharedObjectHealth(ctx, mountedSo, sender)
}

type sharedObjectHealthStreamSender struct {
	strm s4wave_session.SRPCSessionResourceService_WatchSharedObjectHealthStream
}

func (s sharedObjectHealthStreamSender) SendHealth(health *sobject.SharedObjectHealth) error {
	return s.strm.Send(&s4wave_session.WatchSharedObjectHealthResponse{
		Health: health,
	})
}

// watchMountedSharedObjectHealth streams health for an already mounted SharedObject.
func (r *SessionResource) watchMountedSharedObjectHealth(
	ctx context.Context,
	so sobject.SharedObject,
	sender sharedobjecthealth.Sender,
) error {
	if healthAccessor, ok := so.(sobject.SharedObjectHealthAccessor); ok {
		healthCtr, relHealthCtr, err := healthAccessor.AccessSharedObjectHealth(ctx, nil)
		if err != nil {
			return sharedobjecthealth.Wait(
				ctx,
				sender,
				sharedobjecthealth.Error(err),
			)
		}
		defer relHealthCtr()
		return sharedobjecthealth.StreamWatchable(ctx, sender, healthCtr)
	}

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return sharedobjecthealth.Wait(
			ctx,
			sender,
			sharedobjecthealth.Error(err),
		)
	}
	defer relStateCtr()

	return sharedobjecthealth.StreamState(ctx, sender, stateCtr)
}

// loadSharedObjectHealthSnapshot returns one SharedObject health snapshot.
func (r *SessionResource) loadSharedObjectHealthSnapshot(
	ctx context.Context,
	sharedObjectID string,
) (*sobject.SharedObjectHealth, error) {
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}

	if r.cdnLookup != nil {
		if cdnSO, _ := r.cdnLookup(sharedObjectID); cdnSO != nil {
			return r.loadMountedSharedObjectHealthSnapshot(ctx, cdnSO)
		}
	}

	providerAcc := r.session.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, err
	}
	soListCtr, relSoListCtr, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relSoListCtr()

	soListEntry, err := r.lookupSharedObjectListEntry(
		ctx,
		soProvider,
		soListCtr,
		sharedObjectID,
	)
	if err != nil {
		return nil, err
	}
	if soListEntry == nil {
		return sharedobjecthealth.Error(sobject.ErrSharedObjectNotFound), nil
	}

	sessionProviderResourceRef := r.session.GetSessionRef().GetProviderResourceRef().CloneVT()
	sessionProviderResourceRef.Id = sharedObjectID
	if err := sessionProviderResourceRef.Validate(); err != nil {
		return nil, err
	}
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: sessionProviderResourceRef,
		BlockStoreId:        soListEntry.GetRef().GetBlockStoreId(),
	}
	if err := soRef.Validate(); err != nil {
		return nil, err
	}
	if healthProvider, ok := sobject.GetSharedObjectHealthProvider(providerAcc); ok {
		healthCtr, relHealthCtr, err := healthProvider.AccessSharedObjectHealth(
			ctx,
			soRef,
			nil,
		)
		if err != nil {
			return sharedobjecthealth.Error(err), nil
		}
		defer relHealthCtr()
		return sharedobjecthealth.SnapshotWatchable(healthCtr), nil
	}

	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(
		ctx,
		r.session.GetBus(),
		soRef,
		false,
		nil,
	)
	if err != nil {
		return sharedobjecthealth.Error(err), nil
	}
	defer mountedSoRef.Release()

	return r.loadMountedSharedObjectHealthSnapshot(ctx, mountedSo)
}

// loadMountedSharedObjectHealthSnapshot returns one health snapshot for a mounted SO.
func (r *SessionResource) loadMountedSharedObjectHealthSnapshot(
	ctx context.Context,
	so sobject.SharedObject,
) (*sobject.SharedObjectHealth, error) {
	if healthAccessor, ok := so.(sobject.SharedObjectHealthAccessor); ok {
		healthCtr, relHealthCtr, err := healthAccessor.AccessSharedObjectHealth(ctx, nil)
		if err != nil {
			return sharedobjecthealth.Error(err), nil
		}
		defer relHealthCtr()
		return sharedobjecthealth.SnapshotWatchable(healthCtr), nil
	}

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return sharedobjecthealth.Error(err), nil
	}
	defer relStateCtr()
	return sharedobjecthealth.SnapshotState(stateCtr), nil
}
