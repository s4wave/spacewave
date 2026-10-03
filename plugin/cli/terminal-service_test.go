package cli_plugin_test

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	// Prepare a terminal session for the whoami command.
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{
			session: terminalFakeSessionWithSpace("space-123456789", "Alpha"),
		},
	}
	strm := newTerminalStream(
		terminalInput("whoami\n"),
		terminalClose(),
	)

	// Run whoami through the terminal service.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify the ready frame and the session and lock output.
	assertReadyFrame(t, strm)
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> whoami\r\n")
	assertContains(t, out, "Session")
	assertContains(t, out, "session-1")
	assertContains(t, out, "Lock")
	assertContains(t, out, "unlocked (auto)")
	assertSuffix(t, out, "spacewave> ")

	// Verify every output frame uses exactly one CR before each LF.
	var sawCommandLineBreak bool
	for _, frame := range terminalOutputFrames(strm) {
		data := frame.GetData()
		if bytes.Contains(data, []byte("session-1")) &&
			bytes.Contains(data, []byte("\r\n")) {
			sawCommandLineBreak = true
		}
		if bytes.Contains(data, []byte("\r\r\n")) {
			t.Fatalf("output frame contains double-normalized CRLF: %q", data)
		}
		for i, b := range data {
			if b != '\n' {
				continue
			}
			if i == 0 || data[i-1] != '\r' {
				t.Fatalf("output frame contains bare LF: %q", data)
			}
		}
	}
	if !sawCommandLineBreak {
		t.Fatal("whoami output frame did not contain a CRLF line break")
	}

	// Verify whoami opens one runner client.
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliHelpBuiltinsListRunnerCommandMetadata(t *testing.T) {
	// Define the terminal help aliases to exercise.
	tests := []struct {
		name  string
		input string
	}{
		{name: "help", input: "help\n"},
		{name: "question mark", input: "?\n"},
	}

	// Exercise each help alias against runner command metadata.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare the terminal stream for the help alias.
			factory := &terminalFakeFactory{}
			strm := newTerminalStream(
				terminalInput(tt.input),
				terminalClose(),
			)

			// Run the help alias through the terminal service.
			if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
				t.Fatalf("RunCli: %v", err)
			}

			// Verify help includes the supported runner commands.
			out := terminalOutput(strm)
			assertContains(t, out, "Supported browser CLI commands:\r\n")
			for _, want := range expectedRunnerHelpLines() {
				assertContains(t, out, want)
			}

			// Verify help includes terminal builtins and the native CLI settings pointer.
			assertContains(t, out, "  help, ?       show browser CLI help\r\n")
			assertContains(t, out, "  clear         clear the terminal\r\n")
			assertContains(t, out, "  exit          close this browser CLI prompt\r\n")
			assertContains(t, out, "Open Command Line settings for the full native CLI.\r\n")
			assertSuffix(t, out, "spacewave> ")

			// Verify terminal help needs no runner client.
			if factory.newCalls != 0 {
				t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
			}
		})
	}
}

func TestRunCliUnsupportedCommandWritesSupportedSetSettingsPointerAndContinuesPromptLoop(t *testing.T) {
	// Prepare an unsupported command followed by whoami in one terminal stream.
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

	// Run both commands through the terminal prompt loop.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify the unsupported command explains the supported set and whoami still runs.
	out := terminalOutput(strm)
	assertContains(t, out, "deploy now\r\nerror: unsupported browser CLI command: deploy\r\n")
	assertContains(t, out, "supported browser CLI commands: status, whoami, space list, spaces list, help, ?, clear, exit\r\n")
	assertContains(t, out, "Open Command Line settings for the full native CLI.\r\nspacewave> whoami\r\n")
	assertContains(t, out, "Session")
	assertContains(t, out, "session-1")
	if prompts := strings.Count(out, "spacewave> "); prompts != 3 {
		t.Fatalf("prompt count = %d, want 3 in output %q", prompts, out)
	}

	// Verify only whoami opens a runner client.
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliUnsupportedFlagWritesAllowedFlagSet(t *testing.T) {
	// Define unsupported flags and their command-specific allowed sets.
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "top-level command excludes watch flags",
			line: "whoami --watch\n",
			want: "error: unsupported flag: --watch (allowed flags: --output, -o, --session-index)\r\n",
		},
		{
			name: "space list includes watch flags",
			line: "space list --bogus\n",
			want: "error: unsupported flag: --bogus (allowed flags: --output, -o, --session-index, --watch, -w)\r\n",
		},
	}

	// Exercise flag rejection for each command.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare the terminal stream containing the unsupported flag.
			factory := &terminalFakeFactory{}
			strm := newTerminalStream(
				terminalInput(tt.line),
				terminalClose(),
			)

			// Run the command through terminal flag validation.
			if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
				t.Fatalf("RunCli: %v", err)
			}

			// Verify the allowed flag set and the restored prompt.
			out := terminalOutput(strm)
			assertContains(t, out, tt.want)
			assertSuffix(t, out, "spacewave> ")
			if factory.newCalls != 0 {
				t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
			}
		})
	}
}

