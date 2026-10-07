package forge_lib_docker

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// outputsHandle stores blocks in a test World and records the outputs set.
type outputsHandle struct {
	// ExecControllerHandle supplies storage access.
	forge_target.ExecControllerHandle
	// outputs records the values passed to SetOutputs.
	outputs forge_value.ValueSlice
}

// SetOutputs records the published outputs.
func (h *outputsHandle) SetOutputs(_ context.Context, outputs forge_value.ValueSlice, _ bool) error {
	h.outputs = append(h.outputs, outputs...)
	return nil
}

// WriteLog accepts container logs.
func (h *outputsHandle) WriteLog(context.Context, string, string) error {
	return nil
}

// outputsRunner writes files into the mounted output directory at create.
type outputsRunner struct {
	*recordingRunner
	// write populates the host output directory.
	write func(dir string) error
}

// Run applies write to the host directory mounted at /out.
func (r *outputsRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	if len(args) != 0 && args[0] == "create" {
		for _, arg := range args {
			source, ok := strings.CutPrefix(arg, "type=bind,source=")
			if dir, found := strings.CutSuffix(source, ",target=/out"); ok && found {
				if err := r.write(dir); err != nil {
					return nil, err
				}
			}
		}
	}
	return r.recordingRunner.Run(ctx, name, args, env)
}

// newOutputsController builds a Docker controller declaring two outputs and
// storing into a fresh World.
func newOutputsController(t *testing.T, write func(dir string) error) (*Controller, *outputsHandle) {
	// Open a World for output blocks.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Write output blocks through a stage, as an Execution handle does.
	stage, err := tb.WorldState.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stage.Release)
	handle := &outputsHandle{ExecControllerHandle: forge_target.ExecControllerHandleWithAccess(
		"test-exec", "", tb.Engine, stage.AccessWorldState, nil,
	)}

	// Run a container that exits zero after write fills its output directory.
	ctrl := NewController(nil, nil, &Config{
		Image:       "img",
		MilliCpu:    1000,
		MemoryBytes: 1 << 20,
		OutputDir:   "/out",
		Outputs:     []string{"present", "absent"},
	}, &recordingAdmission{name: "test-runtime"})
	ctrl.runner = &outputsRunner{
		recordingRunner: &recordingRunner{outputs: map[string][]byte{
			"create": []byte("container-123\n"),
			"wait":   []byte("0\n"),
		}},
		write: write,
	}
	ctrl.handle = handle
	return ctrl, handle
}

// TestExecuteStoresWrittenOutput publishes exactly the output file the container wrote.
func TestExecuteStoresWrittenOutput(t *testing.T) {
	// Write one of the two declared outputs.
	ctrl, handle := newOutputsController(t, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "present"), []byte("/srv/flag"), 0o644)
	})
	if err := ctrl.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Require one BLOCK_REF output holding the written bytes.
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "present" {
		t.Fatalf("outputs = %v, want only present", handle.outputs)
	}
	if handle.outputs[0].GetValueType() != forge_value.ValueType_ValueType_BLOCK_REF {
		t.Fatalf("output type = %s", handle.outputs[0].GetValueType())
	}
	data, err := forge_target.LoadBlobValueToBytes(t.Context(), handle, handle.outputs[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "/srv/flag" {
		t.Fatalf("output data = %q", data)
	}
}

// TestExecuteRejectsUnreadableOutputs fails the execution on an oversized or
// non-regular output file.
func TestExecuteRejectsUnreadableOutputs(t *testing.T) {
	for name, write := range map[string]func(path string) error{
		"oversized": func(path string) error {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				return err
			}
			return os.Truncate(path, block.MaxBlockSize+1)
		},
		"symlink": func(path string) error {
			return os.Symlink("/etc/passwd", path)
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Write the unreadable entry as the present output.
			ctrl, handle := newOutputsController(t, func(dir string) error {
				return write(filepath.Join(dir, "present"))
			})

			// Require a failed execution with no outputs published.
			if err := ctrl.Execute(t.Context()); err == nil {
				t.Fatal("unreadable output was accepted")
			}
			if len(handle.outputs) != 0 {
				t.Fatalf("outputs published after failure: %v", handle.outputs)
			}
		})
	}
}

// TestDockerIntegrationStoresOutput runs a real container that writes one of
// its two declared outputs. Set FORGE_DOCKER_INTEGRATION=1 to run it; the
// image defaults to debian:sid and FORGE_DOCKER_IMAGE overrides it.
func TestDockerIntegrationStoresOutput(t *testing.T) {
	// Require explicit opt-in and a reachable Docker daemon.
	if os.Getenv("FORGE_DOCKER_INTEGRATION") == "" {
		t.Skip("set FORGE_DOCKER_INTEGRATION=1 to run docker daemon integration")
	}
	runner := NewExecDockerRunner()
	env := []string{"PATH=" + os.Getenv("PATH")}
	if _, err := runner.Run(t.Context(), "docker", []string{"info"}, env); err != nil {
		t.Skip(err.Error())
	}
	image := os.Getenv("FORGE_DOCKER_IMAGE")
	if image == "" {
		image = "debian:sid"
	}

	// Configure a real container under a unique runtime name.
	ctrl, handle := newOutputsController(t, nil)
	name := "forge-docker-output-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctrl.admission = &recordingAdmission{name: name}
	ctrl.runner = runner
	ctrl.conf.Image = image
	ctrl.conf.MemoryBytes = 64 << 20
	ctrl.conf.DockerEnv = map[string]string{"PATH": os.Getenv("PATH")}
	ctrl.conf.Command = []string{"sh", "-c", "printf /srv/flag > /out/present"}

	// Remove the stopped container after the run.
	t.Cleanup(func() {
		_, _ = runner.Run(context.Background(), "docker", []string{"rm", "-f", name}, env)
	})
	if err := ctrl.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Require exactly the written output with its contents.
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "present" {
		t.Fatalf("outputs = %v, want only present", handle.outputs)
	}
	data, err := forge_target.LoadBlobValueToBytes(t.Context(), handle, handle.outputs[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "/srv/flag" {
		t.Fatalf("output data = %q", data)
	}
}
