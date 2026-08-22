//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_space_core "github.com/s4wave/spacewave/core/space"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	"github.com/s4wave/spacewave/sdk/cli/runner"
	s4wave_provider "github.com/s4wave/spacewave/sdk/provider"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// projectID is the canonical project identifier for native state.
const projectID = "spacewave"

// defaultStatePath is the default path for daemon state.
var defaultStatePath = cli_entrypoint.DefaultStatePath(projectID)

// statePathEnvVars are the environment variables that override the daemon state path.
var statePathEnvVars = cli_entrypoint.StatePathEnvVars(projectID)

// socketPathEnvVars are the environment variables that override the daemon socket path directly.
// When set, the CLI dials this exact socket path without joining a state directory.
var socketPathEnvVars = []string{"SPACEWAVE_SOCKET_PATH"}

// socketName is the name of the Unix socket within the state path.
const socketName = "spacewave.sock"

// sdkClient wraps the Resource SDK connection to a running daemon. A
// successfully connected client does not retain the daemon process handle.
type sdkClient struct {
	conn         net.Conn
	srpc         srpc.Client
	resClient    *resource_client.Client
	root         *s4wave_root.Root
	clientCancel context.CancelFunc
}

type nativeClientFactory struct{}

type nativeClient struct {
	client *sdkClient
}

type nativeSession struct {
	*s4wave_session.Session
}

var (
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sockPath)
	}
	connectDaemonBuildClient = buildSDKClient
	connectDaemonStart       = startDaemonProcess
)