func TestRunCliHistoryRecallSupportsCursorEditingWithXtermArrowEscapes(t *testing.T) {
	// Prepare two space responses and an edited history recall.
	sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
	sess.resources.responses = append(sess.resources.responses, terminalSpaceListResponse("space-987654321", "Beta"))
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{session: sess},
	}
	strm := newTerminalStream(
		terminalInput("space list\n"),
		terminalInput("\x1b[A"+strings.Repeat("\x1b[D", 6)+"\x1b[C"+"s\n"),
		terminalClose(),
	)

	// Run the space command and its edited history entry.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify the recalled command changes to the plural alias and returns the next space.
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> space list\r\n")
	assertContains(t, out, "Alpha")
	assertContains(t, out, "spacewave> spaces list")
	assertContains(t, out, "Beta")
	if factory.newCalls != 2 {
		t.Fatalf("NewClient calls = %d, want 2", factory.newCalls)
	}
}

func TestRunCliHistoryDownRestoresNewerEntryAndDraft(t *testing.T) {
	// Prepare terminal history navigation around an unfinished help command.
	factory := &terminalFakeFactory{}
	strm := newTerminalStream(
		terminalInput("help\n"),
		terminalInput("clear\n"),
		terminalInput("hel\x1b[A\x1b[A\x1b[B\x1b[Bp\n"),
		terminalClose(),
	)

	// Run the terminal history and draft restoration sequence.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify history navigation restores the newer command and the unfinished draft.
	out := terminalOutput(strm)
	if clearRecallCount := strings.Count(out, "\r\x1b[2Kspacewave> clear"); clearRecallCount != 2 {
		t.Fatalf("clear history recall count = %d, want 2 in output %q", clearRecallCount, out)
	}
	assertContains(t, out, "\r\x1b[2Kspacewave> hel")
	if helpCount := strings.Count(out, "Supported browser CLI commands:\r\n"); helpCount != 2 {
		t.Fatalf("help output count = %d, want 2 in output %q", helpCount, out)
	}
	if factory.newCalls != 0 {
		t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
	}
}

