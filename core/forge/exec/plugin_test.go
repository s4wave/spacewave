package space_exec

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
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
	return nil
}

func (h *pluginExecHandleStub) SetOutputs(
	ctx context.Context,
	outputs forge_value.ValueSlice,
	clearOld bool,
) error {
	h.outputs = outputs.Clone()
	return nil
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
