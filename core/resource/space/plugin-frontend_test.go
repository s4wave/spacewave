package resource_space

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/core/sobject"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// TestPluginFrontendResource closes queued work and pending calls with its Resource.
func TestPluginFrontendResource(t *testing.T) {
	// Mount a real local SharedObject; no device compiler has started yet.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	tb, existing, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	// Mount the build source SharedObject for the frontend attachment.
	request := setupPluginBuildSource(t, tb, tb.Volume.GetPeerID())
	ref := existing.space.GetSharedObjectRef()
	shared, mount, err := sobject.ExMountSharedObject(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mount.Release()

	// Open the mounted Space compiler attachment under its Session authority.
	body := space_sobject.NewSpaceBody(ref, tb.EngineID, tb.EngineBucketID, tb.EngineVolumeID, shared, tb.BusEngine)
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, shared.GetPeerID().String())
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	opened, err := client.OpenPluginFrontend(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resources.ReleaseResource(opened.GetResourceId()) })

	// The accepted job retains the exact author and the route supplied to Vite.
	execution, object, err := world.LookupObject[*forge_execution.Execution](ctx, tb.WorldState,
		opened.GetExecutionKey(), forge_execution.NewExecutionBlock)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the accepted execution target into the compiler grant.
	var config space_exec.PluginBuildConfig
	_, err = world.AccessObject(ctx, tb.WorldState.AccessWorldState, nil, func(cursor *block.Cursor) error {
		// Read the compiler configuration from the accepted execution target.
		cursor = cursor.Detach(true)
		cursor.ClearAllRefs()
		cursor.SetRefAtCursor(execution.GetTargetRef(), true)
		target, err := forge_target.UnmarshalTarget(ctx, cursor)
		if err != nil {
			return err
		}
		return config.UnmarshalJSON(target.GetExec().GetController().GetConfig())
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.GetFrontendPeerId() != shared.GetPeerID().String() {
		t.Fatal("compiler grant lost the mounted author's identity")
	}

	// Resolve the compiler grant's browser module route.
	serviceID, err := frontend.RouteService(config.GetFrontendRoutePrefix() + "session/Viewer.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(serviceID, frontend.AttachedServicePrefix) {
		t.Fatalf("unexpected frontend route: %s", serviceID)
	}
	removed := make(chan struct{}, 1)
	services, _, route, err := bifrost_rpc.ExLookupRpcService(ctx, tb.Bus, serviceID, "", false, func() {
		select {
		case removed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("frontend route resolved %d services", len(services))
	}
	defer route.Release()

	// Both the attached Resource and its browser module route may be dialing.
	// Start calls through the compiler Resource and its browser route.
	attached := frontend.NewSRPCFrontendClient(resources.client(t, opened.GetResourceId()))
	watch, err := attached.Watch(ctx, &frontend.WatchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	routed := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(services[0])))
	fetch, err := routed.NewStream(ctx, serviceID, "Fetch", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fetch.Close()

	// Release the compiler attachment and verify both calls and its route end.
	if !resources.ReleaseResource(opened.GetResourceId()) {
		t.Fatal("attachment was not retained")
	}
	if _, err := watch.Recv(); err == nil {
		t.Fatal("released attachment kept its Watch open")
	}
	if err := fetch.MsgRecv(&srpc.RawMessage{}); err == nil {
		t.Fatal("released attachment kept its module fetch open")
	}
	select {
	case <-removed:
	case <-ctx.Done():
		t.Fatal("attachment release retained its module route")
	}

	// Resource release cancels a job even before a device claims it.
	execution, object, err = world.LookupObject[*forge_execution.Execution](ctx, tb.WorldState,
		opened.GetExecutionKey(), forge_execution.NewExecutionBlock)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	if !execution.GetResult().GetCanceled() {
		t.Fatal("resource release left its queued compiler running")
	}
}
