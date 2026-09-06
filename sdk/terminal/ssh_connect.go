//go:build !js

package s4wave_terminal

import (
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/link"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	"golang.org/x/crypto/ssh"
)

const sshClientHandshakeTimeout = 30 * time.Second

type terminalFrameSend struct {
	frame *TerminalFrame
	ack   chan error
}

func (r *TerminalResource) connectSshHostTerminal(
	ctx context.Context,
	cancel context.CancelFunc,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	current *Terminal,
) error {
	if r.b == nil {
		return errors.New("terminal resource requires a bus to read SSH credentials")
	}
	if r.ws == nil {
		return errors.New("terminal resource requires world state to open SSH Host terminals")
	}
	if err := r.updateState(ctx, TerminalSessionState_TERMINAL_SESSION_STATE_CONNECTING, "connecting", ""); err != nil {
		return err
	}

	host, err := r.lookupSshHost(ctx, current.GetSshHostObjectKey())
	if err != nil {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "failed to connect", err.Error())
		return err
	}
	clientConfig, address, err := r.buildSshClientConfig(ctx, strm, current.GetSshHostObjectKey(), host)
	if err != nil {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "failed to connect", err.Error())
		return err
	}

	client, err := dialSshClient(ctx, address, clientConfig)
	if err != nil {
		state, status, errMessage := terminalConnectOpenFailureState(ctx, err, "failed to connect")
		_ = r.updateState(context.Background(), state, status, errMessage)
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		state, status, errMessage := terminalConnectOpenFailureState(ctx, err, "failed to open")
		_ = r.updateState(context.Background(), state, status, errMessage)
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "failed to open", err.Error())
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "failed to open", err.Error())
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "failed to open", err.Error())
		return err
	}

	cols, rows := NormalizeTerminalFrameSize(current.GetCols(), current.GetRows())
	if err := session.RequestPty("xterm-256color", int(rows), int(cols), ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		state, status, errMessage := terminalConnectOpenFailureState(ctx, err, "failed to open")
		_ = r.updateState(context.Background(), state, status, errMessage)
		return err
	}

	if command := current.GetCommand(); command != "" {
		err = session.Start(command)
	} else {
		err = session.Shell()
	}
	if err != nil {
		state, status, errMessage := terminalConnectOpenFailureState(ctx, err, "failed to open")
		_ = r.updateState(context.Background(), state, status, errMessage)
		return err
	}
	if err := r.updateState(ctx, TerminalSessionState_TERMINAL_SESSION_STATE_ACTIVE, "active", ""); err != nil {
		return err
	}
	if err := strm.Send(&TerminalFrame{Kind: TerminalFrameKind_TERMINAL_FRAME_KIND_READY}); err != nil {
		return err
	}

	errCh := make(chan terminalConnectResult, 4)
	terminalFrames := make(chan terminalFrameSend, 32)
	var clientClosed atomic.Bool
	var outputWG sync.WaitGroup
	outputWG.Add(2)
	outputDone := make(chan struct{})
	go func() {
		outputWG.Wait()
		close(outputDone)
	}()
	go forwardTerminalFramesToClient(ctx, strm, terminalFrames, errCh)
	go r.forwardClientFramesToSSH(ctx, strm, session, stdin, &clientClosed, errCh)
	go func() {
		defer outputWG.Done()
		r.forwardSSHOutput(ctx, terminalFrames, stdout, errCh)
	}()
	go func() {
		defer outputWG.Done()
		r.forwardSSHOutput(ctx, terminalFrames, stderr, errCh)
	}()
	go r.waitSSHSession(ctx, session, terminalFrames, outputDone, &clientClosed, errCh)

	result := <-errCh
	cancel()
	if result.err != nil && !stderrors.Is(result.err, context.Canceled) && !stderrors.Is(result.err, io.EOF) {
		_ = r.updateState(context.Background(), TerminalSessionState_TERMINAL_SESSION_STATE_FAILED, "terminal failed", result.err.Error())
		return result.err
	}
	if result.updateState {
		return r.updateState(context.Background(), result.finalState, result.status, result.errorMessage)
	}
	return nil
}

func (r *TerminalResource) lookupSshHost(ctx context.Context, objectKey string) (*s4wave_sshhost.SshHost, error) {
	if err := world_types.CheckObjectType(ctx, r.ws, objectKey, s4wave_sshhost.SshHostTypeID); err != nil {
		return nil, err
	}
	host, err := world.LookupObjectBody[*s4wave_sshhost.SshHost](
		ctx,
		r.ws,
		objectKey,
		s4wave_sshhost.NewSshHostBlock,
	)
	if err != nil {
		return nil, err
	}
	if err := host.Validate(); err != nil {
		return nil, err
	}
	if err := s4wave_sshhost.ValidateSshHostCredentialSecrets(ctx, r.ws, host.GetCredentials()); err != nil {
		return nil, err
	}
	return host, nil
}

