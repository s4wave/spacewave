//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

const daemonStartupReadyMessage = "ready"

const daemonStartupErrorPrefix = "error: "

const daemonStartupTimeoutEnvVar = "SPACEWAVE_DAEMON_STARTUP_TIMEOUT"

var defaultDaemonStartupTimeout = time.Minute

// startDaemonProcess starts the current CLI executable in background serve mode
// and waits for a one-shot startup readiness signal.
func startDaemonProcess(ctx context.Context, statePath string) error {
	startupTimeout, err := getDaemonStartupTimeout()
	if err != nil {
		return err
	}

	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	pipeID := "spacewave-daemon-" + randstring.RandomIdentifier(6)
	pipeListener, err := pipesock.BuildPipeListener(le, statePath, pipeID)
	if err != nil {
		return errors.Wrap(err, "listen for daemon startup")
	}
	defer pipeListener.Close()

	exePath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "resolve executable")
	}

	cmd := exec.Command(
		exePath,
		daemonServeArgs(statePath, pipeID)...,
	)
	if err := prepareDaemonStart(cmd); err != nil {
		return err
	}

	nullFile, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return errors.Wrap(err, "open devnull")
	}
	defer nullFile.Close()
	cmd.Stdin = nullFile
	cmd.Stdout = nullFile
	cmd.Stderr = nullFile

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start daemon process")
	}

	if err := waitForDaemonStartup(startCtx, pipeListener); err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Process.Release()
		}
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

func daemonServeArgs(statePath string, pipeID string) []string {
	return []string{
		"--state-path", statePath,
		"serve",
		"--daemon-startup-pipe-id", pipeID,
	}
}

// getDaemonStartupTimeout returns the configured daemon startup timeout.
func getDaemonStartupTimeout() (time.Duration, error) {
	raw := os.Getenv(daemonStartupTimeoutEnvVar)
	if raw == "" {
		return defaultDaemonStartupTimeout, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrap(err, daemonStartupTimeoutEnvVar)
	}
	return dur, nil
}

// waitForDaemonStartup waits for the daemon startup notifier to report status.
func waitForDaemonStartup(ctx context.Context, pipeListener net.Listener) error {
	type startupResult struct {
		msg string
		err error
	}
	resCh := make(chan startupResult, 1)
	go func() {
		conn, err := pipeListener.Accept()
		if err != nil {
			resCh <- startupResult{err: err}
			return
		}
		msg, err := readDaemonStartupMessage(conn)
		resCh <- startupResult{msg: msg, err: err}
	}()
	go func() {
		<-ctx.Done()
		_ = pipeListener.Close()
	}()

	select {
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "wait for daemon startup")
	case res := <-resCh:
		if res.err != nil {
			if ctx.Err() != nil {
				return errors.Wrap(ctx.Err(), "wait for daemon startup")
			}
			return errors.Wrap(res.err, "wait for daemon startup")
		}
		switch {
		case res.msg == daemonStartupReadyMessage:
			return nil
		case strings.HasPrefix(res.msg, daemonStartupErrorPrefix):
			return errors.New(strings.TrimPrefix(res.msg, daemonStartupErrorPrefix))
		default:
			return errors.Errorf("unexpected daemon startup status %q", res.msg)
		}
	}
}

// readDaemonStartupMessage reads the one-shot startup message from the daemon.
func readDaemonStartupMessage(conn net.Conn) (string, error) {
	defer conn.Close()

	msg, err := io.ReadAll(conn)
	if err != nil {
		return "", errors.Wrap(err, "read startup message")
	}
	text := strings.TrimSpace(string(msg))
	if text == "" {
		return "", errors.New("empty daemon startup message")
	}
	return text, nil
}

// daemonStartupNotifier reports startup status from the daemon child process.
type daemonStartupNotifier struct {
	conn net.Conn
	sent bool
}

// newDaemonStartupNotifier connects to the parent startup pipe when requested.
func newDaemonStartupNotifier(
	ctx context.Context,
	statePath string,
	pipeID string,
) (*daemonStartupNotifier, error) {
	if pipeID == "" {
		return nil, nil
	}

	le := logrus.NewEntry(logrus.New())
	conn, err := pipesock.DialPipeListener(ctx, le, statePath, pipeID)
	if err != nil {
		return nil, errors.Wrap(err, "dial daemon startup pipe")
	}
	return &daemonStartupNotifier{conn: conn}, nil
}

// reportReady sends the ready signal to the parent process.
func (n *daemonStartupNotifier) reportReady() error {
	return n.writeMessage(daemonStartupReadyMessage)
}

// reportError sends a startup failure to the parent process.
func (n *daemonStartupNotifier) reportError(err error) {
	if n == nil || err == nil || n.sent {
		return
	}
	_ = n.writeMessage(daemonStartupErrorPrefix + err.Error())
}

// close closes the startup notifier connection.
func (n *daemonStartupNotifier) close() {
	if n == nil || n.conn == nil {
		return
	}
	_ = n.conn.Close()
	n.conn = nil
}

// writeMessage writes a one-shot startup message to the parent process.
func (n *daemonStartupNotifier) writeMessage(msg string) error {
	if n == nil || n.sent {
		return nil
	}
	if _, err := io.WriteString(n.conn, msg+"\n"); err != nil {
		n.close()
		return errors.Wrap(err, "write daemon startup message")
	}
	n.sent = true
	n.close()
	return nil
}
