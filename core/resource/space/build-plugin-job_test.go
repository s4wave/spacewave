//go:build !js && !tinygo

package resource_space

import (
	"context"
	"strings"
	"testing"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	forge_core "github.com/s4wave/spacewave/forge/core"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// TestSpacePluginBuildJob runs the Job created by the public RPC on a Worker
// and checks that the retained result names the built platform and exact source.
func TestSpacePluginBuildJob(t *testing.T) {
	// Start a testbed with a build device.
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	peerID := tb.Volume.GetPeerID()
	request := setupPluginBuildSource(t, tb, peerID)

	// Pin a JavaScript project as the Space source.
	request.PlatformId = "js"
	writeColorsSource(t, tb, peerID, request.GetSourceKey())
	root, _, err := world.LookupRootRef(ctx, tb.Engine, request.GetSourceKey())
	if err != nil {
		t.Fatal(err)
	}

	// Submit through the public RPC, then let the Worker controller run the Job.
	response := submitPluginBuild(t, tb, peerID, request)
	startPluginBuildWorker(t, tb, peerID, "workers/build")
	task := waitPluginBuildTask(t, tb, response)

	// The Task retains the exact source and the builder result.
	source := taskOutput(t, task, "source").GetWorldObjectSnapshot()
	if !source.GetRootRef().EqualVT(root) {
		t.Fatal("task did not retain the exact source")
	}
	if taskOutput(t, task, "build-result").GetBucketRef() == nil {
		t.Fatal("task omitted its builder result")
	}

	// The builder result names the built platform and the exact source.
	artifact := taskOutput(t, task, "manifest").GetWorldObjectSnapshot().GetRootRef()
	provenance, _, err := resultworld.LookupManifestBuildResult(ctx, tb.WorldState, bldr_manifest.NewManifestArtifactKey(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if provenance.GetManifest().GetMeta().GetPlatformId() != "js" || !provenance.GetSourceRef().GetRootRef().EqualVT(root.GetRootRef()) {
		t.Fatalf("build result lost its platform or source: %v", provenance)
	}
}

// submitPluginBuild queues a build through the public RPC of a Space Resource
// mounted with the given Session peer.
func submitPluginBuild(t *testing.T, tb *testbed.Testbed, session peer.ID, request *s4wave_space.BuildSpacePluginRequest) *s4wave_space.BuildSpacePluginResponse {
	// Mount the Space Resource as the submitting Session.
	t.Helper()
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, session.String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))

	// Queue the build.
	response, err := client.BuildSpacePlugin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// writeColorsSource fills the source directory with a small JavaScript plugin project.
func writeColorsSource(t *testing.T, tb *testbed.Testbed, sender peer.ID, sourceKey string) {
	// Open the source directory for writing.
	t.Helper()
	ctx := t.Context()
	object, err := world.MustGetObject(ctx, tb.WorldState, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(object)

	// Write the project configuration and its one frontend module.
	for name, contents := range map[string]string{
		"bldr.yaml": `{"id":"space-colors","manifests":{"space-colors":{"builder":{"id":"bldr/plugin/compiler/js","config":{"viteDisableProjectConfig":true,"modules":[{"kind":"JS_MODULE_KIND_FRONTEND","path":"./Viewer.ts"}]}}}}}`,
		"Viewer.ts": `export const label = "colors"`,
	} {
		_, _, err := unixfs_world.FsMknodWithContent(ctx, object, sender, unixfs_world.FSType_FSType_FS_NODE,
			[]string{name}, unixfs.NewFSCursorNodeType_File(), int64(len(contents)), strings.NewReader(contents), 0o644, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
}

// startPluginBuildWorker runs the Forge controllers that own a Worker's Jobs
// until the test ends.
func startPluginBuildWorker(t *testing.T, tb *testbed.Testbed, workerPeer peer.ID, workerKey string) {
	// Serve the Worker's peer on the bus.
	t.Helper()
	ctx := t.Context()
	publicPeer, err := peer.NewPeerWithID(workerPeer)
	if err != nil {
		t.Fatal(err)
	}
	releasePeer, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, publicPeer), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releasePeer)

	// Resolve Forge ops through the World engine.
	opc := world.NewLookupOpController("forge-ops", tb.EngineID, forge_world.LookupWorldOp)
	releaseOps, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOps)

	// Register the Forge factories and the plugin build handler.
	forge_core.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))
	for _, factory := range space_exec.BridgeFactories(space_exec.NewDefaultRegistryWithBus(tb.Bus)) {
		tb.StaticResolver.AddFactory(factory)
	}

	// Start the Worker controller that runs the Job's Tasks.
	_, worker, err := worker_controller.StartControllerWithConfig(ctx, tb.Bus,
		worker_controller.NewConfig(tb.EngineID, workerKey, workerPeer, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(worker.Release)
}

// waitPluginBuildTask waits for the Job to complete and returns its successful Task.
func waitPluginBuildTask(t *testing.T, tb *testbed.Testbed, response *s4wave_space.BuildSpacePluginResponse) *forge_task.Task {
	// Wait for the Job to report success.
	t.Helper()
	ctx := t.Context()
	state, err := forge_job.WaitJobComplete(ctx, tb.Logger, tb.WorldState, response.GetJobKey())
	if err != nil {
		t.Fatal(err)
	}
	if state.GetResult().GetFailError() != "" {
		t.Fatalf("build job failed: %s", state.GetResult().GetFailError())
	}

	// Read the Task that holds the retained outputs.
	task, err := forge_task.LookupTaskBody(ctx, tb.WorldState, response.GetTaskKey())
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// taskOutput returns the named output of a completed Task.
func taskOutput(t *testing.T, task *forge_task.Task, name string) *forge_value.Value {
	t.Helper()
	for _, output := range task.GetValueSet().GetOutputs() {
		if output.GetName() == name {
			return output
		}
	}
	t.Fatalf("task omitted its %q output", name)
	return nil
}
