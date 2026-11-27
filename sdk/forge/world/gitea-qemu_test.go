//go:build linux && !tinygo

package s4wave_forge_world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/dolthub/vitess/go/mysql"
	"github.com/s4wave/spacewave/db/block"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	forge_task "github.com/s4wave/spacewave/forge/task"
	gitea_testbed "github.com/s4wave/spacewave/internal/testbed/gitea"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

// qemuGitea owns a fresh World and consumes existing, explicitly supplied guest disks.
// A failed run preserves those disks; the test never formats or reseeds them.
type qemuGitea struct {
	*flowgraphFixture
	dir       string
	dockerEnv []string
	image     string
	base      string
	port      int
}

// TestGiteaQEMUFlowgraph replaces a real Forge-owned VM and reads its original work.
// SPACEWAVE_QEMU_FIXTURE must name a fresh fixture prepared by os/qemu; no download,
// host mount, daemon installation or existing user data reset occurs in this test.
func TestGiteaQEMUFlowgraph(t *testing.T) {
	// Require explicit runtime inputs and preserve them independently of test cleanup.
	fixture := os.Getenv("SPACEWAVE_QEMU_FIXTURE")
	if fixture == "" {
		t.Skip("set SPACEWAVE_QEMU_FIXTURE for the real QEMU application proof")
	}
	dockerPath, image := os.Getenv("SPACEWAVE_DOCKER"), os.Getenv("SPACEWAVE_QEMU_IMAGE")
	if dockerPath == "" || image == "" {
		t.Fatal("SPACEWAVE_DOCKER and SPACEWAVE_QEMU_IMAGE are required")
	}
	// Refuse accidental reuse of a prior test's SQL World; retained disks are not reset.
	if _, err := os.Stat(filepath.Join(fixture, "host-world.s4wave")); !os.IsNotExist(err) {
		t.Fatal("fixture already contains a SQL World; prepare a new fixture")
	}
	// Fail stop and remove requests only while the fixture gate exists; all other calls use Docker.
	wrapper := filepath.Join(fixture, "docker-cli")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nif { [ \"$1\" = stop ] || [ \"$1\" = rm ]; } && [ -f \"$SPACEWAVE_STOP_BLOCK\" ]; then echo 'fixture stop temporarily unavailable' >&2; exit 75; fi\nexec \"$SPACEWAVE_REAL_DOCKER\" \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := newWorkerHarnessWithOptions(t, workerHarnessOptions{
		timeout: 10 * time.Minute, memoryBytes: 4 << 30, dockerPath: wrapper,
		storage: []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: filepath.Join(fixture, "host-world.s4wave")})},
	})
	f := &qemuGitea{flowgraphFixture: &flowgraphFixture{workerHarness: h, t: t}, dir: fixture, image: image, dockerEnv: []string{"DOCKER_HOST=" + os.Getenv("DOCKER_HOST"), "SPACEWAVE_REAL_DOCKER=" + dockerPath}}
	f.port = freeQEMUPort(t)
	f.base = "http://127.0.0.1:" + strconv.Itoa(f.port)
	h.tb.StaticResolver.AddFactory(flowgraph_run.NewFactory(h.tb.Bus))

	// Attach one authoritative SQL endpoint to a real durable World object.
	sqlPort, stopSQL := f.attachSQL()
	defer stopSQL()
	startup := filepath.Join(f.dir, "startup.sh")
	if err := os.WriteFile(startup, []byte(giteaGuestStartup(sqlPort)), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run the existing Worker declaration, admission and Docker custody controllers.
	admission := h.newAdmission("qemu-gitea-proof")
	h.tb.StaticResolver.AddFactory(forge_lib_docker.NewWorkerFactory(h.tb.Bus, admission))
	updates := make(chan *s4wave_device.ForgeWorkerDeclaration, 1)
	stopWorker := h.startWorker(admission, updates)
	defer func() {
		if err := stopWorker(); !errors.Is(err, context.Canceled) {
			t.Errorf("Worker stop: %v", err)
		}
	}()
	f.createGraph(startup)
	firstRun := "flowgraph-run/gitea-first"
	f.startRun(firstRun, "flowgraph/gitea")
	f.waitReady(firstRun)
	first := f.reservation(firstRun, "vm")
	firstLogs := f.docker("logs", first.Runtime.Id)
	if !strings.Contains(firstLogs, "SPACEWAVE_OS_APP_STARTED pid=") {
		t.Fatal("ready VM did not report application startup")
	}

	// Create actual Gitea metadata and push a repository containing a compiled program.
	const password = "Spacewave-Fixture-2026!"
	gitea_testbed.Request(t, h.ctx, f.base+"/api/v1/user/repos", password, "POST", `{"name":"proof","private":false,"auto_init":false}`)
	issue := gitea_testbed.Request(t, h.ctx, f.base+"/api/v1/repos/operator/proof/issues", password, "POST", `{"title":"Retained state","body":"Original Spacewave OS issue"}`)
	saved, err := fastjson.ParseBytes(issue)
	if err != nil {
		t.Fatal(err)
	}
	issueNumber := saved.GetInt64("number")
	issueBody := string(saved.GetStringBytes("body"))
	if issueNumber < 1 || issueBody != "Original Spacewave OS issue" {
		t.Fatalf("issue=%s", issue)
	}

	seed := t.TempDir()
	gitea_testbed.Git(t, h.ctx, seed, password, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(seed, "main.c"), []byte("#include <stdio.h>\nint main(void) { puts(\"Spacewave OS retained build\"); return 0; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "Makefile"), []byte("proof: main.c\n\tcc -O2 -o proof main.c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitea_testbed.Git(t, h.ctx, seed, password, "add", ".")
	gitea_testbed.Git(t, h.ctx, seed, password, "commit", "-m", "Retained application build")
	commit := strings.TrimSpace(gitea_testbed.Git(t, h.ctx, seed, password, "rev-parse", "HEAD"))
	gitea_testbed.Git(t, h.ctx, seed, password, "push", f.base+"/operator/proof.git", "main")

	// Deliver the explicit build request through the graph's existing event interface.
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		result, err := flowgraph_run.DeliverEvents(ctx, ws, firstRun, &flowgraph_run.DeliverEventsRequest{NodeId: "build-request", NextCursor: []byte(commit), Events: []*flowgraph_run.DeliveredEvent{{ObjectKey: "sql/gitea", EventId: commit, Port: "commit"}}})
		if err == nil && result != flowgraph_run.DeliverEventsResult_DELIVER_EVENTS_RESULT_DELIVERED {
			return fmt.Errorf("delivery=%s", result)
		}
		return err
	})
	buildTask := f.waitTask(firstRun, "build", true)
	buildReservation := f.reservation(firstRun, "build")
	if !strings.Contains(f.docker("logs", buildReservation.Runtime.Id), commit) {
		t.Fatal("build did not use the pushed commit")
	}
	artifact := f.output(buildTask, "artifact")
	if len(artifact) < 1000 {
		t.Fatalf("compiled artifact has only %d bytes", len(artifact))
	}
	artifactPath := filepath.Join(t.TempDir(), "proof")
	if err := os.WriteFile(artifactPath, artifact, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(h.ctx, artifactPath).CombinedOutput(); err != nil || string(output) != "Spacewave OS retained build\n" {
		t.Fatalf("artifact output=%q err=%v", output, err)
	}

	// Lose the stop response while the actual VM stays live; custody must retain its debit.
	stopGate := filepath.Join(f.dir, "stop-block")
	if err := os.WriteFile(stopGate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(stopGate)
	updates <- nil
	f.wait("pending stop", func() bool {
		current, err := forge_runtime.LookupReservation(f.ctx, f.tb.WorldState, first.ObjectKey())
		if err != nil {
			t.Fatal(err)
		}
		return current.State == forge_runtime.ReservationState_RESERVATION_STATE_PENDING_STOP
	})
	if got := strings.TrimSpace(f.docker("inspect", "--format", "{{.State.Running}}", first.Runtime.Id)); got != "true" {
		t.Fatalf("uncertain stop lost live runtime: %s", got)
	}
	capacity, err := forge_runtime.LookupWorkerCapacity(f.ctx, f.tb.WorldState, harnessWorkerKey)
	if err != nil || capacity.MemoryBytesReserved < 2<<30 {
		t.Fatalf("pending-stop capacity=%+v err=%v", capacity, err)
	}

	// Restore the real stop path and reconcile through the owning Worker's contract.
	if err := os.Remove(stopGate); err != nil {
		t.Fatal(err)
	}
	if err := admission.Renew(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.waitTask(firstRun, "vm", true)
	f.requireStopped(first)
	stoppedLogs := f.docker("logs", first.Runtime.Id)
	f.requireGuestBindings(firstLogs, stoppedLogs)
	if !strings.Contains(stoppedLogs, "SPACEWAVE_OS_APP_STOPPED status=0") || !strings.Contains(stoppedLogs, "SPACEWAVE_OS_PASS") {
		t.Fatal("guest clean-stop receipt missing")
	}

	// Replace the execution on the same graph and raw disks, with the same SQL object.
	updates <- h.declaration()
	secondRun := "flowgraph-run/gitea-replacement"
	f.startRun(secondRun, "flowgraph/gitea")
	f.waitReady(secondRun)
	second := f.reservation(secondRun, "vm")
	if first.ExecutionObjectKey == second.ExecutionObjectKey || first.Runtime.Id == second.Runtime.Id {
		t.Fatal("replacement reused its execution identity")
	}
	replacementLogs := f.docker("logs", second.Runtime.Id)
	f.requireGuestBindings(firstLogs, replacementLogs)
	if f.configDigest(stoppedLogs, "SPACEWAVE_OS_CONFIG_CLOSED") != f.configDigest(replacementLogs, "SPACEWAVE_OS_CONFIG_OPEN") {
		t.Fatal("application configuration changed across replacement")
	}
	readback := gitea_testbed.Request(t, h.ctx, f.base+"/api/v1/repos/operator/proof/issues/"+strconv.FormatInt(issueNumber, 10), password, "GET", "")
	recovered, err := fastjson.ParseBytes(readback)
	if err != nil || string(recovered.GetStringBytes("body")) != issueBody {
		t.Fatalf("recovered issue=%s err=%v", readback, err)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	gitea_testbed.Git(t, h.ctx, filepath.Dir(clone), password, "clone", f.base+"/operator/proof.git", clone)
	if got := strings.TrimSpace(gitea_testbed.Git(t, h.ctx, clone, password, "rev-parse", "HEAD")); got != commit {
		t.Fatalf("recovered commit=%s want=%s", got, commit)
	}
	if got := f.output(buildTask, "artifact"); string(got) != string(artifact) {
		t.Fatal("retained Forge artifact changed")
	}
	updates <- nil
	f.waitTask(secondRun, "vm", true)
	f.requireStopped(second)
	t.Logf("SQL volume=%s object=sql/gitea issue=%d commit=%s artifactBytes=%d firstExecution=%s replacementExecution=%s", h.tb.Volume.GetID(), issueNumber, commit, len(artifact), first.ExecutionObjectKey, second.ExecutionObjectKey)
}

// attachSQL joins the endpoint before closing its store and the enclosing World.
func (f *qemuGitea) attachSQL() (int, func()) {
	// Publish the SQL object and resolve its ordinary World operations.
	release, err := f.tb.Bus.AddController(f.ctx, world.NewLookupOpController("sql-ops", f.tb.EngineID, world.NewSchema(s4wave_sql_world.WorldTypes).LookupOp), nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(release)
	obj, _, err := world.CreateWorldObject(f.ctx, f.tb.WorldState, "sql/gitea", func(cursor *block.Cursor) error { cursor.SetBlock(sql_mysql.NewRootBlock(), true); return nil })
	world.ReleaseObjectState(obj)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := world_types.SetObjectType(f.ctx, f.tb.WorldState, "sql/gitea", s4wave_sql_world.SqlDbTypeID); err != nil {
		f.t.Fatal(err)
	}
	store, err := s4wave_sql_world.NewWorldBackedSql(f.ctx, f.tb.WorldState, "sql/gitea")
	if err != nil {
		f.t.Fatal(err)
	}

	// Bind to loopback; QEMU user networking reaches it through 10.0.2.2.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		store.Close()
		f.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(f.ctx))
	done := make(chan error, 1)
	go func() { done <- store.ServeMySQL(ctx, lis, mysql.NewAuthServerNone()) }()
	stop := func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			f.t.Errorf("SQL stop: %v", err)
		}
		store.Close()
	}
	db, err := sql.Open("mysql", "root@tcp("+lis.Addr().String()+")/")
	if err != nil {
		stop()
		f.t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(f.ctx, "CREATE DATABASE gitea CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		stop()
		f.t.Fatal(err)
	}
	return lis.Addr().(*net.TCPAddr).Port, stop
}

// freeQEMUPort reserves a currently unused loopback port until QEMU takes it.
func freeQEMUPort(t *testing.T) int {
	// Release the probe before starting QEMU, whose bind is the final authority.
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	_ = lis.Close()
	return port
}

// docker inspects only this fixture's explicitly identified containers.
func (f *qemuGitea) docker(args ...string) string {
	// Bound inspection with the test lifetime and capture its actual result.
	command := exec.CommandContext(f.ctx, f.script, args...)
	command.Env = f.dockerEnv
	output, err := command.CombinedOutput()
	if err != nil {
		f.t.Fatalf("docker %v: %v\n%s", args, err, output)
	}
	return string(output)
}

// waitReady waits for Gitea's health check until the run's VM Task ends.
// Docker start, a cold boot and the first migration have no fixed duration on a
// loaded host; the harness timeout bounds the whole proof.
func (f *qemuGitea) waitReady(run string) {
	// Watch World revisions for the VM Task's completion beside the health check.
	ctx, cancel := context.WithCancelCause(f.ctx)
	defer cancel(nil)
	go func() {
		cancel(f.watchTaskEnd(ctx, run, "vm"))
	}()
	gitea_testbed.WaitReady(f.t, ctx, f.base, "")
}

// watchTaskEnd returns why the node's Task can no longer serve, or ctx's error.
func (f *qemuGitea) watchTaskEnd(ctx context.Context, run, node string) error {
	for {
		seq, err := f.tb.WorldState.GetSeqno(ctx)
		if err != nil {
			return err
		}
		if err := f.taskEnded(ctx, run, node); err != nil {
			return err
		}
		if _, err := f.tb.WorldState.WaitSeqno(ctx, seq+1); err != nil {
			return err
		}
	}
}

// taskEnded returns a non-nil error once the node's Task is complete.
func (f *qemuGitea) taskEnded(ctx context.Context, run, node string) error {
	obj, err := world.MustGetObject(ctx, f.tb.WorldState, run)
	if err != nil {
		return err
	}
	defer world.ReleaseObjectState(obj)
	var state *s4wave_flowgraph.FlowgraphRun
	_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
		var err error
		state, err = s4wave_flowgraph.UnmarshalFlowgraphRun(ctx, cursor)
		return err
	})
	if err != nil {
		return err
	}
	for _, a := range state.GetActivations() {
		if a.GetNodeId() != node {
			continue
		}
		task, err := forge_task.LookupTaskBody(ctx, f.tb.WorldState, a.GetTaskKey())
		if err != nil {
			return err
		}
		if task.GetTaskState() == forge_task.State_TaskState_COMPLETE {
			return fmt.Errorf("%s Task completed before Gitea answered: %s", node, task.GetResult().GetFailError())
		}
	}
	return nil
}

// reservation follows the activation's current Pass to its real execution grant.
func (f *qemuGitea) reservation(run, node string) *forge_runtime.Reservation {
	// Resolve only this Task's bounded execution set; no global runtime scan is needed.
	taskKey := f.waitTask(run, node, false)
	task, err := forge_task.LookupTaskBody(f.ctx, f.tb.WorldState, taskKey)
	if err != nil {
		f.t.Fatal(err)
	}
	_, _, passKey, err := forge_task.LookupTaskPass(f.ctx, f.tb.WorldState, taskKey, task.GetPassNonce())
	if err != nil {
		f.t.Fatal(err)
	}
	executions, err := forge_pass.ListPassExecutions(f.ctx, f.tb.WorldState, passKey)
	if err != nil || len(executions) != 1 {
		f.t.Fatalf("executions=%v err=%v", executions, err)
	}
	res, err := forge_runtime.LookupReservation(f.ctx, f.tb.WorldState, forge_runtime.BuildReservationObjectKey(executions[0]))
	if err != nil || res.Runtime.Id == "" {
		f.t.Fatalf("reservation=%+v err=%v", res, err)
	}
	return res
}

// requireStopped checks Forge's durable cleanup facts before replacement.
func (f *qemuGitea) requireStopped(res *forge_runtime.Reservation) {
	// Observe custody release and zero running status of the identified container.
	f.wait("confirmed runtime cleanup", func() bool {
		current, err := forge_runtime.LookupReservation(f.ctx, f.tb.WorldState, res.ObjectKey())
		if err != nil {
			f.t.Fatal(err)
		}
		return current.Cleanup.Complete()
	})
	if got := strings.TrimSpace(f.docker("inspect", "--format", "{{.State.Running}} {{.State.ExitCode}}", res.Runtime.Id)); got != "false 0" {
		f.t.Fatalf("stopped VM=%q", got)
	}
}

// output reads a retained Task artifact through the existing Forge value contract.
func (f *qemuGitea) output(task, name string) []byte {
	// Access the immutable output using the same World storage as its execution.
	value, err := forge_task.ResolveOutput(f.ctx, f.tb.WorldState, task, name)
	if err != nil || value == nil {
		f.t.Fatalf("output %s: %v", name, err)
	}
	handle := forge_target.ExecControllerHandleWithAccess("gitea-proof", f.sender, f.tb.Engine, f.tb.WorldState.AccessWorldState, nil)
	data, err := forge_target.LoadBlobValueToBytes(f.ctx, handle, value)
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

// requireGuestBindings compares the guest's actual Volume/object identities.
func (f *qemuGitea) requireGuestBindings(original, replacement string) {
	// Require exactly the two durable bindings reported by the static init.
	count := 0
	for line := range strings.SplitSeq(original, "\n") {
		if strings.HasPrefix(line, "SPACEWAVE_OS_BINDING role=") {
			count++
			if !strings.Contains(replacement, line) {
				f.t.Fatalf("binding changed: %s", line)
			}
		}
	}
	if count != 2 {
		f.t.Fatalf("reported %d persistent bindings, want 2", count)
	}
}

// configDigest reads only a hash, keeping generated application keys out of receipts.
func (f *qemuGitea) configDigest(log, marker string) string {
	// BusyBox's sha256sum line follows the marker; a shell prompt may precede it.
	for _, tail := range strings.Split(log, marker)[1:] {
		fields := strings.Fields(tail)
		for i := 1; i < len(fields); i++ {
			if fields[i] == "/data/config/app.ini" && len(fields[i-1]) == 64 {
				return fields[i-1]
			}
		}
	}
	f.t.Fatalf("configuration digest missing after %s", marker)
	return ""
}

// createGraph composes one VM Step and an explicitly requested repository build.
func (f *qemuGitea) createGraph(startup string) {
	// Mount only explicit application inputs; build receives no application storage.
	inputs := filepath.Join(filepath.Dir(f.dir), "inputs")
	vm := f.dockerConfig()
	vm.MemoryBytes = 2 << 30
	vm.Devices = []string{"/dev/kvm:/dev/kvm:rw"}
	vm.Command = []string{"python3", "/run_app.py", "--kernel", "/kernel", "--initramfs", "/initramfs", "--disk", "/system.img", "--config-disk", "/configuration.img", "--startup", "/startup.sh", "--port", strconv.Itoa(f.port), "--receipt", "/outputs/stopped"}
	for _, m := range [][3]string{{inputs + "/kernel-root/boot/vmlinuz-6.12.116+deb13-amd64", "/kernel", "ro"}, {inputs + "/app-initramfs.cpio.gz", "/initramfs", "ro"}, {filepath.Join(f.dir, "system.img"), "/system.img", ""}, {filepath.Join(f.dir, "configuration.img"), "/configuration.img", ""}, {filepath.Join(inputs, "run_app.py"), "/run_app.py", "ro"}, {startup, "/startup.sh", "ro"}} {
		vm.Mounts = append(vm.Mounts, &forge_lib_docker.Mount{HostPath: m[0], ContainerPath: m[1], ReadOnly: m[2] == "ro"})
	}
	vm.Outputs = []string{"stopped"}
	build := f.dockerConfig()
	build.Outputs = []string{"artifact"}
	build.Command = []string{"sh", "-ec", "git clone " + f.base + "/operator/proof.git /work; cd /work; git rev-parse HEAD; make; ./proof; cp proof /outputs/artifact"}
	vmNode := f.step(vm, "stopped", false)
	vmNode.Parameters = map[string]string{"sqlObject": "sql/gitea", "filesDisk": filepath.Join(f.dir, "system.img"), "configurationDisk": filepath.Join(f.dir, "configuration.img")}
	vmNode.Step.Target.Inputs = append(vmNode.Step.Target.Inputs, &forge_target.Input{Name: "sql", InputType: forge_target.InputType_InputType_WORLD_OBJECT, WorldObject: &forge_target.InputWorldObject{ObjectKey: "sql/gitea"}})

	// Store one authored graph whose second run uses the identical Target bindings.
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		if _, err := (&s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: "flowgraph/gitea", Name: "Spacewave OS Gitea proof"}).ApplyWorldOp(ctx, nil, ws, f.sender); err != nil {
			return err
		}
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, ws, "flowgraph/gitea", &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
			"vm": vmNode, "build": f.step(build, "artifact", true), "build-request": {TypeId: s4wave_flowgraph.EventSourceNodeTypeID, Ports: []*s4wave_flowgraph.FlowgraphPort{{Name: "commit", Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "activation"}}},
		}, SetConnections: map[string]*s4wave_flowgraph.FlowgraphConnection{"request-build": {OutputNode: "build-request", OutputPort: "commit", InputNode: "build", InputPort: "commit"}}})
		return err
	})
}

