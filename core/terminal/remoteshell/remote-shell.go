//go:build !js

package remoteshell

import (
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/creack/pty"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/sirupsen/logrus"
)

// deviceRemoteShellControllerID identifies the remote shell controller.
const deviceRemoteShellControllerID = "spacewave/device/remote-shell"

// deviceRemoteShellFrameMaxBytes bounds one terminal frame on the stream.
const deviceRemoteShellFrameMaxBytes = 4 * 1024 * 1024

// deviceRemoteShellControllerVersion is the remote shell controller version.
var deviceRemoteShellControllerVersion = controller.MustParseVersion("0.0.1")

// remoteShellPolicy refuses an OPEN request the device policy does not permit.
type remoteShellPolicy func(*s4wave_terminal.TerminalFrame) error

// remoteShellAuthorizer refuses a remote peer that may not open a shell. It
// returns the stream the session uses, also on refusal, so the refusal can be
// reported over it.
type remoteShellAuthorizer func(ctx context.Context, ms link.MountedStream) (stream.Stream, error)

// remoteShellProcess is a running shell attached to a terminal.
type remoteShellProcess interface {
	io.Reader
	io.Writer

	// Resize sets the terminal size.
	Resize(cols, rows uint32) error
	// Close stops the shell.
	Close() error
	// Wait returns the shell's exit code once it exits.
	Wait() (int, error)
}

// remoteShellStarter starts the shell an OPEN request describes.
type remoteShellStarter func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error)

// remoteShellOpenResult carries the received OPEN frame or the receive error.
type remoteShellOpenResult struct {
	// frame is the received terminal frame.
	frame *s4wave_terminal.TerminalFrame
	// err is why the frame could not be received.
	err error
}

// StartHandler registers the daemon-side remote-shell stream handler. Only an
// active session of the local Session's account may open a shell.
func StartHandler(ctx context.Context, le *logrus.Entry, b bus.Bus, policyStore *device_policy.PolicyStore) func() {
	// Require a controller bus before registering the remote shell handler.
	if b == nil {
		return func() {}
	}

	// Configure the remote shell controller with device policy and PTY startup.
	ctrl := &deviceRemoteShellController{
		le:        le.WithField("controller", deviceRemoteShellControllerID),
		b:         b,
		authorize: authorizeAccountSessionPeer(b),
		policy:    resolveRemoteShellPolicy(policyStore),
		starter:   startPtyRemoteShell,
	}

	// Attach the remote shell controller for the supplied context lifetime.
	release, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		le.WithError(err).Warn("device remote-shell handler unavailable")
		return func() {}
	}
	return release
}

// deviceRemoteShellController resolves remote shell stream handlers.
type deviceRemoteShellController struct {
	// le is the logger.
	le *logrus.Entry
	// b is the bus carrying remote shell streams.
	b bus.Bus
	// authorize admits the remote peer before the session starts.
	authorize remoteShellAuthorizer
	// policy admits the OPEN request.
	policy remoteShellPolicy
	// starter starts the shell process.
	starter remoteShellStarter
}

// GetControllerInfo returns the controller info.
func (c *deviceRemoteShellController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		deviceRemoteShellControllerID,
		deviceRemoteShellControllerVersion,
		"device remote shell controller",
	)
}

// Execute returns immediately; the controller only resolves directives.
func (c *deviceRemoteShellController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective resolves stream handlers for the remote shell protocol.
func (c *deviceRemoteShellController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(link.HandleMountedStream)
	if !ok {
		return nil, nil
	}
	if dir.HandleMountedStreamProtocolID() != s4wave_terminal.RemoteShellProtocolID {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]link.MountedStreamHandler{&deviceRemoteShellHandler{
		le:        c.le,
		b:         c.b,
		authorize: c.authorize,
		policy:    c.policy,
		starter:   c.starter,
	}})), nil
}

// Close releases nothing; the controller holds no resources.
func (c *deviceRemoteShellController) Close() error {
	return nil
}

// deviceRemoteShellHandler runs one remote shell session per stream.
type deviceRemoteShellHandler struct {
	// le is the logger.
	le *logrus.Entry
	// b is the bus holding the peer link.
	b bus.Bus
	// authorize admits the remote peer before the session starts.
	authorize remoteShellAuthorizer
	// policy admits the OPEN request.
	policy remoteShellPolicy
	// starter starts the shell process.
	starter remoteShellStarter
}