func TestRunCliCtrlCCancelsHungCommandAndReprintsPrompt(t *testing.T) {
	// Start a terminal service whose session mount waits for cancellation.
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{blockMount: true},
	}
	strm := newInteractiveTerminalStream()
	done := runTerminalServiceAsync(t, factory, strm)

	// Submit whoami and interrupt its blocked mount with Ctrl-C.
	strm.receive(terminalInput("whoami\n"))
	waitForTerminalOutput(t, strm, "spacewave> whoami\r\n")
	strm.receive(terminalInput("\x03"))
	waitForTerminalOutput(t, strm, "^C\r\nspacewave> ")

	// Close the terminal stream and wait for the service to release it.
	strm.receive(terminalClose())
	strm.closeRecv()
	waitForTerminalService(t, done)

	// Verify interruption restores the prompt without exposing a cancellation error.
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> whoami\r\n")
	assertContains(t, out, "^C\r\nspacewave> ")
	if strings.Contains(out, "context canceled") {
		t.Fatalf("interrupted command surfaced context cancellation: %q", out)
	}
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliClearWritesXtermClearSequenceAndPrompt(t *testing.T) {
	// Prepare a terminal stream containing the clear builtin.
	factory := &terminalFakeFactory{}
	strm := newTerminalStream(
		terminalInput("clear\n"),
		terminalClose(),
	)

	// Run clear through the terminal service.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify clear emits the xterm clear sequence without opening a runner client.
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> clear\r\n\x1b[2J\x1b[Hspacewave> ")
	if factory.newCalls != 0 {
		t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
	}
}

func TestRunCliExitSendsExitFrame(t *testing.T) {
	// Prepare a terminal stream containing the exit builtin.
	factory := &terminalFakeFactory{}
	strm := newTerminalStream(
		terminalInput("exit\n"),
	)

	// Run exit through the terminal service.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify exit sends a successful exit frame without opening a runner client.
	exit := findTerminalFrame(strm, s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT)
	if exit == nil {
		t.Fatalf("no EXIT frame sent; sent frames: %#v", strm.sent)
	}
	if exit.GetExitCode() != 0 {
		t.Fatalf("exit code = %d, want 0", exit.GetExitCode())
	}
	if factory.newCalls != 0 {
		t.Fatalf("NewClient calls = %d, want 0", factory.newCalls)
	}
}

func TestRunCliWatchModeStreamsIncrementalOutputUntilCtrlCReprintsPrompt(t *testing.T) {
	// Start a terminal session with two space updates followed by a blocking watch.
	resources := newBlockingResourcesListStream(
		terminalSpaceListResponse("space-123456789", "Alpha"),
		terminalSpaceListResponse("space-987654321", "Beta"),
	)
	sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
	sess.resources = resources
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{session: sess},
	}
	strm := newInteractiveTerminalStream()
	done := runTerminalServiceAsync(t, factory, strm)

	// Run the space watch and verify both updates arrive in separate output frames.
	strm.receive(terminalInput("space list --watch\n"))
	waitForTerminalOutput(t, strm, "Alpha")
	waitForTerminalOutput(t, strm, "Beta")
	assertOutputFrameCountAtLeast(t, strm, 3)

	// Interrupt the space watch and verify the prompt returns.
	strm.receive(terminalInput("\x03"))
	waitForTerminalOutput(t, strm, "^C\r\nspacewave> ")

	// Close the terminal stream and wait for watch cleanup.
	strm.receive(terminalClose())
	strm.closeRecv()
	waitForTerminalService(t, done)

	// Verify the watch output and cancellation handling.
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> space list --watch\r\n")
	assertContains(t, out, "\n--- ")
	assertContains(t, out, "Beta")
	assertContains(t, out, "^C\r\nspacewave> ")
	if strings.Contains(out, "context canceled") {
		t.Fatalf("interrupted watch surfaced context cancellation: %q", out)
	}

	// Verify interruption closes the watch and uses one runner client.
	if !resources.closed {
		t.Fatal("resources list stream was not closed")
	}
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliChunksLargeCommandOutputFrames(t *testing.T) {
	// Prepare a space list large enough to require multiple terminal output frames.
	sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
	sess.resources = &terminalFakeResourcesListStream{
		responses: []*s4wave_session.WatchResourcesListResponse{terminalSpacesListResponse(320)},
	}
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{session: sess},
	}
	strm := newTerminalStream(
		terminalInput("space list --output=json\n"),
		terminalClose(),
	)

	// Run the large space list through the terminal service.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify output frames respect the size limit and include the final space.
	outputFrames := terminalOutputFrames(strm)
	var chunked bool
	for _, frame := range outputFrames {
		if got := len(frame.GetData()); got > 4096 {
			t.Fatalf("output frame size = %d, want <= 4096", got)
		}
		if len(frame.GetData()) == 4096 {
			chunked = true
		}
	}
	if !chunked {
		t.Fatalf("large command output was not split into a 4096-byte chunk; frame sizes: %v", terminalOutputFrameSizes(strm))
	}
	assertContains(t, terminalOutput(strm), "space-0319")
}

func TestRunCliWatchFalseRunsSpaceList(t *testing.T) {
	// Prepare a space list command with watch explicitly disabled.
	sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
	factory := &terminalFakeFactory{
		client: &terminalFakeClient{session: sess},
	}
	strm := newTerminalStream(
		terminalInput("space list --watch=false\n"),
		terminalClose(),
	)

	// Run the space list through the terminal service.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify the space output and runner resource cleanup.
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
	// Prepare a runner client factory that returns a connection failure.
	factory := &terminalFakeFactory{
		newClientErr: errors.New("daemon unavailable"),
	}
	strm := newTerminalStream(
		terminalInput("whoami\n"),
		terminalClose(),
	)

	// Run whoami through the failing runner client factory.
	if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
		t.Fatalf("RunCli: %v", err)
	}

	// Verify the terminal reports the connection failure and restores its prompt.
	out := terminalOutput(strm)
	assertContains(t, out, "spacewave> whoami\r\n")
	assertContains(t, out, "error: daemon unavailable\r\nspacewave> ")
	if factory.newCalls != 1 {
		t.Fatalf("NewClient calls = %d, want 1", factory.newCalls)
	}
}

