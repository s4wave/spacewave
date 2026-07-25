//go:build !js

package resource_listener

import (
	"context"
	stderrors "errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
	"github.com/sirupsen/logrus"
)

// gitPrefix is the path prefix that resolves relative to git repo root.
const gitPrefix = "git:"

// homePrefix is the path prefix that resolves relative to the user home dir.
const homePrefix = "~/"

// RequesterNameDefault is the display name used when the peer does
// not identify itself. Surfaces in the UI prompt.
const RequesterNameDefault = "spacewave serve"

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	le := c.GetLogger()
	b := c.GetBus()

	sockPath, err := c.GetConfig().DetermineSocketPath()
	if err != nil {
		return errors.Wrap(err, "determine socket path")
	}
	if sockPath == "" {
		le.Warn("listener socket path not configured, skipping")
		return nil
	}

	absPath, err := resolveSocketPath(le, sockPath)
	if err != nil {
		return errors.Wrap(err, "resolve socket path")
	}

	status := GetProcessStatusBroker()
	status.SetSocketPath(absPath)
	defer status.SetListening(false)

	le.Info("waiting for resource service")
	serviceID := resource.SRPCResourceServiceServiceID
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(ctx, b, serviceID, "", true, nil)
	if err != nil {
		return err
	}
	if len(invokers) == 0 {
		le.Warn("resource service not found")
		return nil
	}
	defer invokerRef.Release()

	broker := GetProcessYieldBroker()

	for {
		handoff, handoffWaitCh := broker.SnapshotHandoff()
		if handoffBlocksListener(handoff) {
			le.Infof("resource listener waiting for runtime handoff reclaim on %s", absPath)
			select {
			case <-ctx.Done():
				return nil
			case <-handoffWaitCh:
				continue
			}
		}
		yielded, err := c.serveOnce(ctx, le, invokers[0], absPath, broker, status)
		if err != nil {
			return err
		}
		if !yielded || ctx.Err() != nil {
			return nil
		}
		reclaimCh := broker.BeginHandoff(RequesterNameDefault, absPath)
		le.Info("runtime handed off, waiting for reclaim signal")
		select {
		case <-ctx.Done():
			broker.ClearHandoff()
			return nil
		case <-reclaimCh:
			le.Info("reclaim signal received, re-binding socket")
		}
	}
}

func handoffBlocksListener(handoff yield_policy.HandoffState) bool {
	return handoff.Active
}

// serveOnce takes over the socket, listens, serves until either the
// serve context is canceled externally or a daemon-control Shutdown
// is honored. The returned bool is true when the listener yielded
// cleanly so the caller can enter the reclaim-wait loop.
func (c *Controller) serveOnce(
	parentCtx context.Context,
	le *logrus.Entry,
	invoker srpc.Invoker,
	absPath string,
	broker *yield_policy.Broker,
	status *StatusBroker,
) (bool, error) {
	if err := listener_control.EnsureSocketAvailable(parentCtx, le, absPath); err != nil {
		return false, errors.Wrap(err, "ensure socket available")
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return false, err
	}

	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: absPath, Net: "unix"})
	if err != nil {
		return false, err
	}
	defer lis.Close()

	if err := os.Chmod(absPath, 0o600); err != nil {
		le.WithError(err).Warn("failed to chmod socket")
	}

	le.Infof("resource listener listening on %s", absPath)
	status.SetListening(true)
	defer status.SetListening(false)

	serveCtx, serveCancel := context.WithCancel(parentCtx)
	defer serveCancel()

	yieldCh := make(chan struct{})
	var yieldOnce sync.Once
	mux := srpc.NewMux(invoker)
	policy := broker.MakePolicy(RequesterNameDefault, absPath)
	controlHandler := listener_control.NewHandler(policy, func() {
		le.Info("daemon control shutdown approved, yielding socket")
		yieldOnce.Do(func() {
			close(yieldCh)
		})
		_ = lis.Close()
	})
	if err := mux.Register(controlHandler); err != nil {
		return false, errors.Wrap(err, "register daemon control handler")
	}
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		return false, errors.Wrap(err, "register trace service")
	}

	go func() {
		<-serveCtx.Done()
		lis.Close()
	}()

	srv := srpc.NewServer(mux)
	drainClients, err := acceptCountingListener(serveCtx, lis, srv, status)
	yielded := false
	select {
	case <-yieldCh:
		yielded = true
		select {
		case <-controlHandler.ShutdownComplete():
		case <-parentCtx.Done():
		}
	default:
	}
	serveCanceled := serveCtx.Err() != nil
	serveCancel()
	drainClients()
	if err != nil && (serveCanceled || yielded || stderrors.Is(err, net.ErrClosed)) {
		err = nil
	}
	if err != nil {
		return false, err
	}
	return yielded, nil
}