// HandleMountedStream refuses an unauthorized peer, then runs the session.
func (h *deviceRemoteShellHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	go func() {
		// Admit the stream, frame it, and close it when the session ends.
		strm, err := h.authorize(ctx, ms)
		defer strm.Close()
		session := stream_packet.NewSession(strm, deviceRemoteShellFrameMaxBytes)

		// Refuse a peer outside the account before touching the link.
		if err != nil {
			h.le.WithError(err).WithField("remote-peer", ms.GetPeerID().String()).Warn("remote shell peer refused")
			_ = sendTerminalError(session, "remote shell refused: "+err.Error())
			return
		}

		// Retain the peer link while its remote shell session runs.
		_, elRef, err := h.b.AddDirective(
			link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()),
			nil,
		)
		if err != nil {
			h.le.WithError(err).Warn("remote shell link hold failed")
			return
		}
		defer elRef.Release()

		// Run the terminal protocol until the shell or stream ends.
		if err := runRemoteShellSession(ctx, h.le, session, h.policy, h.starter); err != nil && ctx.Err() == nil {
			h.le.WithError(err).Warn("remote shell session stopped")
		}
	}()
	return nil
}

// runRemoteShellSession admits the OPEN request and runs the shell over session.
func runRemoteShellSession(
	ctx context.Context,
	le *logrus.Entry,
	session *stream_packet.Session,
	policy remoteShellPolicy,
	starter remoteShellStarter,
) error {
	// Receive the terminal opening request before evaluating device policy.
	openFrame, err := receiveRemoteShellOpenFrame(ctx, session)
	if err != nil {
		return errors.Wrap(err, "receive terminal open frame")
	}
	if openFrame.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN {
		return sendTerminalError(session, "expected terminal OPEN frame")
	}

	// Require device policy to permit the requested remote shell.
	if policy != nil {
		if err := policy(openFrame); err != nil {
			return sendTerminalError(session, err.Error())
		}
	}

	// Require a process starter for the remote shell request.
	if starter == nil {
		return sendTerminalError(session, "remote shell starter unavailable")
	}

	// Start the shell process and close it when the session ends.
	proc, err := starter(ctx, openFrame)
	if err != nil {
		return sendTerminalError(session, err.Error())
	}
	defer proc.Close()

	// Notify the terminal client that the shell is ready.
	if err := session.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY,
	}); err != nil {
		return err
	}

	// Run shell input, output, and process completion under one session context.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 3)
	go pumpRemoteShellOutput(ctx, session, proc, errCh)
	go waitRemoteShellProcess(session, proc, errCh)
	go receiveRemoteShellInput(ctx, session, proc, errCh)

	// End the shell session on process completion, stream failure, or cancellation.
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

// receiveRemoteShellOpenFrame receives the first frame or returns when ctx ends.
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

// pumpRemoteShellOutput copies shell output to the session until the process
// output ends.
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

// waitRemoteShellProcess reports the shell exit to the session.
func waitRemoteShellProcess(
	session *stream_packet.Session,
	proc remoteShellProcess,
	errCh chan<- error,
) {
	// Collect the shell process exit code and any failure detail.
	exitCode, err := proc.Wait()
	exitErr := ""
	if err != nil {
		exitErr = err.Error()
	}

	// Deliver the process exit status to the terminal client.
	if serr := session.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind:     s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT,
		ExitCode: remoteShellExitCode(exitCode),
		Error:    exitErr,
	}); serr != nil {
		errCh <- serr
		return
	}

	// Complete the shell session with the process result.
	errCh <- err
}

// receiveRemoteShellInput applies input and resize frames to the shell.
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

// sendTerminalError reports msg to the client and returns it as an error.
func sendTerminalError(session *stream_packet.Session, msg string) error {
	if err := session.SendMsg(&s4wave_terminal.TerminalFrame{
		Kind:  s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR,
		Error: msg,
	}); err != nil {
		return err
	}
	return errors.New(msg)
}

// authorizeAccountSessionPeer admits a remote peer that the local Session's
// transport authorizes: an active session of the same account. The transport
// closes the shell's stream if it later refuses the peer. A stream whose local
// peer runs no Session transport on b is refused.
func authorizeAccountSessionPeer(b bus.Bus) remoteShellAuthorizer {
	return func(ctx context.Context, ms link.MountedStream) (stream.Stream, error) {
		// Resolve the transport of the Session the stream reached.
		st, release, err := transport.ResolveSessionTransport(ctx, b, ms.GetLink().GetLocalPeer(), nil)
		if err != nil {
			return ms.GetStream(), err
		}
		defer release()
		if st == nil {
			return ms.GetStream(), errors.New("no session transport for the local peer")
		}
		return st.AdmitStream(ctx, ms)
	}
}

