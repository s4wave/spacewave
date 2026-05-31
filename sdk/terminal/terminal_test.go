package s4wave_terminal

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

func TestTerminalValidatePinsDeviceTarget(t *testing.T) {
	term := &Terminal{
		Name:            "Build Host Shell",
		DeviceObjectKey: "devices/build-host",
		DevicePeerId:    "12D3KooWDevice",
		TargetKind:      TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE,
		Environment:     []string{"TERM=xterm-256color"},
	}
	if err := term.Validate(); err != nil {
		t.Fatalf("valid terminal failed validation: %v", err)
	}

	term.DevicePeerId = ""
	if err := term.Validate(); err == nil {
		t.Fatal("expected missing device peer id to fail validation")
	}

	term.DevicePeerId = "12D3KooWDevice"
	term.Environment = []string{"BROKEN"}
	if err := term.Validate(); err == nil {
		t.Fatal("expected malformed environment entry to fail validation")
	}

	term.Environment = []string{"=value"}
	if err := term.Validate(); err == nil {
		t.Fatal("expected empty environment key to fail validation")
	}
}

func TestTerminalValidatePinsSshHostTarget(t *testing.T) {
	term := &Terminal{
		Name:             "Prod SSH Shell",
		SshHostObjectKey: "hosts/prod",
		TargetKind:       TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST,
		Environment:      []string{"TERM=xterm-256color"},
	}
	if err := term.Validate(); err != nil {
		t.Fatalf("valid SSH Host terminal failed validation: %v", err)
	}
	if got := EffectiveTerminalTargetKind(term); got != TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST {
		t.Fatalf("target kind = %s", got.String())
	}

	term.SshHostObjectKey = ""
	if err := term.Validate(); err == nil {
		t.Fatal("expected missing SSH Host object key to fail validation")
	}
}

func TestCreateTerminalOpValidate(t *testing.T) {
	op := NewCreateTerminalOp(
		"terminal/build-host-1",
		"Build Host Shell",
		"devices/build-host",
		"12D3KooWDevice",
		time.Unix(10, 0),
	)
	if err := op.Validate(); err != nil {
		t.Fatalf("valid create terminal op failed validation: %v", err)
	}
	if got := effectiveCreateTerminalOpTargetKind(op); got != TerminalTargetKind_TERMINAL_TARGET_KIND_DEVICE {
		t.Fatalf("target kind = %s", got.String())
	}

	op.ObjectKey = ""
	if err := op.Validate(); err != world.ErrEmptyObjectKey {
		t.Fatalf("missing object key error = %v, want %v", err, world.ErrEmptyObjectKey)
	}
}

func TestCreateSshHostTerminalOpValidate(t *testing.T) {
	op := NewCreateSshHostTerminalOp(
		"terminal/prod-ssh-1",
		"Prod SSH Shell",
		"hosts/prod",
		time.Unix(10, 0),
	)
	if err := op.Validate(); err != nil {
		t.Fatalf("valid create SSH Host terminal op failed validation: %v", err)
	}
	if got := effectiveCreateTerminalOpTargetKind(op); got != TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST {
		t.Fatalf("target kind = %s", got.String())
	}

	op.SshHostObjectKey = ""
	if err := op.Validate(); err == nil {
		t.Fatal("expected missing SSH Host object key to fail validation")
	}
}

func TestNormalizeTerminalFrameSize(t *testing.T) {
	cols, rows := NormalizeTerminalFrameSize(0, 0)
	if cols != DefaultTerminalCols || rows != DefaultTerminalRows {
		t.Fatalf("default size = %dx%d", cols, rows)
	}

	cols, rows = NormalizeTerminalFrameSize(120, 40)
	if cols != 120 || rows != 40 {
		t.Fatalf("explicit size = %dx%d", cols, rows)
	}
}

func TestTerminalMarshalBlockRoundTrip(t *testing.T) {
	term := &Terminal{
		Name:            "Build Host Shell",
		DeviceObjectKey: "devices/build-host",
		DevicePeerId:    "12D3KooWDevice",
		Cols:            120,
		Rows:            40,
		CreatedAt:       timestamppb.New(time.Unix(10, 0)),
	}
	data, err := term.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock() error = %v", err)
	}

	got := &Terminal{}
	if err := got.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock() error = %v", err)
	}
	if !got.EqualVT(term) {
		t.Fatalf("round trip = %#v, want %#v", got, term)
	}
}

func TestTerminalSshHostTargetDoesNotStoreCredentialMaterial(t *testing.T) {
	rawCredential := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nspacewave-secret\n-----END OPENSSH PRIVATE KEY-----")
	term := &Terminal{
		Name:             "Prod SSH Terminal",
		SshHostObjectKey: "hosts/prod",
		TargetKind:       TerminalTargetKind_TERMINAL_TARGET_KIND_SSH_HOST,
	}
	data, err := term.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock() error = %v", err)
	}
	if bytes.Contains(data, rawCredential) {
		t.Fatal("Terminal block contains raw SSH credential bytes")
	}
}

