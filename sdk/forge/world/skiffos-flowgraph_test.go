//go:build linux && !tinygo

package s4wave_forge_world

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/pierrec/lz4/v4"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_lib_skiffos "github.com/s4wave/spacewave/forge/lib/skiffos"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/os/boot"
	os_image "github.com/s4wave/spacewave/os/image"
	os_image_sign "github.com/s4wave/spacewave/os/image/sign"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

// skiffosGraph is the build Flowgraph's object key.
const skiffosGraph = "flowgraph/skiffos"

// TestSkiffOSFlowgraph builds a SkiffOS workspace from a Flowgraph Step, lands
// the boot set in the World as a UnixFS tree, and reruns the graph with equal
// inputs, which completes from the first Task's outputs without a build.
// newSkiffOSProof names the settings it reads.
func TestSkiffOSFlowgraph(t *testing.T) {
	f := newSkiffOSProof(t)
	build := f.stepNode(forge_lib_skiffos.ControllerID, f.buildConfig("intel/x64", "virt/qemu"), forge_lib_skiffos.OutputImages, true)
	f.storeGraph(skiffosGraph, "SkiffOS QEMU build", map[string]*s4wave_flowgraph.FlowgraphNode{"build": build}, nil)

	// The first run builds and stores the boot set.
	f.startRun("flowgraph-run/skiffos-first", skiffosGraph)
	first := f.waitTask("flowgraph-run/skiffos-first", "build", true)
	if cached, err := forge_task.LookupTaskCached(f.ctx, f.tb.WorldState, first); err != nil || cached != "" {
		t.Fatalf("first build cached=%q err=%v", cached, err)
	}
	images := f.bootSet(first)

	// The second run completes from the first Task without a Pass.
	f.startRun("flowgraph-run/skiffos-second", skiffosGraph)
	second := f.waitTask("flowgraph-run/skiffos-second", "build", true)
	cached, err := forge_task.LookupTaskCached(f.ctx, f.tb.WorldState, second)
	if err != nil || cached != first {
		t.Fatalf("second build cached=%q err=%v, want %q", cached, err, first)
	}
	passes, err := forge_task.ListTaskPasses(f.ctx, f.tb.WorldState, second)
	if err != nil || len(passes) != 0 {
		t.Fatalf("second build passes=%v err=%v", passes, err)
	}
	if again := f.bootSet(second); !again.EqualsRef(images) {
		t.Fatalf("second boot set %s, want %s", again.MarshalString(), images.MarshalString())
	}
	t.Logf("task cache hit: %s completed with the outputs of %s", second, first)
}