// connectDaemon connects to the running daemon via Unix socket.
// It joins statePath with the canonical socket name and never starts or
// takes over a daemon implicitly.
func connectDaemon(ctx context.Context, statePath string) (*sdkClient, error) {
	sockPath := filepath.Join(statePath, socketName)
	conn, err := connectDaemonDial(ctx, sockPath)
	if err != nil {
		return nil, connectDaemonNotListeningError(sockPath)
	}
	client, err := connectDaemonBuildClient(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

// connectDaemonWithAutostart connects to a running daemon or starts one in
// statePath when no daemon is reachable. The parent keeps the child process
// handle until Resource Init succeeds so startup failure can stop and wait for
// the child. A successful connection releases the handle before returning.
func connectDaemonWithAutostart(ctx context.Context, statePath string) (*sdkClient, error) {
	sockPath := filepath.Join(statePath, socketName)
	_, statErr := os.Stat(sockPath)
	conn, err := connectDaemonDial(ctx, sockPath)
	if err != nil {
		if statErr == nil && !stderrors.Is(err, syscall.ECONNREFUSED) {
			return nil, errors.Wrapf(err, "connect to existing daemon socket %s", sockPath)
		}
		if statErr == nil {
			if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
				return nil, errors.Wrap(err, "remove stale daemon socket")
			}
		}
		daemonCmd, startErr := connectDaemonStart(ctx, statePath)
		if startErr != nil {
			return nil, errors.Wrap(startErr, "start daemon")
		}
		conn, err = connectDaemonDial(ctx, sockPath)
		if err != nil {
			stopErr := terminateDaemonCmd(daemonCmd, statePath)
			if stopErr != nil {
				return nil, errors.Wrapf(err, "connect to %s; stop started daemon: %v", sockPath, stopErr)
			}
			return nil, errors.Wrapf(err, "connect to %s", sockPath)
		}
		client, buildErr := connectDaemonBuildClient(ctx, conn)
		if buildErr != nil {
			conn.Close()
			stopErr := terminateDaemonCmd(daemonCmd, statePath)
			if stopErr != nil {
				return nil, errors.Wrapf(buildErr, "stop started daemon: %v", stopErr)
			}
			return nil, buildErr
		}
		// The daemon is self-sustaining after the client handshake succeeds.
		// Release the process handle so the daemon persists for subsequent
		// commands; do not store it on the client, since close must not kill
		// a healthy daemon.
		releaseDaemonCmd(daemonCmd)
		return client, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "connect to %s", sockPath)
	}
	client, err := connectDaemonBuildClient(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

// connectDaemonAtSocket dials an existing daemon socket at the exact given
// path. It never autostarts a daemon. Intended for connecting to a running
// desktop app or any pre-existing socket the caller has resolved by path.
// Dial failure is reported as a hard error with actionable guidance: the
// caller asked for connect-only semantics, so silently spawning a new
// daemon would violate intent.
func connectDaemonAtSocket(ctx context.Context, sockPath string) (*sdkClient, error) {
	conn, err := connectDaemonDial(ctx, sockPath)
	if err != nil {
		return nil, connectDaemonNotListeningError(sockPath)
	}
	client, err := connectDaemonBuildClient(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

// connectDaemonFromContext picks the right connection path based on CLI flags
// visible in the context lineage. If --socket-path (or its env var) is set,
// dial that socket directly. Otherwise resolve the state path and attach to or
// start its daemon socket.
func connectDaemonFromContext(ctx context.Context, c *cli.Context, statePathFallback string) (*sdkClient, error) {
	if sockPath := effectiveSocketPath(c, ""); sockPath != "" {
		return connectDaemonAtSocket(ctx, sockPath)
	}
	resolved, err := resolveStatePathFromContext(c, statePathFallback)
	if err != nil {
		return nil, err
	}
	return connectDaemonWithAutostart(ctx, resolved)
}

// connectDaemonWithResolvedFallback honors --socket-path when present,
// otherwise attaches to or starts an already-resolved state path.
func connectDaemonWithResolvedFallback(ctx context.Context, c *cli.Context, resolved string) (*sdkClient, error) {
	if sockPath := effectiveSocketPath(c, ""); sockPath != "" {
		return connectDaemonAtSocket(ctx, sockPath)
	}
	return connectDaemonWithAutostart(ctx, resolved)
}

// connectDaemonNotListeningError returns the connect-only daemon error.
func connectDaemonNotListeningError(sockPath string) error {
	return errors.Errorf(
		"no daemon listening at %s: start the Spacewave desktop app or run "+
			"`spacewave serve` with a matching --state-path; see the Command Line "+
			"settings page in the app for guidance",
		sockPath,
	)
}

// buildSDKClient constructs the Resource SDK client over an accepted daemon
// connection. The initial ResourceClient Init handshake is bounded by the
// daemon startup timeout so it cannot block forever when the core plugin
// has not loaded yet. On success the client's stream lifetime is tied to
// the caller's context via a cancel stored on sdkClient, so subsequent RPCs
// remain valid after the handshake bound expires.
func buildSDKClient(ctx context.Context, conn net.Conn) (*sdkClient, error) {
	srpcClient, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "create srpc client")
	}

	// Bound the initial ResourceClient handshake without canceling the
	// client's lifetime on success. NewClient derives its stream context
	// from the passed context; a timeout context would cancel the stream
	// after the deadline even on success. Instead, use a manual cancel
	// context and a select: on timeout, cancel and fail; on success, keep
	// the context alive and store the cancel for close.
	handshakeTimeout, err := getDaemonStartupTimeout()
	if err != nil {
		conn.Close()
		return nil, err
	}
	clientCtx, clientCancel := context.WithCancel(ctx)

	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	type buildResult struct {
		client *resource_client.Client
		root   *s4wave_root.Root
		err    error
	}
	resultCh := make(chan buildResult, 1)
	go func() {
		resClient, err := resource_client.NewClient(clientCtx, resourceSvc)
		if err != nil {
			resultCh <- buildResult{err: err}
			return
		}
		rootRef := resClient.AccessRootResource()
		root, err := s4wave_root.NewRoot(resClient, rootRef)
		if err != nil {
			resClient.Release()
			resultCh <- buildResult{err: err}
			return
		}
		resultCh <- buildResult{client: resClient, root: root}
	}()

	select {
	case <-time.After(handshakeTimeout):
		clientCancel()
		conn.Close()
		// Join the NewClient goroutine. After canceling the context
		// and closing the conn, NewClient must return promptly. If it
		// produces a client despite the cancel, release root before
		// client to drop the root reference first.
		if res := <-resultCh; res.client != nil {
			if res.root != nil {
				res.root.Release()
			}
			res.client.Release()
		}
		return nil, errors.New("resource client: init handshake timed out")
	case res := <-resultCh:
		if res.err != nil {
			clientCancel()
			conn.Close()
			return nil, errors.Wrap(res.err, "resource client")
		}
		return &sdkClient{
			conn:         conn,
			srpc:         srpcClient,
			resClient:    res.client,
			root:         res.root,
			clientCancel: clientCancel,
		}, nil
	}
}

func buildSDKClientFromInvoker(ctx context.Context, invoker srpc.Invoker) (*sdkClient, error) {
	srpcClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		return nil, errors.Wrap(err, "resource client")
	}

	rootRef := resClient.AccessRootResource()
	root, err := s4wave_root.NewRoot(resClient, rootRef)
	if err != nil {
		resClient.Release()
		return nil, errors.Wrap(err, "root resource")
	}

	return &sdkClient{
		srpc:      srpcClient,
		resClient: resClient,
		root:      root,
	}, nil
}

func (nativeClientFactory) NewClient(ctx context.Context, c *cli.Context) (runner.Client, error) {
	client, err := connectDaemonFromContext(ctx, c, defaultStatePath)
	if err != nil {
		return nil, err
	}
	return &nativeClient{client: client}, nil
}

func (nativeClientFactory) StatusEndpoint(ctx context.Context, c *cli.Context) (string, error) {
	if sockPath := effectiveSocketPath(c, ""); sockPath != "" {
		return sockPath, nil
	}
	resolved, err := resolveStatePathFromContext(c, defaultStatePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, socketName), nil
}

func (c *nativeClient) Close() {
	c.client.close()
}

func (c *nativeClient) MountSession(ctx context.Context, idx uint32) (runner.Session, error) {
	sess, err := c.client.mountSession(ctx, idx)
	if err != nil {
		return nil, err
	}
	return &nativeSession{Session: sess}, nil
}

func (s *nativeSession) WatchResourcesList(ctx context.Context) (runner.ResourcesListStream, error) {
	return s.Session.WatchResourcesList(ctx)
}

func (s *nativeSession) WatchLockState(ctx context.Context) (runner.LockStateStream, error) {
	return s.Session.WatchLockState(ctx)
}

// mountSession mounts a session by index and returns the Session SDK wrapper.
func (c *sdkClient) mountSession(ctx context.Context, idx uint32) (*s4wave_session.Session, error) {
	resp, err := c.root.MountSessionByIdx(ctx, idx)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		return nil, errors.Errorf("no session found at index %d", idx)
	}

	sessRef := c.resClient.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(c.resClient, sessRef)
	if err != nil {
		sessRef.Release()
		return nil, errors.Wrap(err, "session resource")
	}
	return sess, nil
}

