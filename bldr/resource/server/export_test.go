package resource_server

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
		for _, c := range s.clients {
			if c.released {
				continue
			}
			count += len(c.resources)
			count += len(c.attachedResources)
		}
	})
	return count
}