// TestSpacewaveOSRehearsal builds the QEMU board with the spacewave/os package
// from a Flowgraph Step whose boot input is a World object, checks that the
// boot set's initramfs holds spacewave-init alone, and rehearses it: an image
// Step seeds the image store, and a Docker Step boots the boot set in QEMU on
// that store and passes only when the guest grows its disk, creates and
// writes its data Volume, and stops cleanly. The test then signs the image's
// root as the Space owner and installs it: the install Step writes the disk
// for the owner's authorization and refuses one from another key. A last run
// rehearses a boot set whose initramfs is not an archive, which must fail with
// the guest's console kept in the Execution log.
// SPACEWAVE_OS_INIT names the static spacewave-init executable,
// SPACEWAVE_QEMU_IMAGE the os/qemu tool image, SPACEWAVE_DOCKER the Docker CLI
// and DOCKER_HOST its daemon, which must see this host's filesystem and grant
// /dev/kvm. The build uses the settings newSkiffOSProof reads.
func TestSpacewaveOSRehearsal(t *testing.T) {
	// Require the static init and the Docker runtime for QEMU.
	initPath, image, dockerPath := os.Getenv("SPACEWAVE_OS_INIT"), os.Getenv("SPACEWAVE_QEMU_IMAGE"), os.Getenv("SPACEWAVE_DOCKER")
	if initPath == "" || image == "" || dockerPath == "" {
		t.Skip("set SPACEWAVE_OS_INIT, SPACEWAVE_QEMU_IMAGE and SPACEWAVE_DOCKER")
	}
	initBytes, err := os.ReadFile(initPath)
	if err != nil {
		t.Fatal(err)
	}
	osDir, err := filepath.Abs("../../../os")
	if err != nil {
		t.Fatal(err)
	}

	// Store the boot inputs, the config package and the rehearsal tools as
	// World trees, so the build's reuse key covers every byte it reads.
	f := newSkiffOSProof(t)
	makeInitramfs, err := os.ReadFile(filepath.Join(osDir, "make-initramfs.sh"))
	if err != nil {
		t.Fatal(err)
	}
	f.storeTree("os/boot", fstest.MapFS{
		"init":              {Data: initBytes, Mode: 0o755},
		"make-initramfs.sh": {Data: makeInitramfs, Mode: 0o755},
	})
	f.storeTree("os/skiff", os.DirFS(filepath.Join(osDir, "skiff")))
	tools := fstest.MapFS{}
	for _, name := range []string{"rehearse.sh", "install.sh", "run_app.py", "kill_trial.py", "refusal_trial.py"} {
		data, err := os.ReadFile(filepath.Join(osDir, "qemu", name))
		if err != nil {
			t.Fatal(err)
		}
		tools[name] = &fstest.MapFile{Data: data, Mode: 0o755}
	}
	f.storeTree("os/qemu-tools", tools)

	// The build feeds its boot set to the rehearsal through a connection.
	conf := f.buildConfig("intel/x64", "virt/qemu", "spacewave/os")
	conf.ExtraConfigInputs = []string{"configs"}
	conf.InputEnv = map[string]string{"boot": "SPACEWAVE_OS_BOOT"}
	build := f.stepNode(forge_lib_skiffos.ControllerID, conf, forge_lib_skiffos.OutputImages, true)
	build.Step.Target.Inputs = []*forge_target.Input{worldObjectInput("boot", "os/boot"), worldObjectInput("configs", "os/skiff")}
	seed := f.imageNode(image, dockerPath)
	rehearse := f.rehearsalNode(image, dockerPath)
	rehearse.Ports = append(rehearse.Ports, inputPort(forge_lib_skiffos.OutputImages), inputPort(osImage))
	sign := f.signNode(os_image_sign.InputImage)
	f.storeGraph("flowgraph/spacewave-os", "Spacewave OS build, rehearsal and sign", map[string]*s4wave_flowgraph.FlowgraphNode{
		"build": build, "image": seed, "rehearse": rehearse, "sign": sign,
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"boot-set":     {OutputNode: "build", OutputPort: forge_lib_skiffos.OutputImages, InputNode: "rehearse", InputPort: forge_lib_skiffos.OutputImages},
		"image":        {OutputNode: "image", OutputPort: osImage, InputNode: "rehearse", InputPort: osImage},
		"signed-image": {OutputNode: "image", OutputPort: osImage, InputNode: "sign", InputPort: os_image_sign.InputImage},
		"receipt":      {OutputNode: "rehearse", OutputPort: "rehearsed", InputNode: "sign", InputPort: os_image_sign.InputRehearsed},
	})

	// The boot set's initramfs holds exactly spacewave-init's archive.
	f.startRun("flowgraph-run/spacewave-os", "flowgraph/spacewave-os")
	images := f.bootSet(f.waitTask("flowgraph-run/spacewave-os", "build", true))
	names := f.readInitramfs(images, initBytes)
	want := []string{"proc", "dev", "tmp", "modules", "dev/console", "init"}
	if !slices.Equal(names, want) {
		t.Fatalf("initramfs entries %v, want %v", names, want)
	}

	// The rehearsal boots it on the image, grows the store, creates and writes
	// the data Volume, stops cleanly and survives the kill trial.
	rehearsed, failure := f.waitDone("flowgraph-run/spacewave-os", "rehearse")
	if failure != "" {
		t.Fatalf("rehearsal failed: %s\n%s", failure, f.executionLog(rehearsed))
	}
	if value, err := forge_task.ResolveOutput(f.ctx, f.tb.WorldState, rehearsed, "rehearsed"); err != nil || value.IsEmpty() {
		t.Fatalf("rehearsal receipt=%v err=%v", value, err)
	}
	console := f.executionLog(rehearsed)
	for _, line := range []string{
		"SPACEWAVE_OS_BACKING_GROWN path=", "SPACEWAVE_OS_PINNED signers=", "SPACEWAVE_OS_AUTHORIZED signer=",
		"SPACEWAVE_OS_DATA_CREATED path=", "SPACEWAVE_OS_CONSOLE_READY pid=", "SPACEWAVE_OS_REHEARSAL_WROTE",
		"SPACEWAVE_OS_PASS", "KILL_TRIAL_PASS",
	} {
		if !strings.Contains(console, line) {
			t.Fatalf("rehearsal console lacks %q:\n%s", line, console)
		}
	}

	// The sign Step authorizes the rehearsed image's root as the Worker, the
	// Space owner's Device, with a key it asks for and never holds.
	imageTree := f.taskTree(f.waitTask("flowgraph-run/spacewave-os", "image", true), osImage)
	root := f.imageRoot(imageTree)
	authorization := f.taskTree(f.waitTask("flowgraph-run/spacewave-os", "sign", true), os_image_sign.OutputAuthorization)
	if authorized, signer := f.imageAuthorization(authorization); !authorized.EqualsRef(root) || signer != f.workerPeerID {
		t.Fatalf("authorized %s by %s, want the image root %s by the Worker %s", authorized.MarshalString(), signer, root.MarshalString(), f.workerPeerID)
	}

	// Keep the rehearsed boot set, image and authorization as World objects,
	// and install them with a card naming the Worker.
	f.storeRef("os/images", images)
	f.storeRef("os/image", imageTree)
	f.storeRef("os/authorization", authorization)
	f.storeCard("os/card", f.workerKey)
	installed, failure := f.install("owner", image, dockerPath, "os/authorization")
	if failure != "" {
		t.Fatalf("install failed: %s\n%s", failure, f.executionLog(installed))
	}
	if log := f.executionLog(installed); !strings.Contains(log, "SPACEWAVE_OS_CARD_INSTALLED root="+root.MarshalString()) {
		t.Fatalf("install log lacks the image root %s:\n%s", root.MarshalString(), log)
	}
	disk := f.taskTree(installed, "installed")
	f.openTree(disk, func(bfs billy.Filesystem) error {
		for _, name := range []string{"disk.img", "bzImage", "rootfs.cpio.lz4"} {
			if _, err := bfs.Stat(name); err != nil {
				return err
			}
		}
		return nil
	})

	// An authorization from a key the card does not name installs nothing.
	_, stranger, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	f.storeAuthorization("os/authorization-stranger", stranger, root)
	refused, failure := f.install("stranger", image, dockerPath, "os/authorization-stranger")
	if failure == "" {
		t.Fatal("install accepted an authorization from an unpinned key")
	}
	if log := f.executionLog(refused); !strings.Contains(log, "is not pinned") {
		t.Fatalf("refused install log lacks the pin failure:\n%s", log)
	}

	// A boot set whose initramfs is no archive fails, keeping the console.
	f.storeTree("os/broken-images", fstest.MapFS{
		"bzImage":         {Data: f.readTreeFile(images, "bzImage"), Mode: 0o644},
		"rootfs.cpio.lz4": {Data: []byte("not an initramfs\n"), Mode: 0o644},
	})
	broken := f.rehearsalNode(image, dockerPath)
	broken.Step.Target.Inputs = append(broken.Step.Target.Inputs, worldObjectInput(forge_lib_skiffos.OutputImages, "os/broken-images"), worldObjectInput(osImage, "os/image"))
	brokenSign := f.signNode()
	brokenSign.Step.Target.Inputs = []*forge_target.Input{worldObjectInput(os_image_sign.InputImage, "os/image")}
	f.storeGraph("flowgraph/spacewave-os-broken", "Spacewave OS broken rehearsal", map[string]*s4wave_flowgraph.FlowgraphNode{
		"rehearse": broken, "sign": brokenSign,
	}, map[string]*s4wave_flowgraph.FlowgraphConnection{
		"receipt": {OutputNode: "rehearse", OutputPort: "rehearsed", InputNode: "sign", InputPort: os_image_sign.InputRehearsed},
	})
	f.startRun("flowgraph-run/spacewave-os-broken", "flowgraph/spacewave-os-broken")
	failed, failure := f.waitDone("flowgraph-run/spacewave-os-broken", "rehearse")
	if failure == "" {
		t.Fatal("broken boot set passed its rehearsal")
	}
	if console := f.executionLog(failed); !strings.Contains(console, "Kernel panic") {
		t.Fatalf("failed rehearsal log lacks the guest's panic:\n%s", console)
	}

	// The failure pauses the run before the sign Step, which never starts.
	f.wait("broken run pause", func() bool {
		return f.readRun("flowgraph-run/spacewave-os-broken").GetState() == s4wave_flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_PAUSED
	})
	for _, a := range f.readRun("flowgraph-run/spacewave-os-broken").GetActivations() {
		if a.GetNodeId() == "sign" {
			t.Fatalf("failed rehearsal activated the sign Step as %s", a.GetTaskKey())
		}
	}
	t.Logf("boot set %s rehearsed by %s and installed by %s; broken boot set failed: %s", images.MarshalString(), rehearsed, installed, failure)
}