// mountSpace mounts a space by shared object ID and returns the SpaceResourceService client.
func (c *sdkClient) mountSpace(ctx context.Context, sess *s4wave_session.Session, sharedObjectID string) (s4wave_space.SRPCSpaceResourceServiceClient, func(), error) {
	soResp, err := sess.MountSharedObject(ctx, sharedObjectID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	soRef := c.resClient.CreateResourceReference(soResp.GetResourceId())
	soClient, err := soRef.GetClient()
	if err != nil {
		soRef.Release()
		return nil, nil, errors.Wrap(err, "shared object client")
	}

	soSvc := s4wave_sobject.NewSRPCSharedObjectResourceServiceClient(soClient)
	bodyResp, err := soSvc.MountSharedObjectBody(ctx, &s4wave_sobject.MountSharedObjectBodyRequest{})
	if err != nil {
		soRef.Release()
		return nil, nil, errors.Wrap(err, "mount shared object body")
	}

	bodyRef := c.resClient.CreateResourceReference(bodyResp.GetResourceId())
	bodyClient, err := bodyRef.GetClient()
	if err != nil {
		bodyRef.Release()
		soRef.Release()
		return nil, nil, errors.Wrap(err, "space body client")
	}

	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(bodyClient)
	cleanup := func() {
		bodyRef.Release()
		soRef.Release()
	}
	return spaceSvc, cleanup, nil
}

// resolveSpaceID resolves a space argument to a shared object ID.
func (c *sdkClient) resolveSpaceID(ctx context.Context, sess *s4wave_session.Session, spaceID string) (string, error) {
	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return "", errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return "", errors.Wrap(err, "recv resources list")
	}

	return resolveSpaceIDFromList(spaceID, resp.GetSpacesList())
}

