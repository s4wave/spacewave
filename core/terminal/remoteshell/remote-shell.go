//go:build !js

package remoteshell

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/creack/pty"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/sirupsen/logrus"
)

const deviceRemoteShellControllerID = "spacewave/device/remote-shell"

const deviceRemoteShellFrameMaxBytes = 4 * 1024 * 1024

var deviceRemoteShellControllerVersion = controller.MustParseVersion("0.0.1")

type remoteShellPolicy func(*s4wave_terminal.TerminalFrame) error

type remoteShellProcess interface {
	io.Reader
	io.Writer
	Resize(cols, rows uint32) error
	Close() error
	Wait() (int, error)
}

type remoteShellStarter func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error)

type remoteShellOpenResult struct {
	frame *s4wave_terminal.TerminalFrame
	err   error
}

// StartHandler registers the daemon-side remote-shell stream handler.
func StartHandler(ctx context.Context, le *logrus.Entry, b bus.Bus) func() {
	if b == nil {
		return func() {}
	}
	ctrl := &deviceRemoteShellController{
		le:      le.WithField("controller", deviceRemoteShellControllerID),
		b:       b,
		policy:  denyRemoteShellByDefault,
		starter: startPtyRemoteShell,
	}
	release, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		le.WithError(err).Warn("device remote-shell handler unavailable")
		return func() {}
	}
	return release
}

type deviceRemoteShellController struct {
	le      *logrus.Entry
	b       bus.Bus
	policy  remoteShellPolicy
	starter remoteShellStarter
}

func (c *deviceRemoteShellController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		deviceRemoteShellControllerID,
		deviceRemoteShellControllerVersion,
		"device remote shell controller",
	)
}

func (c *deviceRemoteShellController) Execute(ctx context.Context) error {
	return nil
}

func (c *deviceRemoteShellController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(link.HandleMountedStream)
	if !ok {
		return nil, nil
	}
	if dir.HandleMountedStreamProtocolID() != s4wave_terminal.RemoteShellProtocolID {
		return nil, nil
	}
	return directive.Resolvers(&deviceRemoteShellResolver{
		le:      c.le,
		b:       c.b,
		policy:  c.policy,
		starter: c.starter,
	}), nil
}

func (c *deviceRemoteShellController) Close() error {
	return nil
}

type deviceRemoteShellResolver struct {
	le      *logrus.Entry
	b       bus.Bus
	policy  remoteShellPolicy
	starter remoteShellStarter
}

func (r *deviceRemoteShellResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.AddValue(link.MountedStreamHandler(&deviceRemoteShellHandler{
		le:      r.le,
		b:       r.b,
		policy:  r.policy,
		starter: r.starter,
	}))
	return nil
}

type deviceRemoteShellHandler struct {
	le      *logrus.Entry
	b       bus.Bus
	policy  remoteShellPolicy
	starter remoteShellStarter
}

func (h *deviceRemoteShellHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	_, elRef, err := h.b.AddDirective(
		link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		defer elRef.Release()
		defer ms.GetStream().Close()
		session := stream_packet.NewSession(ms.GetStream(), deviceRemoteShellFrameMaxBytes)
		if err := runRemoteShellSession(ctx, h.le, session, h.policy, h.starter); err != nil && ctx.Err() == nil {
			h.le.WithError(err).Warn("remote shell session stopped")
		}
	}()
	return nil
}

func runRemoteShellSession(
	ctx context.Context,
	le *logrus.Entry,
	session *stream_packet.Session,
	policy remoteShellPolicy,
	starter remoteShellStarter,
) error {
	openFrame, err := receiveRemoteShellOpenFrame(ctx, session)
	if err != nil {
		return errors.Wrap(err, "receive terminal open frame")
	}
	if openFrame.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN {
		return sendTerminalError(session, "expected terminal OPEN frame")
	}
	if policy != nil {
		if err := policy(openFrame); err != nil {
			return sendTerminalError(session, err.Error())
		}
	}
	if starter == nil {
		return sendTerminalError(session, "remote shell starter unavailable")
	}

	proc, err := starter(ctx, openFrame)
	if err != nil {
		return sendTerminalError(session, err.Error())
	}
	defer proc.Close()

	if err := session.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY,
	}); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 3)
	go pumpRemoteShellOutput(ctx, session, proc, errCh)
	go waitRemoteShellProcess(session, proc, errCh)
	go receiveRemoteShellInput(ctx, session, proc, errCh)

	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = ctx.Err()
	}
	cancel()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		le.WithError(err).Debug("remote shell stream ended")
		return err
	}
	return nil
}

