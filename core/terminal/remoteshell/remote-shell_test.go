//go:build !js

package remoteshell

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/sirupsen/logrus"
)

func TestRemoteShellSessionDeniesPolicyBeforeStartingProcess(t *testing.T) {
	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	started := false
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			serverSession,
			func(*s4wave_terminal.TerminalFrame) error {
				return errors.New("terminal disabled by local policy")
			},
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				started = true
				return nil, errors.New("unexpected start")
			},
		)
	}()

	// Request a remote shell through the terminal opening frame.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
	}); err != nil {
		t.Fatal(err)
	}

	// Receive the terminal response to the remote shell request.
	got := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(got); err != nil {
		t.Fatal(err)
	}

	// Verify the terminal response reports the device policy denial.
	if got.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR {
		t.Fatalf("response kind = %s", got.GetKind().String())
	}
	if got.GetError() != "terminal disabled by local policy" {
		t.Fatalf("error = %q", got.GetError())
	}
	if started {
		t.Fatal("process started despite policy denial")
	}

	// Verify that the remote shell session ends with the expected result.
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected policy denial error")
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}
}

func TestRemoteShellPolicyStoreMissingPolicyDeniesBeforeStartingProcess(t *testing.T) {
	// Load the device policy that controls remote shell startup.
	store, err := device_policy.NewPolicyStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}

	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			serverSession,
			resolveRemoteShellPolicy(store),
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				started <- struct{}{}
				return nil, errors.New("unexpected start")
			},
		)
	}()

	// Request a remote shell through the terminal opening frame.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
	}); err != nil {
		t.Fatal(err)
	}

	// Receive the terminal response to the remote shell request.
	got := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(got); err != nil {
		t.Fatal(err)
	}

	// Verify the terminal response reports the device policy denial.
	if got.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR {
		t.Fatalf("response kind = %s", got.GetKind().String())
	}
	if got.GetError() != "terminal disabled by local policy" {
		t.Fatalf("error = %q", got.GetError())
	}

	// Verify whether device policy allowed the process starter to run.
	select {
	case <-started:
		t.Fatal("process started despite missing policy denial")
	default:
	}

	// Verify that the remote shell session ends with the expected result.
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected missing policy denial error")
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}
}

func TestRemoteShellPolicyStoreAllowsEnabledPolicy(t *testing.T) {
	// Save an enabled device policy for the remote shell request.
	stateRoot := t.TempDir()
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{
		RemoteShell: &device_policy.RemoteShellPolicy{Enabled: true},
	}); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	// Load the device policy that controls remote shell startup.
	store, err := device_policy.NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}

	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	proc := newFakeRemoteShellProcess()
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			serverSession,
			resolveRemoteShellPolicy(store),
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				started <- struct{}{}
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

	// Verify whether device policy allowed the process starter to run.
	select {
	case <-started:
	default:
		t.Fatal("enabled policy did not start process")
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
}

func TestRemoteShellPolicyReloadBeforeOpenDeniesWithoutStartingProcess(t *testing.T) {
	// Save an enabled device policy for the remote shell request.
	stateRoot := t.TempDir()
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{
		RemoteShell: &device_policy.RemoteShellPolicy{Enabled: true},
	}); err != nil {
		t.Fatalf("write enabled policy: %v", err)
	}

	// Load the device policy that controls remote shell startup.
	store, err := device_policy.NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}

	// Connect the remote shell server and terminal client through an in-memory stream.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Run a remote shell session with the process starter under test.
	serverSession := stream_packet.NewSession(serverConn, deviceRemoteShellFrameMaxBytes)
	clientSession := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	started := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- runRemoteShellSession(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			serverSession,
			resolveRemoteShellPolicy(store),
			func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
				started <- struct{}{}
				return nil, errors.New("unexpected start")
			},
		)
	}()

	// Replace the device policy with a denial before the opening request.
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{
		RemoteShell: &device_policy.RemoteShellPolicy{
			Enabled: false,
			Detail:  "terminal disabled after reload",
		},
	}); err != nil {
		t.Fatalf("write disabled policy: %v", err)
	}

	// Reload the saved device policy before checking the opening request.
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// Request a remote shell through the terminal opening frame.
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
	}); err != nil {
		t.Fatal(err)
	}

	// Receive the terminal response to the remote shell request.
	got := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(got); err != nil {
		t.Fatal(err)
	}

	// Verify the terminal response reports the device policy denial.
	if got.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR {
		t.Fatalf("response kind = %s", got.GetKind().String())
	}
	if got.GetError() != "terminal disabled after reload" {
		t.Fatalf("error = %q", got.GetError())
	}

	// Verify whether device policy allowed the process starter to run.
	select {
	case <-started:
		t.Fatal("process started despite disabled reload before OPEN")
	default:
	}

	// Verify that the remote shell session ends with the expected result.
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected disabled policy denial error")
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}
}

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
			nil,
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
			nil,
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
			nil,
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

type fakeRemoteShellProcess struct {
	input     bytes.Buffer
	readCh    chan []byte
	done      chan struct{}
	closeOnce sync.Once
	readOnce  sync.Once
	cols      uint32
	rows      uint32
	closed    bool
}

func newFakeRemoteShellProcess() *fakeRemoteShellProcess {
	return &fakeRemoteShellProcess{
		readCh: make(chan []byte),
		done:   make(chan struct{}),
	}
}

func (p *fakeRemoteShellProcess) Read(buf []byte) (int, error) {
	data, ok := <-p.readCh
	if !ok {
		return 0, io.EOF
	}
	return copy(buf, data), nil
}

func (p *fakeRemoteShellProcess) Write(buf []byte) (int, error) {
	return p.input.Write(buf)
}

func (p *fakeRemoteShellProcess) Resize(cols, rows uint32) error {
	p.cols = cols
	p.rows = rows
	return nil
}

func (p *fakeRemoteShellProcess) Close() error {
	p.closeOnce.Do(func() {
		p.closed = true
		close(p.done)
		p.closeOutput()
	})
	return nil
}

func (p *fakeRemoteShellProcess) closeOutput() {
	p.readOnce.Do(func() {
		close(p.readCh)
	})
}

func (p *fakeRemoteShellProcess) Wait() (int, error) {
	<-p.done
	return 0, nil
}
