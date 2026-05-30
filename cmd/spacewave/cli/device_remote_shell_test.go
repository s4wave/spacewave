//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/sirupsen/logrus"
)

func TestRemoteShellSessionDeniesPolicyBeforeStartingProcess(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

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

	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
	}); err != nil {
		t.Fatal(err)
	}
	got := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(got); err != nil {
		t.Fatal(err)
	}
	if got.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR {
		t.Fatalf("response kind = %s", got.GetKind().String())
	}
	if got.GetError() != "terminal disabled by local policy" {
		t.Fatalf("error = %q", got.GetError())
	}
	if started {
		t.Fatal("process started despite policy denial")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected policy denial error")
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}
}

func TestRemoteShellSessionForwardsInputResizeAndClose(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

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

	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
		Cols: 120,
		Rows: 40,
	}); err != nil {
		t.Fatal(err)
	}
	ready := &s4wave_terminal.TerminalFrame{}
	if err := clientSession.RecvMsg(ready); err != nil {
		t.Fatal(err)
	}
	if ready.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY {
		t.Fatalf("ready kind = %s", ready.GetKind().String())
	}

	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
		Data: []byte("whoami\n"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE,
		Cols: 100,
		Rows: 30,
	}); err != nil {
		t.Fatal(err)
	}
	if err := clientSession.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("remote shell session error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote shell session did not stop")
	}

	if got := proc.input.String(); got != "whoami\n" {
		t.Fatalf("input = %q", got)
	}
	if proc.cols != 100 || proc.rows != 30 {
		t.Fatalf("resize = %dx%d", proc.cols, proc.rows)
	}
	if !proc.closed {
		t.Fatal("process was not closed")
	}
}

type fakeRemoteShellProcess struct {
	input  bytes.Buffer
	readCh chan []byte
	done   chan struct{}
	once   sync.Once
	cols   uint32
	rows   uint32
	closed bool
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
	p.once.Do(func() {
		p.closed = true
		close(p.done)
		close(p.readCh)
	})
	return nil
}

func (p *fakeRemoteShellProcess) Wait() (int, error) {
	<-p.done
	return 0, nil
}