// dockerConfig supplies the real CLI, private daemon and finite capacity request.
func (f *qemuGitea) dockerConfig() *forge_lib_docker.Config {
	return &forge_lib_docker.Config{DockerPath: f.script, DockerEnv: map[string]string{"DOCKER_HOST": os.Getenv("DOCKER_HOST"), "SPACEWAVE_REAL_DOCKER": os.Getenv("SPACEWAVE_DOCKER"), "SPACEWAVE_STOP_BLOCK": filepath.Join(f.dir, "stop-block")}, Image: f.image, NetworkMode: "host", MilliCpu: 1000, MemoryBytes: 512 << 20, StopTimeoutSeconds: 45, OutputDir: "/outputs"}
}

// step resolves the ordinary Docker Target and declares its graph ports.
func (f *qemuGitea) step(conf *forge_lib_docker.Config, output string, input bool) *s4wave_flowgraph.FlowgraphNode {
	// Encode the typed controller settings through the existing Target resolver.
	config, err := conf.MarshalJSON()
	raw := []byte(`{"outputs":[{"name":` + strconv.Quote(output) + `,"outputType":"OutputType_EXEC","execOutput":` + strconv.Quote(output) + `}],"exec":{"controller":{"id":"forge/lib/docker","config":` + string(config) + `}}}`)
	if err != nil {
		f.t.Fatal(err)
	}
	target, err := target_json.ResolveYAML(f.ctx, f.tb.Bus, raw)
	if err != nil {
		f.t.Fatal(err)
	}
	node := &s4wave_flowgraph.FlowgraphNode{TypeId: s4wave_flowgraph.StepNodeTypeID, Step: &s4wave_flowgraph.FlowgraphStep{Target: target}, Ports: []*s4wave_flowgraph.FlowgraphPort{{Name: output, Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "activation"}}}
	if input {
		node.Ports = append(node.Ports, &s4wave_flowgraph.FlowgraphPort{Name: "commit", Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT, TypeId: "activation"})
	}
	return node
}