// osImage names the image Step's output, the image store, and the input
// that reads it.
const osImage = "image"

// dockerConfig returns a Docker Step config for the os/qemu tool image that
// mounts the named inputs, runs command and writes output under /outputs.
func dockerConfig(image, dockerPath, output string, inputMounts map[string]string, command ...string) *forge_lib_docker.Config {
	return &forge_lib_docker.Config{
		DockerPath: dockerPath, DockerEnv: map[string]string{"DOCKER_HOST": os.Getenv("DOCKER_HOST")},
		Image: image, NetworkMode: "none",
		MilliCpu: 1000, MemoryBytes: 2 << 30, StopTimeoutSeconds: 45,
		InputMounts: inputMounts, Command: command,
		OutputDir: "/outputs", Outputs: []string{output},
	}
}

// imageNode returns a Step that seeds an image store with os/boot's init and
// the tool image's static busybox.
func (f *flowgraphFixture) imageNode(image, dockerPath string) *s4wave_flowgraph.FlowgraphNode {
	conf := dockerConfig(image, dockerPath, osImage, map[string]string{"boot": "/boot-input"},
		"/boot-input/init", "seed", "/outputs/"+osImage, "/bin/busybox")
	node := f.stepNode(forge_lib_docker.ControllerID, conf, osImage, false)
	node.Step.Target.Inputs = []*forge_target.Input{worldObjectInput("boot", "os/boot")}
	return node
}

