package sharedobjecthealth

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
)

// Sender sends SharedObject health snapshots.
type Sender interface {
	SendHealth(*sobject.SharedObjectHealth) error
}

// StreamWatchable streams SharedObject health from a watchable.
func StreamWatchable(
	ctx context.Context,
	sender Sender,
	healthCtr ccontainer.Watchable[*sobject.SharedObjectHealth],
) error {
	if err := sender.SendHealth(Loading()); err != nil {
		return err
	}
	return ccontainer.WatchChanges(
		ctx,
		nil,
		healthCtr,
		func(health *sobject.SharedObjectHealth) error {
			return sender.SendHealth(CloneOrLoading(health))
		},
		nil,
	)
}

// StreamState streams ready/loading health derived from SharedObject state.
func StreamState(
	ctx context.Context,
	sender Sender,
	stateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) error {
	if err := sender.SendHealth(Loading()); err != nil {
		return err
	}
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return sender.SendHealth(Loading())
			}
			return sender.SendHealth(Ready())
		},
		nil,
	)
}

// Wait sends loading, sends one health snapshot, and waits for cancellation.
func Wait(ctx context.Context, sender Sender, health *sobject.SharedObjectHealth) error {
	if err := sender.SendHealth(Loading()); err != nil {
		return err
	}
	if err := sender.SendHealth(health); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// SnapshotWatchable returns the current watchable health snapshot.
func SnapshotWatchable(
	healthCtr ccontainer.Watchable[*sobject.SharedObjectHealth],
) *sobject.SharedObjectHealth {
	return CloneOrLoading(healthCtr.GetValue())
}

// SnapshotState returns the current state-derived health snapshot.
func SnapshotState(
	stateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
) *sobject.SharedObjectHealth {
	if stateCtr.GetValue() == nil {
		return Loading()
	}
	return Ready()
}

// Loading returns the common SharedObject loading health.
func Loading() *sobject.SharedObjectHealth {
	return sobject.NewSharedObjectLoadingHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	)
}

// Ready returns the common SharedObject ready health.
func Ready() *sobject.SharedObjectHealth {
	return sobject.NewSharedObjectReadyHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	)
}

// Error returns SharedObject health for an error.
func Error(err error) *sobject.SharedObjectHealth {
	return sobject.BuildSharedObjectHealthFromError(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		err,
	)
}

// CloneOrLoading clones non-nil health and replaces nil with loading.
func CloneOrLoading(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
	if health == nil {
		return Loading()
	}
	return health.CloneVT()
}
