package resource_server

import "context"

// CountTrackedResources returns the number of resource handles currently
// tracked across all live clients, including root, owned, and attached
// resources.
//
// It is a test-only seam for the leak regression. A value-only lookup that
// acquires and then releases a resource must leave this count unchanged, so
// the test can observe owner state without adding a production handle-count
// API.
func (s *ResourceServer) CountTrackedResources() int {
	var count int
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		count = s.countTrackedResourcesLocked()
	})
	return count
}

// WaitTrackedResourceCount waits for the tracked resource count to equal want
// and returns the last observed count when ctx ends first.
func (s *ResourceServer) WaitTrackedResourceCount(ctx context.Context, want int) int {
	for {
		var count int
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			count = s.countTrackedResourcesLocked()
			waitCh = getWaitCh()
		})
		if count == want {
			return count
		}
		select {
		case <-ctx.Done():
			return count
		case <-waitCh:
		}
	}
}

func (s *ResourceServer) countTrackedResourcesLocked() int {
	var count int
	for _, client := range s.clients {
		if client.released {
			continue
		}
		count += len(client.resources)
		count += len(client.attachedResources)
	}
	return count
}
