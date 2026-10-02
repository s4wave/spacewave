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
	// Require the SharedObject ID.
	ctx := strm.Context()
	sharedObjectID := req.GetSharedObjectId()
	if sharedObjectID == "" {
		return errors.New("shared_object_id is required")
	}
	sender := sharedObjectHealthStreamSender{strm: strm}

	// Watch a CDN SharedObject directly.
	if r.cdnLookup != nil {
		if cdnSO, _ := r.cdnLookup(sharedObjectID); cdnSO != nil {
			return r.watchMountedSharedObjectHealth(ctx, cdnSO, sender)
		}
	}

	// Find the Session's SharedObject provider.
	providerAcc := r.session.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return err
	}

	// Look up the SharedObject's list entry.
	soListCtr, relSoListCtr, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return err
	}
	defer relSoListCtr()
	soListEntry, err := lookupSharedObjectListEntry(
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

	// Build the SharedObject reference in the Session's account.
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

	// Prefer the provider's health watch.
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

	// Otherwise mount the SharedObject and read its health.
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
	// Stream the health accessor when the SharedObject exposes one.
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

	// Otherwise stream health derived from SharedObject state.
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
	// Require the SharedObject ID.
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}

	// Snapshot a CDN SharedObject directly.
	if r.cdnLookup != nil {
		if cdnSO, _ := r.cdnLookup(sharedObjectID); cdnSO != nil {
			return r.loadMountedSharedObjectHealthSnapshot(ctx, cdnSO)
		}
	}

	// Find the Session's SharedObject provider.
	providerAcc := r.session.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, err
	}

	// Look up the SharedObject's list entry.
	soListCtr, relSoListCtr, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relSoListCtr()
	soListEntry, err := lookupSharedObjectListEntry(
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

	// Build the SharedObject reference in the Session's account.
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

	// Prefer the provider's health watch.
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

	// Otherwise mount the SharedObject and read its health.
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
	// Read the health accessor when the SharedObject exposes one.
	if healthAccessor, ok := so.(sobject.SharedObjectHealthAccessor); ok {
		healthCtr, relHealthCtr, err := healthAccessor.AccessSharedObjectHealth(ctx, nil)
		if err != nil {
			return sharedobjecthealth.Error(err), nil
		}
		defer relHealthCtr()
		return sharedobjecthealth.SnapshotWatchable(healthCtr), nil
	}

	// Otherwise snapshot health derived from SharedObject state.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return sharedobjecthealth.Error(err), nil
	}
	defer relStateCtr()
	return sharedobjecthealth.SnapshotState(stateCtr), nil
}
