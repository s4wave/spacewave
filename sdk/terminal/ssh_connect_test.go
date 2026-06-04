//go:build !js

package s4wave_terminal_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/s4wave/spacewave/testbed"
	"golang.org/x/crypto/ssh"
)

func TestConnectTerminalOpensSshHostSession(t *testing.T) {
	ctx := t.Context()
	tb, soProvider, release := setupTerminalSecretTest(ctx, t)
	defer release()

	addr, hostKey, closeServer := startTerminalTestSSHServer(t, "ssh-password")
	defer closeServer()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	portNum, err := net.LookupPort("tcp", port)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/ssh/password",
		DisplayName: "SSH password",
		Kind:        s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType,
		Value:       []byte("ssh-password"),
		Timestamp:   time.Unix(10, 0),
	}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	hostOp := s4wave_sshhost.NewCreateSshHostOp(
		"hosts/prod",
		"Prod SSH",
		&s4wave_sshhost.SshHostEndpoint{
			Host:     host,
			Port:     uint32(portNum),
			Username: "deploy",
		},
		&s4wave_sshhost.SshHostCredentialRefs{
			PasswordSecretObjectKey: "secrets/ssh/password",
		},
		[]*s4wave_sshhost.SshHostKeyPin{{
			Algorithm:         hostKey.Type(),
			Sha256Fingerprint: ssh.FingerprintSHA256(hostKey),
		}},
		time.Unix(11, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, hostOp, tb.Volume.GetPeerID()); err != nil {
		t.Fatalf("create SSH Host: %v", err)
	}

	termOp := s4wave_terminal.NewCreateSshHostTerminalOp(
		"terminal/prod-ssh",
		"Prod SSH Terminal",
		"hosts/prod",
		time.Unix(12, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, termOp, tb.Volume.GetPeerID()); err != nil {
		t.Fatalf("create Terminal: %v", err)
	}
	objState, found, err := tb.WorldState.GetObject(ctx, "terminal/prod-ssh")
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

	streamCtx, cancelStream := context.WithTimeout(ctx, 5*time.Second)
	defer cancelStream()
	strm := newBlockingTerminalConnectStream(streamCtx)
	strm.sendDelay = 10 * time.Millisecond
	err = s4wave_terminal.NewTerminalResource(tb.Bus, tb.WorldState, tb.Engine, "terminal/prod-ssh", state).
		ConnectTerminal(strm)
	strm.closeRecv()
	if err != nil {
		t.Fatalf("ConnectTerminal: %v", err)
	}

	frames := strm.sentFrames()
	if !terminalFramesContainKind(frames, s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY) {
		t.Fatalf("sent frames missing READY: %#v", frames)
	}
	if !terminalFramesContainOutput(frames, "ssh ready\n") {
		t.Fatalf("sent frames missing SSH output: %#v", frames)
	}
	if !terminalFramesContainKind(frames, s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT) {
		t.Fatalf("sent frames missing EXIT: %#v", frames)
	}

	updated, err := readTerminalObject(ctx, objState)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetState() != s4wave_terminal.TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED {
		t.Fatalf("state = %s", updated.GetState().String())
	}
	if updated.GetStatus() != "exited" {
		t.Fatalf("status = %q", updated.GetStatus())
	}
}

func TestConnectTerminalPromptsAndRemembersUnknownSshHostKey(t *testing.T) {
	ctx := t.Context()
	tb, soProvider, release := setupTerminalSecretTest(ctx, t)
	defer release()

	addr, _, closeServer := startTerminalTestSSHServer(t, "ssh-password")
	defer closeServer()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	portNum, err := net.LookupPort("tcp", port)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/ssh/password",
		DisplayName: "SSH password",
		Kind:        s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType,
		Value:       []byte("ssh-password"),
		Timestamp:   time.Unix(10, 0),
	}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	hostOp := s4wave_sshhost.NewCreateSshHostOp(
		"hosts/prod",
		"Prod SSH",
		&s4wave_sshhost.SshHostEndpoint{
			Host:     host,
			Port:     uint32(portNum),
			Username: "deploy",
		},
		&s4wave_sshhost.SshHostCredentialRefs{
			PasswordSecretObjectKey: "secrets/ssh/password",
		},
		nil,
		time.Unix(11, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, hostOp, tb.Volume.GetPeerID()); err != nil {
		t.Fatalf("create SSH Host: %v", err)
	}

	termOp := s4wave_terminal.NewCreateSshHostTerminalOp(
		"terminal/prod-ssh",
		"Prod SSH Terminal",
		"hosts/prod",
		time.Unix(12, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, termOp, tb.Volume.GetPeerID()); err != nil {
		t.Fatalf("create Terminal: %v", err)
	}
	objState, found, err := tb.WorldState.GetObject(ctx, "terminal/prod-ssh")
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

	streamCtx, cancelStream := context.WithTimeout(ctx, 5*time.Second)
	defer cancelStream()
	strm := newBlockingTerminalConnectStream(streamCtx)
	strm.autoTrustHostKey = true
	err = s4wave_terminal.NewTerminalResource(tb.Bus, tb.WorldState, tb.Engine, "terminal/prod-ssh", state).
		ConnectTerminal(strm)
	strm.closeRecv()
	if err != nil {
		t.Fatalf("ConnectTerminal: %v", err)
	}

	frames := strm.sentFrames()
	if !terminalFramesContainOutputSubstring(frames, "Trust this host key and continue connecting?") {
		t.Fatalf("sent frames missing trust prompt: %#v", frames)
	}
	if !terminalFramesContainOutput(frames, "ssh ready\n") {
		t.Fatalf("sent frames missing SSH output: %#v", frames)
	}

	sshHostObj, found, err := tb.WorldState.GetObject(ctx, "hosts/prod")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("SSH Host object was not created")
	}
	sshHost, err := readSshHostObject(ctx, sshHostObj)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sshHost.GetHostKeyPins()); got != 1 {
		t.Fatalf("remembered host key pins = %d, want 1", got)
	}
	if sshHost.GetHostKeyPins()[0].GetSha256Fingerprint() == "" {
		t.Fatal("remembered host key missing SHA256 fingerprint")
	}
}