// rehearsalNode returns a Step that rehearses the boot set on its images
// input and the image store on its image input with os/boot's init and the
// os/qemu-tools scripts, in QEMU under the Docker image.
func (f *flowgraphFixture) rehearsalNode(image, dockerPath string) *s4wave_flowgraph.FlowgraphNode {
	conf := dockerConfig(image, dockerPath, "rehearsed",
		map[string]string{forge_lib_skiffos.OutputImages: "/images", osImage: "/image", "boot": "/boot-input", "tools": "/tools"},
		"sh", "/tools/rehearse.sh", "/images", "/image", "/boot-input/init", "/outputs/rehearsed")
	conf.Devices = []string{"/dev/kvm:/dev/kvm:rw"}
	node := f.stepNode(forge_lib_docker.ControllerID, conf, "rehearsed", false)
	node.Step.Target.Inputs = []*forge_target.Input{worldObjectInput("boot", "os/boot"), worldObjectInput("tools", "os/qemu-tools")}
	return node
}

// install runs a graph whose one Step installs the os/image store with the
// os/card and the authorization object, beside the os/images boot set, and
// returns its Task and failure.
func (f *flowgraphFixture) install(name, image, dockerPath, authorization string) (string, string) {
	conf := dockerConfig(image, dockerPath, "installed", map[string]string{
		forge_lib_skiffos.OutputImages: "/images", osImage: "/image", "boot": "/boot-input",
		"tools": "/tools", "card": "/card", "authorization": "/authorization",
	}, "sh", "/tools/install.sh", "/images", "/image", "/boot-input/init", "/card", "/authorization", "/outputs/installed")
	node := f.stepNode(forge_lib_docker.ControllerID, conf, "installed", false)
	node.Step.Target.Inputs = []*forge_target.Input{
		worldObjectInput(forge_lib_skiffos.OutputImages, "os/images"), worldObjectInput(osImage, "os/image"),
		worldObjectInput("boot", "os/boot"), worldObjectInput("tools", "os/qemu-tools"),
		worldObjectInput("card", "os/card"), worldObjectInput("authorization", authorization),
	}
	graph, run := "flowgraph/spacewave-os-install-"+name, "flowgraph-run/spacewave-os-install-"+name
	f.storeGraph(graph, "Spacewave OS install "+name, map[string]*s4wave_flowgraph.FlowgraphNode{"install": node}, nil)
	f.startRun(run, graph)
	return f.waitDone(run, "install")
}