func TestRunCliCloseOrEOFReturnsAfterMountedCommandReleasesRunnerResources(t *testing.T) {
	// Define terminal shutdown by close frame and by receive EOF.
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

	// Exercise each shutdown path after a mounted space command.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare a mounted session and the selected terminal shutdown sequence.
			sess := terminalFakeSessionWithSpace("space-123456789", "Alpha")
			client := &terminalFakeClient{session: sess}
			factory := &terminalFakeFactory{client: client}
			strm := newTerminalStream(tt.frames...)

			// Run the space command through the selected shutdown path.
			if err := cli_plugin.NewTerminalService(factory).RunCli(strm); err != nil {
				t.Fatalf("RunCli: %v", err)
			}

			// Verify the command output and restored terminal prompt.
			out := terminalOutput(strm)
			assertContains(t, out, "spacewave> space list\r\n")
			assertContains(t, out, "space-12...")
			assertContains(t, out, "Alpha")
			assertSuffix(t, out, "spacewave> ")

			// Verify shutdown closes the runner client, mounted session, and resource watch.
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
		ctx:        context.Background(),
		recv:       append([]*s4wave_terminal.TerminalFrame(nil), frames...),
		sentNotify: make(chan struct{}),
	}
}

func newInteractiveTerminalStream() *terminalStream {
	return &terminalStream{
		ctx:        context.Background(),
		recvCh:     make(chan *s4wave_terminal.TerminalFrame),
		sentNotify: make(chan struct{}),
	}
}

type terminalStream struct {
	ctx context.Context

	mu                      sync.Mutex
	recv                    []*s4wave_terminal.TerminalFrame
	recvCh                  chan *s4wave_terminal.TerminalFrame
	sent                    []*s4wave_terminal.TerminalFrame
	sentNotify              chan struct{}
	waitForPromptBeforeNext bool
	nextPromptCount         int

	closeSend bool
	closed    bool
}

func (s *terminalStream) Context() context.Context {
	return s.ctx
}

func (s *terminalStream) Send(frame *s4wave_terminal.TerminalFrame) error {
	// Record the sent terminal frame and notify output waiters under the stream lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, cloneTerminalFrame(frame))
	close(s.sentNotify)
	s.sentNotify = make(chan struct{})
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
	// Receive interactive terminal frames until the channel closes or the context ends.
	if s.recvCh != nil {
		select {
		case frame, ok := <-s.recvCh:
			if !ok {
				return nil, io.EOF
			}
			return cloneTerminalFrame(frame), nil
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		}
	}

	// Wait for the previous command prompt before consuming queued terminal input.
	if s.waitForPromptBeforeNext {
		s.waitForPromptBeforeNext = false
		if err := s.waitForPromptCount(s.nextPromptCount); err != nil {
			return nil, err
		}
	}

	// Return EOF when the queued terminal frames are exhausted.
	if len(s.recv) == 0 {
		return nil, io.EOF
	}

	// Consume the next queued terminal frame and track its expected command prompt.
	frame := s.recv[0]
	s.recv = s.recv[1:]
	if frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT && bytes.ContainsAny(frame.GetData(), "\r\n") {
		s.mu.Lock()
		s.nextPromptCount = strings.Count(terminalOutputLocked(s), "spacewave> ") + 1
		s.waitForPromptBeforeNext = true
		s.mu.Unlock()
	}
	return cloneTerminalFrame(frame), nil
}