func resolveSpaceIDFromList(spaceID string, spaces []*s4wave_space_core.SpaceSoListEntry) (string, error) {
	if len(spaces) == 0 {
		return "", errors.New("no spaces found; specify --space")
	}
	if spaceID != "" {
		for _, sp := range spaces {
			id := sp.GetEntry().GetRef().GetProviderResourceRef().GetId()
			if id == spaceID {
				return spaceID, nil
			}
		}
		for _, sp := range spaces {
			if sp.GetSpaceMeta().GetName() == spaceID {
				return sp.GetEntry().GetRef().GetProviderResourceRef().GetId(), nil
			}
		}
		return spaceID, nil
	}
	if len(spaces) > 1 {
		return "", errors.New("multiple spaces found; specify --space")
	}
	return spaces[0].GetEntry().GetRef().GetProviderResourceRef().GetId(), nil
}

// getSpaceByName finds a space by name and returns its shared object ID.
// If name is empty, returns the first space found.
func (c *sdkClient) getSpaceByName(ctx context.Context, sess *s4wave_session.Session, name string) (string, error) {
	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return "", errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return "", errors.Wrap(err, "recv resources list")
	}

	spaces := resp.GetSpacesList()
	if len(spaces) == 0 {
		return "", errors.New("no spaces found")
	}

	if name == "" {
		return spaces[0].GetEntry().GetRef().GetProviderResourceRef().GetId(), nil
	}

	for _, sp := range spaces {
		if sp.GetSpaceMeta().GetName() == name {
			return sp.GetEntry().GetRef().GetProviderResourceRef().GetId(), nil
		}
	}
	return "", errors.Errorf("space %q not found", name)
}

// accessWorldEngine accesses the world engine via a space's SpaceResourceService.
func (c *sdkClient) accessWorldEngine(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) (*sdk_engine.SDKEngine, func(), error) {
	engine, _, cleanup, err := c.accessWorldEngineWithRef(ctx, spaceSvc)
	return engine, cleanup, err
}

// accessWorldEngineWithRef accesses the world engine and returns the engine resource reference.
func (c *sdkClient) accessWorldEngineWithRef(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) (*sdk_engine.SDKEngine, resource_client.ResourceRef, func(), error) {
	worldResp, err := spaceSvc.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "access world")
	}

	engineRef := c.resClient.CreateResourceReference(worldResp.GetResourceId())
	engine, err := sdk_engine.NewSDKEngine(c.resClient, engineRef)
	if err != nil {
		engineRef.Release()
		return nil, nil, nil, errors.Wrap(err, "create sdk engine")
	}

	cleanup := func() {
		engine.Release()
	}
	return engine, engineRef, cleanup, nil
}

