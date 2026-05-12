package resource_world

import "time"

// WorldStateOperationRecord describes one WorldStateResource operation.
type WorldStateOperationRecord struct {
	// Name is the WorldStateResource method name.
	Name string
	// Duration is the elapsed wall-clock duration.
	Duration time.Duration
	// Error is set when the operation returned an error.
	Error string
	// FilterCount is the number of graph filters in the request.
	FilterCount int
	// Limit is the relevant request limit.
	Limit int
	// ResultSetCount is the number of result groups returned.
	ResultSetCount int
	// ResultQuadCount is the total number of graph quads returned.
	ResultQuadCount int
	// StartKeyCount is the number of graph path start keys.
	StartKeyCount int
	// StepCount is the number of graph path steps.
	StepCount int
	// PageSize is the requested page size for resource-backed operations.
	PageSize int
	// ResultObjectCount is the number of object keys returned by the operation.
	ResultObjectCount int
	// ResourceCreated indicates that the operation attached a resource.
	ResourceCreated bool
}

// WorldStateOperationObserver observes WorldStateResource operation records.
type WorldStateOperationObserver func(WorldStateOperationRecord)

// WorldStateResourceOption configures a WorldStateResource.
type WorldStateResourceOption func(*WorldStateResource)

// WithWorldStateOperationObserver configures WorldStateResource operation accounting.
func WithWorldStateOperationObserver(observer WorldStateOperationObserver) WorldStateResourceOption {
	return func(r *WorldStateResource) {
		r.operationObserver = observer
	}
}
