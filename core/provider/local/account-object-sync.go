package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/sobject"
)

// accountObjectSync ties a SharedObject's sync, copy, and hosting work
// to its presence in the local account inventory. Account retirement also cancels it.
type accountObjectSync struct {
	sync    *routine.RoutineContainer
	copy    *routine.RoutineContainer
	hosting *routine.RoutineContainer
	cancel  context.CancelFunc
	release func()
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
		for _, worker := range []*routine.RoutineContainer{object.sync, object.copy, object.hosting} {
			if worker == nil {
				continue
			}
			if exited, _ := worker.SetRoutine(nil); exited != nil {
				<-exited
			}
		}
		object.release()
	}
}