func receiveRemoteShellOpenFrame(
	ctx context.Context,
	session *stream_packet.Session,
) (*s4wave_terminal.TerminalFrame, error) {
	resultCh := make(chan remoteShellOpenResult, 1)
	go func() {
		frame := &s4wave_terminal.TerminalFrame{}
		resultCh <- remoteShellOpenResult{
			frame: frame,
			err:   session.RecvMsg(frame),
		}
	}()

	select {
	case result := <-resultCh:
		return result.frame, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func pumpRemoteShellOutput(
	ctx context.Context,
	session *stream_packet.Session,
	proc remoteShellProcess,
	errCh chan<- error,
) {
	buf := make([]byte, 8192)
	for {
		n, err := proc.Read(buf)
		if n > 0 {
			frame := &s4wave_terminal.TerminalFrame{
				Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT,
				Data: bytes.Clone(buf[:n]),
			}
			if serr := session.SendMsg(frame); serr != nil {
				errCh <- serr
				return
			}
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				errCh <- ctxErr
				return
			}
			_ = proc.Close()
			return
		}
		if err := ctx.Err(); err != nil {
			errCh <- err
			return
		}
	}
}

func waitRemoteShellProcess(
	session *stream_packet.Session,
	proc remoteShellProcess,
	errCh chan<- error,
) {
	exitCode, err := proc.Wait()
	if serr := session.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind:     s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT,
		ExitCode: int32(exitCode),
		Error:    errorString(err),
	}); serr != nil {
		errCh <- serr
		return
	}
	errCh <- err
}

func receiveRemoteShellInput(
	ctx context.Context,
	session *stream_packet.Session,
	proc remoteShellProcess,
	errCh chan<- error,
) {
	for {
		frame := &s4wave_terminal.TerminalFrame{}
		if err := session.RecvMsg(frame); err != nil {
			errCh <- err
			return
		}
		switch frame.GetKind() {
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT:
			if len(frame.GetData()) != 0 {
				if _, err := proc.Write(frame.GetData()); err != nil {
					errCh <- err
					return
				}
			}
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE:
			cols, rows := s4wave_terminal.NormalizeTerminalFrameSize(frame.GetCols(), frame.GetRows())
			if err := proc.Resize(cols, rows); err != nil {
				errCh <- err
				return
			}
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
			if err := proc.Close(); err != nil {
				errCh <- err
			}
			return
		default:
			errCh <- errors.Errorf("unsupported remote shell frame kind %s", frame.GetKind().String())
			return
		}
		if err := ctx.Err(); err != nil {
			errCh <- err
			return
		}
	}
}

func sendTerminalError(session *stream_packet.Session, msg string) error {
	if err := session.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind:  s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR,
		Error: msg,
	}); err != nil {
		return err
	}
	return errors.New(msg)
}

func denyRemoteShellByDefault(openFrame *s4wave_terminal.TerminalFrame) error {
	return errors.New("terminal disabled by local policy")
}

func startPtyRemoteShell(ctx context.Context, openFrame *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
	cmd := buildRemoteShellCommand(ctx, openFrame)
	cols, rows := s4wave_terminal.NormalizeTerminalFrameSize(openFrame.GetCols(), openFrame.GetRows())
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, err
	}
	return &ptyRemoteShellProcess{cmd: cmd, ptmx: ptmx}, nil
}

func buildRemoteShellCommand(ctx context.Context, openFrame *s4wave_terminal.TerminalFrame) *exec.Cmd {
	shell := defaultRemoteShell()
	command := openFrame.GetCommand()
	args := []string{}
	if command != "" && runtime.GOOS == "windows" {
		args = []string{"/C", command}
	}
	if command != "" && runtime.GOOS != "windows" {
		args = []string{"-lc", command}
	}
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Env = append(os.Environ(), openFrame.GetEnvironment()...)
	return cmd
}

func defaultRemoteShell() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

type ptyRemoteShellProcess struct {
	cmd  *exec.Cmd
	ptmx *os.File
	once sync.Once
}

func (p *ptyRemoteShellProcess) Read(buf []byte) (int, error) {
	return p.ptmx.Read(buf)
}

func (p *ptyRemoteShellProcess) Write(buf []byte) (int, error) {
	return p.ptmx.Write(buf)
}

func (p *ptyRemoteShellProcess) Resize(cols, rows uint32) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *ptyRemoteShellProcess) Close() error {
	var err error
	p.once.Do(func() {
		err = p.ptmx.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
	return err
}

func (p *ptyRemoteShellProcess) Wait() (int, error) {
	err := p.cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), err
	}
	return -1, err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// _ is a type assertion
var (
	_ controller.Controller     = (*deviceRemoteShellController)(nil)
	_ directive.Resolver        = (*deviceRemoteShellResolver)(nil)
	_ link.MountedStreamHandler = (*deviceRemoteShellHandler)(nil)
)