// giteaGuestStartup installs only absent configuration in the retained UnixFS object.
func giteaGuestStartup(sqlPort int) string {
	return fmt.Sprintf(`ip link set eth0 up
ip addr add 10.0.2.15/24 dev eth0
ip route add default via 10.0.2.2
mkdir -p /data/config/home /data/config/work /data/repos
chmod 777 /data/config /data/config/home /data/config/work /data/repos
export HOME=/data/config/home GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'
if [ ! -f /data/config/app.ini ]; then
cat > /data/config/app.ini <<'INI'
APP_NAME = Spacewave OS proof
RUN_USER = git
RUN_MODE = prod
[database]
DB_TYPE = mysql
HOST = 10.0.2.2:%d
NAME = gitea
USER = root
SSL_MODE = disable
CHARSET = utf8mb4
[security]
INSTALL_LOCK = true
[repository]
DEFAULT_BRANCH = main
ROOT = /data/repos
[server]
HTTP_ADDR = 0.0.0.0
HTTP_PORT = 3000
ROOT_URL = http://localhost:3000/
OFFLINE_MODE = true
DISABLE_SSH = true
APP_DATA_PATH = /data/config/work/data
[indexer]
ISSUE_INDEXER_TYPE = db
[actions]
ENABLED = false
[log]
MODE = console
LEVEL = warn
INI
chmod 666 /data/config/app.ini
fi
if [ ! -f /data/config/initialized ]; then
su -p -s /bin/sh git -c '/bin/gitea --work-path /data/config/work --config /data/config/app.ini migrate' && su -p -s /bin/sh git -c '/bin/gitea --work-path /data/config/work --config /data/config/app.ini admin user create --username operator --password Spacewave-Fixture-2026! --email fixture@example.invalid --admin --must-change-password=false' && touch /data/config/initialized
fi
echo SPACEWAVE_OS_CONFIG_OPEN
sha256sum /data/config/app.ini
su -p -s /bin/sh git -c 'exec /bin/gitea --work-path /data/config/work --config /data/config/app.ini web' &
GITEA_JOB=$!
echo SPACEWAVE_OS_APP_STARTED pid=$GITEA_JOB
`, sqlPort)
}