func setupTerminalSecretTest(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, sobject.SharedObjectProvider, func()) {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	accountID := "terminal-test-" + sobject.NewSOOperationLocalID()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	return tb, soProvider, func() {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
}

func startTerminalTestSSHServer(t *testing.T, password string) (string, ssh.PublicKey, func()) {
	t.Helper()
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if conn.User() == "deploy" && string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("unauthorized")
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				handleTerminalTestSSHConn(conn, config)
			})
		}
	}()
	return listener.Addr().String(), signer.PublicKey(), func() {
		_ = listener.Close()
		<-done
		wg.Wait()
	}
}

func handleTerminalTestSSHConn(conn net.Conn, config *ssh.ServerConfig) {
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(reqs)
	for next := range chans {
		if next.ChannelType() != "session" {
			_ = next.Reject(ssh.UnknownChannelType, "unsupported channel")
			continue
		}
		ch, requests, err := next.Accept()
		if err != nil {
			continue
		}
		go handleTerminalTestSSHSession(ch, requests)
	}
}

func handleTerminalTestSSHSession(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer ch.Close()
	for req := range requests {
		switch req.Type {
		case "pty-req":
			_ = req.Reply(true, nil)
		case "window-change":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "exec", "shell":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			_, _ = ch.Write([]byte("ssh ready\n"))
			time.Sleep(25 * time.Millisecond)
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

type blockingTerminalConnectStream struct {
	ctx              context.Context
	recv             chan *s4wave_terminal.TerminalFrame
	sendDelay        time.Duration
	autoTrustHostKey bool
	sendActive       atomic.Bool
	sentMu           sync.Mutex
	sent             []*s4wave_terminal.TerminalFrame
}

func newBlockingTerminalConnectStream(ctx context.Context) *blockingTerminalConnectStream {
	return &blockingTerminalConnectStream{
		ctx:  ctx,
		recv: make(chan *s4wave_terminal.TerminalFrame),
	}
}

func (s *blockingTerminalConnectStream) Context() context.Context {
	return s.ctx
}

func (s *blockingTerminalConnectStream) MsgSend(msg srpc.Message) error {
	frame, ok := msg.(*s4wave_terminal.TerminalFrame)
	if ok {
		return s.Send(frame)
	}
	return nil
}

func (s *blockingTerminalConnectStream) MsgRecv(msg srpc.Message) error {
	frame, ok := msg.(*s4wave_terminal.TerminalFrame)
	if !ok {
		return nil
	}
	return s.RecvTo(frame)
}

func (s *blockingTerminalConnectStream) CloseSend() error {
	return nil
}

func (s *blockingTerminalConnectStream) Close() error {
	return nil
}

func (s *blockingTerminalConnectStream) Send(frame *s4wave_terminal.TerminalFrame) error {
	if !s.sendActive.CompareAndSwap(false, true) {
		return errors.New("concurrent terminal Send")
	}
	defer s.sendActive.Store(false)
	if s.sendDelay != 0 {
		time.Sleep(s.sendDelay)
	}
	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	s.sent = append(s.sent, frame.CloneVT())
	if s.autoTrustHostKey &&
		frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT &&
		strings.Contains(string(frame.GetData()), "Trust this host key and continue connecting?") {
		go func() {
			s.recv <- &s4wave_terminal.TerminalFrame{
				Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
				Data: []byte("y"),
			}
		}()
	}
	return nil
}

func (s *blockingTerminalConnectStream) SendAndClose(frame *s4wave_terminal.TerminalFrame) error {
	if frame != nil {
		return s.Send(frame)
	}
	return nil
}

func (s *blockingTerminalConnectStream) Recv() (*s4wave_terminal.TerminalFrame, error) {
	select {
	case frame, ok := <-s.recv:
		if !ok {
			return nil, io.EOF
		}
		return frame, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *blockingTerminalConnectStream) RecvTo(frame *s4wave_terminal.TerminalFrame) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*frame = *next
	return nil
}

func (s *blockingTerminalConnectStream) closeRecv() {
	close(s.recv)
}

func (s *blockingTerminalConnectStream) sentFrames() []*s4wave_terminal.TerminalFrame {
	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	return append([]*s4wave_terminal.TerminalFrame(nil), s.sent...)
}

func terminalFramesContainKind(frames []*s4wave_terminal.TerminalFrame, kind s4wave_terminal.TerminalFrameKind) bool {
	for _, frame := range frames {
		if frame.GetKind() == kind {
			return true
		}
	}
	return false
}

func terminalFramesContainOutput(frames []*s4wave_terminal.TerminalFrame, output string) bool {
	for _, frame := range frames {
		if frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT && string(frame.GetData()) == output {
			return true
		}
	}
	return false
}

func terminalFramesContainOutputSubstring(frames []*s4wave_terminal.TerminalFrame, output string) bool {
	for _, frame := range frames {
		if frame.GetKind() == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT && strings.Contains(string(frame.GetData()), output) {
			return true
		}
	}
	return false
}

func readTerminalObject(ctx context.Context, objState world.ObjectState) (*s4wave_terminal.Terminal, error) {
	var state *s4wave_terminal.Terminal
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = s4wave_terminal.UnmarshalTerminal(ctx, bcs)
		return uerr
	})
	return state, err
}

func readSshHostObject(ctx context.Context, objState world.ObjectState) (*s4wave_sshhost.SshHost, error) {
	var state *s4wave_sshhost.SshHost
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = s4wave_sshhost.UnmarshalSshHost(ctx, bcs)
		return uerr
	})
	return state, err
}