func (s *terminalStream) waitForPromptCount(want int) error {
	deadline := time.After(2 * time.Second)
	for {
		s.mu.Lock()
		out := terminalOutputLocked(s)
		notify := s.sentNotify
		s.mu.Unlock()
		if strings.Count(out, "spacewave> ") >= want {
			return nil
		}
		select {
		case <-notify:
		case <-deadline:
			return errors.Errorf("timed out waiting for prompt %d in output %q", want, out)
		}
	}
}

func (s *terminalStream) receive(frame *s4wave_terminal.TerminalFrame) {
	s.recvCh <- frame
}

func (s *terminalStream) closeRecv() {
	close(s.recvCh)
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
	for _, frame := range terminalOutputFrames(strm) {
		out.Write(frame.GetData())
	}
	return out.String()
}

func terminalOutputFrames(strm *terminalStream) []*s4wave_terminal.TerminalFrame {
	// Copy output frames while holding the terminal stream lock.
	strm.mu.Lock()
	defer strm.mu.Unlock()
	var out []*s4wave_terminal.TerminalFrame
	for _, frame := range strm.sent {
		if frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT {
			out = append(out, cloneTerminalFrame(frame))
		}
	}
	return out
}

func terminalOutputFrameSizes(strm *terminalStream) []int {
	frames := terminalOutputFrames(strm)
	sizes := make([]int, 0, len(frames))
	for _, frame := range frames {
		sizes = append(sizes, len(frame.GetData()))
	}
	return sizes
}

func findTerminalFrame(strm *terminalStream, kind s4wave_terminal.TerminalFrameKind) *s4wave_terminal.TerminalFrame {
	strm.mu.Lock()
	defer strm.mu.Unlock()
	for _, frame := range strm.sent {
		if frame.GetKind() == kind {
			return cloneTerminalFrame(frame)
		}
	}
	return nil
}

func expectedRunnerHelpLines() []string {
	commands := runner.NewCommands(runner.Config{})
	lines := make([]string, 0, 5)
	for _, cmd := range commands {
		lines = append(lines, expectedHelpLine(cmd.Name, cmd.Usage))
		for _, sub := range cmd.Subcommands {
			lines = append(lines, expectedHelpLine(cmd.Name+" "+sub.Name, sub.Usage))
			for _, alias := range cmd.Aliases {
				lines = append(lines, expectedHelpLine(alias+" "+sub.Name, sub.Usage))
			}
		}
	}
	return lines
}

func expectedHelpLine(name, usage string) string {
	padding := "  "
	if pad := 14 - len(name); pad > 0 {
		padding = strings.Repeat(" ", pad)
	}
	return "  " + name + padding + usage + "\r\n"
}

func runTerminalServiceAsync(t *testing.T, factory *terminalFakeFactory, strm *terminalStream) <-chan error {
	// Start the terminal service asynchronously and wait for its initial prompt.
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- cli_plugin.NewTerminalService(factory).RunCli(strm)
	}()
	waitForTerminalOutput(t, strm, "spacewave> ")
	return done
}

func waitForTerminalService(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunCli: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunCli did not return")
	}
}

func waitForTerminalOutput(t *testing.T, strm *terminalStream, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		strm.mu.Lock()
		out := terminalOutputLocked(strm)
		notify := strm.sentNotify
		strm.mu.Unlock()
		if strings.Contains(out, want) {
			return
		}
		select {
		case <-notify:
		case <-deadline:
			t.Fatalf("expected terminal output to contain %q; got %q", want, out)
		}
	}
}

