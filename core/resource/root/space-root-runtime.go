//go:build !js

package resource_root

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/net/util/randstring"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

const spaceRootRuntimeSocketName = "spacewave.sock"

const spaceRootRuntimeStartupReadyMessage = "ready"

const spaceRootRuntimeStartupErrorPrefix = "error: "

const spaceRootRuntimeStartupTimeoutEnvVar = "SPACEWAVE_DAEMON_STARTUP_TIMEOUT"

var spaceRootRuntimeStartupTimeout = time.Minute

type spaceRootRuntimeClient struct {
	conn      net.Conn
	resClient *resource_client.Client
	root      spaceRootRuntimeRoot
}

type spaceRootRuntimeRoot interface {
	WatchSessions(context.Context) (s4wave_root.SRPCRootResourceService_WatchSessionsClient, error)
}

var connectSpaceRootRuntimeFunc = connectSpaceRootRuntime

// WatchSpaceRootRuntime streams sessions from a selected configured root daemon.
func (s *CoreRootServer) WatchSpaceRootRuntime(
	req *s4wave_root.WatchSpaceRootRuntimeRequest,
	strm s4wave_root.SRPCRootResourceService_WatchSpaceRootRuntimeStream,
) error {
	ctx := strm.Context()
	alias, err := s.lookupReadySpaceRootAlias(ctx, req.GetAliasId())
	if err != nil {
		return sendSpaceRootRuntimeError(strm, req.GetAliasId(), "", "", err)
	}
	statePath := alias.GetNative().GetPath()
	socketPath := filepath.Join(statePath, spaceRootRuntimeSocketName)

	if err := strm.Send(&s4wave_root.WatchSpaceRootRuntimeResponse{
		Status:     s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_CONNECTING,
		AliasId:    alias.GetAliasId(),
		StatePath:  statePath,
		SocketPath: socketPath,
	}); err != nil {
		return err
	}

	client, err := connectSpaceRootRuntimeFunc(ctx, statePath)
	if err != nil && req.GetAutostart() {
		if err := strm.Send(&s4wave_root.WatchSpaceRootRuntimeResponse{
			Status:     s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_STARTING,
			AliasId:    alias.GetAliasId(),
			StatePath:  statePath,
			SocketPath: socketPath,
		}); err != nil {
			return err
		}
		if _, statErr := os.Stat(socketPath); statErr == nil {
			if removeErr := os.Remove(socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return sendSpaceRootRuntimeError(strm, alias.GetAliasId(), statePath, socketPath, errors.Wrap(removeErr, "remove stale daemon socket"))
			}
		}
		if startErr := s.startSpaceRootRuntimeDaemon(ctx, statePath); startErr != nil {
			return sendSpaceRootRuntimeError(strm, alias.GetAliasId(), statePath, socketPath, errors.Wrap(startErr, "start daemon"))
		}
		client, err = connectSpaceRootRuntimeFunc(ctx, statePath)
	}
	if err != nil {
		return sendSpaceRootRuntimeError(strm, alias.GetAliasId(), statePath, socketPath, err)
	}
	defer client.Close()

	watch, err := client.root.WatchSessions(ctx)
	if err != nil {
		return sendSpaceRootRuntimeError(strm, alias.GetAliasId(), statePath, socketPath, errors.Wrap(err, "watch sessions"))
	}
	defer watch.Close()

	for {
		resp, err := watch.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return sendSpaceRootRuntimeError(strm, alias.GetAliasId(), statePath, socketPath, errors.Wrap(err, "watch sessions"))
		}
		if err := strm.Send(&s4wave_root.WatchSpaceRootRuntimeResponse{
			Status:     s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_READY,
			AliasId:    alias.GetAliasId(),
			StatePath:  statePath,
			SocketPath: socketPath,
			Sessions:   resp.GetSessions(),
		}); err != nil {
			return err
		}
	}
}

func (s *CoreRootServer) lookupReadySpaceRootAlias(
	ctx context.Context,
	aliasID string,
) (*s4wave_root.SpaceRootAliasRecord, error) {
	aliasID = strings.TrimSpace(aliasID)
	if aliasID == "" {
		return nil, errors.New("space root alias id is required")
	}
	records, err := s.snapshotSpaceRootAliases(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.GetAliasId() != aliasID {
			continue
		}
		if record.GetStatus() != s4wave_root.SpaceRootStatus_SpaceRootStatus_READY {
			return nil, errors.Errorf("configured root is not ready: %s", record.GetStatusMessage())
		}
		return record, nil
	}
	return nil, errors.Errorf("configured root alias %q was not found", aliasID)
}

