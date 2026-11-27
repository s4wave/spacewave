package forge_lib_docker

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"

	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	"github.com/sirupsen/logrus"
)

// hostDir is the private host directory holding one execution's output
// directory and input checkouts, each bind-mounted into the container. Its
// root, created mode 0700, keeps other host users out. Bind mounts reach the
// directories under it regardless.
type hostDir struct {
	// root is removed with everything under it after the container exits.
	root string
	// outputDir is the host directory mounted at output_dir, or empty.
	outputDir string
	// mounts bind outputDir and each input checkout into the container.
	mounts []*Mount
}

// prepareHostDir creates the host directories the configured outputs and
// input mounts need and checks out each input. The directory is empty, with
// no root, when the config needs neither. The caller removes it.
func (c *Controller) prepareHostDir(ctx context.Context) (_ *hostDir, retErr error) {
	// Create the private root only when something is mounted from it.
	outputs, inputMounts := c.conf.GetOutputs(), c.conf.GetInputMounts()
	if len(outputs) == 0 && len(inputMounts) == 0 {
		return &hostDir{}, nil
	}
	root, err := os.MkdirTemp("", "forge-docker-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(root)
		}
	}()
	d := &hostDir{root: root}

	// Create the output directory, which the container writes as the Worker's
	// user, so the Worker can read every output and remove what is left.
	if len(outputs) != 0 {
		d.outputDir = filepath.Join(root, "out")
		if err := os.Mkdir(d.outputDir, 0o700); err != nil {
			return nil, err
		}
		d.mounts = append(d.mounts, &Mount{HostPath: d.outputDir, ContainerPath: c.conf.GetOutputDir()})
	}

	// Check out each input in name order and mount it read-only.
	inputsDir := filepath.Join(root, "inputs")
	names := slices.Sorted(maps.Keys(inputMounts))
	if err := forge_lib_util_inputfs.CheckoutAll(ctx, c.handle, c.inputVals, inputsDir, names); err != nil {
		return nil, err
	}
	for _, name := range names {
		d.mounts = append(d.mounts, &Mount{
			HostPath:      filepath.Join(inputsDir, name),
			ContainerPath: inputMounts[name],
			ReadOnly:      true,
		})
	}
	return d, nil
}

// remove deletes the root and everything under it, logging a failure, which
// leaves only disk to reclaim.
func (d *hostDir) remove(le *logrus.Entry) {
	if d.root == "" {
		return
	}
	if err := os.RemoveAll(d.root); err != nil {
		le.WithError(err).Warn("remove docker host directory")
	}
}