func terminalOutputLocked(strm *terminalStream) string {
	var out strings.Builder
	for _, frame := range strm.sent {
		if frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT {
			out.Write(frame.GetData())
		}
	}
	return out.String()
}

func assertOutputFrameCountAtLeast(t *testing.T, strm *terminalStream, want int) {
	t.Helper()
	if got := len(terminalOutputFrames(strm)); got < want {
		t.Fatalf("output frame count = %d, want at least %d", got, want)
	}
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
	session    runner.Session
	mountErr   error
	blockMount bool
	closed     bool
}

func (c *terminalFakeClient) Close() {
	c.closed = true
}

func (c *terminalFakeClient) MountSession(ctx context.Context, idx uint32) (runner.Session, error) {
	if c.mountErr != nil {
		return nil, c.mountErr
	}
	if c.blockMount {
		<-ctx.Done()
		return nil, ctx.Err()
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
		resources: &terminalFakeResourcesListStream{responses: []*s4wave_session.WatchResourcesListResponse{
			terminalSpaceListResponse(id, name),
		}},
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
	s.resources.ctx = ctx
	return s.resources, nil
}

func (s *terminalFakeSession) WatchLockState(ctx context.Context) (runner.LockStateStream, error) {
	return s.lock, nil
}

func (s *terminalFakeSession) WatchRecoveryStatus(ctx context.Context) (*s4wave_status.RecoveryStatus, error) {
	return nil, nil
}

func terminalSpaceListResponse(id, name string) *s4wave_session.WatchResourcesListResponse {
	return &s4wave_session.WatchResourcesListResponse{
		SpacesList: []*space_pb.SpaceSoListEntry{{
			Entry: &sobject_pb.SharedObjectListEntry{Ref: &sobject_pb.SharedObjectRef{
				ProviderResourceRef: &provider_pb.ProviderResourceRef{Id: id},
			}},
			SpaceMeta: &space_pb.SpaceSoMeta{Name: name},
		}},
	}
}

func terminalSpacesListResponse(count int) *s4wave_session.WatchResourcesListResponse {
	resp := &s4wave_session.WatchResourcesListResponse{}
	for i := range count {
		resp.SpacesList = append(resp.SpacesList, &space_pb.SpaceSoListEntry{
			Entry: &sobject_pb.SharedObjectListEntry{Ref: &sobject_pb.SharedObjectRef{
				ProviderResourceRef: &provider_pb.ProviderResourceRef{Id: "space-" + zeroPad(i, 4)},
			}},
			SpaceMeta: &space_pb.SpaceSoMeta{Name: "Space " + zeroPad(i, 3) + " with enough metadata to force JSON chunking"},
		})
	}
	return resp
}

func zeroPad(value, width int) string {
	raw := strconv.Itoa(value)
	if len(raw) >= width {
		return raw
	}
	return strings.Repeat("0", width-len(raw)) + raw
}

func newBlockingResourcesListStream(responses ...*s4wave_session.WatchResourcesListResponse) *terminalFakeResourcesListStream {
	return &terminalFakeResourcesListStream{
		responses:           responses,
		blockAfterResponses: true,
	}
}

type terminalFakeResourcesListStream struct {
	responses           []*s4wave_session.WatchResourcesListResponse
	idx                 int
	ctx                 context.Context
	blockAfterResponses bool
	closed              bool
}

func (s *terminalFakeResourcesListStream) Close() error {
	s.closed = true
	return nil
}

func (s *terminalFakeResourcesListStream) Recv() (*s4wave_session.WatchResourcesListResponse, error) {
	if s.idx >= len(s.responses) {
		if s.blockAfterResponses {
			<-s.ctx.Done()
			return nil, s.ctx.Err()
		}
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

var (
	_ s4wave_cli_terminal.SRPCCliTerminalService_RunCliStream = (*terminalStream)(nil)
	_ runner.ClientFactory                                    = (*terminalFakeFactory)(nil)
	_ runner.Client                                           = (*terminalFakeClient)(nil)
	_ runner.Session                                          = (*terminalFakeSession)(nil)
)
