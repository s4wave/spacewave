package cli_plugin_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	provider_pb "github.com/s4wave/spacewave/core/provider"
	session_pb "github.com/s4wave/spacewave/core/session"
	sobject_pb "github.com/s4wave/spacewave/core/sobject"
	space_pb "github.com/s4wave/spacewave/core/space"
	cli_plugin "github.com/s4wave/spacewave/plugin/cli"
	"github.com/s4wave/spacewave/sdk/cli/runner"
	s4wave_cli_terminal "github.com/s4wave/spacewave/sdk/cli/terminal"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

func TestRunCliSupportedWhoamiCommandWritesRunnerOutput(t *testing.T) {
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{
			session: terminalFakeSessionWithSpace("space-123456789", "Alpha"),
		},
	}
	strm := newTerminalStream(
		terminalInput("whoami\n"),
		terminalClose(),
	)

	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	assertReadyFrame(t, strm)
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> whoami\r\n")
	assertContains(t, out, "Session")
	assertContains(t, out, "session-1")
	assertContains(t, out, "Lock")
	assertContains(t, out, "unlocked (auto)")
	assertSuffix(t, out, "spacewave> ")
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliUnsupportedCommandWritesErrorAndContinuesPromptLoop(t *testing.T) {
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{
			session: terminalFakeSessionWithSpace("space-123456789", "Alpha"),
		},
	}
	strm := newTerminalStream(
		terminalInput("deploy now\n"),
		terminalInput("whoami\n"),
		terminalClose(),
	)

	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	out := terminalOutput(strm)
	assertContains(t, out, "deploy now\r\nerror: unsupported browser CLI command: deploy\r\nspacewave> whoami\r\n")
	assertContains(t, out, "Session")
	assertContains(t, out, "session-1")
	if prompts := strings.Count(out, "spacewave> "); prompts != 3 {
		t.Fatalf("prompt count = %d, want 3 in output %q", prompts, out)
	}
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliWatchModeCommandWritesTerminalErrorReprintsPromptAndSkipsRunnerFactory(t *testing.T) {
	factory := &terminalFakeFactory{}
	strm := newTerminalStream(
		terminalInput("space list --watch\n"),
		terminalInput("deploy now\n"),
		terminalClose(),
	)

	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> space list --watch\r\nerror: browser CLI terminal does not support watch mode\r\nspacewave> deploy now\r\n")
	assertContains(t, out, "error: unsupported browser CLI command: deploy\r\nspacewave> ")
	if prompts := strings.Count(out, "spacewave> "); prompts != 3 {
		t.Fatalf("prompt count = %d, want 3 in output %q", prompts, out)
	}
	if factory.newCalls != 0 {
		t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
	}
}

func TestRunCliShortWatchTrueCommandWritesTerminalErrorReprintsPromptAndSkipsRunnerFactory(t *testing.T) {
	factory := &terminalFakeFactory{}
	strm := newTerminalStream(
		terminalInput("space list -w=true\n"),
		terminalInput("deploy now\n"),
		terminalClose(),
	)

	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> space list -w=true\r\nerror: browser CLI terminal does not support watch mode\r\nspacewave> deploy now\r\n")
	assertContains(t, out, "error: unsupported browser CLI command: deploy\r\nspacewave> ")
	if prompts := strings.Count(out, "spacewave> "); prompts != 3 {
		t.Fatalf("prompt count = %d, want 3 in output %q", prompts, out)
	}
	if factory.newCalls != 0 {
		t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
	}
}

func TestRunCliWatchFalseRunsSpaceList(t *testing.T) {
	sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{session: sess},
	}
	strm := newTerminalStream(
		terminalInput("space list --watch=false\n"),
		terminalClose(),
	)

	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> space list --watch=false\r\n")
	assertContains(t, out, "space-12...")
	assertContains(t, out, "Alpha")
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
	if sess.resources == nil || !sess.resources.closed {
		t.Fatal("resources list stream was not closed")
	}
}

