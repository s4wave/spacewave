package forge_lib_docker

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
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

// TestExecuteStoresDirectoryOutput stores a directory output as a UnixFS tree
// that holds its files and skips a link the container planted to a host file.
func TestExecuteStoresDirectoryOutput(t *testing.T) {
	// Write a directory with one file and a link to a host file.
	ctrl, handle := newOutputsController(t, func(dir string) error {
		out := filepath.Join(dir, "present")
		if err := os.MkdirAll(filepath.Join(out, "sub"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "sub", "file"), []byte("tree"), 0o644); err != nil {
			return err
		}
		return os.Symlink("/etc/passwd", filepath.Join(out, "leak"))
	})
	if err := ctrl.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "present" {
		t.Fatalf("outputs = %v, want only present", handle.outputs)
	}

	// Read the stored tree: the file is present and the link is not.
	ctx := t.Context()
	err := handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		treeCs := cs.Clone()
		treeCs.SetRootRef(handle.outputs[0].GetBlockRef())
		tree := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, treeCs, nil)
		root, err := unixfs.NewFSHandle(tree)
		if err != nil {
			tree.Release()
			return err
		}
		defer root.Release()
		files := unixfs_iofs.NewFS(ctx, root)
		data, err := fs.ReadFile(files, "sub/file")
		if err != nil || string(data) != "tree" {
			t.Fatalf("sub/file = %q, %v", data, err)
		}
		if _, err := fs.Stat(files, "leak"); err == nil {
			t.Fatal("stored the link to a host file")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

// dockerIntegration returns the runner, environment and image for a test
// against the host's Docker daemon, skipping the test unless
// FORGE_DOCKER_INTEGRATION=1 is set and the daemon answers. The image defaults
// to debian:sid and FORGE_DOCKER_IMAGE overrides it.
func dockerIntegration(t *testing.T) (DockerRunner, []string, string) {
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
	return runner, env, image
}

// TestDockerIntegrationStoresOutput runs a real container that writes one of
// its two declared outputs, as dockerIntegration describes.
func TestDockerIntegrationStoresOutput(t *testing.T) {
	// Require explicit opt-in and a reachable Docker daemon.
	runner, env, image := dockerIntegration(t)

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

// TestDockerIntegrationMapsCards runs a real container holding the host's
// detected graphics cards, as the Worker's user, and requires it to see
// exactly their render nodes and the compute interface. It runs as
// dockerIntegration describes, on a host whose sysfs reports a card.
func TestDockerIntegrationMapsCards(t *testing.T) {
	// Require explicit opt-in, a Docker daemon and a card on this host.
	runner, env, image := dockerIntegration(t)
	cards, err := s4wave_device.DetectGraphicsCards(os.DirFS("/sys"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) == 0 {
		t.Skip("no graphics card on this host")
	}

	// Map the cards' nodes, as a Worker grant does, and list what the container sees.
	var devices, renderNodes []string
	for _, card := range cards {
		for _, device := range card.GetDevicePaths() {
			if !slices.Contains(devices, device) {
				devices = append(devices, device)
			}
			if dir, node := path.Split(device); dir == "/dev/dri/" {
				renderNodes = append(renderNodes, node)
			}
		}
	}
	ctrl, handle := newOutputsController(t, nil)
	name := "forge-docker-cards-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctrl.admission = &recordingAdmission{name: name, devices: devices}
	ctrl.runner = runner
	ctrl.conf.Image = image
	ctrl.conf.MemoryBytes = 64 << 20
	ctrl.conf.NetworkMode = "none"
	ctrl.conf.DockerEnv = map[string]string{"PATH": os.Getenv("PATH")}
	ctrl.conf.Cards = &s4wave_device.GraphicsCardRequest{Count: uint32(len(cards))} //nolint:gosec // a host has few cards.
	ctrl.conf.Command = []string{"sh", "-c", "test -r /dev/kfd && test -w /dev/kfd && ls /dev/dri > /out/present"}
	t.Cleanup(func() {
		_, _ = runner.Run(context.Background(), "docker", []string{"rm", "-f", name}, env)
	})
	if err := ctrl.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Require exactly the cards' render nodes inside the container.
	if len(handle.outputs) != 1 {
		t.Fatalf("outputs = %v, want only present", handle.outputs)
	}
	data, err := forge_target.LoadBlobValueToBytes(t.Context(), handle, handle.outputs[0])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(renderNodes)
	if got := strings.Fields(string(data)); !slices.Equal(got, renderNodes) {
		t.Fatalf("container sees %v, want %v", got, renderNodes)
	}
	t.Logf("container sees %v and /dev/kfd", renderNodes)
}