// imageRoot returns the selected root of the image store in the tree at root.
func (f *flowgraphFixture) imageRoot(tree *block.BlockRef) *block.BlockRef {
	// Copy the store file out of the tree, streaming it.
	dir := f.t.TempDir()
	f.openTree(tree, func(bfs billy.Filesystem) error {
		src, err := bfs.Open("image.s4wave")
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.Create(filepath.Join(dir, "image.s4wave"))
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		return errors.Join(err, dst.Close())
	})

	// Read its selected root as the boot and the sign Step do.
	root, err := boot.ReadImageRoot(f.ctx, dir)
	if err != nil {
		f.t.Fatal(err)
	}
	return root
}

// signNode returns a Step that signs its image input's root once its
// rehearsed input arrives, with an input port for each named connected input.
func (f *flowgraphFixture) signNode(connected ...string) *s4wave_flowgraph.FlowgraphNode {
	node := f.stepNode(os_image_sign.ControllerID, &os_image_sign.Config{}, os_image_sign.OutputAuthorization, false)
	for _, name := range append(connected, os_image_sign.InputRehearsed) {
		node.Ports = append(node.Ports, inputPort(name))
	}
	return node
}

// imageAuthorization returns the image root the authorization tree at root
// authorizes and its signer, verified as a machine pinning the Worker does.
func (f *flowgraphFixture) imageAuthorization(root *block.BlockRef) (*block.BlockRef, peer.ID) {
	dir := f.t.TempDir()
	path := filepath.Join(dir, os_image.CardAuthorization)
	if err := os.WriteFile(path, f.readTreeFile(root, os_image.CardAuthorization), 0o600); err != nil {
		f.t.Fatal(err)
	}
	signed, err := os_image.ReadAuthorization(path)
	if err != nil {
		f.t.Fatal(err)
	}
	authorized, signer, err := os_image.VerifyImageRoot(signed, []peer.ID{f.workerPeerID})
	if err != nil {
		f.t.Fatal(err)
	}
	return authorized, signer
}

