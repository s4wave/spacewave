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

func TestBuildCreateArgsPinsEnvMountsWorkdirImageCommand(t *testing.T) {
	t.Setenv("FORGE_DOCKER_SENTINEL", "host-secret")

	conf := &Config{
		Image:   "ghbot:dev",
		Workdir: "/work/repo",
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

	got := buildCreateArgs(conf)
	want := []string{
		"create",
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

	got := buildDockerEnv(&Config{
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

func TestExecuteRunsCreateStartWait(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string][]byte{
			"create": []byte("container-123\n"),
			"start":  []byte("container-123\n"),
			"wait":   []byte("0\n"),
		},
	}
	ctrl := NewController(nil, nil, &Config{
		DockerPath: "docker-test",
		DockerEnv:  map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"},
		Image:      "ghbot:dev",
		Workdir:    "/work/repo",
		Env:        map[string]string{"GHBOT_SOCKET": "/run/ghbot.sock"},
		Mounts: []*Mount{
			{HostPath: "/host/socket", ContainerPath: "/run/ghbot.sock"},
		},
		Command: []string{"ghbot-agent", "-h"},
	})
	ctrl.runner = runner
	ctrl.handle = noopExecHandle{}

	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatal(err.Error())
	}

	want := []recordedCommand{
		{
			name: "docker-test",
			args: []string{
				"create",
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
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, runner.commands)
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
	})
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
		args: []string{"stop", "--time", "3", "container-123"},
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
	commands    []recordedCommand
	waitStarted chan struct{}
	waitCancel  func()
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
	return r.outputs[args[0]], nil
}

type noopExecHandle struct{}

func (noopExecHandle) GetExecutionUniqueId() string {
	return "test-exec"
}

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
