package s4wave_terminal

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

func TestTerminalValidatePinsDeviceTarget(t *testing.T) {
	term := &Terminal{
		Name:            "Build Host Shell",
		DeviceObjectKey: "devices/build-host",
		DevicePeerId:    "12D3KooWDevice",
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

	op.ObjectKey = ""
	if err := op.Validate(); err != world.ErrEmptyObjectKey {
		t.Fatalf("missing object key error = %v, want %v", err, world.ErrEmptyObjectKey)
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

func TestLookupCreateTerminalOp(t *testing.T) {
	op, err := LookupCreateTerminalOp(context.Background(), CreateTerminalOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*CreateTerminalOp); !ok {
		t.Fatalf("expected CreateTerminalOp, got %T", op)
	}
}

func TestForwardClientFramesReportsClosed(t *testing.T) {
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
	go (&TerminalResource{}).forwardClientFrames(context.Background(), strm, frameSession, errCh)

	gotRemote := &TerminalFrame{}
	if err := clientSession.RecvMsg(gotRemote); err != nil {
		t.Fatal(err)
	}
	if gotRemote.GetKind() != TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE {
		t.Fatalf("remote frame kind = %s", gotRemote.GetKind().String())
	}

	select {
	case result := <-errCh:
		if result.err != nil {
			t.Fatalf("client forward result error = %v", result.err)
		}
		if !result.updateState {
			t.Fatal("client close did not request terminal state update")
		}
		if result.finalState != TerminalSessionState_TERMINAL_SESSION_STATE_CLOSED {
			t.Fatalf("final state = %s", result.finalState.String())
		}
		if result.status != "closed" {
			t.Fatalf("status = %q", result.status)
		}
	case <-time.After(time.Second):
		t.Fatal("client frame forwarder did not stop")
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
