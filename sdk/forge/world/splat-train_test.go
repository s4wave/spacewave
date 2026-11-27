//go:build linux && !tinygo

package s4wave_forge_world

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/s4wave/spacewave/db/block"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/crypto"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
	s4wave_scene_capture "github.com/s4wave/spacewave/sdk/scene/capture"
)

// Keys of the capture Flowgraph, its run, the Device its train Step is
// placed on and the posed frames it reads.
const (
	splatGraphKey  = "flowgraph/capture"
	splatRunKey    = "flowgraph-run/capture"
	splatDeviceKey = "device/forge"
	splatPosesKey  = "scene/poses"
)

// TestSplatTrainStepOnCard runs the capture Flowgraph's train Step, placed
// on a Device whose Worker offers this host's one graphics card. The Step's
// reservation must hold the card while it trains, and the run must store a
// splat with Gaussians in it. readSplatTrain names the settings it reads.
func TestSplatTrainStepOnCard(t *testing.T) {
	s := readSplatTrain(t)
	f := s.startWorker(t, workerHarnessOptions{}, "splat-train")
	f.storeWorkerDevice(splatDeviceKey)
	f.placeTrain(s)

	started := time.Now()
	f.startRun(splatRunKey, splatGraphKey)
	f.waitTask(splatRunKey, "train", false)
	f.waitCards("the card reserved", s.cards[0].GetId())
	gaussians := f.finishTrain()
	t.Logf("trained %d steps on %s in %s: %d Gaussians", s.steps, s.cards[0].GetId(), time.Since(started).Round(time.Second), gaussians)
}

// Environment the killed host process reads from its parent.
const (
	// splatKilledWorldEnv names the World file of the host process to kill.
	splatKilledWorldEnv = "SPACEWAVE_SPLAT_KILLED_WORLD"
	// splatKilledKeyEnv holds the Worker's raw peer key in hex.
	splatKilledKeyEnv = "SPACEWAVE_SPLAT_KILLED_KEY"
	// splatKilledReady is the line the host process prints once the train
	// Step holds the card.
	splatKilledReady = "SPLAT TRAIN HOLDS THE CARD"
)

// TestSplatTrainWorkerKilled kills the host process running the train Step
// with SIGKILL, as a crash or the kernel's OOM killer does, and starts a new
// one on the same World. The dead process runs none of its cleanup, so its
// container keeps training on the card. The new Worker must stop that
// container before it starts another, then resume the Step to a complete
// splat and return the card.
//
// The test runs its own binary as the host process to kill, in a process
// group of its own, holding a World on disk.
func TestSplatTrainWorkerKilled(t *testing.T) {
	s := readSplatTrain(t)
	if path := os.Getenv(splatKilledWorldEnv); path != "" {
		s.holdCardUntilKilled(t, path)
		return
	}

	// Generate the Worker's key, which both host processes serve.
	key, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := key.Raw()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "world.s4wave")

	// Start the first host process and wait until its container trains.
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Minute)
	defer cancel()
	events := followDockerEvents(t, ctx, s.dockerPath, s.image)
	host := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSplatTrainWorkerKilled$", "-test.v", "-test.timeout=35m")
	host.Env = append(os.Environ(), splatKilledWorldEnv+"="+path, splatKilledKeyEnv+"="+hex.EncodeToString(raw))
	host.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	host.Stderr = os.Stderr
	out, err := host.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	ready := false
	for lines := bufio.NewScanner(out); !ready && lines.Scan(); {
		ready = lines.Text() == splatKilledReady
	}
	if !ready {
		_ = host.Wait()
		t.Fatal("the first host process exited before its train Step held the card")
	}
	orphan := events.next(t, "start")

	// Kill the host process and every process in its group, then require
	// the container to have outlived it.
	if err := syscall.Kill(-host.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = host.Wait()
	running, err := exec.CommandContext(ctx, s.dockerPath, "inspect", "-f", "{{.State.Running}}", orphan).Output()
	if err != nil || strings.TrimSpace(string(running)) != "true" {
		t.Fatalf("container %s after the kill: running=%q err=%v", orphan, running, err)
	}
	t.Logf("killed the host process; container %s still trains", orphan)

	// A new host process on the same World stops the orphan before it
	// starts the Step again, then finishes it.
	f := s.startWorker(t, workerHarnessOptions{
		storage:   []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: path})},
		workerKey: key,
		reopen:    true,
	}, "splat-train-2")
	f.attachRun(splatRunKey)
	events.expect(t, "die", orphan)
	events.next(t, "start")
	f.waitCards("the card reserved again", s.cards[0].GetId())
	gaussians := f.finishTrain()
	t.Logf("resumed the train Step after the kill: %d Gaussians", gaussians)
}

