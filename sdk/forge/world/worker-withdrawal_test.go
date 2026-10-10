//go:build unix && !tinygo

package s4wave_forge_world

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	resolver_ctrl "github.com/aperturerobotics/controllerbus/controller/resolver"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

// fakeDockerScript stands in for the Docker CLI. Container creation answers
// with the reserved runtime name as the container ID. Wait blocks on the Job's
// gate fifo and prints the first exit status addressed to its container or to
// any container. Stop addresses the status of a stopped container, as Docker
// reports, so a stop that outlives its wait cannot end a later container. The
// script uses only shell builtins because the CLI environment is empty.
const fakeDockerScript = `#!/bin/sh
case "$1" in
create) echo "$3" ;;
wait)
	echo > "$GATE/started"
	while read id status < "$GATE/gate"; do
		if [ "$id" = "$2" ] || [ "$id" = any ]; then
			echo "$status"
			exit 0
		fi
	done
	;;
stop) for id; do :; done; echo "$id 137" > "$GATE/gate" ;;
esac
`

// testbedResolverBus keeps a Worker execution from attaching its own factory
// resolver to the Forge testbed bus. The testbed already resolves the Forge
// controller factories, and a second resolver would also load the testbed's
// Hydra controllers again and unload them when the execution exits.
type testbedResolverBus struct {
	bus.Bus
}

// AddController skips the execution's factory resolver.
func (b testbedResolverBus) AddController(ctx context.Context, ctrl controller.Controller, cb func(error)) (func(), error) {
	if _, ok := ctrl.(*resolver_ctrl.Controller); ok {
		return func() {}, nil
	}
	return b.Bus.AddController(ctx, ctrl, cb)
}

// dockerGate controls one fake Docker container from the test.
type dockerGate struct {
	// dir holds the fifos shared with the fake Docker CLI.
	dir string
	// started receives a line when the container begins waiting.
	started *os.File
	// gate sends the container its exit status.
	gate *os.File
}

// newDockerGate creates the fifos of one fake container. The test holds both
// open, so the fake CLI never blocks on a missing peer.
func newDockerGate(t *testing.T) *dockerGate {
	t.Helper()
	g := &dockerGate{dir: t.TempDir()}
	for _, name := range []string{"started", "gate"} {
		path := filepath.Join(g.dir, name)
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		if name == "started" {
			g.started = f
		} else {
			g.gate = f
		}
	}
	return g
}

