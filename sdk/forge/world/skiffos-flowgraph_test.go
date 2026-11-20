//go:build linux && !tinygo

package s4wave_forge_world

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	forge_lib_skiffos "github.com/s4wave/spacewave/forge/lib/skiffos"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
)

// skiffosGraph is the build Flowgraph's object key.
const skiffosGraph = "flowgraph/skiffos"

// TestSkiffOSFlowgraph builds a SkiffOS workspace from a Flowgraph Step, lands
// the boot set in the World as a UnixFS tree, and reruns the graph with equal
// inputs, which completes from the first Task's outputs without a build.
//
// SPACEWAVE_SKIFFOS_DIR names the SkiffOS checkout; the Step owns the
// workspace SPACEWAVE_SKIFFOS_WORKSPACE (default spacewave-forge) in it and
// touches no other. SPACEWAVE_SKIFFOS_WORLD optionally keeps the World.
func TestSkiffOSFlowgraph(t *testing.T) {
	// Require an explicit checkout: the build takes tens of minutes.
	skiffDir := os.Getenv("SPACEWAVE_SKIFFOS_DIR")
	if skiffDir == "" {
		t.Skip("set SPACEWAVE_SKIFFOS_DIR for the real SkiffOS build")
	}
	workspace := os.Getenv("SPACEWAVE_SKIFFOS_WORKSPACE")
	if workspace == "" {
		workspace = "spacewave-forge"
	}
	worldPath := os.Getenv("SPACEWAVE_SKIFFOS_WORLD")
	if worldPath == "" {
		worldPath = filepath.Join(t.TempDir(), "world.s4wave")
	}
	h := newWorkerHarnessWithOptions(t, workerHarnessOptions{
		timeout: 4 * time.Hour, memoryBytes: 2 << 30,
		storage: []db_testbed.Option{db_testbed.WithVolumeConfig(&volume_s4db.Config{Path: worldPath})},
	})
	f := &flowgraphFixture{workerHarness: h, t: t}
	h.tb.StaticResolver.AddFactory(flowgraph_run.NewFactory(h.tb.Bus))
	h.tb.StaticResolver.AddFactory(forge_lib_skiffos.NewFactory())

	// Run the Worker that serves the runs; the Step runs where it is placed.
	updates := make(chan *s4wave_device.ForgeWorkerDeclaration, 1)
	stopWorker := h.startWorker(h.newAdmission("skiffos-proof"), updates)
	defer func() {
		if err := stopWorker(); !errors.Is(err, context.Canceled) {
			t.Errorf("Worker stop: %v", err)
		}
	}()
	f.createSkiffOSGraph(&forge_lib_skiffos.Config{
		SkiffDir: skiffDir, Workspace: workspace, Configs: []string{"intel/x64", "virt/qemu"},
	})

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

// createSkiffOSGraph stores a graph of one SkiffOS build Step.
func (f *flowgraphFixture) createSkiffOSGraph(conf *forge_lib_skiffos.Config) {
	// Resolve the Target through the ordinary JSON resolver.
	config, err := conf.MarshalJSON()
	if err != nil {
		f.t.Fatal(err)
	}
	out := strconv.Quote(forge_lib_skiffos.OutputImages)
	raw := []byte(`{"outputs":[{"name":` + out + `,"outputType":"OutputType_EXEC","execOutput":` + out + `}],"exec":{"controller":{"id":` + strconv.Quote(forge_lib_skiffos.ControllerID) + `,"config":` + string(config) + `},"reuseOutputs":true}}`)
	target, err := target_json.ResolveYAML(f.ctx, f.tb.Bus, raw)
	if err != nil {
		f.t.Fatal(err)
	}
	if !target.GetExec().GetReuseOutputs() {
		f.t.Fatal("target lost reuseOutputs")
	}
	build := &s4wave_flowgraph.FlowgraphNode{
		TypeId: s4wave_flowgraph.StepNodeTypeID,
		Step:   &s4wave_flowgraph.FlowgraphStep{Target: target},
		Ports: []*s4wave_flowgraph.FlowgraphPort{{
			Name: forge_lib_skiffos.OutputImages, Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "activation",
		}},
	}
	f.transact(func(ctx context.Context, ws world.WorldState) error {
		if _, err := (&s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: skiffosGraph, Name: "SkiffOS QEMU build"}).ApplyWorldOp(ctx, nil, ws, f.sender); err != nil {
			return err
		}
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, ws, skiffosGraph, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"build": build},
		})
		return err
	})
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