// storeCard stores a card naming owner as the only signer, with a one-use
// writer grant on a Space owner signs.
func (f *flowgraphFixture) storeCard(key string, owner crypto.PrivKey) {
	ownerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		f.t.Fatal(err)
	}
	grant, _, err := sobject.BuildSOInviteMessage("os-rehearsal-space", owner, "local", &sobject.SOInvite{
		Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, MaxUses: 1,
		ExpiresAt: timestamp.New(time.Now().Add(time.Hour)),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.storeTree(key, fstest.MapFS{
		os_image.CardSigners: {Data: []byte(ownerID.String() + "\n"), Mode: 0o644},
		os_image.CardGrant:   {Data: f.packed(grant), Mode: 0o644},
	})
}

// storeAuthorization stores signer's authorization of the image root.
func (f *flowgraphFixture) storeAuthorization(key string, signer crypto.PrivKey, root *block.BlockRef) {
	signed, err := os_image.SignImageRoot(signer, root)
	if err != nil {
		f.t.Fatal(err)
	}
	f.storeTree(key, fstest.MapFS{os_image.CardAuthorization: {Data: f.packed(signed), Mode: 0o644}})
}

// packed returns msg as one line of packedmsg text.
func (f *flowgraphFixture) packed(msg interface{ MarshalVT() ([]byte, error) }) []byte {
	data, err := msg.MarshalVT()
	if err != nil {
		f.t.Fatal(err)
	}
	return []byte(packedmsg.EncodePackedMessage(data) + "\n")
}

// inputPort returns an input port named name that activates on its connection.
func inputPort(name string) *s4wave_flowgraph.FlowgraphPort {
	return &s4wave_flowgraph.FlowgraphPort{
		Name: name, Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT, TypeId: "activation",
	}
}

// worldObjectInput binds the input name to the World object key.
func worldObjectInput(name, key string) *forge_target.Input {
	return &forge_target.Input{
		Name: name, InputType: forge_target.InputType_InputType_WORLD_OBJECT,
		WorldObject: &forge_target.InputWorldObject{ObjectKey: key},
	}
}

// skiffOSProof runs SkiffOS build Steps on a Worker in a durable World.
type skiffOSProof struct {
	*flowgraphFixture
	// skiffDir is the SkiffOS checkout on this host.
	skiffDir string
	// revision is the checkout's commit, which the build Steps pin.
	revision string
	// workspace is the SkiffOS workspace the Steps own.
	workspace string
}

// buildConfig returns a build of the pinned checkout with the config list.
func (f *skiffOSProof) buildConfig(configs ...string) *forge_lib_skiffos.Config {
	return &forge_lib_skiffos.Config{
		SkiffDir: f.skiffDir, SkiffRevision: f.revision, Workspace: f.workspace, Configs: configs,
	}
}

// newSkiffOSProof starts a Worker for SkiffOS build Steps, or skips the test.
// SPACEWAVE_SKIFFOS_DIR names the SkiffOS checkout; the Steps own the
// workspace SPACEWAVE_SKIFFOS_WORKSPACE (default spacewave-forge) in it and
// touch no other. SPACEWAVE_SKIFFOS_WORLD optionally keeps the World.
func newSkiffOSProof(t *testing.T) *skiffOSProof {
	// Require an explicit checkout: the build takes tens of minutes.
	skiffDir := os.Getenv("SPACEWAVE_SKIFFOS_DIR")
	if skiffDir == "" {
		t.Skip("set SPACEWAVE_SKIFFOS_DIR for the real SkiffOS build")
	}
	head, err := exec.Command("git", "-C", skiffDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	workspace := os.Getenv("SPACEWAVE_SKIFFOS_WORKSPACE")
	if workspace == "" {
		workspace = "spacewave-forge"
	}
	worldPath := os.Getenv("SPACEWAVE_SKIFFOS_WORLD")
	if worldPath == "" {
		worldPath = filepath.Join(t.TempDir(), "world.s4wave")
	}

	// Open the World and register the Flowgraph, SkiffOS and Docker controllers.
	h := newWorkerHarnessWithOptions(t, workerHarnessOptions{
		timeout: 4 * time.Hour, memoryBytes: 4 << 30,
		storage: []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: worldPath})},
	})
	h.tb.StaticResolver.AddFactory(flowgraph_run.NewFactory(h.tb.Bus))
	h.tb.StaticResolver.AddFactory(forge_lib_skiffos.NewFactory())
	h.tb.StaticResolver.AddFactory(os_image_sign.NewFactory(h.tb.Bus))
	admission := h.newAdmission("skiffos-proof")
	h.tb.StaticResolver.AddFactory(forge_lib_docker.NewWorkerFactory(h.tb.Bus, admission))

	// Run the Worker that serves the runs; the Step runs where it is placed.
	updates := make(chan *s4wave_device.ForgeWorkerDeclaration, 1)
	stopWorker := h.startWorker(admission, updates)
	t.Cleanup(func() {
		if err := stopWorker(); !errors.Is(err, context.Canceled) {
			t.Errorf("Worker stop: %v", err)
		}
	})
	return &skiffOSProof{
		flowgraphFixture: &flowgraphFixture{workerHarness: h, t: t},
		skiffDir:         skiffDir,
		revision:         strings.TrimSpace(string(head)),
		workspace:        workspace,
	}
}

// stepNode resolves a Step Target running the controller with conf through
// the ordinary JSON resolver, with one output port for its output.
func (f *flowgraphFixture) stepNode(controllerID string, conf interface{ MarshalJSON() ([]byte, error) }, output string, reuse bool) *s4wave_flowgraph.FlowgraphNode {
	// Resolve the Target, declaring the output and output reuse.
	config, err := conf.MarshalJSON()
	if err != nil {
		f.t.Fatal(err)
	}
	out := strconv.Quote(output)
	raw := []byte(`{"outputs":[{"name":` + out + `,"outputType":"OutputType_EXEC","execOutput":` + out + `}],"exec":{"controller":{"id":` + strconv.Quote(controllerID) + `,"config":` + string(config) + `},"reuseOutputs":` + strconv.FormatBool(reuse) + `}}`)
	target, err := target_json.ResolveYAML(f.ctx, f.tb.Bus, raw)
	if err != nil {
		f.t.Fatal(err)
	}
	if target.GetExec().GetReuseOutputs() != reuse {
		f.t.Fatal("target lost reuseOutputs")
	}

	// Carry the Target in a Step node whose output port activates its readers.
	return &s4wave_flowgraph.FlowgraphNode{
		TypeId: s4wave_flowgraph.StepNodeTypeID,
		Step:   &s4wave_flowgraph.FlowgraphStep{Target: target},
		Ports: []*s4wave_flowgraph.FlowgraphPort{{
			Name: output, Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "activation",
		}},
	}
}