// acceptCountingListener serves accepted clients independently while reporting
// accept/close transitions to the status broker. It returns a drain function
// after accepting stops so the owner can release the listener synchronously,
// finish a takeover acknowledgement, and only then close active clients.
func acceptCountingListener(
	ctx context.Context,
	lis net.Listener,
	srv *srpc.Server,
	status *StatusBroker,
) (func(), error) {
	var clients sync.WaitGroup
	var connectionsMtx sync.Mutex
	connections := make(map[*countingConn]struct{})
	closeConnections := func() {
		connectionsMtx.Lock()
		active := make([]*countingConn, 0, len(connections))
		for conn := range connections {
			active = append(active, conn)
		}
		connectionsMtx.Unlock()
		for _, conn := range active {
			_ = conn.Close()
		}
	}

	drainOnce := sync.Once{}
	drainClients := func() {
		drainOnce.Do(func() {
			closeConnections()
			clients.Wait()
		})
	}

	for {
		nc, err := lis.Accept()
		if err != nil {
			return drainClients, err
		}
		status.AddClient()
		tracked := &countingConn{Conn: nc, status: status}
		mc, err := srpc.NewMuxedConn(tracked, false, nil)
		if err != nil {
			_ = tracked.Close()
			continue
		}

		connectionsMtx.Lock()
		connections[tracked] = struct{}{}
		clients.Add(1)
		connectionsMtx.Unlock()
		go func() {
			defer clients.Done()
			defer func() {
				connectionsMtx.Lock()
				delete(connections, tracked)
				connectionsMtx.Unlock()
				_ = tracked.Close()
			}()
			_ = srv.AcceptMuxedConn(ctx, mc)
		}()
	}
}

// countingConn wraps a net.Conn so the status broker is notified
// exactly once when the connection closes.
type countingConn struct {
	net.Conn
	status    *StatusBroker
	closeOnce sync.Once
}

// Close closes the underlying connection and decrements the
// connected-client count exactly once.
func (c *countingConn) Close() error {
	c.closeOnce.Do(func() {
		c.status.RemoveClient()
	})
	return c.Conn.Close()
}

// resolveSocketPath resolves a socket path configuration value.
//
// Supported prefixes:
//   - "git:" resolves relative to the git repo root. If the git root
//     lookup fails (e.g. running outside a git repo), falls back to
//     resolving relative to the current working directory.
//   - "~/" resolves relative to the user home directory.
//   - All other paths resolve relative to cwd via filepath.Abs.
func resolveSocketPath(le *logrus.Entry, p string) (string, error) {
	if strings.HasPrefix(p, gitPrefix) {
		rel := p[len(gitPrefix):]
		root, err := gitroot.FindRepoRoot()
		if err != nil {
			le.WithError(err).Debug("git root unavailable, resolving relative to cwd")
			return filepath.Abs(rel)
		}
		return filepath.Join(root, rel), nil
	}
	if strings.HasPrefix(p, homePrefix) {
		rel := p[len(homePrefix):]
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.Wrap(err, "get home dir")
		}
		return filepath.Join(home, rel), nil
	}
	return filepath.Abs(p)
}
