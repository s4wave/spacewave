package space_exec

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_controller "github.com/s4wave/spacewave/forge/execution/controller"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// TestPluginExecWaitingStatus observes the same durable Execution while its
// plugin client is unavailable and after the client becomes available.
func TestPluginExecWaitingStatus(t *testing.T) {
	// Gate the plugin load and execution stream independently.
	const pluginID = "example-plugin"
	available := make(chan struct{})
	waitingPosted := make(chan struct{})
	responses := make(chan *PluginExecResponse)
	streamStarted := make(chan struct{})
	client := &pluginExecClientStub{stream: &pluginExecStreamStub{ch: responses}, streamStarted: streamStarted}

	// Register a plugin loader that records its wait before the client is available.
	registry := NewRegistry()
	registry.Register(PluginExecConfigID, newPluginExecHandler(nil, func(ctx context.Context, b bus.Bus, id string, onWaiting func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
		// Record the requested plugin's durable wait status.
		if id != pluginID {
			return nil, nil, errors.Errorf("unexpected plugin: %s", id)
		}
		if err := onWaiting(); err != nil {
			return nil, nil, err
		}
		close(waitingPosted)

		// Retain plugin demand until the client arrives or execution is canceled.
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-available:
			return client, nil, nil
		}
	}))

	// Start the native Execution controller with the gated plugin handler.
	tb, peerID := setupIntegrationTest(t, registry)

	// Build the plugin controller config for this execution.
	conf := &PluginExecConfig{PluginId: pluginID, ControllerId: "example-controller"}
	configData, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	const execKey = "exec/waiting-plugin"
	createTestExecution(t, tb.Context, tb.WorldState, peerID, execKey, PluginExecConfigID, configData)

	// Retain the native controller while the plugin remains unavailable.
	controllerConf := execution_controller.NewConfig(tb.EngineID, execKey, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
	_, ctrlRef, err := execution_controller.StartControllerWithConfig(t.Context(), tb.Bus, controllerConf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrlRef.Release)

	// Inspect the authoritative Execution after its plugin wait is posted.
	<-waitingPosted
	waiting, stateRef, err := forge_execution.LookupExecution(t.Context(), tb.WorldState, execKey)
	if err != nil {
		world.ReleaseObjectState(stateRef)
		t.Fatal(err)
	}
	world.ReleaseObjectState(stateRef)
	if got := waiting.GetWaitingPluginId(); got != pluginID {
		t.Fatalf("waiting plugin = %q, want %q", got, pluginID)
	}
	if waiting.GetExecutionState() != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("waiting execution state = %v", waiting.GetExecutionState())
	}

	// Release plugin demand and inspect the cleared wait status.
	close(available)
	<-streamStarted
	proceeding, proceedingRef, err := forge_execution.LookupExecution(t.Context(), tb.WorldState, execKey)
	if err != nil {
		world.ReleaseObjectState(proceedingRef)
		t.Fatal(err)
	}
	world.ReleaseObjectState(proceedingRef)
	if got := proceeding.GetWaitingPluginId(); got != "" {
		t.Fatalf("proceeding waiting plugin = %q", got)
	}

	// Complete the plugin stream and verify the durable Execution result.
	responses <- &PluginExecResponse{}
	close(responses)
	completed, err := forge_execution.WaitExecutionComplete(t.Context(), tb.Logger, tb.WorldState, execKey)
	if err != nil {
		t.Fatal(err)
	}
	assertComplete(t, completed)
	if got := completed.GetWaitingPluginId(); got != "" {
		t.Fatalf("completed waiting plugin = %q", got)
	}
}

// TestPluginExecHandlerImportsOutputFiles retains plugin files in the output mount.
func TestPluginExecHandlerImportsOutputFiles(t *testing.T) {
	// Configure the plugin handler within the test lifecycle.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Retain a writable storage cursor for the plugin output mount.
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)
	handle := &pluginExecHandleStub{cursor: cursor}
	handler := &pluginExecHandler{handle: handle}

	// Apply plugin file outputs through the storage handle.
	resp := &PluginExecResponse{
		OutputFiles: []*PluginExecOutputFile{{
			Path: "nested/result.txt",
			Data: []byte("hello output"),
		}},
	}
	if err := handler.applyResponse(ctx, resp); err != nil {
		t.Fatal(err)
	}
	if len(handle.outputs) != 1 {
		t.Fatalf("outputs: %#v", handle.outputs)
	}
	out := handle.outputs[0]
	if out.GetName() != "output" || out.GetBucketRef().GetRootRef().GetEmpty() {
		t.Fatalf("output value: %#v", out)
	}

	// Read the retained output file through the UnixFS mount.
	cs := cursor.Clone()
	cs.SetRootRef(out.GetBucketRef().GetRootRef())
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, nil)
	t.Cleanup(fs.Release)
	fh, err := unixfs.NewFSHandle(fs)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(fh.Release)

	// Verify the plugin output content through the filesystem view.
	bfs := unixfs_billy.NewBillyFS(ctx, fh, "", time.Now())
	data, err := billy_util.ReadFile(bfs, "nested/result.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(data, []byte("hello output")) {
		t.Fatalf("file data: %q", string(data))
	}
}