// storeGraph creates the Flowgraph key holding nodes and connections.
func (f *flowgraphFixture) storeGraph(key, name string, nodes map[string]*s4wave_flowgraph.FlowgraphNode, conns map[string]*s4wave_flowgraph.FlowgraphConnection) {
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		if _, err := (&s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: key, Name: name}).ApplyWorldOp(ctx, nil, ws, f.sender); err != nil {
			return err
		}
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, ws, key, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: nodes, SetConnections: conns,
		})
		return err
	})
}

// storeTree writes fsys as a UnixFS tree and creates the World object key
// rooted at it, in one write transaction.
func (f *flowgraphFixture) storeTree(key string, fsys fs.FS) {
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		// Write the tree's blocks into the World's bucket.
		var root *block.BlockRef
		err := ws.AccessWorldState(ctx, nil, func(cs *bucket_lookup.Cursor) error {
			btx, bcs := cs.BuildTransactionAtRef(nil, nil)
			if err := unixfs_block.CreateFromFS(ctx, bcs, fsys, timestamp.Now()); err != nil {
				return err
			}
			var err error
			root, _, err = btx.Write(ctx, true)
			return err
		})
		if err != nil {
			return err
		}

		// Root the object at the tree.
		obj, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{RootRef: root})
		world.ReleaseObjectState(obj)
		return err
	})
}

// storeRef creates the World object key rooted at the stored tree root.
func (f *flowgraphFixture) storeRef(key string, root *block.BlockRef) {
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{RootRef: root})
		world.ReleaseObjectState(obj)
		return err
	})
}

// taskTree returns the tree root the Task stored as its output name.
func (f *flowgraphFixture) taskTree(taskKey, name string) *block.BlockRef {
	value, err := forge_task.ResolveOutput(f.ctx, f.tb.WorldState, taskKey, name)
	if err != nil || value.GetBlockRef().GetEmpty() {
		f.t.Fatalf("output %s=%v err=%v", name, value, err)
	}
	return value.GetBlockRef()
}