func (r *TerminalResource) buildSshClientConfig(
	ctx context.Context,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	hostObjectKey string,
	host *s4wave_sshhost.SshHost,
) (*ssh.ClientConfig, string, error) {
	endpoint := s4wave_sshhost.NormalizeSshHostEndpoint(host.GetEndpoint())
	auth, err := r.buildSshAuthMethods(ctx, host.GetCredentials())
	if err != nil {
		return nil, "", err
	}
	return &ssh.ClientConfig{
		User: endpoint.GetUsername(),
		Auth: auth,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if s4wave_sshhost.SshHostKeyPinsMatchPublicKey(host.GetHostKeyPins(), key) {
				return nil
			}
			if len(host.GetHostKeyPins()) != 0 {
				return errors.Errorf("ssh host key for %s is not pinned", endpoint.GetHost())
			}
			pin := s4wave_sshhost.NewSshHostKeyPinFromPublicKey(
				key,
				time.Now(),
				sshHostTrustAcceptedByPeerID(strm.Context()),
			)
			accepted, err := promptSshHostKeyTrust(ctx, strm, endpoint.GetHost(), key, pin)
			if err != nil {
				return err
			}
			if !accepted {
				return errors.Errorf("ssh host key for %s was not trusted", endpoint.GetHost())
			}
			if err := s4wave_sshhost.RememberSshHostKeyPin(ctx, r.engine, hostObjectKey, pin); err != nil {
				return errors.Wrap(err, "remember SSH host key")
			}
			return nil
		},
	}, net.JoinHostPort(endpoint.GetHost(), strconv.FormatUint(uint64(endpoint.GetPort()), 10)), nil
}

func (r *TerminalResource) buildSshAuthMethods(ctx context.Context, refs *s4wave_sshhost.SshHostCredentialRefs) ([]ssh.AuthMethod, error) {
	if refs == nil {
		return nil, nil
	}
	var auth []ssh.AuthMethod
	var passphrase []byte
	var err error
	if key := refs.GetPassphraseSecretObjectKey(); key != "" {
		passphrase, err = r.readSshCredentialPayload(ctx, key, s4wave_secret.SecretKindSSHPassphrase)
		if err != nil {
			return nil, err
		}
	}
	if key := refs.GetPrivateKeySecretObjectKey(); key != "" {
		privateKey, err := r.readSshCredentialPayload(ctx, key, s4wave_secret.SecretKindSSHPrivateKey)
		if err != nil {
			return nil, err
		}
		var signer ssh.Signer
		if len(passphrase) != 0 {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(privateKey, passphrase)
		} else {
			signer, err = ssh.ParsePrivateKey(privateKey)
		}
		if err != nil {
			return nil, errors.Wrap(err, "parse SSH private key")
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if key := refs.GetPasswordSecretObjectKey(); key != "" {
		password, err := r.readSshCredentialPayload(ctx, key, s4wave_secret.SecretKindSSHPassword)
		if err != nil {
			return nil, err
		}
		auth = append(auth, ssh.Password(string(password)))
	}
	return auth, nil
}

func (r *TerminalResource) readSshCredentialPayload(ctx context.Context, objectKey, expectedKind string) ([]byte, error) {
	if err := world_types.CheckObjectType(ctx, r.ws, objectKey, s4wave_secret.SecretTypeID); err != nil {
		return nil, err
	}
	secret, err := world.LookupObjectBody[*s4wave_secret.Secret](
		ctx,
		r.ws,
		objectKey,
		s4wave_secret.NewSecretBlock,
	)
	if err != nil {
		return nil, err
	}
	return s4wave_secret.ReadSSHCredentialPayload(ctx, r.b, secret, expectedKind)
}

func promptSshHostKeyTrust(
	ctx context.Context,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	hostname string,
	key ssh.PublicKey,
	pin *s4wave_sshhost.SshHostKeyPin,
) (bool, error) {
	if err := strm.Send(&TerminalFrame{
		Kind:                      TerminalFrameKind_TERMINAL_FRAME_KIND_SSH_HOST_KEY_TRUST_CHALLENGE,
		SshTrustHost:              hostname,
		SshTrustAlgorithm:         pin.GetAlgorithm(),
		SshTrustSha256Fingerprint: pin.GetSha256Fingerprint(),
		SshTrustPublicKey:         string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(key))),
	}); err != nil {
		return false, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		frame, err := strm.Recv()
		if err != nil {
			return false, err
		}
		if frame == nil {
			continue
		}
		switch frame.GetKind() {
		case TerminalFrameKind_TERMINAL_FRAME_KIND_SSH_HOST_KEY_TRUST_RESPONSE:
			return frame.GetSshTrustAccepted(), nil
		case TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
			TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE:
			continue
		case TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
			return false, nil
		default:
			return false, errors.Errorf("unsupported terminal client frame kind %s while trusting SSH host key", frame.GetKind().String())
		}
	}
}

