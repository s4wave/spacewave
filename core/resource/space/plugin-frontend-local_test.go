//go:build !js && !tinygo

package resource_space

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	frontend "github.com/s4wave/spacewave/bldr/frontend"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_fetch "github.com/s4wave/spacewave/bldr/web/fetch"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/core/sobject"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// TestLocalPluginFrontend joins the mounted Space Resource, granted worker,
// accepted source edits, and real Vite compiler without a self network link.
func TestLocalPluginFrontend(t *testing.T) {
	// Mount the disposable Session's Space and prepare an ordinary source project.
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	tb, existing, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()
	ref := existing.space.GetSharedObjectRef()
	shared, mount, err := sobject.ExMountSharedObject(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mount.Release()
	request := setupPluginBuildSource(t, tb, shared.GetPeerID())
	body := space_sobject.NewSpaceBody(ref, tb.EngineID, tb.EngineBucketID, tb.EngineVolumeID, shared, tb.BusEngine)
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, shared.GetPeerID().String())
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)
	object, err := world.MustGetObject(ctx, tb.WorldState, request.GetSourceKey())
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(object)
	const source = `export const label = "colors"`
	for name, contents := range map[string]string{
		"bldr.yaml": `{"id":"space-colors","manifests":{"space-colors":{"builder":{"id":"bldr/plugin/compiler/js","config":{"modules":[{"kind":"JS_MODULE_KIND_FRONTEND","path":"./Viewer.ts"}]}}}}}`,
		"Viewer.ts": source,
	} {
		_, _, err := unixfs_world.FsMknodWithContent(ctx, object, shared.GetPeerID(), unixfs_world.FSType_FSType_FS_NODE,
			[]string{name}, unixfs.NewFSCursorNodeType_File(), int64(len(contents)), strings.NewReader(contents), 0o644, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}

	// The worker and execution controller own scheduling, compilation, and shutdown.
	publicPeer, err := peer.NewPeerWithID(shared.GetPeerID())
	if err != nil {
		t.Fatal(err)
	}
	releasePeer, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, publicPeer), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePeer()
	tb.StaticResolver.AddFactory(worker_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(execution_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))
	for _, factory := range space_exec.BridgeFactories(space_exec.NewDefaultRegistryWithBus(tb.Bus)) {
		tb.StaticResolver.AddFactory(factory)
	}
	_, worker, err := worker_controller.StartControllerWithConfig(ctx, tb.Bus,
		worker_controller.NewConfig(tb.EngineID, "workers/build", shared.GetPeerID(), true))
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Release()
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	opened, err := client.OpenPluginFrontend(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer resources.ReleaseResource(opened.GetResourceId())
	attached := frontend.NewSRPCFrontendClient(resources.client(t, opened.GetResourceId()))
	watch, err := attached.Watch(ctx, &frontend.WatchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	snapshot, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}

	// Module fetches use the same attachment that delivers accepted source edits.
	fetch := func() string {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "http://space.test"+snapshot.GetSession().GetRoutePrefix()+"Viewer.ts", nil).WithContext(ctx)
		if err := web_fetch.Fetch(ctx, func(ctx context.Context) (web_fetch.SRPCFetchService_FetchClient, error) {
			return attached.Fetch(ctx)
		}, request, response); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("module status %d: %s", response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	if !strings.Contains(fetch(), "colors") {
		t.Fatal("initial compiler source is missing")
	}
	_, _, err = unixfs_world.FsWriteAt(ctx, object, shared.GetPeerID(), unixfs_world.FSType_FSType_FS_NODE,
		[]string{"Viewer.ts"}, 0, []byte(strings.ReplaceAll(source, "colors", "colours")), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := watch.Recv(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fetch(), "colours") {
		t.Fatal("compiler did not receive the accepted World edit")
	}

	// Closing the owning Resource cancels its durable execution and pending Watch.
	resources.ReleaseResource(opened.GetResourceId())
	if _, err := watch.Recv(); err == nil {
		t.Fatal("released Resource retained its Watch")
	}
	execution, err := forge_execution.WaitExecutionComplete(ctx, tb.Logger, tb.WorldState, opened.GetExecutionKey())
	if err != nil {
		t.Fatal(err)
	}
	if !execution.GetResult().GetCanceled() {
		t.Fatal("released Resource left its execution running")
	}
}
