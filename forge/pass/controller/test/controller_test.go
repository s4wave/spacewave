package pass_controller_testing

import (
	"bytes"
	"context"
	"testing"
	"time"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/sirupsen/logrus"

	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// TestPassController verifies a pass can run to completion in a testbed.
func TestPassController(t *testing.T) {
	runMockPass(t)
}

// TestBodiesMatchUnmarshal checks that the body GetObjectBodiesBatch returns
// for each Forge type and the V86 image decodes to the same message as the
// block cursor's Unmarshal with the object's block type, so a listing may
// decode bodies from one batch instead of one cursor per object.
func TestBodiesMatchUnmarshal(t *testing.T) {
	tb := runMockPass(t)
	ctx, ws := tb.Context, tb.WorldState
	peerID := tb.Volume.GetPeerID()

	// Create the Forge objects the pass did not, and a V86 image.
	if _, _, err := forge_cluster.CreateCluster(ctx, ws, "cluster/1", "cluster-1", peerID, peerID); err != nil {
		t.Fatal(err)
	}
	job, _, err := forge_job.CreateJobWithTasks(ctx, ws, peerID, "job/1", map[string]*forge_target.Target{
		"task-a": {Exec: &forge_target.Exec{Disable: true}},
	}, peerID, nil, timestamp.Now())
	world.ReleaseObjectState(job)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_worker.CreateWorker(ctx, ws, "worker/1", "worker-1", nil, peerID); err != nil {
		t.Fatal(err)
	}
	img := &s4wave_vm.V86Image{Name: "image", Version: "1", Platform: "v86", Distro: "alpine", Tags: []string{"a", "b"}}
	if _, err := s4wave_vm.NewCreateV86ImageOp("v86/1", img, time.Now()).ApplyWorldOp(ctx, logrus.NewEntry(logrus.New()), ws, peerID); err != nil {
		t.Fatal(err)
	}

	keys := []string{
		"pass/1",
		forge_pass.BuildPassExecutionObjKey("pass/1", peerID.String()),
		"cluster/1",
		"job/1",
		forge_job.NewJobTaskKey("job/1", "task-a"),
		"worker/1",
		"v86/1",
	}
	for _, key := range keys {
		typeID, err := world_types.GetObjectType(ctx, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		body, unmarshaled := readBodyAndUnmarshal(t, ws, key, typeID)
		if body == nil {
			t.Logf("%s (%s): no registered block type, so the cursor read fails today", key, typeID)
			continue
		}
		if !bytes.Equal(body, unmarshaled) {
			t.Fatalf("%s (%s): body %x, unmarshal %x", key, typeID, body, unmarshaled)
		}
		t.Logf("%s (%s): body equals unmarshal, %d bytes", key, typeID, len(body))
	}
}

// readBodyAndUnmarshal returns the body from the World's required page method
// and the bytes the block cursor resource returns for Unmarshal with
// typeID: the block decoded by the type's constructor and marshaled again.
// It returns nil, nil when typeID has no registered block type.
func readBodyAndUnmarshal(t *testing.T, ws world.WorldState, key, typeID string) ([]byte, []byte) {
	// Read the object's body through the same page method as the batch reader.
	t.Helper()
	ctx := t.Context()
	bodies, _, _, err := ws.GetObjectBodiesBatchPage(ctx, []string{key}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 1 || !bodies[0].Exists {
		t.Fatalf("%s: bodies = %v", key, bodies)
	}

	// Open the comparison object only when the cursor can decode its type.
	bt, err := space_world_optypes.LookupBlockType(ctx, typeID)
	if err != nil || bt == nil {
		return nil, nil
	}
	obj, found, err := ws.GetObject(ctx, key)
	if err != nil || !found {
		t.Fatalf("%s: found %v, %v", key, found, err)
	}
	defer world.ReleaseObjectState(obj)

	// Decode the root block with the same constructor as the cursor resource.
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled []byte
	_, err = world.AccessObject(ctx, obj.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		blk, err := bcs.Unmarshal(ctx, bt.Constructor)
		if err != nil {
			return err
		}
		unmarshaled, err = blk.MarshalVT()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return bodies[0].Body, unmarshaled
}

// runMockPass runs the mock target to completion in a new testbed.
func runMockPass(t *testing.T) *testbed.Testbed {
	// Prepare the context and testbed for the pass execution.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the factories required by the target.
	b, sr := tb.Bus, tb.StaticResolver
	sr.AddFactory(boilerplate_controller.NewFactory(b))
	sr.AddFactory(forge_lib_kvtx.NewFactory(b))

	// End to end test of building a target and running in a testbed.
	tgt, err := target_mock.ResolveMockTarget(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Run one pass and retain its final state.
	ts := timestamp.Now()
	outState, err := tb.RunPassWithTarget(tgt, nil, 1, ts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outState.GetPassState() != forge_pass.State_PassState_COMPLETE {
		t.Fatalf("expected pass state COMPLETE but got %s", outState.GetPassState().String())
	}
	return tb
}
