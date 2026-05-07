package space_exec

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
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
	req          *PluginExecRequest
	resp         *PluginExecResponse
	err          error
	stream       *pluginExecStreamStub
	streamErr    error
	streamCalled bool
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

func (s *pluginExecClientStub) ExecuteStream(
	ctx context.Context,
	req *PluginExecRequest,
) (SRPCPluginExecService_ExecuteStreamClient, error) {
	s.req = req
	s.streamCalled = true
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	return s.stream, nil
}

type pluginExecStreamStub struct {
	resps []*PluginExecResponse
	ch    chan *PluginExecResponse
	idx   int
}

func (s *pluginExecStreamStub) Context() context.Context {
	return context.Background()
}

func (s *pluginExecStreamStub) MsgSend(srpc.Message) error {
	return nil
}

func (s *pluginExecStreamStub) MsgRecv(srpc.Message) error {
	return nil
}

func (s *pluginExecStreamStub) CloseSend() error {
	return nil
}

func (s *pluginExecStreamStub) Close() error {
	return nil
}

func (s *pluginExecStreamStub) Recv() (*PluginExecResponse, error) {
	if s.ch != nil {
		resp, ok := <-s.ch
		if !ok {
			return nil, io.EOF
		}
		return resp, nil
	}
	if s.idx >= len(s.resps) {
		return nil, io.EOF
	}
	resp := s.resps[s.idx]
	s.idx++
	return resp, nil
}

func (s *pluginExecStreamStub) RecvTo(resp *PluginExecResponse) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*resp = *next
	return nil
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
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "ran plugin controller" {
		t.Fatalf("logs: %#v", handle.logs)
	}
	if len(handle.outputs) != 1 || handle.outputs[0].GetName() != "result" {
		t.Fatalf("outputs: %#v", handle.outputs)
	}
}

func TestPluginExecHandlerStreamsLogsBeforeCompletion(t *testing.T) {
	ctx := context.Background()
	ch := make(chan *PluginExecResponse)
	client := &pluginExecClientStub{stream: &pluginExecStreamStub{ch: ch}}
	conf := &PluginExecConfig{
		PluginId:         "glados-core",
		ControllerId:     "glados/workfront/runner/claude",
		ControllerConfig: []byte{1, 2, 3},
	}
	configData, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	handle := &pluginExecHandleStub{}
	handler := &pluginExecHandler{
		handle: handle,
		conf:   conf,
		inputs: forge_target.InputMap{},
		load: func(ctx context.Context, b bus.Bus, pluginID string) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return client, nil, nil
		},
	}
	handler.conf = &PluginExecConfig{}
	if err := handler.conf.UnmarshalVT(configData); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error)
	go func() {
		errs <- handler.Execute(ctx)
	}()
	ch <- &PluginExecResponse{
		Logs: []*PluginExecLog{{
			Level:   "info",
			Message: "transcript: /tmp/glados.log",
		}},
	}
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "transcript: /tmp/glados.log" {
		t.Fatalf("streamed logs before completion: %#v", handle.logs)
	}
	select {
	case err := <-errs:
		t.Fatalf("handler returned before stream closed: %v", err)
	default:
	}
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

func TestPluginExecHandlerRejectsEmptyStream(t *testing.T) {
	ctx := context.Background()
	client := &pluginExecClientStub{stream: &pluginExecStreamStub{}}
	handler := &pluginExecHandler{
		handle: &pluginExecHandleStub{},
		conf: &PluginExecConfig{
			PluginId:         "glados-core",
			ControllerId:     "glados/workfront/runner/claude",
			ControllerConfig: []byte{1, 2, 3},
		},
		inputs: forge_target.InputMap{},
		load: func(ctx context.Context, b bus.Bus, pluginID string) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return client, nil, nil
		},
	}
	err := handler.Execute(ctx)
	if err == nil {
		t.Fatal("expected empty stream error")
	}
	if err.Error() != "plugin exec stream completed without a response" {
		t.Fatalf("error: %v", err)
	}
}

func TestPluginExecHandlerFallsBackToUnaryExecute(t *testing.T) {
	ctx := context.Background()
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
			PluginId:         "glados-core",
			ControllerId:     "glados/workfront/runner/claude",
			ControllerConfig: []byte{1, 2, 3},
		},
		inputs: forge_target.InputMap{},
		load: func(ctx context.Context, b bus.Bus, pluginID string) (SRPCPluginExecServiceClient, directive.Reference, error) {
			return client, nil, nil
		},
	}
	if err := handler.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	handle := handler.handle.(*pluginExecHandleStub)
	if len(handle.logs) != 1 || handle.logs[0].GetMessage() != "unary fallback" {
		t.Fatalf("fallback logs: %#v", handle.logs)
	}
}
