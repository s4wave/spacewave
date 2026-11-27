//go:build !js

package resource_world_test

import (
	"context"
	"slices"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// TestWorldStateResourceTypeTraits checks that the type count listing marks
// internal types and that typed listings return the titles, states and times
// a type declares.
func TestWorldStateResourceTypeTraits(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	t.Cleanup(cleanup)

	// Serve the four host types. The plugin type stays unknown.
	const pluginTypeID = "test/plugin"
	rel, err := tb.Bus.AddController(ctx, blocktype_controller.NewController(func(_ context.Context, typeID string) (blocktype.BlockType, error) {
		switch typeID {
		case space_world.SpaceSettingsTypeID:
			return blocktype.New[space_world.SpaceSettings](typeID), nil
		case forge_worker.WorkerTypeID:
			return blocktype.New[forge_worker.Worker](typeID), nil
		case forge_job.JobTypeID:
			return blocktype.New[forge_job.Job](typeID), nil
		case s4wave_device.DeviceTypeID:
			return blocktype.New[s4wave_device.Device](typeID), nil
		}
		return nil, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)

	// Create one settings object, two named workers and a plugin object.
	create := func(key, typeID string, blk block.Block) {
		obj, _, err := world.CreateWorldObject(ctx, tb.WorldState, key, func(bcs *block.Cursor) error {
			bcs.SetBlock(blk, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if err := world_types.SetObjectType(ctx, tb.WorldState, key, typeID); err != nil {
			t.Fatal(err)
		}
	}
	create("settings", space_world.SpaceSettingsTypeID, &space_world.SpaceSettings{})
	create("worker/a", forge_worker.WorkerTypeID, &forge_worker.Worker{Name: "Alpha"})
	create("worker/b", forge_worker.WorkerTypeID, &forge_worker.Worker{})
	create("devices/thumper", s4wave_device.DeviceTypeID, &s4wave_device.Device{Label: "thumper"})
	create("devices/new", s4wave_device.DeviceTypeID, &s4wave_device.Device{})
	create("plugin/a", pluginTypeID, &space_world.SpaceSettings{})
	made := timestamppb.New(time.Unix(1_700_000_000, 0))
	create("job/a", forge_job.JobTypeID, &forge_job.Job{JobState: forge_job.State_JobState_RUNNING, Timestamp: made})
	create("job/b", forge_job.JobTypeID, &forge_job.Job{JobState: forge_job.State_JobState_COMPLETE, Result: &forge_value.Result{Success: true}})

	resource := resource_world.NewMount(nil, tb.Bus, nil).NewWorldStateResource(tb.WorldState)
	client := s4wave_world.NewSRPCWorldStateResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(resource.GetMux()))))

	// Only the settings type is internal.
	counts, err := client.ListTypeCounts(ctx, &s4wave_world.ListTypeCountsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	internal := make(map[string]bool)
	for _, e := range counts.GetTypeCounts() {
		internal[e.GetTypeId()] = e.GetInternal()
	}
	if !internal[space_world.SpaceSettingsTypeID] || internal[forge_worker.WorkerTypeID] || internal[pluginTypeID] {
		t.Fatalf("internal flags %v", internal)
	}

	// Workers carry their names, but an unnamed worker shows its key.
	list := func(typeID string, declared bool) *s4wave_world.ListObjectsWithTypeResponse {
		resp, err := client.ListObjectsWithType(ctx, &s4wave_world.ListObjectsWithTypeRequest{TypeId: typeID, Limit: 10, Declared: declared})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if got := list(forge_worker.WorkerTypeID, true).GetTitles(); !slices.Equal(got, []string{"Alpha", ""}) {
		t.Fatalf("worker titles %q", got)
	}
	if got := list(forge_worker.WorkerTypeID, false).GetTitles(); got != nil {
		t.Fatalf("titles without asking: %q", got)
	}
	if got := list(s4wave_device.DeviceTypeID, true).GetTitles(); !slices.Equal(got, []string{"thumper", ""}) {
		t.Fatalf("device titles %q", got)
	}
	if got := list(pluginTypeID, true).GetTitles(); got != nil {
		t.Fatalf("plugin titles %q", got)
	}

	// Jobs declare states and times but no titles. Workers declare no
	// states.
	if got := list(forge_job.JobTypeID, true); got.GetTitles() != nil || !slices.Equal(got.GetStates(), []string{"running", "passed"}) {
		t.Fatalf("job titles %q states %q", got.GetTitles(), got.GetStates())
	}
	if got := list(forge_worker.WorkerTypeID, true).GetStates(); got != nil {
		t.Fatalf("worker states %q", got)
	}
	md, err := client.GetObjectMetadataBatch(ctx, &s4wave_world.GetObjectMetadataBatchRequest{ObjectKeys: []string{"job/a", "worker/a"}, Declared: true})
	if err != nil {
		t.Fatal(err)
	}
	job, worker := md.GetMetadata()[0], md.GetMetadata()[1]
	if job.GetDeclaration().GetState() != "running" || !job.GetDeclaration().GetTime().AsTime().Equal(made.AsTime()) || worker.GetDeclaration().GetTitle() != "Alpha" || worker.GetDeclaration().GetState() != "" || worker.GetDeclaration().GetTime() != nil {
		t.Fatalf("metadata %v", md.GetMetadata())
	}
}
