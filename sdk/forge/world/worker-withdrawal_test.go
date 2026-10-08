//go:build unix && !tinygo

package s4wave_forge_world

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
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
// with an ID, and wait blocks on the Job's gate fifo and prints the exit status
// written to it. Stop writes the status of a stopped container, as Docker
// reports. The script uses only shell builtins because the CLI environment is
// empty.
const fakeDockerScript = `#!/bin/sh
case "$1" in
create) echo fake-container ;;
wait) echo > "$GATE/started"; read status < "$GATE/gate"; echo "$status" ;;
stop) echo 137 > "$GATE/gate" ;;
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

// waitStarted blocks until the container is running its wait.
func (g *dockerGate) waitStarted(t *testing.T) {
	t.Helper()
	if err := g.started.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := g.started.Read(make([]byte, 1)); err != nil {
		t.Fatalf("container did not start: %v", err)
	}
}

// exit lets the container exit with status 0.
func (g *dockerGate) exit(t *testing.T) {
	t.Helper()
	if _, err := g.gate.WriteString("0\n"); err != nil {
		t.Fatal(err)
	}
}

// TestForgeWorkerWithdrawalDrainsAndHoldsNewJobs runs the real Forge
// controllers on a Worker whose Device declaration is withdrawn and restored.
// A Job running at withdrawal is stopped and reports its result, and a Job
// submitted afterward does not fail but runs once the declaration returns.
func TestForgeWorkerWithdrawalDrainsAndHoldsNewJobs(t *testing.T) {
	// Start a Forge testbed whose bus will run the Worker's controllers.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
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
	defer releaseOps()

	// Create the Worker and the Cluster that places Jobs on it.
	workerPeer, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	const workerKey, clusterKey = "worker/1", "cluster/1"
	if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, workerKey, "test-worker", []*identity.Keypair{keypair}, sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.CreateCluster(ctx, tb.WorldState, clusterKey, "test-cluster", workerPeer.GetPeerID(), sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.AssignWorkerToCluster(ctx, tb.WorldState, clusterKey, workerKey, sender); err != nil {
		t.Fatal(err)
	}

	// Admit Docker Executions through the Worker's admission.
	admission := NewWorkerAdmission(tb.Engine, workerKey, workerPeer.GetPeerID(), "claim-1", forge_lib_docker.NewStopper(forge_lib_docker.NewExecDockerRunner()))
	tb.StaticResolver.AddFactory(forge_lib_docker.NewFactory(tb.Bus, admission))

	// Run the Worker execution with a declaration the test can withdraw.
	declaration := &s4wave_device.ForgeWorkerDeclaration{
		WorkerObjectKey: workerKey, MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"docker"},
	}
	updates := make(chan *s4wave_device.ForgeWorkerDeclaration, 1)
	executeCtx, cancelExecute := context.WithCancel(ctx)
	stream := &changingWorkerDeclarationStream{ctx: executeCtx, updates: updates, initialDeclaration: declaration}
	resource := &forgeWorkerResource{
		objectKey: workerKey, b: testbedResolverBus{Bus: tb.Bus}, le: le, peerID: workerPeer.GetPeerID(), engineID: tb.EngineID,
		admission:            admission,
		openDeclarationWatch: func(context.Context, bus.Bus) (workerDeclarationStream, error) { return stream, nil },
	}
	executeDone := make(chan error, 1)
	go func() { executeDone <- resource.Execute(nil, &forgeWorkerExecuteStream{ctx: executeCtx}) }()
	defer func() {
		cancelExecute()
		if err := <-executeDone; !stderrors.Is(err, context.Canceled) {
			t.Errorf("Worker execution ended with %v", err)
		}
	}()

	// Define a Job submission that runs a fake Docker container.
	script := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(script, []byte(fakeDockerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	submit := func(jobKey string, gate *dockerGate) {
		// Resolve the Target whose Execution runs the fake Docker CLI.
		t.Helper()
		target, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(`
exec:
  controller:
    id: forge/lib/docker
    config:
      image: test-image
      milliCpu: 500
      memoryBytes: 1048576
      dockerPath: `+script+`
      dockerEnv:
        GATE: `+gate.dir+`
`))
		if err != nil {
			t.Fatal(err)
		}

		// Create the Job with its Task in one transaction.
		tx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		object, _, err := forge_job.CreateJobWithTasks(ctx, tx, sender, jobKey, map[string]*forge_target.Target{"docker": target}, "", nil, timestamp.Now())
		world.ReleaseObjectState(object)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			tx.Discard()
			t.Fatal(err)
		}

		// Place the Job on the Cluster, and so on the Worker.
		if _, _, err := forge_cluster.AssignJobToCluster(ctx, tb.WorldState, clusterKey, jobKey, sender); err != nil {
			t.Fatal(err)
		}
	}

	// Submit a Job and wait until its container is running.
	running, held := newDockerGate(t), newDockerGate(t)
	submit("job/running", running)
	running.waitStarted(t)

	// Withdraw the declaration. Draining stops the running container, and the
	// Job still reports its result because its controllers keep running.
	updates <- nil
	result, err := forge_job.WaitJobComplete(ctx, le, tb.WorldState, "job/running")
	if err != nil {
		t.Fatal(err)
	}
	if failure := result.GetResult().GetFailError(); !strings.Contains(failure, "exited with status 137") {
		t.Fatalf("running Job result = %q, want the stopped container's exit status", failure)
	}

	// A Job submitted while withdrawn waits for capacity instead of failing.
	submit("job/held", held)
	holdCtx, cancelHold := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancelHold()
	if job, err := forge_job.WaitJobComplete(holdCtx, le, tb.WorldState, "job/held"); err == nil {
		t.Fatalf("Job submitted after withdrawal ended: %q", job.GetResult().GetFailError())
	} else if holdCtx.Err() == nil {
		t.Fatal(err)
	}

	// Restoring the declaration runs the waiting Job to success.
	updates <- declaration
	held.waitStarted(t)
	held.exit(t)
	result, err = forge_job.WaitJobComplete(ctx, le, tb.WorldState, "job/held")
	if err != nil {
		t.Fatal(err)
	}
	if failure := result.GetResult().GetFailError(); failure != "" {
		t.Fatalf("held Job failed: %s", failure)
	}
}
