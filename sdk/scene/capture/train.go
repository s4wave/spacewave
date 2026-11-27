// Package s4wave_scene_capture builds the Steps of the capture Flowgraph,
// which turns posed photos of a room into a Gaussian splat for a Scene.
package s4wave_scene_capture

import (
	"strconv"

	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

const (
	// InputPoses names the train Step's input: a COLMAP dataset, an images
	// directory beside sparse/0 holding cameras, images and points3D.
	InputPoses = "poses"
	// OutputSplat names the train Step's output: a directory holding the
	// trained splat as SplatFile.
	OutputSplat = "splat"
	// SplatFile is the trained splat's file name in the output directory, a
	// binary little-endian PLY of 3D Gaussians with degree 3 coefficients.
	SplatFile = "splat.ply"
)

// TrainConfig returns the Docker config of the train Step: Brush, from the
// image this package's Dockerfile builds, trains a splat from the poses
// input for steps iterations on one whole graphics card and writes it to the
// splat output.
//
// Brush trains through Vulkan, so any card vendor serves, and the run holds
// the card until it exits; a second train Step waits for it. The container
// has no network and runs as the Worker's user, so HOME points at its /tmp.
func TrainConfig(image string, steps uint32) *forge_lib_docker.Config {
	out := "/outputs/" + OutputSplat
	n := strconv.FormatUint(uint64(steps), 10)
	return &forge_lib_docker.Config{
		Image:       image,
		NetworkMode: "none",
		MilliCpu:    2000,
		MemoryBytes: 6 << 30,
		Cards:       &s4wave_device.GraphicsCardRequest{Count: 1},
		Env:         map[string]string{"HOME": "/tmp"},
		InputMounts: map[string]string{InputPoses: "/" + InputPoses},
		OutputDir:   "/outputs",
		Outputs:     []string{OutputSplat},
		Command: []string{
			"brush_app", "/" + InputPoses,
			"--total-steps", n, "--export-every", n,
			"--export-path", out, "--export-name", SplatFile,
		},
	}
}
