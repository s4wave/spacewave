package provider_local

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/sobject"
)

// accountObjectSync ties a SharedObject's sync, copy, hosting, and holders work
// to its presence in the local account inventory. Account retirement also cancels it.
type accountObjectSync struct {
	sync    *routine.RoutineContainer
	copy    *routine.RoutineContainer
	hosting *routine.RoutineContainer
	// holders keeps the bucket exchange's holders current, or is nil when the
	// bucket has no exchange.
	holders *routine.RoutineContainer
	cancel  context.CancelFunc
	release func()
}

// workers returns the object's routines that exist.
func (o *accountObjectSync) workers() []*routine.RoutineContainer {
	return slices.DeleteFunc([]*routine.RoutineContainer{o.sync, o.copy, o.hosting, o.holders}, func(worker *routine.RoutineContainer) bool {
		return worker == nil
	})
}

// removeMissingObjects stops deleted objects before starting new inventory work.
func (s *p2pSyncState) removeMissingObjects(list *sobject.SharedObjectList) {
	// Record the object IDs present in the new inventory.
	present := make(map[string]struct{}, len(list.GetSharedObjects()))
	for _, entry := range list.GetSharedObjects() {
		present[entry.GetRef().GetProviderResourceRef().GetId()] = struct{}{}
	}

	// Stop and collect every sync missing from the inventory.
	var removed []*accountObjectSync
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		for id, object := range s.soSync {
			if _, found := present[id]; found {
				continue
			}
			removed = append(removed, object)
			delete(s.soSync, id)
			delete(s.copyProgress, id)
		}
		if len(removed) != 0 {
			bcast()
		}
	})

	// Release each removed sync after its workers exit.
	for _, object := range removed {
		object.cancel()
		for _, worker := range object.workers() {
			if exited, _ := worker.SetRoutine(nil); exited != nil {
				<-exited
			}
		}
		object.release()
	}
}
