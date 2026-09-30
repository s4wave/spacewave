package forge_lib_docker

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"
)

// TestBuildCreateArgsPinsEnvMountsWorkdirImageCommand preserves explicit container options.
func TestBuildCreateArgsPinsEnvMountsWorkdirImageCommand(t *testing.T) {
	// Seed a host environment value that must not reach Docker.
	t.Setenv("FORGE_DOCKER_SENTINEL", "host-secret")

	// Configure explicit container options for the Docker create command.
	conf := &Config{
		Image:       "ghbot:dev",
		Workdir:     "/work/repo",
		MilliCpu:    1500,
		MemoryBytes: 1 << 30,
		Env: map[string]string{
			"BETA":  "two",
			"ALPHA": "one",
		},
		Mounts: []*Mount{
			{HostPath: "/host/socket", ContainerPath: "/run/ghbot.sock"},
			{HostPath: "/host/work", ContainerPath: "/work/repo", ReadOnly: true},
		},
		Command: []string{"ghbot-agent", "-h"},
	}

	// Compare the Docker create arguments with their explicit configuration.
	got := buildCreateArgs(conf, "")
	want := []string{
		"create",
		"--cpus", "1.5", "--memory", "1073741824",
		"--workdir", "/work/repo",
		"--env", "ALPHA=one",
		"--env", "BETA=two",
		"--mount", "type=bind,source=/host/socket,target=/run/ghbot.sock",
		"--mount", "type=bind,source=/host/work,target=/work/repo,readonly",
		"ghbot:dev",
		"ghbot-agent", "-h",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("create args mismatch\nwant: %#v\n got: %#v", want, got)
	}
	for _, arg := range got {
		if arg == "FORGE_DOCKER_SENTINEL=host-secret" {
			t.Fatal("host environment leaked into container env args")
		}
	}
}

// TestBuildDockerEnvIsExplicit excludes the host environment from Docker commands.
func TestBuildDockerEnvIsExplicit(t *testing.T) {
	// Seed a host environment value that must not reach Docker.
	t.Setenv("FORGE_DOCKER_SENTINEL", "host-secret")

	// Require Docker CLI environment to contain only configured entries.
	got := BuildDockerEnv(&Config{
		DockerEnv: map[string]string{
			"DOCKER_HOST": "unix:///var/run/docker.sock",
		},
	})
	want := []string{"DOCKER_HOST=unix:///var/run/docker.sock"}
	if !slices.Equal(got, want) {
		t.Fatalf("docker env mismatch\nwant: %#v\n got: %#v", want, got)
	}
	if slices.Contains(got, "FORGE_DOCKER_SENTINEL=host-secret") {
		t.Fatal("host environment leaked into docker CLI env")
	}
}

// TestExecuteRejectsMissingRequestBeforeCreate keeps the Docker effect behind
// the target's explicit capacity declaration.
func TestExecuteRejectsMissingRequestBeforeCreate(t *testing.T) {
	// Configure an offline Docker runner and execution admission grant.
	runner := &recordingRunner{}
	ctrl := NewController(nil, nil, &Config{Image: "img"}, &recordingAdmission{name: "unused"})
	ctrl.runner = runner
	ctrl.handle = noopExecHandle{}

	// Reject execution before any Docker effect when admission data is absent.
	if err := ctrl.Execute(t.Context()); err == nil {
		t.Fatal("missing capacity request was accepted")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Docker commands ran before request validation: %+v", runner.commands)
	}
}

