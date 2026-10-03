package forge_lib_docker

import (
	"reflect"
	"testing"

	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// TestStopperUsesPersistedRuntimeIdentity keeps crash recovery on the same
// Docker CLI path and endpoint that created the container.
func TestStopperUsesPersistedRuntimeIdentity(t *testing.T) {
	// Stop the container through a runner that records its persisted CLI identity.
	runner := &recordingRunner{}
	stopped, err := NewStopper(runner).StopRuntime(t.Context(), forge_runtime.BackendRuntimeIdentity{
		Backend: "docker", ID: "spacewave-runtime", StopCommand: "docker-test",
		StopEnv: []string{"DOCKER_HOST=unix:///test.sock"}, StopTimeoutSeconds: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the stop result and the exact saved command and environment.
	if !stopped {
		t.Fatal("runtime stop not confirmed")
	}
	want := recordedCommand{
		name: "docker-test", args: []string{"stop", "--time", "3", "spacewave-runtime"},
		env: []string{"DOCKER_HOST=unix:///test.sock"},
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0], want) {
		t.Fatalf("stop command = %+v, want %+v", runner.commands, want)
	}
}