func connectSpaceRootRuntime(ctx context.Context, statePath string) (*spaceRootRuntimeClient, error) {
	socketPath := filepath.Join(statePath, spaceRootRuntimeSocketName)
	conn, err := dialSpaceRootRuntime(ctx, socketPath)
	if err != nil {
		return nil, spaceRootRuntimeNotListeningError(socketPath)
	}
	client, err := buildSpaceRootRuntimeClient(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

func dialSpaceRootRuntime(ctx context.Context, socketPath string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", socketPath)
}

func buildSpaceRootRuntimeClient(ctx context.Context, conn net.Conn) (*spaceRootRuntimeClient, error) {
	srpcClient, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "create srpc client")
	}

	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "resource client")
	}

	rootRef := resClient.AccessRootResource()
	root, err := s4wave_root.NewRoot(resClient, rootRef)
	if err != nil {
		rootRef.Release()
		resClient.Release()
		conn.Close()
		return nil, errors.Wrap(err, "root resource")
	}

	return &spaceRootRuntimeClient{
		conn:      conn,
		resClient: resClient,
		root:      root,
	}, nil
}

func (c *spaceRootRuntimeClient) Close() {
	if c.root != nil {
		if releaser, ok := c.root.(interface{ Release() }); ok {
			releaser.Release()
		}
	}
	if c.resClient != nil {
		c.resClient.Release()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

func (s *CoreRootServer) startSpaceRootRuntimeDaemon(ctx context.Context, statePath string) error {
	startupTimeout, err := getSpaceRootRuntimeStartupTimeout()
	if err != nil {
		return err
	}

	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	pipeID := "spacewave-daemon-" + randstring.RandomIdentifier(6)
	pipeListener, err := pipesock.BuildPipeListener(s.le, statePath, pipeID)
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
		"--state-path", statePath,
		"serve",
		"--daemon-startup-pipe-id", pipeID,
	)

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

	if err := waitForSpaceRootRuntimeStartup(startCtx, pipeListener); err != nil {
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

func getSpaceRootRuntimeStartupTimeout() (time.Duration, error) {
	raw := os.Getenv(spaceRootRuntimeStartupTimeoutEnvVar)
	if raw == "" {
		return spaceRootRuntimeStartupTimeout, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrap(err, spaceRootRuntimeStartupTimeoutEnvVar)
	}
	return dur, nil
}

func waitForSpaceRootRuntimeStartup(ctx context.Context, pipeListener net.Listener) error {
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
		msg, err := readSpaceRootRuntimeStartupMessage(conn)
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
		case res.msg == spaceRootRuntimeStartupReadyMessage:
			return nil
		case strings.HasPrefix(res.msg, spaceRootRuntimeStartupErrorPrefix):
			return errors.New(strings.TrimPrefix(res.msg, spaceRootRuntimeStartupErrorPrefix))
		default:
			return errors.Errorf("unexpected daemon startup status %q", res.msg)
		}
	}
}

func readSpaceRootRuntimeStartupMessage(conn net.Conn) (string, error) {
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

func sendSpaceRootRuntimeError(
	strm s4wave_root.SRPCRootResourceService_WatchSpaceRootRuntimeStream,
	aliasID string,
	statePath string,
	socketPath string,
	err error,
) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(errors.Cause(err), syscall.ENOENT) {
		err = errors.Errorf("no daemon listening at %s", socketPath)
	}
	return strm.Send(&s4wave_root.WatchSpaceRootRuntimeResponse{
		Status:     s4wave_root.SpaceRootRuntimeStatus_SpaceRootRuntimeStatus_ERROR,
		AliasId:    aliasID,
		StatePath:  statePath,
		SocketPath: socketPath,
		Error:      err.Error(),
	})
}

func spaceRootRuntimeNotListeningError(socketPath string) error {
	return errors.Errorf(
		"no daemon listening at %s: start Spacewave for this state root or enable autostart",
		socketPath,
	)
}