// resolveRemoteShellPolicy admits OPEN requests while the device policy enables
// the remote shell.
func resolveRemoteShellPolicy(store *device_policy.PolicyStore) remoteShellPolicy {
	return func(openFrame *s4wave_terminal.TerminalFrame) error {
		policy := store.Snapshot()
		if !policy.GetRemoteShell().GetEnabled() {
			detail := policy.GetRemoteShell().GetDetail()
			if detail == "" {
				detail = "terminal disabled by local policy"
			}
			return errors.New(detail)
		}
		return nil
	}
}

// startPtyRemoteShell starts the requested shell in a PTY.
func startPtyRemoteShell(ctx context.Context, openFrame *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
	// Start the requested shell in a PTY sized for the terminal client.
	cmd := buildRemoteShellCommand(ctx, openFrame)
	cols, rows := s4wave_terminal.NormalizeTerminalFrameSize(openFrame.GetCols(), openFrame.GetRows())
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: ptyDimension(cols),
		Rows: ptyDimension(rows),
	})
	if err != nil {
		return nil, err
	}
	return &ptyRemoteShellProcess{cmd: cmd, ptmx: ptmx}, nil
}

// buildRemoteShellCommand builds the shell command an OPEN request describes.
func buildRemoteShellCommand(ctx context.Context, openFrame *s4wave_terminal.TerminalFrame) *exec.Cmd {
	// Select the shell and platform arguments for the requested command.
	shell := defaultRemoteShell()
	command := openFrame.GetCommand()
	args := []string{}
	if command != "" && runtime.GOOS == "windows" {
		args = []string{"/C", command}
	}
	if command != "" && runtime.GOOS != "windows" {
		args = []string{"-lc", command}
	}

	// Bind the shell command to cancellation and the requested environment.
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Env = append(os.Environ(), openFrame.GetEnvironment()...)
	return cmd
}

// defaultRemoteShell returns the platform's interactive shell.
func defaultRemoteShell() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

// ptyRemoteShellProcess is a shell process attached to a local PTY.
type ptyRemoteShellProcess struct {
	// cmd is the running shell.
	cmd *exec.Cmd
	// ptmx is the PTY master attached to the shell.
	ptmx *os.File

	// closeOnce guards closeErr, which holds the PTY close result.
	closeOnce sync.Once
	closeErr  error
}

// Read reads shell output from the PTY.
func (p *ptyRemoteShellProcess) Read(buf []byte) (int, error) {
	return p.ptmx.Read(buf)
}

// Write writes shell input to the PTY.
func (p *ptyRemoteShellProcess) Write(buf []byte) (int, error) {
	return p.ptmx.Write(buf)
}

// Resize sets the PTY window size.
func (p *ptyRemoteShellProcess) Resize(cols, rows uint32) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: ptyDimension(cols), Rows: ptyDimension(rows)})
}

// remoteShellExitCode clamps a process exit code to the protocol's int32.
func remoteShellExitCode(value int) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value) //nolint:gosec // the explicit int32 bounds preserve the terminal protocol's signed exit code.
}

// ptyDimension clamps a terminal dimension to the kernel's uint16 field.
func ptyDimension(value uint32) uint16 {
	if value > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(value) //nolint:gosec // the explicit MaxUint16 bound protects the kernel pty field.
}

// Close closes the PTY and kills the shell once.
// Every call returns the first close result.
func (p *ptyRemoteShellProcess) Close() error {
	p.closeOnce.Do(func() {
		// Close the PTY and terminate its attached shell process.
		p.closeErr = p.ptmx.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
	return p.closeErr
}

// Wait waits for the shell to exit and returns its exit code.
func (p *ptyRemoteShellProcess) Wait() (int, error) {
	// Report a clean exit without an error.
	err := p.cmd.Wait()
	if err == nil {
		return 0, nil
	}

	// Report the process exit code when the shell exited unsuccessfully.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), err
	}
	return -1, err
}

// _ is a type assertion
var (
	_ controller.Controller     = (*deviceRemoteShellController)(nil)
	_ link.MountedStreamHandler = (*deviceRemoteShellHandler)(nil)
)