func TestLookupCreateTerminalOp(t *testing.T) {
	op, err := LookupCreateTerminalOp(context.Background(), CreateTerminalOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*CreateTerminalOp); !ok {
		t.Fatalf("expected CreateTerminalOp, got %T", op)
	}
}

func TestForwardClientFramesSendsCloseWithoutCompleting(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	frameSession := stream_packet.NewSession(serverConn, terminalFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, terminalFrameMaxBytes)
	strm := &testTerminalConnectStream{
		ctx: context.Background(),
		recv: []*TerminalFrame{{
			Kind: TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE,
		}},
	}
	errCh := make(chan terminalConnectResult, 1)
	var clientClosed atomic.Bool
	go (&TerminalResource{}).forwardClientFrames(context.Background(), strm, frameSession, &clientClosed, errCh)

	gotRemote := &TerminalFrame{}
	if err := clientSession.RecvMsg(gotRemote); err != nil {
		t.Fatal(err)
	}
	if gotRemote.GetKind() != TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE {
		t.Fatalf("remote frame kind = %s", gotRemote.GetKind().String())
	}
	if !clientClosed.Load() {
		t.Fatal("client close flag was not set")
	}

	select {
	case result := <-errCh:
		t.Fatalf("client close completed terminal before remote result: %#v", result)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestForwardRemoteFramesReportsClosedAfterClientCloseAndExit(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	objectKey := "terminal/close-exit"
	op := NewCreateTerminalOp(
		objectKey,
		"Build Host Terminal",
		"devices/build-host",
		"12D3KooWDevice",
		time.Unix(10, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	objState, found, err := tb.WorldState.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("terminal object was not created")
	}
	state, err := readTerminalObject(ctx, objState)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	frameSession := stream_packet.NewSession(serverConn, terminalFrameMaxBytes)
	remoteSession := stream_packet.NewSession(clientConn, terminalFrameMaxBytes)
	strm := &testTerminalConnectStream{ctx: ctx}
	errCh := make(chan terminalConnectResult, 1)
	var clientClosed atomic.Bool
	clientClosed.Store(true)
	go NewTerminalResource(nil, tb.WorldState, tb.Engine, objectKey, state).forwardRemoteFrames(
		ctx,
		strm,
		frameSession,
		&clientClosed,
		errCh,
	)

	if err := remoteSession.SendMsg(&TerminalFrame{
		Kind:     TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT,
		ExitCode: 0,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-errCh:
		if result.err != nil {
			t.Fatalf("remote forward result error = %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote frame forwarder did not stop")
	}
	if len(strm.sent) != 1 || strm.sent[0].GetKind() != TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT {
		t.Fatalf("sent frames = %#v", strm.sent)
	}

	updated, err := readTerminalObject(ctx, objState)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetState() != TerminalSessionState_TERMINAL_SESSION_STATE_CLOSED {
		t.Fatalf("state = %s", updated.GetState().String())
	}
	if updated.GetStatus() != "closed" {
		t.Fatalf("status = %q", updated.GetStatus())
	}
}

func TestTerminalConnectOpenFailureStateUsesDisconnectedForCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	state, status, errMessage := terminalConnectOpenFailureState(ctx, stderrors.New("context canceled"), "failed to connect")
	if state != TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED {
		t.Fatalf("state = %s", state.String())
	}
	if status != "disconnected" {
		t.Fatalf("status = %q", status)
	}
	if errMessage != "" {
		t.Fatalf("error = %q", errMessage)
	}

	state, status, errMessage = terminalConnectOpenFailureState(context.Background(), stderrors.New("dial failed"), "failed to connect")
	if state != TerminalSessionState_TERMINAL_SESSION_STATE_FAILED {
		t.Fatalf("state = %s", state.String())
	}
	if status != "failed to connect" {
		t.Fatalf("status = %q", status)
	}
	if errMessage != "dial failed" {
		t.Fatalf("error = %q", errMessage)
	}
}

type testTerminalConnectStream struct {
	ctx  context.Context
	recv []*TerminalFrame
	sent []*TerminalFrame
}

func (s *testTerminalConnectStream) Context() context.Context {
	return s.ctx
}

func (s *testTerminalConnectStream) MsgSend(msg srpc.Message) error {
	frame, ok := msg.(*TerminalFrame)
	if ok {
		s.sent = append(s.sent, frame)
	}
	return nil
}

func (s *testTerminalConnectStream) MsgRecv(srpc.Message) error {
	return io.EOF
}

func (s *testTerminalConnectStream) CloseSend() error {
	return nil
}

func (s *testTerminalConnectStream) Close() error {
	return nil
}

func (s *testTerminalConnectStream) Send(frame *TerminalFrame) error {
	s.sent = append(s.sent, frame)
	return nil
}

func (s *testTerminalConnectStream) SendAndClose(frame *TerminalFrame) error {
	if frame != nil {
		s.sent = append(s.sent, frame)
	}
	return nil
}

func (s *testTerminalConnectStream) Recv() (*TerminalFrame, error) {
	if len(s.recv) == 0 {
		return nil, io.EOF
	}
	frame := s.recv[0]
	s.recv = s.recv[1:]
	return frame, nil
}

func (s *testTerminalConnectStream) RecvTo(frame *TerminalFrame) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*frame = *next
	return nil
}
