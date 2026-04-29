package space_exec

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

type pluginExecClientStub struct {
	req  *PluginExecRequest
	resp *PluginExecResponse
	err  error
}

func (s *pluginExecClientStub) SRPCClient() srpc.Client {
	return nil
}

func (s *pluginExecClientStub) Execute(
	ctx context.Context,
	req *PluginExecRequest,
) (*PluginExecResponse, error) {
	s.req = req
	return s.resp, s.err
}

type pluginExecHandleStub struct {
	logs    []*PluginExecLog
	outputs forge_value.ValueSlice
	cursor  *bucket_lookup.Cursor
}

func (h *pluginExecHandleStub) GetExecutionUniqueId() string {
	return "test-exec"
}

func (h *pluginExecHandleStub) GetPeerId() peer.ID {
	return ""
}

func (h *pluginExecHandleStub) GetTimestamp() *timestamp.Timestamp {
	return &timestamp.Timestamp{}
}

func (h *pluginExecHandleStub) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if h.cursor == nil {
		return nil
	}
	if ref != nil && !ref.GetRootRef().GetEmpty() {
		cs := h.cursor.Clone()
		cs.SetRootRef(ref.GetRootRef())
		return cb(cs)
	}
	return cb(h.cursor)
}

func (h *pluginExecHandleStub) SetOutputs(
	ctx context.Context,
	outputs forge_value.ValueSlice,
	clearOld bool,
) error {
	h.outputs = outputs.Clone()
	return nil
}

func TestPluginExecHandlerImportsOutputFiles(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.StandardLogger()))
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	handle := &pluginExecHandleStub{cursor: cursor}
	handler := &pluginExecHandler{handle: handle}

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

	cs := cursor.Clone()
	cs.SetRootRef(out.GetBucketRef().GetRootRef())
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, nil)
	defer fs.Release()
	fh, err := unixfs.NewFSHandle(fs)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fh.Release()
	bfs := unixfs_billy.NewBillyFS(ctx, fh, "", time.Now())
	data, err := billy_util.ReadFile(bfs, "nested/result.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(data, []byte("hello output")) {
		t.Fatalf("file data: %q", string(data))
	}
}

func (h *pluginExecHandleStub) WriteLog(ctx context.Context, level, message string) error {
	h.logs = append(h.logs, &PluginExecLog{Level: level, Message: message})
	return nil
}

func TestPluginExecConfigRoundTrip(t *testing.T) {
	conf := &PluginExecConfig{
		PluginId:         "glados-core",
		ControllerId:     "glados/exec-controller/v86/browser",
		ControllerConfig: []byte{1, 2, 3},
	}
	if err := conf.Validate(); err != nil {
		t.Fatal(err)
	}
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

func TestPluginExecHandlerCallsPluginService(t *testing.T) {
	ctx := context.Background()
	client := &pluginExecClientStub{
		resp: &PluginExecResponse{
			Logs: []*PluginExecLog{
				{Level: "info", Message: "ran plugin controller"},
			},
			Outputs: []*forge_value.Value{
				forge_value.NewValue("result"),
			},
		},
	}
	load := func(ctx context.Context, b bus.Bus, pluginID string) (SRPCPluginExecServiceClient, directive.Reference, error) {
		if pluginID != "glados-core" {
			t.Fatalf("plugin id: %s", pluginID)
		}
		return client, nil, nil
	}

	conf := &PluginExecConfig{
		PluginId:         "glados-core",
		ControllerId:     "glados/exec-controller/v86/browser",
		ControllerConfig: []byte{4, 5, 6},
	}
	configData, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	handle := &pluginExecHandleStub{}
	factory := newPluginExecHandler(
		nil,
		func(ctx context.Context, b bus.Bus, pluginID string) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return load(ctx, b, pluginID)
		},
	)
	handler, err := factory(
		ctx,
		logrus.NewEntry(logrus.StandardLogger()),
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
	if err := handler.Execute(ctx); err != nil {
		t.Fatal(err)
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
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "ran plugin controller" {
		t.Fatalf("logs: %#v", handle.logs)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "result" {
		t.Fatalf("outputs: %#v", handle.outputs)
	}
}