// openTree calls fn with the UnixFS tree at root as a read-only billy
// filesystem, valid only while fn runs.
func (f *flowgraphFixture) openTree(root *block.BlockRef, fn func(billy.Filesystem) error) {
	handle := forge_target.ExecControllerHandleWithAccess("skiffos-proof", f.sender, f.tb.Engine, f.tb.WorldState.AccessWorldState, nil)
	err := handle.AccessStorage(f.ctx, &bucket.ObjectRef{RootRef: root}, func(cs *bucket_lookup.Cursor) error {
		ufs := unixfs_block_fs.NewFS(f.ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, nil)
		fh, err := unixfs.NewFSHandle(ufs)
		if err != nil {
			ufs.Release()
			return err
		}
		defer fh.Release()
		return fn(unixfs_billy.NewBillyFS(f.ctx, fh, "", time.Now()))
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

// readTreeFile returns the bytes of the file name in the tree at root.
func (f *flowgraphFixture) readTreeFile(root *block.BlockRef, name string) []byte {
	var data []byte
	f.openTree(root, func(bfs billy.Filesystem) error {
		file, err := bfs.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		return err
	})
	return data
}

// readInitramfs checks that the boot set at root carries no Buildroot root
// image and that its rootfs.cpio.lz4 holds init with exactly initBytes. It
// returns the archive's entry names in order, without the trailer.
func (f *flowgraphFixture) readInitramfs(root *block.BlockRef, initBytes []byte) []string {
	var names []string
	f.openTree(root, func(bfs billy.Filesystem) error {
		// Every root image but the init's archive must be gone.
		entries, err := bfs.ReadDir("")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if name := entry.Name(); strings.HasPrefix(name, "rootfs.") && name != "rootfs.cpio.lz4" {
				return errors.New("boot set still holds " + name)
			}
		}

		// Walk the legacy lz4 newc archive the kernel unpacks.
		file, err := bfs.Open("rootfs.cpio.lz4")
		if err != nil {
			return err
		}
		defer file.Close()
		names, err = listNewc(lz4.NewReader(file), "init", initBytes)
		return err
	})
	return names
}

// waitDone waits for the graph activation's Task to complete and returns its
// key and failure, empty on success.
func (f *flowgraphFixture) waitDone(runKey, node string) (string, string) {
	taskKey := f.waitTask(runKey, node, false)
	var failure string
	f.wait(node+" completion", func() bool {
		task, err := forge_task.LookupTaskBody(f.ctx, f.tb.WorldState, taskKey)
		if err != nil {
			f.t.Fatal(err)
		}
		failure = task.GetResult().GetFailError()
		return task.GetTaskState() == forge_task.State_TaskState_COMPLETE
	})
	return taskKey, failure
}

// executionLog returns the messages of the Task's current Execution log, one
// per line.
func (f *flowgraphFixture) executionLog(taskKey string) string {
	// Resolve the Task's current Pass and its single Execution.
	task, err := forge_task.LookupTaskBody(f.ctx, f.tb.WorldState, taskKey)
	if err != nil {
		f.t.Fatal(err)
	}
	_, _, passKey, err := forge_task.LookupTaskPass(f.ctx, f.tb.WorldState, taskKey, task.GetPassNonce())
	if err != nil || passKey == "" {
		f.t.Fatalf("pass=%q err=%v", passKey, err)
	}
	executions, err := forge_pass.ListPassExecutions(f.ctx, f.tb.WorldState, passKey)
	if err != nil || len(executions) != 1 {
		f.t.Fatalf("executions=%v err=%v", executions, err)
	}

	// Join its retained log entries.
	execution, obj, err := forge_execution.LookupExecution(f.ctx, f.tb.WorldState, executions[0])
	world.ReleaseObjectState(obj)
	if err != nil {
		f.t.Fatal(err)
	}
	var log strings.Builder
	for _, entry := range execution.GetLogEntries() {
		log.WriteString(entry.GetMessage())
		log.WriteByte('\n')
	}
	return log.String()
}

// listNewc reads a newc cpio archive and returns its entry names in order,
// without the trailer, and checks that the entry named want holds wantBytes.
func listNewc(r io.Reader, want string, wantBytes []byte) ([]string, error) {
	var names []string
	var hdr [110]byte
	for {
		// Read the fixed header; its fields are 8-digit hex numbers.
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, err
		}
		if string(hdr[:6]) != "070701" {
			return nil, errors.New("not a newc archive")
		}
		fileSize, err1 := strconv.ParseUint(string(hdr[54:62]), 16, 32)
		nameSize, err2 := strconv.ParseUint(string(hdr[94:102]), 16, 32)
		if err := errors.Join(err1, err2); err != nil || nameSize == 0 {
			return nil, errors.Join(errors.New("bad newc header"), err)
		}

		// The name ends in NUL and pads the header to a multiple of four.
		name := make([]byte, (110+nameSize+3)&^3-110)
		if _, err := io.ReadFull(r, name); err != nil {
			return nil, err
		}
		entry := string(name[:nameSize-1])
		if entry == "TRAILER!!!" {
			return names, nil
		}
		names = append(names, entry)

		// The data pads to a multiple of four; compare the wanted entry's.
		data := make([]byte, (fileSize+3)&^3)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		if entry == want && !bytes.Equal(data[:fileSize], wantBytes) {
			return nil, errors.New(want + " differs from the supplied bytes")
		}
	}
}

// bootSet logs the Task's stored boot set and returns its root.
func (f *flowgraphFixture) bootSet(taskKey string) *block.BlockRef {
	// Read the output's UnixFS root directory from World storage.
	value, err := forge_task.ResolveOutput(f.ctx, f.tb.WorldState, taskKey, forge_lib_skiffos.OutputImages)
	if err != nil || value.GetBlockRef().GetEmpty() {
		f.t.Fatalf("output images=%v err=%v", value, err)
	}
	root := value.GetBlockRef()
	handle := forge_target.ExecControllerHandleWithAccess("skiffos-proof", f.sender, f.tb.Engine, f.tb.WorldState.AccessWorldState, nil)
	var files int
	err = handle.AccessStorage(f.ctx, &bucket.ObjectRef{RootRef: root}, func(cs *bucket_lookup.Cursor) error {
		_, bcs := cs.BuildTransaction(nil)
		tree, err := unixfs_block.NewFSTree(f.ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
		if err != nil {
			return err
		}
		ds, err := tree.Readdir()
		if err != nil || ds == nil {
			return errors.Join(err, errors.New("boot set is empty"))
		}
		for ds.Next() {
			ent, dirent, err := ds.FollowEntry()
			if err != nil {
				return err
			}
			files++
			f.t.Logf("boot set %s: %s %d bytes", taskKey, dirent.GetName(), ent.GetFSNode().GetFile().GetTotalSize())
		}
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Logf("boot set %s: %d entries, root %s", taskKey, files, root.MarshalString())
	return root
}