// TestPluginExecConfigRoundTrip preserves plugin controller config through its codec.
func TestPluginExecConfigRoundTrip(t *testing.T) {
	// Build the plugin controller config for this execution.
	conf := &PluginExecConfig{
		PluginId:         "example-core",
		ControllerId:     "example/exec-controller/v86/browser",
		ControllerConfig: []byte{1, 2, 3},
	}
	if err := conf.Validate(); err != nil {
		t.Fatal(err)
	}

	// Decode the plugin config and compare its typed fields.
	data, err := conf.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	out := &PluginExecConfig{}
	if err := out.UnmarshalBlock(data); err != nil {
		t.Fatal(err)
	}
	if !conf.EqualsConfig(out) {
		t.Fatal("plugin exec config roundtrip mismatch")
	}
}

// TestPluginExecHandlerCallsPluginService forwards config and inputs and retains results.
func TestPluginExecHandlerCallsPluginService(t *testing.T) {
	// Configure the plugin handler within the test lifecycle.
	ctx := t.Context()
	client := &pluginExecClientStub{
		stream: &pluginExecStreamStub{
			resps: []*PluginExecResponse{{
				Logs: []*PluginExecLog{
					{Level: "info", Message: "ran plugin controller"},
				},
				Outputs: []*forge_value.Value{
					forge_value.NewValue("result"),
				},
			}},
		},
	}

	// Resolve the configured plugin service directly.
	load := func(ctx context.Context, b bus.Bus, pluginID string, onWaiting func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
		if pluginID != "example-core" {
			t.Fatalf("plugin id: %s", pluginID)
		}
		return client, nil, nil
	}

	// Build the plugin controller config for this execution.
	conf := &PluginExecConfig{
		PluginId:         "example-core",
		ControllerId:     "example/exec-controller/v86/browser",
		ControllerConfig: []byte{4, 5, 6},
	}
	configData, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Construct the bridge from its public handler factory.
	handle := &pluginExecHandleStub{}
	factory := newPluginExecHandler(
		nil,
		func(ctx context.Context, b bus.Bus, pluginID string, onWaiting func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return load(ctx, b, pluginID, onWaiting)
		},
	)
	handler, err := factory(
		ctx,
		logrus.NewEntry(logrus.New()),
		nil,
		handle,
		forge_target.InputMap{
			"source": forge_target.NewInputValueInline(forge_value.NewValue("source")),
		},
		configData,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Run the bridge and inspect the plugin request and retained response.
	if err := handler.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if !client.streamCalled {
		t.Fatal("streaming plugin exec was not used")
	}
	if client.req.GetControllerId() != conf.GetControllerId() {
		t.Fatalf("controller id: %s", client.req.GetControllerId())
	}
	if string(client.req.GetControllerConfig()) != string(conf.GetControllerConfig()) {
		t.Fatal("controller config mismatch")
	}
	if len(client.req.GetInputs()) != 1 || client.req.GetInputs()[0].GetName() != "source" {
		t.Fatalf("inputs: %#v", client.req.GetInputs())
	}

	// Inspect the response forwarded through the Execution handle.
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "ran plugin controller" {
		t.Fatalf("logs: %#v", handle.logs)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "result" {
		t.Fatalf("outputs: %#v", handle.outputs)
	}
}

// TestPluginExecHandlerStreamsLogsBeforeCompletion retains progress while execution is open.
func TestPluginExecHandlerStreamsLogsBeforeCompletion(t *testing.T) {
	// Configure the plugin handler within the test lifecycle.
	ctx := t.Context()
	ch := make(chan *PluginExecResponse)
	client := &pluginExecClientStub{stream: &pluginExecStreamStub{ch: ch}}

	// Build the plugin controller config for this execution.
	conf := &PluginExecConfig{
		PluginId:         "example-core",
		ControllerId:     "example/workfront/runner/claude",
		ControllerConfig: []byte{1, 2, 3},
	}
	configData, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Record plugin progress as the gated execution stream runs.
	logCh := make(chan *PluginExecLog, 2)
	handle := &pluginExecHandleStub{logCh: logCh}
	handler := &pluginExecHandler{
		handle: handle,
		conf:   conf,
		inputs: forge_target.InputMap{},
		load: func(ctx context.Context, b bus.Bus, pluginID string, onWaiting func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return client, nil, nil
		},
	}
	handler.conf = &PluginExecConfig{}
	if err := handler.conf.UnmarshalVT(configData); err != nil {
		t.Fatal(err)
	}

	// Deliver the first progress entry without completing the plugin stream.
	errs := make(chan error)
	go func() {
		errs <- handler.Execute(ctx)
	}()
	ch <- &PluginExecResponse{
		Logs: []*PluginExecLog{{
			Level:   "info",
			Message: "transcript: /tmp/example.log",
		}},
	}
	if log := <-logCh; log.GetMessage() != "transcript: /tmp/example.log" {
		t.Fatalf("streamed log: %#v", log)
	}
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "transcript: /tmp/example.log" {
		t.Fatalf("streamed logs before completion: %#v", handle.logs)
	}
	select {
	case err := <-errs:
		t.Fatalf("handler returned before stream closed: %v", err)
	default:
	}

	// Complete the stream and join the handler before checking its final outputs.
	ch <- &PluginExecResponse{
		Logs: []*PluginExecLog{{
			Level:   "info",
			Message: "complete",
		}},
		Outputs: []*forge_value.Value{
			forge_value.NewValue("result"),
		},
	}
	close(ch)
	if log := <-logCh; log.GetMessage() != "complete" {
		t.Fatalf("final streamed log: %#v", log)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if len(handle.logs) != 2 || handle.logs[1].GetMessage() != "complete" {
		t.Fatalf("final logs: %#v", handle.logs)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "result" {
		t.Fatalf("outputs: %#v", handle.outputs)
	}
}

// TestPluginExecHandlerRejectsEmptyStream requires a plugin result before EOF.
func TestPluginExecHandlerRejectsEmptyStream(t *testing.T) {
	// Configure the plugin handler within the test lifecycle.
	ctx := t.Context()
	client := &pluginExecClientStub{stream: &pluginExecStreamStub{}}
	handler := &pluginExecHandler{
		handle: &pluginExecHandleStub{},
		conf: &PluginExecConfig{
			PluginId:         "example-core",
			ControllerId:     "example/workfront/runner/claude",
			ControllerConfig: []byte{1, 2, 3},
		},
		inputs: forge_target.InputMap{},
		load: func(ctx context.Context, b bus.Bus, pluginID string, onWaiting func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return client, nil, nil
		},
	}

	// Require an explicit error for the empty plugin stream.
	err := handler.Execute(ctx)
	if err == nil {
		t.Fatal("expected empty stream error")
	}
	if err.Error() != "plugin exec stream completed without a response" {
		t.Fatalf("error: %v", err)
	}
}

// TestPluginExecHandlerFallsBackToUnaryExecute preserves unary-only plugin consumers.
func TestPluginExecHandlerFallsBackToUnaryExecute(t *testing.T) {
	// Configure the plugin handler within the test lifecycle.
	ctx := t.Context()
	client := &pluginExecClientStub{
		streamErr: errors.New("stream unavailable"),
		resp: &PluginExecResponse{
			Logs: []*PluginExecLog{{
				Level:   "info",
				Message: "unary fallback",
			}},
		},
	}
	handler := &pluginExecHandler{
		handle: &pluginExecHandleStub{},
		conf: &PluginExecConfig{
			PluginId:         "example-core",
			ControllerId:     "example/workfront/runner/claude",
			ControllerConfig: []byte{1, 2, 3},
		},
		inputs: forge_target.InputMap{},
		load: func(ctx context.Context, b bus.Bus, pluginID string, onWaiting func() error) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return client, nil, nil
		},
	}

	// Run the bridge and inspect the plugin request and retained response.
	if err := handler.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if client.req.GetExecutionObjectKey() != "exec/test" || client.req.GetClaimEpoch() != 0 {
		t.Fatalf("standalone handle context changed: %+v", client.req)
	}
	handle := handler.handle.(*pluginExecHandleStub)
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "unary fallback" {
		t.Fatalf("fallback logs: %#v", handle.logs)
	}
}