// close releases all resources, closes the connection, and cancels the
// Resource client context.
func (c *sdkClient) close() {
	if c.root != nil {
		c.root.Release()
	}
	if c.resClient != nil {
		c.resClient.Release()
	}
	if c.clientCancel != nil {
		c.clientCancel()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

var daemonShutdownGracePeriod = 2 * time.Second

var daemonForcedShutdownTimeout = 2 * time.Second

// terminateDaemonCmd asks the daemon control service to stop, then falls back
// to the platform interrupt. It waits for normal cleanup and kills any process
// left in the started tree only after the grace period. cmd.Wait reaps the
// leader exactly once.
func terminateDaemonCmd(cmd *exec.Cmd, statePath string) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	shutdownErr := requestStartedDaemonShutdown(statePath)
	if shutdownErr != nil {
		shutdownErr = stderrors.Join(shutdownErr, interruptDaemon(pid))
	}
	graceTimer := time.NewTimer(daemonShutdownGracePeriod)
	select {
	case <-waitCh:
		graceTimer.Stop()
		return killDaemonTree(pid)
	case <-graceTimer.C:
	}

	killErr := killDaemonTree(pid)
	var cleanupErr error
	if killErr != nil {
		select {
		case <-waitCh:
			return nil
		default:
		}
		leaderKillErr := cmd.Process.Kill()
		cleanupErr = stderrors.Join(killErr, leaderKillErr)
	}

	forceTimer := time.NewTimer(daemonForcedShutdownTimeout)
	defer forceTimer.Stop()
	select {
	case <-waitCh:
		return cleanupErr
	case <-forceTimer.C:
		return stderrors.Join(
			shutdownErr,
			cleanupErr,
			errors.New("timed out waiting for started daemon to stop"),
		)
	}
}

func requestStartedDaemonShutdown(statePath string) error {
	if statePath == "" {
		return errors.New("daemon state path is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), daemonShutdownGracePeriod)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(statePath, socketName))
	if err != nil {
		return err
	}
	defer conn.Close()
	return requestDaemonShutdown(ctx, conn)
}

// releaseDaemonCmd releases the process handle and any platform process-tree
// handle without killing the child. It is called after Resource Init succeeds
// so later commands can reuse the daemon.
func releaseDaemonCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = releaseDaemonTree(cmd.Process.Pid)
	_ = cmd.Process.Release()
}

// resolveStatePath resolves the state path, making it absolute if needed.
// For relative paths, checks cwd first, then falls back to git repo root.
func resolveStatePath(statePath string) (string, error) {
	if filepath.IsAbs(statePath) {
		return statePath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cwdPath := filepath.Join(cwd, statePath)
	sockPath := filepath.Join(cwdPath, socketName)
	if _, err := os.Stat(sockPath); err == nil {
		return cwdPath, nil
	}
	root, err := gitroot.FindRepoRoot()
	if err == nil {
		gitPath := filepath.Join(root, statePath)
		gitSock := filepath.Join(gitPath, socketName)
		if _, err := os.Stat(gitSock); err == nil {
			return gitPath, nil
		}
	}
	return cwdPath, nil
}

// projectLocalStateDirName is the canonical project-local state directory
// name searched in cwd / git-root when no --state-path flag is specified.
// Shares the dot-prefixed project identifier used by the shared default
// state root (~/.spacewave on darwin and linux).
const projectLocalStateDirName = "." + projectID

// discoverProjectLocalStatePath returns the path of a directory in cwd or
// git-root that contains a live daemon socket under the canonical
// project-local state directory name. Dev workflows that keep a daemon
// under a project's working tree (cwd/.spacewave/spacewave.sock) take
// precedence over the shared user-level state root.
func discoverProjectLocalStatePath() (string, bool) {
	if cwd, err := os.Getwd(); err == nil {
		cwdPath := filepath.Join(cwd, projectLocalStateDirName)
		if _, err := os.Stat(filepath.Join(cwdPath, socketName)); err == nil {
			return cwdPath, true
		}
	}
	if root, err := gitroot.FindRepoRoot(); err == nil {
		gitPath := filepath.Join(root, projectLocalStateDirName)
		if _, err := os.Stat(filepath.Join(gitPath, socketName)); err == nil {
			return gitPath, true
		}
	}
	return "", false
}

// statePathUserSet reports whether --state-path was explicitly provided
// via a CLI flag or environment variable on any context in the lineage.
// When false, the shared default applies.
func statePathUserSet(c *cli.Context) bool {
	_, set := lineageFlagSet(c, "state-path")
	return set
}

// lineageFlagValue walks c.Lineage() and returns the first non-empty
// value for the named flag. found is true when a context in the
// lineage carries the flag, even if its value is empty.
func lineageFlagValue(c *cli.Context, name string) (value string, found bool) {
	if c == nil {
		return "", false
	}
	for _, ctx := range c.Lineage() {
		if !hasLocalFlag(ctx, name) {
			continue
		}
		found = true
		if v := ctx.String(name); v != "" {
			return v, true
		}
	}
	return "", found
}

// lineageFlagSet walks c.Lineage() and returns the value of the named
// flag along with whether any context in the lineage explicitly set
// the flag (via CLI argument or matching env var).
func lineageFlagSet(c *cli.Context, name string) (value string, set bool) {
	if c == nil {
		return "", false
	}
	for _, ctx := range c.Lineage() {
		if !hasLocalFlag(ctx, name) {
			continue
		}
		if ctx.IsSet(name) {
			return ctx.String(name), true
		}
	}
	return "", false
}

// resolveStatePathFromContext resolves the effective state path from CLI state.
//
// When --state-path was not explicitly set, dev workflows with a live
// daemon socket in cwd/.spacewave or git-root/.spacewave take precedence
// over the shared default. Explicit --state-path values skip project
// discovery and use relative-path resolution as before.
func resolveStatePathFromContext(c *cli.Context, fallback string) (string, error) {
	if !statePathUserSet(c) {
		if discovered, ok := discoverProjectLocalStatePath(); ok {
			return discovered, nil
		}
	}
	return resolveStatePath(effectiveStatePath(c, fallback))
}

func effectiveStatePath(c *cli.Context, fallback string) string {
	if value, _ := lineageFlagValue(c, "state-path"); value != "" {
		return value
	}
	if fallback != "" {
		return fallback
	}
	return defaultStatePath
}

// effectiveSocketPath returns the --socket-path value from the nearest
// CLI context that carries the flag (or its env var fallback), or the
// fallback string if none is set. An empty return value means "not set"
// and signals the caller to fall back to state-path resolution.
func effectiveSocketPath(c *cli.Context, fallback string) string {
	if value, _ := lineageFlagValue(c, "socket-path"); value != "" {
		return value
	}
	return fallback
}

func hasLocalFlag(c *cli.Context, name string) bool {
	return slices.Contains(c.LocalFlagNames(), name)
}

// mountSpaceContents mounts space contents and returns the SpaceContentsResourceService client.
func (c *sdkClient) mountSpaceContents(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) (s4wave_space.SRPCSpaceContentsResourceServiceClient, func(), error) {
	resp, err := spaceSvc.MountSpaceContents(ctx, &s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount space contents")
	}

	ref := c.resClient.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, nil, errors.Wrap(err, "space contents client")
	}

	svc := s4wave_space.NewSRPCSpaceContentsResourceServiceClient(client)
	cleanup := func() {
		ref.Release()
	}
	return svc, cleanup, nil
}

// lookupProvider accesses a provider resource by ID and returns the ProviderResourceService client.
func (c *sdkClient) lookupProvider(ctx context.Context, providerID string) (s4wave_provider.SRPCProviderResourceServiceClient, func(), error) {
	resourceID, err := c.root.LookupProvider(ctx, providerID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "lookup provider")
	}

	ref := c.resClient.CreateResourceReference(resourceID)
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, nil, errors.Wrap(err, "provider client")
	}

	svc := s4wave_provider.NewSRPCProviderResourceServiceClient(client)
	cleanup := func() {
		ref.Release()
	}
	return svc, cleanup, nil
}