// holdCardUntilKilled is the host process TestSplatTrainWorkerKilled kills:
// it starts the train Step on a World stored at path, prints
// splatKilledReady once the Step holds the card and then waits.
func (s *splatTrain) holdCardUntilKilled(t *testing.T, path string) {
	raw, err := hex.DecodeString(os.Getenv(splatKilledKeyEnv))
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.UnmarshalEd25519PrivateKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	f := s.startWorker(t, workerHarnessOptions{
		storage:   []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: path})},
		workerKey: key,
	}, "splat-train-1")
	f.storeWorkerDevice(splatDeviceKey)
	f.placeTrain(s)
	f.startRun(splatRunKey, splatGraphKey)
	f.waitTask(splatRunKey, "train", false)
	f.waitCards("the card reserved", s.cards[0].GetId())
	fmt.Println(splatKilledReady)
	<-f.ctx.Done()
	t.Fatal("the host process was not killed")
}

// splatTrain holds the train Step's settings, read from the environment.
type splatTrain struct {
	// poses is the COLMAP dataset directory the Step reads.
	poses string
	// image is the image sdk/scene/capture/Dockerfile builds.
	image string
	// dockerPath is the Docker CLI.
	dockerPath string
	// steps is the number of training steps.
	steps uint32
	// cards is this host's one graphics card.
	cards []*s4wave_device.GraphicsCard
}

// readSplatTrain reads the COLMAP dataset SPACEWAVE_SPLAT_POSES names, the
// image SPACEWAVE_SPLAT_IMAGE names and the training steps
// SPACEWAVE_SPLAT_STEPS sets (default 500). It skips the test unless both
// names are set, Docker is installed and the host has exactly one card.
func readSplatTrain(t *testing.T) *splatTrain {
	s := &splatTrain{poses: os.Getenv("SPACEWAVE_SPLAT_POSES"), image: os.Getenv("SPACEWAVE_SPLAT_IMAGE"), steps: 500}
	if s.poses == "" || s.image == "" {
		t.Skip("set SPACEWAVE_SPLAT_POSES and SPACEWAVE_SPLAT_IMAGE")
	}
	var err error
	if s.dockerPath, err = exec.LookPath("docker"); err != nil {
		t.Skip(err.Error())
	}
	if s.cards, err = s4wave_device.DetectGraphicsCards(os.DirFS("/sys")); err != nil {
		t.Fatal(err)
	}
	if len(s.cards) != 1 {
		t.Skipf("host has %d graphics cards, want 1", len(s.cards))
	}
	if v := os.Getenv("SPACEWAVE_SPLAT_STEPS"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		s.steps = uint32(n)
	}
	return s
}

// startWorker starts a harness with the given storage and Worker key, the
// Flowgraph run and Docker controllers, and a Worker execution offering the
// card under claimID.
func (s *splatTrain) startWorker(t *testing.T, options workerHarnessOptions, claimID string) *flowgraphFixture {
	options.timeout, options.memoryBytes = 30*time.Minute, 8<<30
	options.dockerPath, options.cards = s.dockerPath, s.cards
	h := newWorkerHarnessWithOptions(t, options)
	h.tb.StaticResolver.AddFactory(flowgraph_run.NewFactory(h.tb.Bus))
	admission := h.newAdmission(claimID)
	h.tb.StaticResolver.AddFactory(forge_lib_docker.NewWorkerFactory(h.tb.Bus, admission))
	stop := h.startWorker(admission, nil)
	t.Cleanup(func() { _ = stop() })
	return &flowgraphFixture{workerHarness: h, t: t}
}

