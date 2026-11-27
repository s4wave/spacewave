package resource_session

// resourcesListProjectionEvent carries one generation's Space index projection.
type resourcesListProjectionEvent struct {
	// id selects the shared object in the current resource list.
	id string
	// generation rejects output from a replaced projection worker.
	generation uint64
	// objectType is the projected index ObjectType, or empty when unavailable.
	objectType string
}

// apply records a projection when its current worker changes the visible type.
func (e resourcesListProjectionEvent) apply(workers map[string]resourcesListProjectionWorker, objectTypes map[string]string) bool {
	// Cancellation can race with delivery; only the current generation may publish.
	worker, ok := workers[e.id]
	if !ok || worker.generation != e.generation || objectTypes[e.id] == e.objectType {
		return false
	}

	// Retain the current type for the next resource-list response.
	objectTypes[e.id] = e.objectType
	return true
}
