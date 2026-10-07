//go:build !js

package remoteshell

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/sirupsen/logrus"
)

func TestRemoteShellSessionStopsBeforeOpenWhenContextCanceled(t *testing.T) {
	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a cancelable remote shell session before any opening request.
	ctx, cancel := context.WithCancel(context.Background())

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	started := false
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			ctx,
			logrus.NewEntry(logrus.New()),
			serverSession,
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				started = true
				return nil, errors.New("unexpected start")
			},
		)
	}()

	// Cancel the remote shell session before the client sends an opening frame.
	cancel()

	// Verify that the remote shell session ends with the expected result.
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("remote shell session error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}

	// Verify cancellation left the process starter untouched.
	if started {
		t.Fatal("process started before OPEN")
	}
}

func TestRemoteShellSessionForwardsInputResizeAndClose(t *testing.T) {
	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	proc := newFakeRemoteShellProcess()
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			serverSession,
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				return proc, nil
			},
		)
	}()

	// Request a remote shell through the terminal opening frame.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
		Cols: 120,
		Rows: 40,
	}); err != nil {
		t.Fatal(err)
	}

	// Receive the shell readiness response from the server.
	ready := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(ready); err != nil {
		t.Fatal(err)
	}

	// Verify that the requested shell reached its ready state.
	if ready.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY {
		t.Fatalf("ready kind = %s", ready.GetKind().String())
	}

	// Send terminal input to the remote shell process.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
		Data: []byte("whoami\n"),
	}); err != nil {
		t.Fatal(err)
	}

	// Resize the remote shell process through the terminal protocol.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE,
		Cols: 100,
		Rows: 30,
	}); err != nil {
		t.Fatal(err)
	}

	// Ask the remote shell to close its process.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE,
	}); err != nil {
		t.Fatal(err)
	}

	// Receive the shell process exit status from the server.
	exitFrame := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(exitFrame); err != nil {
		t.Fatal(err)
	}

	// Verify that the server delivered a terminal exit frame.
	if exitFrame.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT {
		t.Fatalf("exit kind = %s", exitFrame.GetKind().String())
	}

	// Verify that the remote shell session ends with the expected result.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("remote shell session error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}

	// Verify the shell process received the requested input and size.
	if got := proc.input.String(); got != "whoami\n" {
		t.Fatalf("input = %q", got)
	}
	if proc.cols != 100 || proc.rows != 30 {
		t.Fatalf("resize = %dx%d", proc.cols, proc.rows)
	}

	// Verify the terminal close request closed the shell process.
	if !proc.closed {
		t.Fatal("process was not closed")
	}
}

func TestRemoteShellSessionSendsExitWhenOutputReadEndsBeforeWait(t *testing.T) {
	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	proc := newFakeRemoteShellProcess()
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			serverSession,
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				return proc, nil
			},
		)
	}()

	// Request a remote shell through the terminal opening frame.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
	}); err != nil {
		t.Fatal(err)
	}

	// Receive the shell readiness response from the server.
	ready := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(ready); err != nil {
		t.Fatal(err)
	}

	// Verify that the requested shell reached its ready state.
	if ready.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY {
		t.Fatalf("ready kind = %s", ready.GetKind().String())
	}

	// End shell output before the process wait completes.
	proc.closeOutput()

	// Receive the shell process exit status from the server.
	exitFrame := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(exitFrame); err != nil {
		t.Fatal(err)
	}

	// Verify that the server delivered a terminal exit frame.
	if exitFrame.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT {
		t.Fatalf("exit kind = %s", exitFrame.GetKind().String())
	}

	// Verify that the remote shell session ends with the expected result.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("remote shell session error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}

	// Verify ending shell output also closes the process.
	if !proc.closed {
		t.Fatal("process was not closed")
	}
}
