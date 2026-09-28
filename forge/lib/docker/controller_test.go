package forge_lib_docker

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// recordingAdmission supplies an offline admission boundary to Docker tests.
type recordingAdmission struct {
	name         string
	release      func(context.Context) error
	executionKey string
	request      *Config
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

func TestBuildCreateArgsPinsEnvMountsWorkdirImageCommand(t *testing.T) {
	t.Setenv("FORGE_DOCKER_SENTINEL", "host-secret")

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

func TestBuildDockerEnvIsExplicit(t *testing.T) {
	t.Setenv("FORGE_DOCKER_SENTINEL", "host-secret")

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
	runner := &recordingRunner{}
	ctrl := NewController(nil, nil, &Config{Image: "img"}, &recordingAdmission{name: "unused"})
	ctrl.runner = runner
	ctrl.handle = noopExecHandle{}
	if err := ctrl.Execute(t.Context()); err == nil {
		t.Fatal("missing capacity request was accepted")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Docker commands ran before request validation: %+v", runner.commands)
	}
}

func TestExecuteRunsCreateStartWait(t *testing.T) {
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

	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatal(err.Error())
	}
	if admission.executionKey != "exec/test" || admission.request.GetMilliCpu() != 1000 || admission.request.GetMemoryBytes() != 1<<20 {
		t.Fatalf("Docker request did not reach admission: key=%q request=%+v", admission.executionKey, admission.request)
	}

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
	runner := &recordingRunner{outputs: map[string][]byte{
		"create": []byte("container-123\n"),
		"wait":   []byte("2\n"),
		"logs":   []byte("compile failed\n"),
	}, stderr: []byte("missing package\n")}
	ctrl := NewController(nil, nil, &Config{Image: "go:latest", MilliCpu: 1000, MemoryBytes: 1 << 20}, &recordingAdmission{name: "test-runtime"})
	ctrl.runner = runner
	handle := &recordingExecHandle{}
	ctrl.handle = handle

	err := ctrl.Execute(context.Background())
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

func TestExecuteStopsContainerOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

	<-runner.waitStarted
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

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

func TestDockerIntegrationSkippedWithoutDaemon(t *testing.T) {
	if os.Getenv("FORGE_DOCKER_INTEGRATION") == "" {
		t.Skip("set FORGE_DOCKER_INTEGRATION=1 to run docker daemon integration")
	}
	if _, err := NewExecDockerRunner().Run(context.Background(), "docker", []string{"info"}, nil); err != nil {
		t.Skip(err.Error())
	}
}

type recordedCommand struct {
	name string
	args []string
	env  []string
}

type recordingRunner struct {
	outputs     map[string][]byte
	errors      map[string]error
	stderr      []byte
	commands    []recordedCommand
	waitStarted chan struct{}
	waitCancel  func()
}

// Logs records the Docker log read and returns its separate output streams.
func (r *recordingRunner) Logs(ctx context.Context, name, containerID string, env []string) ([]byte, []byte, error) {
	stdout, err := r.Run(ctx, name, []string{"logs", containerID}, env)
	return stdout, r.stderr, err
}

// recordedLog is one retained Execution log entry.
type recordedLog struct {
	level   string
	message string
}

// recordingExecHandle records output written by the Docker controller.
type recordingExecHandle struct {
	noopExecHandle
	logs []recordedLog
}

// WriteLog records one container output stream.
func (h *recordingExecHandle) WriteLog(ctx context.Context, level, message string) error {
	h.logs = append(h.logs, recordedLog{level, message})
	return nil
}

func (r *recordingRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	cmd := recordedCommand{
		name: name,
		args: slices.Clone(args),
		env:  slices.Clone(env),
	}
	r.commands = append(r.commands, cmd)
	if len(args) == 0 {
		return nil, nil
	}
	if args[0] == "wait" && r.waitStarted != nil {
		close(r.waitStarted)
		r.waitCancel()
		<-ctx.Done()
		return nil, context.Canceled
	}
	if err := r.errors[args[0]]; err != nil {
		return nil, err
	}
	return r.outputs[args[0]], nil
}

type noopExecHandle struct{}

func (noopExecHandle) GetExecutionUniqueId() string {
	return "test-exec"
}

// GetExecutionObjectKey identifies the fake's one durable attempt.
func (noopExecHandle) GetExecutionObjectKey() string { return "exec/test" }

func (noopExecHandle) GetPeerId() peer.ID {
	return ""
}

func (noopExecHandle) GetTimestamp() *timestamp.Timestamp {
	return &timestamp.Timestamp{}
}

func (noopExecHandle) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return nil
}

func (noopExecHandle) SetOutputs(
	ctx context.Context,
	outps forge_value.ValueSlice,
	clearOld bool,
) error {
	return nil
}

func (noopExecHandle) WriteLog(ctx context.Context, level, message string) error {
	return nil
}

func (noopExecHandle) SetWaitingPlugin(context.Context, string) error { return nil }
