package forge_lib_docker

import "context"

// Admission reserves Worker capacity before a Docker runtime is created.
type Admission interface {
	// Reserve debits the declared target request for one Execution attempt.
	// It waits while the Worker admits no new work, and returns an error only
	// for a failed request or an ended context.
	Reserve(ctx context.Context, executionKey string, conf *Config) (Reservation, error)
}

// Reservation fences one named Docker runtime and releases its capacity.
type Reservation interface {
	// Launch records stop custody and fences Docker create and start against drain.
	// A drain that fences the launch returns forge_runtime.ErrCapacityDraining
	// and voids the reservation; the caller releases it and reserves again.
	Launch(ctx context.Context, createAndStart func(runtimeName string) error) error
	// Release stops the runtime and credits capacity once.
	Release(ctx context.Context) error
}