// accessAccount accesses a provider account resource by provider ID and account ID.
func (c *sdkClient) accessAccount(ctx context.Context, providerID, accountID string) (s4wave_account.SRPCAccountResourceServiceClient, func(), error) {
	providerSvc, providerCleanup, err := c.lookupProvider(ctx, providerID)
	if err != nil {
		return nil, nil, err
	}

	resp, err := providerSvc.AccessProviderAccount(ctx, &s4wave_provider.AccessProviderAccountRequest{AccountId: accountID})
	if err != nil {
		providerCleanup()
		return nil, nil, errors.Wrap(err, "access provider account")
	}

	ref := c.resClient.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		providerCleanup()
		return nil, nil, errors.Wrap(err, "account client")
	}

	svc := s4wave_account.NewSRPCAccountResourceServiceClient(client)
	cleanup := func() {
		ref.Release()
		providerCleanup()
	}
	return svc, cleanup, nil
}

// accessTypedObject accesses a typed object resource via an engine resource reference.
// engineRef is the resource reference for the engine (from accessWorldEngine).
// Returns the SRPC client for the typed resource, the resource ID, type ID, and cleanup function.
func (c *sdkClient) accessTypedObject(ctx context.Context, engineRef resource_client.ResourceRef, objectKey string) (srpc.Client, uint32, string, func(), error) {
	engineClient, err := engineRef.GetClient()
	if err != nil {
		return nil, 0, "", nil, errors.Wrap(err, "engine client")
	}

	typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(engineClient)
	resp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{ObjectKey: objectKey})
	if err != nil {
		return nil, 0, "", nil, errors.Wrap(err, "access typed object")
	}

	ref := c.resClient.CreateResourceReference(resp.GetResourceId())
	typedClient, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, 0, "", nil, errors.Wrap(err, "typed object client")
	}

	cleanup := func() {
		ref.Release()
	}
	return typedClient, resp.GetResourceId(), resp.GetTypeId(), cleanup, nil
}

