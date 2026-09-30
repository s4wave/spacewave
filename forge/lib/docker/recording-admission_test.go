package forge_lib_docker

import "context"

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
}

// Reserve returns a deterministic named runtime grant.
func (a *recordingAdmission) Reserve(_ context.Context, key string, conf *Config) (Reservation, error) {
	a.executionKey = key
	a.request = conf.CloneVT()
	return a, nil
}

// Launch executes Docker creation under the fake's grant.
func (a *recordingAdmission) Launch(_ context.Context, createAndStart func(string) error) error {
	return createAndStart(a.name)
}

// Release records the configured stop effect when this test needs one.
func (a *recordingAdmission) Release(ctx context.Context) error {
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