func TestRunCliRunnerCommandErrorWritesTerminalErrorAndReprintsPrompt(t *testing.T) {
	factory := &terminalFakeFactory{
		newClientErr: errors.New("daemon unavailable"),
	}
	strm := newTerminalStream(
		terminalInput("whoami\n"),
		terminalClose(),
	)

	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> whoami\r\n")
	assertContains(t, out, "error: daemon unavailable\r\nspacewave> ")
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliCloseOrEOFReturnsAfterMountedCommandReleasesRunnerResources(t *testing.T) {
	tests := []struct {
		name   string
		frames []*s4wave_terminal.TerminalFrame
	}{
		{
			name: "close frame",
			frames: []*s4wave_terminal.TerminalFrame{
				terminalInput("space list\n"),
				terminalClose(),
			},
		},
		{
			name: "recv eof",
			frames: []*s4wave_terminal.TerminalFrame{
				terminalInput("space list\n"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
			client := &terminalFakeClient{session: sess}
			factory := &terminalFakeFactory{client: client}
			strm := newTerminalStream(tt.frames...)

			if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
				t.Fatalf("RunCli: %v", err)
			}

			out := terminalOutput(strm)
			assertContains(t, out, "spacewave> space list\r\n")
			assertContains(t, out, "space-12...")
			assertContains(t, out, "Alpha")
			assertSuffix(t, out, "spacewave> ")
			if !client.closed {
				t.Fatal("runner client was not closed")
			}
			if !sess.released {
				t.Fatal("mounted session was not released")
			}
			if sess.resources == nil || !sess.resources.closed {
				t.Fatal("resources list stream was not closed")
			}
		})
	}
}

func newTerminalStream(frames ...*s4wave_terminal.TerminalFrame) *terminalStream {
	return &terminalStream{
		ctx:  context.Background(),
		recv: append([]*s4wave_terminal.TerminalFrame(nil), frames...),
	}
}

type terminalStream struct {
	ctx context.Context

	recv []*s4wave_terminal.TerminalFrame
	sent []*s4wave_terminal.TerminalFrame

	closeSend bool
	closed    bool
}

func (s *terminalStream) Context() context.Context {
	return s.ctx
}

func (s *terminalStream) Send(frame *s4wave_terminal.TerminalFrame) error {
	s.sent = append(s.sent, cloneTerminalFrame(frame))
	return nil
}

func (s *terminalStream) SendAndClose(frame *s4wave_terminal.TerminalFrame) error {
	if frame != nil {
		if err := s.Send(frame); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

func (s *terminalStream) Recv() (*s4wave_terminal.TerminalFrame, error) {
	if len(s.recv) == 0 {
		return nil, io.EOF
	}
	frame := s.recv[0]
	s.recv = s.recv[1:]
	return cloneTerminalFrame(frame), nil
}

func (s *terminalStream) RecvTo(msg *s4wave_terminal.TerminalFrame) error {
	frame, err := s.Recv()
	if err != nil {
		return err
	}
	*msg = *frame
	return nil
}

func (s *terminalStream) MsgSend(msg srpc.Message) error {
	frame, ok := msg.(*s4wave_terminal.TerminalFrame)
	if !ok {
		return errors.Errorf("unexpected sent message type %T", msg)
	}
	return s.Send(frame)
}

func (s *terminalStream) MsgRecv(msg srpc.Message) error {
	frame, ok := msg.(*s4wave_terminal.TerminalFrame)
	if !ok {
		return errors.Errorf("unexpected recv message type %T", msg)
	}
	return s.RecvTo(frame)
}

func (s *terminalStream) CloseSend() error {
	s.closeSend = true
	return nil
}

func (s *terminalStream) Close() error {
	s.closed = true
	return nil
}

func terminalInput(input string) *s4wave_terminal.TerminalFrame {
	return &s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
		Data: []byte(input),
	}
}

func terminalClose() *s4wave_terminal.TerminalFrame {
	return &s4wave_terminal.TerminalFrame{Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE}
}

func cloneTerminalFrame(frame *s4wave_terminal.TerminalFrame) *s4wave_terminal.TerminalFrame {
	if frame == nil {
		return nil
	}
	clone := *frame
	clone.Data = append([]byte(nil), frame.GetData()...)
	return &clone
}

func terminalOutput(strm *terminalStream) string {
	var out strings.Builder
	for _, frame := range strm.sent {
		if frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT {
			out.Write(frame.GetData())
		}
	}
	return out.String()
}

func assertReadyFrame(t *testing.T, strm *terminalStream) {
	t.Helper()
	if len(strm.sent) == 0 {
		t.Fatal("no terminal frames sent")
	}
	if got := strm.sent[0].GetKind(); got != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY {
		t.Fatalf("first sent frame kind = %v, want READY", got)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertSuffix(t *testing.T, got, want string) {
	t.Helper()
	if !strings.HasSuffix(got, want) {
		t.Fatalf("expected %q to end with %q", got, want)
	}
}

type terminalFakeFactory struct {
	client       runner.Client
	newClientErr error
	newCalls     int
}

func (f *terminalFakeFactory) NewClient(ctx context.Context, c *cli.Context) (runner.Client, error) {
	f.newCalls++
	if f.newClientErr != nil {
		return nil, f.newClientErr
	}
	return f.client, nil
}

func (f *terminalFakeFactory) StatusEndpoint(ctx context.Context, c *cli.Context) (string, error) {
	return "/injected/spacewave.sock", nil
}

type terminalFakeClient struct {
	session  runner.Session
	mountErr error
	closed   bool
}

func (c *terminalFakeClient) Close() {
	c.closed = true
}

func (c *terminalFakeClient) MountSession(ctx context.Context, idx uint32) (runner.Session, error) {
	if c.mountErr != nil {
		return nil, c.mountErr
	}
	return c.session, nil
}

type terminalFakeSession struct {
	info      *s4wave_session.GetSessionInfoResponse
	resources *terminalFakeResourcesListStream
	lock      *terminalFakeLockStateStream
	released  bool
}

func terminalFakeSessionWithSpace(id, name string) *terminalFakeSession {
	return &terminalFakeSession{
		info: &s4wave_session.GetSessionInfoResponse{
			SessionRef: &session_pb.SessionRef{ProviderResourceRef: &provider_pb.ProviderResourceRef{
				Id:                "session-1",
				ProviderId:        "local",
				ProviderAccountId: "account-1",
			}},
			PeerId: "peer-1",
		},
		resources: &terminalFakeResourcesListStream{responses: []*s4wave_session.WatchResourcesListResponse{{
			SpacesList: []*space_pb.SpaceSoListEntry{{
				Entry: &sobject_pb.SharedObjectListEntry{Ref: &sobject_pb.SharedObjectRef{
					ProviderResourceRef: &provider_pb.ProviderResourceRef{Id: id},
				}},
				SpaceMeta: &space_pb.SpaceSoMeta{Name: name},
			}},
		}}},
		lock: &terminalFakeLockStateStream{responses: []*s4wave_session.WatchLockStateResponse{{
			Mode:   session_pb.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK,
			Locked: false,
		}}},
	}
}

func (s *terminalFakeSession) Release() {
	s.released = true
}

func (s *terminalFakeSession) GetSessionInfo(ctx context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	return s.info, nil
}

func (s *terminalFakeSession) WatchResourcesList(ctx context.Context) (runner.ResourcesListStream, error) {
	return s.resources, nil
}

func (s *terminalFakeSession) WatchLockState(ctx context.Context) (runner.LockStateStream, error) {
	return s.lock, nil
}

func (s *terminalFakeSession) WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error) {
	return nil, nil
}

type terminalFakeResourcesListStream struct {
	responses []*s4wave_session.WatchResourcesListResponse
	idx       int
	closed    bool
}

func (s *terminalFakeResourcesListStream) Close() error {
	s.closed = true
	return nil
}

func (s *terminalFakeResourcesListStream) Recv() (*s4wave_session.WatchResourcesListResponse, error) {
	if s.idx >= len(s.responses) {
		return nil, io.EOF
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

type terminalFakeLockStateStream struct {
	responses []*s4wave_session.WatchLockStateResponse
	idx       int
	closed    bool
}

func (s *terminalFakeLockStateStream) Close() error {
	s.closed = true
	return nil
}

func (s *terminalFakeLockStateStream) Recv() (*s4wave_session.WatchLockStateResponse, error) {
	if s.idx >= len(s.responses) {
		return nil, io.EOF
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

var _ s4wave_cli_terminal.SRPCCliTerminalService_RunCliStream = (*terminalStream)(nil)
var _ runner.ClientFactory = (*terminalFakeFactory)(nil)
var _ runner.Client = (*terminalFakeClient)(nil)
var _ runner.Session = (*terminalFakeSession)(nil)