// TestExecuteRunsCreateStartWait retains admitted Docker output after a successful run.
func TestExecuteRunsCreateStartWait(t *testing.T) {
	// Configure an offline Docker runner and execution admission grant.
	runner := &recordingRunner{
		outputs: map[string][]byte{
			"create": []byte("container-123\n"),
			"start":  []byte("container-123\n"),
			"wait":   []byte("0\n"),
			"logs":   []byte("build complete\n"),
		},
		stderr: []byte("warning\n"),
	}
	admission := &recordingAdmission{name: "test-runtime"}
	ctrl := NewController(nil, nil, &Config{
		DockerPath:  "docker-test",
		DockerEnv:   map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"},
		Image:       "ghbot:dev",
		MilliCpu:    1000,
		MemoryBytes: 1 << 20,
		Workdir:     "/work/repo",
		Env:         map[string]string{"GHBOT_SOCKET": "/run/ghbot.sock"},
		Mounts: []*Mount{
			{HostPath: "/host/socket", ContainerPath: "/run/ghbot.sock"},
		},
		Command: []string{"ghbot-agent", "-h"},
	}, admission)
	ctrl.runner = runner
	handle := &recordingExecHandle{}
	ctrl.handle = handle

	// Execute the admitted Docker commands and inspect their retained output.
	if err := ctrl.Execute(t.Context()); err != nil {
		t.Fatal(err.Error())
	}
	if admission.executionKey != "exec/test" || admission.request.GetMilliCpu() != 1000 || admission.request.GetMemoryBytes() != 1<<20 {
		t.Fatalf("Docker request did not reach admission: key=%q request=%+v", admission.executionKey, admission.request)
	}

	// Compare the complete Docker command sequence with the admitted runtime name.
	want := []recordedCommand{
		{
			name: "docker-test",
			args: []string{
				"create", "--name", "test-runtime", "--cpus", "1", "--memory", "1048576",
				"--workdir", "/work/repo",
				"--env", "GHBOT_SOCKET=/run/ghbot.sock",
				"--mount", "type=bind,source=/host/socket,target=/run/ghbot.sock",
				"ghbot:dev",
				"ghbot-agent", "-h",
			},
			env: []string{"DOCKER_HOST=unix:///var/run/docker.sock"},
		},
		{name: "docker-test", args: []string{"start", "container-123"}, env: []string{"DOCKER_HOST=unix:///var/run/docker.sock"}},
		{name: "docker-test", args: []string{"wait", "container-123"}, env: []string{"DOCKER_HOST=unix:///var/run/docker.sock"}},
		{name: "docker-test", args: []string{"logs", "container-123"}, env: []string{"DOCKER_HOST=unix:///var/run/docker.sock"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, runner.commands)
	}
	if !reflect.DeepEqual(handle.logs, []recordedLog{{"info", "build complete\n"}, {"error", "warning\n"}}) {
		t.Fatalf("retained output mismatch: %#v", handle.logs)
	}
}

// TestExecuteRetainsOutputOnFailure verifies failed command output survives in
// the Execution log along with its nonzero result.
func TestExecuteRetainsOutputOnFailure(t *testing.T) {
	// Configure an offline Docker runner and execution admission grant.
	runner := &recordingRunner{outputs: map[string][]byte{
		"create": []byte("container-123\n"),
		"wait":   []byte("2\n"),
		"logs":   []byte("compile failed\n"),
	}, stderr: []byte("missing package\n")}
	ctrl := NewController(nil, nil, &Config{Image: "go:latest", MilliCpu: 1000, MemoryBytes: 1 << 20}, &recordingAdmission{name: "test-runtime"})
	ctrl.runner = runner
	handle := &recordingExecHandle{}
	ctrl.handle = handle

	// Preserve logs when the Docker container exits with a failure.
	err := ctrl.Execute(t.Context())
	if err == nil || err.Error() != "docker container exited with status 2" {
		t.Fatalf("unexpected exit result: %v", err)
	}
	if !reflect.DeepEqual(handle.logs, []recordedLog{{"info", "compile failed\n"}, {"error", "missing package\n"}}) {
		t.Fatalf("failed command output missing: %#v", handle.logs)
	}
}

// TestExecuteLaunchFailuresReleaseNamedGrant checks that Docker create and
// start errors still surrender the preactivated named runtime grant.
func TestExecuteLaunchFailuresReleaseNamedGrant(t *testing.T) {
	for _, stage := range []string{"create", "start"} {
		t.Run(stage, func(t *testing.T) {
			// Configure an offline Docker runner and execution admission grant.
			runner := &recordingRunner{
				outputs: map[string][]byte{"create": []byte("container-123\n")},
				errors:  map[string]error{stage: context.Canceled},
			}
			released := 0
			grant := &recordingAdmission{name: "named-runtime", release: func(context.Context) error {
				released++
				return nil
			}}
			ctrl := NewController(nil, nil, &Config{Image: "img", MilliCpu: 1000, MemoryBytes: 1 << 20}, grant)
			ctrl.runner = runner
			ctrl.handle = noopExecHandle{}

			// Execute the failed Docker launch and inspect release of its named grant.
			if err := ctrl.Execute(t.Context()); err == nil {
				t.Fatal("failed Docker launch returned no error")
			}
			if released != 1 {
				t.Fatalf("grant released %d times, want once", released)
			}
			if len(runner.commands) == 0 || !slices.Contains(runner.commands[0].args, "named-runtime") {
				t.Fatalf("Docker create lost runtime name: %+v", runner.commands)
			}
		})
	}
}

// TestExecuteStopsContainerOnCancel releases the runtime grant after cancellation.
func TestExecuteStopsContainerOnCancel(t *testing.T) {
	// Give the offline runner a cancellation gate at container wait.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// Configure an offline Docker runner and execution admission grant.
	runner := &recordingRunner{
		outputs: map[string][]byte{
			"create": []byte("container-123\n"),
			"start":  []byte("container-123\n"),
			"stop":   []byte("container-123\n"),
		},
		waitStarted: make(chan struct{}),
		waitCancel:  cancel,
	}
	ctrl := NewController(nil, nil, &Config{
		DockerPath:         "docker-test",
		Image:              "ghbot:dev",
		StopTimeoutSeconds: 3,
		MilliCpu:           1000,
		MemoryBytes:        1 << 20,
	}, &recordingAdmission{name: "test-runtime", release: func(ctx context.Context) error {
		_, err := runner.Run(ctx, "docker-test", []string{"stop", "--time", "3", "test-runtime"}, []string{})
		return err
	}})
	ctrl.runner = runner
	ctrl.handle = noopExecHandle{}

	// Join the Docker execution after the runner signals cancellation.
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

	// Observe the runner's wait gate and join cancellation.
	<-runner.waitStarted
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Require release of the named Docker runtime with its configured stop timeout.
	wantStop := recordedCommand{
		name: "docker-test",
		args: []string{"stop", "--time", "3", "test-runtime"},
		env:  []string{},
	}
	if !slices.ContainsFunc(runner.commands, func(cmd recordedCommand) bool {
		return reflect.DeepEqual(cmd, wantStop)
	}) {
		t.Fatalf("missing stop command in %#v", runner.commands)
	}
}

// TestDockerIntegrationSkippedWithoutDaemon runs only with explicit daemon opt-in.
func TestDockerIntegrationSkippedWithoutDaemon(t *testing.T) {
	if os.Getenv("FORGE_DOCKER_INTEGRATION") == "" {
		t.Skip("set FORGE_DOCKER_INTEGRATION=1 to run docker daemon integration")
	}
	if _, err := NewExecDockerRunner().Run(context.Background(), "docker", []string{"info"}, nil); err != nil {
		t.Skip(err.Error())
	}
}