func sshHostTrustAcceptedByPeerID(ctx context.Context) string {
	ms := link.GetMountedStreamContext(ctx)
	if ms == nil {
		return ""
	}
	return ms.GetPeerID().String()
}

func dialSshClient(ctx context.Context, address string, config *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(sshHandshakeDeadline(ctx)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	close(done)
	if err != nil {
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = clientConn.Close()
		return nil, err
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func sshHandshakeDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(sshClientHandshakeTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		return ctxDeadline
	}
	return deadline
}

func forwardTerminalFramesToClient(
	ctx context.Context,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	frames <-chan terminalFrameSend,
	errCh chan<- terminalConnectResult,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case send := <-frames:
			if send.frame == nil {
				if send.ack != nil {
					send.ack <- nil
					close(send.ack)
				}
				continue
			}
			err := strm.Send(send.frame)
			if send.ack != nil {
				send.ack <- err
				close(send.ack)
			}
			if err != nil {
				errCh <- terminalConnectResult{err: err}
				return
			}
		}
	}
}

func (r *TerminalResource) forwardClientFramesToSSH(
	ctx context.Context,
	strm SRPCTerminalResourceService_ConnectTerminalStream,
	session *ssh.Session,
	stdin io.WriteCloser,
	clientClosed *atomic.Bool,
	errCh chan<- terminalConnectResult,
) {
	for {
		frame, err := strm.Recv()
		if err != nil {
			_ = session.Close()
			errCh <- terminalConnectResult{
				err:         err,
				updateState: true,
				finalState:  TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
				status:      "disconnected",
			}
			return
		}
		if frame == nil {
			continue
		}
		switch frame.GetKind() {
		case TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT:
			if len(frame.GetData()) != 0 {
				if _, err := stdin.Write(frame.GetData()); err != nil {
					errCh <- terminalConnectResult{err: err}
					return
				}
			}
		case TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE:
			cols, rows := NormalizeTerminalFrameSize(frame.GetCols(), frame.GetRows())
			if err := session.WindowChange(int(rows), int(cols)); err != nil {
				errCh <- terminalConnectResult{err: err}
				return
			}
		case TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
			clientClosed.Store(true)
			_ = session.Close()
			return
		default:
			errCh <- terminalConnectResult{err: errors.Errorf("unsupported terminal client frame kind %s", frame.GetKind().String())}
			return
		}
		if err := ctx.Err(); err != nil {
			errCh <- terminalConnectResult{
				err:         err,
				updateState: true,
				finalState:  TerminalSessionState_TERMINAL_SESSION_STATE_DISCONNECTED,
				status:      "disconnected",
			}
			return
		}
	}
}

func (r *TerminalResource) forwardSSHOutput(
	ctx context.Context,
	terminalFrames chan<- terminalFrameSend,
	reader io.Reader,
	errCh chan<- terminalConnectResult,
) {
	buf := make([]byte, 8192)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if !queueTerminalFrame(ctx, terminalFrames, &TerminalFrame{
				Kind: TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT,
				Data: bytes.Clone(buf[:n]),
			}) {
				return
			}
		}
		if err != nil {
			if stderrors.Is(err, io.EOF) || ctx.Err() != nil {
				return
			}
			errCh <- terminalConnectResult{err: err}
			return
		}
	}
}

func (r *TerminalResource) waitSSHSession(
	ctx context.Context,
	session *ssh.Session,
	terminalFrames chan<- terminalFrameSend,
	outputDone <-chan struct{},
	clientClosed *atomic.Bool,
	errCh chan<- terminalConnectResult,
) {
	waitErr := session.Wait()
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := stderrors.AsType[*ssh.ExitError](waitErr); ok {
			exitCode = exitErr.ExitStatus()
		} else if !clientClosed.Load() {
			errCh <- terminalConnectResult{err: waitErr}
			return
		}
	}
	select {
	case <-outputDone:
	case <-ctx.Done():
		return
	}
	if err := queueTerminalFrameAndWait(ctx, terminalFrames, &TerminalFrame{
		Kind:     TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT,
		ExitCode: int32(exitCode),
		Error:    sshTerminalErrorString(waitErr),
	}); err != nil {
		errCh <- terminalConnectResult{err: err}
		return
	}
	finalState, status, errMessage := terminalConnectExitState(clientClosed.Load())
	errCh <- terminalConnectResult{
		updateState:  true,
		finalState:   finalState,
		status:       status,
		errorMessage: errMessage,
	}
}

func queueTerminalFrame(ctx context.Context, terminalFrames chan<- terminalFrameSend, frame *TerminalFrame) bool {
	select {
	case terminalFrames <- terminalFrameSend{frame: frame}:
		return true
	case <-ctx.Done():
		return false
	}
}

func queueTerminalFrameAndWait(ctx context.Context, terminalFrames chan<- terminalFrameSend, frame *TerminalFrame) error {
	ack := make(chan error, 1)
	select {
	case terminalFrames <- terminalFrameSend{frame: frame, ack: ack}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sshTerminalErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
