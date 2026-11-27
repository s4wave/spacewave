package resource_server

import (
	"slices"
)

// removeRoute removes one routing entry and adjusts the live handle count.
// Generation locks may acquire bcast; no server lock acquires a generation lock.
func (s *ResourceServer) removeRoute(id uint32, count int) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(s.routes, id)
		s.trackedCount -= count
		if count != 0 {
			broadcast()
		}
	})
}

// releaseClientGeneration transfers the generation's maps to an exclusive drain.
func (s *ResourceServer) releaseClientGeneration(client *RemoteResourceClient) {
	// Detach in constant space before walking the private tree or calling cleanup.
	// Accepted registrations belong to exactly one drain. After detach, registration
	// returns ErrClientReleased and the caller retains its cleanup obligation.
	var retired *RemoteResourceClient
	client.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Transfer the tree exactly once, including resources racing registration.
		if client.released {
			return
		}
		client.released = true
		retired = &RemoteResourceClient{
			server: s, released: true, rootResourceID: client.rootResourceID,
			resources: client.resources, children: client.children,
			attachedResources: client.attachedResources,
		}
		client.resources, client.children, client.attachedResources = nil, nil, nil

		// Remove admission and subtract the complete detached handle count.
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			delete(s.clients, client.clientID)
			s.trackedCount -= len(retired.resources) + len(retired.attachedResources)
			broadcast()
		})
		broadcast()
	})
	if retired == nil {
		return
	}

	// Cancel all attached transports before running native resource callbacks.
	for id, attached := range retired.attachedResources {
		s.removeRoute(id, 0)
		attached.cancel()
	}

	// Preserve child-first, ascending sibling order, followed by detached roots.
	var releaseFns []func()
	retired.releaseAllChildrenLocked(retired.rootResourceID, &releaseFns)
	remaining := make([]uint32, 0, len(retired.resources))
	for id := range retired.resources {
		remaining = append(remaining, id)
	}
	slices.Sort(remaining)
	for _, id := range remaining {
		retired.releaseAllChildrenLocked(id, &releaseFns)
	}

	// Every callback observes the entire generation as unavailable and runs unlocked.
	for _, releaseFn := range releaseFns {
		releaseFn()
	}
}