// waitStarted blocks until the container is running its wait. Darwin does not
// poll FIFOs, so a timer bounds the read instead of a read deadline.
func (g *dockerGate) waitStarted(t *testing.T) {
	t.Helper()
	read := make(chan error, 1)
	go func() {
		_, err := g.started.Read(make([]byte, 1))
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatalf("container did not start: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("container did not start within 30s")
	}
}

// exit lets the container exit with status 0.
func (g *dockerGate) exit(t *testing.T) {
	t.Helper()
	if _, err := g.gate.WriteString("any 0\n"); err != nil {
		t.Fatal(err)
	}
}

// workerHarness runs the real Forge controllers of one Worker that a Cluster
// places Jobs on.
type workerHarness struct {
	// ctx bounds the test.
	ctx context.Context
	// le is the test logger.
	le *logrus.Entry
	// tb is the Forge testbed whose bus runs the Worker's controllers.
	tb *forge_testbed.Testbed
	// sender submits the Jobs.
	sender peer.ID
	// workerPeerID is the Worker's peer.
	workerPeerID peer.ID
	// script is the fake Docker CLI the Jobs run.
	script string
}

// Worker and Cluster object keys of the harness.
const harnessWorkerKey, harnessClusterKey = "worker/1", "cluster/1"

// newWorkerHarness creates the Worker and the Cluster that places Jobs on it.
func newWorkerHarness(t *testing.T) *workerHarness {
	// Start a Forge testbed whose bus will run the Worker's controllers.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Resolve the Forge world operations the Jobs submit.
	sender := tb.Volume.GetPeerID()
	releaseOps, err := tb.Bus.AddController(ctx, world.NewLookupOpController("forge-ops", tb.EngineID, forge_world.LookupWorldOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOps)

	// Create the Worker and the Cluster that places Jobs on it.
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, harnessWorkerKey, "test-worker", []*identity.Keypair{keypair}, sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.CreateCluster(ctx, tb.WorldState, harnessClusterKey, "test-cluster", workerPeer.GetPeerID(), sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.AssignWorkerToCluster(ctx, tb.WorldState, harnessClusterKey, harnessWorkerKey, sender); err != nil {
		t.Fatal(err)
	}

	// Install the fake Docker CLI.
	script := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(script, []byte(fakeDockerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	return &workerHarness{
		ctx:          ctx,
		le:           logrus.NewEntry(logrus.New()),
		tb:           tb,
		sender:       sender,
		workerPeerID: workerPeer.GetPeerID(),
		script:       script,
	}
}

// declaration returns the Worker declaration the Device publishes.
func (h *workerHarness) declaration() *s4wave_device.ForgeWorkerDeclaration {
	return &s4wave_device.ForgeWorkerDeclaration{
		WorkerObjectKey: harnessWorkerKey, MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"docker"},
	}
}

// newAdmission constructs the admission of one Worker execution lifetime.
func (h *workerHarness) newAdmission(claimID string) *WorkerAdmission {
	return NewWorkerAdmission(h.tb.Engine, harnessWorkerKey, h.workerPeerID, claimID, forge_lib_docker.NewStopper(forge_lib_docker.NewExecDockerRunner()))
}

// startWorker runs one Worker execution that receives declaration updates.
// The returned function ends the execution cleanly and returns its error.
func (h *workerHarness) startWorker(admission *WorkerAdmission, updates <-chan *s4wave_device.ForgeWorkerDeclaration) func() error {
	// Build the Worker resource around the test declaration stream.
	executeCtx, cancelExecute := context.WithCancel(h.ctx)
	stream := &changingWorkerDeclarationStream{ctx: executeCtx, updates: updates, initialDeclaration: h.declaration()}
	resource := &forgeWorkerResource{
		objectKey: harnessWorkerKey, b: testbedResolverBus{Bus: h.tb.Bus}, le: h.le, peerID: h.workerPeerID, engineID: h.tb.EngineID,
		admission:            admission,
		openDeclarationWatch: func(context.Context, bus.Bus) (workerDeclarationStream, error) { return stream, nil },
	}

	// Run the execution until the returned function cancels it.
	executeDone := make(chan error, 1)
	go func() { executeDone <- resource.Execute(nil, &forgeWorkerExecuteStream{ctx: executeCtx}) }()
	return func() error {
		cancelExecute()
		return <-executeDone
	}
}

// submit creates a Job whose Task runs one fake Docker container and places
// it on the Worker.
func (h *workerHarness) submit(t *testing.T, jobKey string, gate *dockerGate) {
	// Resolve the Target whose Execution runs the fake Docker CLI.
	t.Helper()
	target, err := target_json.ResolveYAML(h.ctx, h.tb.Bus, []byte(`
exec:
  controller:
    id: forge/lib/docker
    config:
      image: test-image
      milliCpu: 500
      memoryBytes: 1048576
      dockerPath: `+h.script+`
      dockerEnv:
        GATE: `+gate.dir+`
`))
	if err != nil {
		t.Fatal(err)
	}

	// Create the Job with its Task in one transaction.
	tx, err := h.tb.Engine.NewTransaction(h.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	object, _, err := forge_job.CreateJobWithTasks(h.ctx, tx, h.sender, jobKey, map[string]*forge_target.Target{"docker": target}, "", nil, timestamp.Now())
	world.ReleaseObjectState(object)
	if err == nil {
		err = tx.Commit(h.ctx)
	}
	if err != nil {
		tx.Discard()
		t.Fatal(err)
	}

	// Place the Job on the Cluster, and so on the Worker.
	if _, _, err := forge_cluster.AssignJobToCluster(h.ctx, h.tb.WorldState, harnessClusterKey, jobKey, h.sender); err != nil {
		t.Fatal(err)
	}
}

// waitJobFailure waits for a Job to complete and returns its failure message.
func (h *workerHarness) waitJobFailure(t *testing.T, jobKey string) string {
	t.Helper()
	result, err := forge_job.WaitJobComplete(h.ctx, h.le, h.tb.WorldState, jobKey)
	if err != nil {
		t.Fatal(err)
	}
	return result.GetResult().GetFailError()
}

// TestForgeWorkerWithdrawalDrainsAndHoldsNewJobs runs the real Forge
// controllers on a Worker whose Device declaration is withdrawn and restored.
// A Job running at withdrawal is stopped and reports its result, and a Job
// submitted afterward does not fail but runs once the declaration returns.
func TestForgeWorkerWithdrawalDrainsAndHoldsNewJobs(t *testing.T) {
	// Run the Worker with a declaration the test can withdraw.
	h := newWorkerHarness(t)
	admission := h.newAdmission("claim-1")
	h.tb.StaticResolver.AddFactory(forge_lib_docker.NewWorkerFactory(h.tb.Bus, admission))
	updates := make(chan *s4wave_device.ForgeWorkerDeclaration, 1)
	stopWorker := h.startWorker(admission, updates)
	defer func() {
		if err := stopWorker(); !stderrors.Is(err, context.Canceled) {
			t.Errorf("Worker execution ended with %v", err)
		}
	}()

	// Submit a Job and wait until its container is running.
	running, held := newDockerGate(t), newDockerGate(t)
	h.submit(t, "job/running", running)
	running.waitStarted(t)

	// Withdraw the declaration. Draining stops the running container, and the
	// Job still reports its result because its controllers keep running.
	updates <- nil
	if failure := h.waitJobFailure(t, "job/running"); !strings.Contains(failure, "exited with status 137") {
		t.Fatalf("running Job result = %q, want the stopped container's exit status", failure)
	}

	// A Job submitted while withdrawn waits for capacity instead of failing.
	h.submit(t, "job/held", held)
	holdCtx, cancelHold := context.WithTimeout(h.ctx, 250*time.Millisecond)
	defer cancelHold()
	if job, err := forge_job.WaitJobComplete(holdCtx, h.le, h.tb.WorldState, "job/held"); err == nil {
		t.Fatalf("Job submitted after withdrawal ended: %q", job.GetResult().GetFailError())
	} else if holdCtx.Err() == nil {
		t.Fatal(err)
	}

	// Restoring the declaration runs the waiting Job to success.
	updates <- h.declaration()
	held.waitStarted(t)
	held.exit(t)
	if failure := h.waitJobFailure(t, "job/held"); failure != "" {
		t.Fatalf("held Job failed: %s", failure)
	}
}

// restartedAdmission forwards Docker reservations to the admission of the
// current Worker execution, as a restarted daemon registers its own.
type restartedAdmission struct {
	current atomic.Pointer[WorkerAdmission]
}

// Reserve reserves through the current Worker execution's admission.
func (a *restartedAdmission) Reserve(ctx context.Context, executionKey string, conf *forge_lib_docker.Config) (forge_lib_docker.Reservation, error) {
	return a.current.Load().Reserve(ctx, executionKey, conf)
}

// TestForgeWorkerRestartResumesRunningJob ends a Worker execution cleanly
// while a Job's container runs, as a daemon stop does. Stop custody ends the
// container, and the next Worker execution resumes the same Execution in a
// new container that runs the Job to success.
func TestForgeWorkerRestartResumesRunningJob(t *testing.T) {
	// Run the first Worker execution and start a Job's container.
	h := newWorkerHarness(t)
	admission := &restartedAdmission{}
	admission.current.Store(h.newAdmission("claim-1"))
	h.tb.StaticResolver.AddFactory(forge_lib_docker.NewWorkerFactory(h.tb.Bus, admission))
	stopWorker := h.startWorker(admission.current.Load(), nil)
	gate := newDockerGate(t)
	h.submit(t, "job/restart", gate)
	gate.waitStarted(t)

	// Stop the Worker execution as a clean daemon stop does.
	if err := stopWorker(); !stderrors.Is(err, context.Canceled) {
		t.Fatalf("first Worker execution ended with %v", err)
	}

	// The next Worker execution relaunches the Execution's container.
	admission.current.Store(h.newAdmission("claim-2"))
	stopWorker = h.startWorker(admission.current.Load(), nil)
	defer func() {
		if err := stopWorker(); !stderrors.Is(err, context.Canceled) {
			t.Errorf("second Worker execution ended with %v", err)
		}
	}()
	gate.waitStarted(t)
	gate.exit(t)
	if failure := h.waitJobFailure(t, "job/restart"); failure != "" {
		t.Fatalf("resumed Job failed: %s", failure)
	}
}
