package forge_lib_docker

import (
	"context"

	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// recordingAdmission supplies an offline admission boundary to Docker tests.
type recordingAdmission struct {
	// name is the preactivated runtime returned to the controller.
	name string
	// release records the runtime cleanup effect when configured.
	release func(context.Context) error
	// executionKey records the attempt submitted for admission.
	executionKey string
	// request retains a copy of the admitted Docker configuration.
	request *Config
	// drainedLaunches is how many launches a drain fences before one proceeds.
	drainedLaunches int
	// reserves counts the reservations requested.
	reserves int
	// releases counts the reservations released.
	releases int
}

// Reserve returns a deterministic named runtime grant.
func (a *recordingAdmission) Reserve(_ context.Context, key string, conf *Config) (Reservation, error) {
	a.reserves++
	a.executionKey = key
	a.request = conf.CloneVT()
	return a, nil
}

// Launch executes Docker creation under the fake's grant.
func (a *recordingAdmission) Launch(_ context.Context, createAndStart func(string) error) error {
	if a.drainedLaunches != 0 {
		a.drainedLaunches--
		return forge_runtime.ErrCapacityDraining
	}
	return createAndStart(a.name)
}

// Release records the configured stop effect when this test needs one.
func (a *recordingAdmission) Release(ctx context.Context) error {
	a.releases++
	if a.release != nil {
		return a.release(ctx)
	}
	return nil
}

// _ verifies the production admission and reservation contracts.
var (
	_ Admission   = (*recordingAdmission)(nil)
	_ Reservation = (*recordingAdmission)(nil)
)