// placeTrain stores the posed frames and the capture Flowgraph, whose train
// Step reads them, placed on the Device.
func (f *flowgraphFixture) placeTrain(s *splatTrain) {
	f.storeTree(splatPosesKey, os.DirFS(s.poses))
	conf := s4wave_scene_capture.TrainConfig(s.image, s.steps)
	conf.DockerPath = s.dockerPath
	conf.DockerEnv = map[string]string{"PATH": os.Getenv("PATH")}
	train := f.stepNode(forge_lib_docker.ControllerID, conf, s4wave_scene_capture.OutputSplat, false)
	train.Step.Target.Inputs = []*forge_target.Input{worldObjectInput(s4wave_scene_capture.InputPoses, splatPosesKey)}
	f.storeGraph(splatGraphKey, "Capture", map[string]*s4wave_flowgraph.FlowgraphNode{"train": train}, nil)
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, ws, splatGraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"train": {Copies: []*s4wave_flowgraph.FlowgraphPlacement{{
				Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: splatDeviceKey,
			}}}},
		})
		return err
	})
}

// finishTrain waits for the train Step to complete, requires its splat to
// hold Gaussians and the card to return, and returns the Gaussian count.
func (f *flowgraphFixture) finishTrain() uint64 {
	taskKey := f.waitTask(splatRunKey, "train", true)
	gaussians := f.splatGaussians(f.taskTree(taskKey, s4wave_scene_capture.OutputSplat))
	if gaussians == 0 {
		f.t.Fatal("trained splat holds no Gaussians")
	}
	f.waitCards("the card released")
	return gaussians
}

// waitCards waits until the Worker's live reservations hold exactly the
// cards named by ids.
func (f *flowgraphFixture) waitCards(what string, ids ...string) {
	f.wait(what, func() bool {
		capacity, err := forge_runtime.LookupWorkerCapacity(f.ctx, f.tb.WorldState, harnessWorkerKey)
		return err == nil && slices.Equal(capacity.GetReservedCardIds(), ids)
	})
}

// storeWorkerDevice creates the Device key on the Worker's peer, linking the
// harness Worker as its Forge Worker capability.
func (f *flowgraphFixture) storeWorkerDevice(key string) {
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, key, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_device.Device{
				PeerId:     f.workerPeerID.String(),
				Label:      "forge",
				SetupState: s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
				Capabilities: []*s4wave_device.DeviceCapability{{
					Id:    "forge-worker",
					Kind:  s4wave_device.DeviceCapabilityKindForgeWorker,
					State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
					Link:  &s4wave_device.DeviceCapabilityLink{ObjectKey: harnessWorkerKey, TypeId: forge_worker.WorkerTypeID},
				}},
			}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, ws, key, s4wave_device.DeviceTypeID)
	})
}

// splatGaussians returns the Gaussian count of the splat in the output tree
// at root, from its PLY header's vertex element.
func (f *flowgraphFixture) splatGaussians(root *block.BlockRef) uint64 {
	var count uint64
	f.openTree(root, func(bfs billy.Filesystem) error {
		file, err := bfs.Open(s4wave_scene_capture.SplatFile)
		if err != nil {
			return err
		}
		defer file.Close()
		header := bufio.NewReader(file)
		for {
			line, err := header.ReadString('\n')
			if err != nil {
				return err
			}
			line = strings.TrimSpace(line)
			if n, ok := strings.CutPrefix(line, "element vertex "); ok {
				count, err = strconv.ParseUint(n, 10, 64)
				return err
			}
			if line == "end_header" {
				return nil
			}
		}
	})
	return count
}