// lookupSpacewaveProvider looks up the spacewave provider and returns an SDK wrapper.
// If providerID is empty, defaults to "spacewave".
func (c *sdkClient) lookupSpacewaveProvider(ctx context.Context, providerID string) (*s4wave_provider_spacewave.SpacewaveProvider, func(), error) {
	if providerID == "" {
		providerID = "spacewave"
	}
	resourceID, err := c.root.LookupProvider(ctx, providerID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "lookup provider")
	}

	ref := c.resClient.CreateResourceReference(resourceID)
	prov, err := s4wave_provider_spacewave.NewSpacewaveProvider(c.resClient, ref)
	if err != nil {
		ref.Release()
		return nil, nil, errors.Wrap(err, "spacewave provider")
	}

	cleanup := func() {
		prov.Release()
	}
	return prov, cleanup, nil
}

// lookupLocalProvider looks up the local provider and returns an SDK wrapper.
func (c *sdkClient) lookupLocalProvider(ctx context.Context) (*s4wave_provider_local.LocalProvider, func(), error) {
	resourceID, err := c.root.LookupProvider(ctx, "local")
	if err != nil {
		return nil, nil, errors.Wrap(err, "lookup provider")
	}

	ref := c.resClient.CreateResourceReference(resourceID)
	prov, err := s4wave_provider_local.NewLocalProvider(c.resClient, ref)
	if err != nil {
		ref.Release()
		return nil, nil, errors.Wrap(err, "local provider")
	}

	cleanup := func() {
		prov.Release()
	}
	return prov, cleanup, nil
}

// objectMount holds one mounted daemon-to-world-object chain: session,
// space, world engine, and typed object client. The unexported cleanup
// fields are set by mountObjectChain and run by release.
type objectMount struct {
	client      *sdkClient
	sess        *s4wave_session.Session
	spaceSvc    s4wave_space.SRPCSpaceResourceServiceClient
	engineRef   resource_client.ResourceRef
	engine      *sdk_engine.SDKEngine
	typedClient srpc.Client
	objectKey   string

	typedCleanup  func()
	engineCleanup func()
	spaceCleanup  func()
}

// release unwinds the whole chain in reverse acquisition order.
func (m *objectMount) release() {
	m.typedCleanup()
	m.engineCleanup()
	m.spaceCleanup()
	m.sess.Release()
	m.client.close()
}

// mountObjectChain connects to the daemon and mounts session -> space ->
// world engine -> typed object for the URI. When resolveObjectKey is nil,
// uri.objectKey is used verbatim; otherwise it resolves the key against
// the mounted space service.
func mountObjectChain(
	c *cli.Context,
	statePath string,
	uri fsURI,
	resolveObjectKey func(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) (string, error),
) (*objectMount, func(), error) {
	ctx := c.Context

	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return nil, nil, err
	}
	var rels []func()
	unwind := func() {
		for i := len(rels) - 1; i >= 0; i-- {
			rels[i]()
		}
	}
	fail := func(err error) (*objectMount, func(), error) {
		unwind()
		return nil, nil, err
	}

	sess, err := client.mountSession(ctx, uri.sessionIdx)
	if err != nil {
		return fail(err)
	}
	rels = append(rels, sess.Release)

	spaceID := uri.spaceID
	if spaceID == "" {
		spaceID, err = client.getSpaceByName(ctx, sess, "")
		if err != nil {
			return fail(errors.Wrap(err, "resolve default space"))
		}
	}

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return fail(err)
	}
	rels = append(rels, spaceCleanup)

	objectKey := uri.objectKey
	if resolveObjectKey != nil {
		objectKey, err = resolveObjectKey(ctx, spaceSvc)
		if err != nil {
			return fail(err)
		}
	}

	engine, engineRef, engineCleanup, err := client.accessWorldEngineWithRef(ctx, spaceSvc)
	if err != nil {
		return fail(err)
	}
	rels = append(rels, engineCleanup)

	typedClient, _, _, typedCleanup, err := client.accessTypedObject(ctx, engineRef, objectKey)
	if err != nil {
		return fail(errors.Wrap(err, "access typed object for "+objectKey))
	}
	rels = append(rels, typedCleanup)

	mount := &objectMount{
		client:        client,
		sess:          sess,
		spaceSvc:      spaceSvc,
		engineRef:     engineRef,
		engine:        engine,
		typedClient:   typedClient,
		objectKey:     objectKey,
		typedCleanup:  typedCleanup,
		engineCleanup: engineCleanup,
		spaceCleanup:  spaceCleanup,
	}
	cleanup := func() { unwind() }
	return mount, cleanup, nil
}
