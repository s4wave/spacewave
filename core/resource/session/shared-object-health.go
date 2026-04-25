package resource_session

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
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
	if err := strm.Send(&s4wave_session.WatchSharedObjectHealthResponse{
		Health: sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	}); err != nil {
		return err
	}

	if r.cdnLookup != nil {
		if cdnSO, _ := r.cdnLookup(sharedObjectID); cdnSO != nil {
			return r.watchMountedSharedObjectHealth(ctx, cdnSO, strm)
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
		providerAcc,
		soListCtr,
		sharedObjectID,
	)
	if err != nil {
		return err
	}
	if soListEntry == nil {
		return waitSharedObjectHealth(
			ctx,
			strm,
			sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				sobject.ErrSharedObjectNotFound,
			),
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
		return watchSharedObjectHealthWatchable(ctx, strm, healthCtr)
	}

	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(
		ctx,
		r.session.GetBus(),
		soRef,
		false,
		nil,
	)
	if err != nil {
		return waitSharedObjectHealth(
			ctx,
			strm,
			sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				err,
			),
		)
	}
	defer mountedSoRef.Release()

	return r.watchMountedSharedObjectHealth(ctx, mountedSo, strm)
}

// watchSharedObjectHealthWatchable streams SharedObject health from a watchable.
func watchSharedObjectHealthWatchable(
	ctx context.Context,
	strm s4wave_session.SRPCSessionResourceService_WatchSharedObjectHealthStream,
	healthCtr ccontainer.Watchable[*sobject.SharedObjectHealth],
) error {
	return ccontainer.WatchChanges(
		ctx,
		nil,
		healthCtr,
		func(health *sobject.SharedObjectHealth) error {
			if health == nil {
				health = sobject.NewSharedObjectLoadingHealth(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				)
			}
			return strm.Send(&s4wave_session.WatchSharedObjectHealthResponse{
				Health: health,
			})
		},
		nil,
	)
}

// watchMountedSharedObjectHealth streams health for an already mounted SharedObject.
func (r *SessionResource) watchMountedSharedObjectHealth(
	ctx context.Context,
	so sobject.SharedObject,
	strm s4wave_session.SRPCSessionResourceService_WatchSharedObjectHealthStream,
) error {
	if healthAccessor, ok := so.(sobject.SharedObjectHealthAccessor); ok {
		healthCtr, relHealthCtr, err := healthAccessor.AccessSharedObjectHealth(ctx, nil)
		if err != nil {
			return waitSharedObjectHealth(
				ctx,
				strm,
				sobject.BuildSharedObjectHealthFromError(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
					err,
				),
			)
		}
		defer relHealthCtr()
		return watchSharedObjectHealthWatchable(ctx, strm, healthCtr)
	}

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return waitSharedObjectHealth(
			ctx,
			strm,
			sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				err,
			),
		)
	}
	defer relStateCtr()

	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return strm.Send(&s4wave_session.WatchSharedObjectHealthResponse{
					Health: sobject.NewSharedObjectLoadingHealth(
						sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
					),
				})
			}
			return strm.Send(&s4wave_session.WatchSharedObjectHealthResponse{
				Health: sobject.NewSharedObjectReadyHealth(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				),
			})
		},
		nil,
	)
}

// waitSharedObjectHealth sends one health snapshot and waits for cancellation.
func waitSharedObjectHealth(
	ctx context.Context,
	strm s4wave_session.SRPCSessionResourceService_WatchSharedObjectHealthStream,
	health *sobject.SharedObjectHealth,
) error {
	if err := strm.Send(&s4wave_session.WatchSharedObjectHealthResponse{
		Health: health,
	}); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
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
		providerAcc,
		soListCtr,
		sharedObjectID,
	)
	if err != nil {
		return nil, err
	}
	if soListEntry == nil {
		return sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			sobject.ErrSharedObjectNotFound,
		), nil
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
			return sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				err,
			), nil
		}
		defer relHealthCtr()
		return cloneOrLoadingHealth(healthCtr.GetValue()), nil
	}

	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(
		ctx,
		r.session.GetBus(),
		soRef,
		false,
		nil,
	)
	if err != nil {
		return sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		), nil
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
			return sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				err,
			), nil
		}
		defer relHealthCtr()
		return cloneOrLoadingHealth(healthCtr.GetValue()), nil
	}

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		), nil
	}
	defer relStateCtr()
	if stateCtr.GetValue() == nil {
		return sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		), nil
	}
	return sobject.NewSharedObjectReadyHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	), nil
}

func cloneOrLoadingHealth(
	health *sobject.SharedObjectHealth,
) *sobject.SharedObjectHealth {
	if health == nil {
		return sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		)
	}
	return health.CloneVT()
}
