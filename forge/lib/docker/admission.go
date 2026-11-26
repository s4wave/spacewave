package forge_lib_docker

import (
	"context"

	forge_target "github.com/s4wave/spacewave/forge/target"
)

// Admission reserves Worker capacity before a Docker runtime is created.
type Admission interface {
	// Reserve debits the declared target request for one Execution attempt.
	// It waits while the Worker admits no new work or live reservations hold
	// what the request needs, and returns an error only for a failed request,
	// such as one the Worker could never hold, or an ended context. While live
	// reservations hold what it needs, it reports the capacity wait through
	// waiting, and clears it with a nil wait before it returns a grant.
	Reserve(ctx context.Context, executionKey string, conf *Config, waiting Waiting) (Reservation, error)
}

// Waiting reports what blocks a Reserve, or nil once nothing does, with the
// signature of forge_target.ExecControllerHandle's SetWaiting.
type Waiting func(ctx context.Context, waiting *forge_target.Waiting) error

// Reservation fences one named Docker runtime and releases its capacity.
type Reservation interface {
	// Launch records stop custody and fences Docker create and start against drain.
	// A drain that fences the launch returns forge_runtime.ErrCapacityDraining
	// and voids the reservation; the caller releases it and reserves again.
	Launch(ctx context.Context, createAndStart func(runtimeName string) error) error
	// Devices returns the host device nodes of the graphics cards the
	// reservation holds, each once, for the container to map.
	Devices() []string
	// Release stops the runtime and credits capacity once.
	Release(ctx context.Context) error
}
