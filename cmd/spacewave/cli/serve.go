//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	resource "github.com/s4wave/spacewave/bldr/resource"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	terminal_remoteshell "github.com/s4wave/spacewave/core/terminal/remoteshell"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	db_world "github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

const daemonPluginHostObjectKey = "plugin-host"

// newServeCommand builds the serve command that starts the daemon
// with a resource service socket listener.
func newServeCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	var startupPipeID string
	var runtimeTracePath string
	var takeover bool
	return &cli.Command{
		Name:  "serve",
		Usage: "start the daemon and listen for CLI connections",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "takeover",
				Usage:       "ask any existing runtime on the socket to yield",
				Destination: &takeover,
			},
			&cli.StringFlag{
				Name:        "daemon-startup-pipe-id",
				Usage:       "internal startup pipe identifier",
				Destination: &startupPipeID,
				Hidden:      true,
			},
			&cli.StringFlag{
				Name:        "trace",
				Usage:       "write a Go runtime trace for the daemon process",
				EnvVars:     []string{daemonTracePathEnvVar},
				Destination: &runtimeTracePath,
			},
		},
		Action: func(c *cli.Context) (retErr error) {
			return runWithRuntimeTrace(runtimeTracePath, func() error {
				return runServeCommand(c, getBus, startupPipeID, takeover)
			})
		},
	}
}

func runServeCommand(
	c *cli.Context,
	getBus func() cli_entrypoint.CliBus,
	startupPipeID string,
	takeover bool,
) (retErr error) {
	ctx := c.Context

	resolved, err := resolveStatePathFromContext(c, "")
	if err != nil {
		return err
	}
	startupNotifier, err := newDaemonStartupNotifier(ctx, resolved, startupPipeID)
	if err != nil {
		return err
	}
	defer func() {
		if startupNotifier == nil {
			return
		}
		if retErr != nil {
			startupNotifier.reportError(retErr)
			return
		}
		startupNotifier.close()
	}()

	sockPath := filepath.Join(resolved, socketName)
	cliBus := getBus()
	if cliBus == nil {
		return errors.New("bus not initialized")
	}
	le := cliBus.GetLogger()
	serveCtx, serveCancel := context.WithCancel(ctx)
	handoffBroker := resource_listener.GetProcessYieldBroker()
	handoffBroker.BeginHandoff("spacewave serve", sockPath)
	defer func() {
		serveCancel()
		handoffBroker.Reclaim()
	}()
	releasePluginRuntime, err := startDaemonPluginRuntime(serveCtx, resolved, cliBus)
	if err != nil {
		return err
	}
	defer releasePluginRuntime()

	idleTimeout, err := getDaemonIdleTimeout()
	if err != nil {
		return err
	}

	le.Info("waiting for resource service")
	invoker, invokerRef, err := waitForResourceService(
		serveCtx,
		cliBus,
		cliBus.GetPluginHostObjectKey() != "",
	)
	if err != nil {
		return err
	}
	defer invokerRef.Release()
	devicePolicy, err := device_policy.NewPolicyStore(resolved)
	if err != nil {
		return err
	}
	startDeviceLauncherUpdateProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker)
	startDevicePolicyCapabilityProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker, devicePolicy)
	releaseDeviceRemoteShell := terminal_remoteshell.StartHandler(serveCtx, le, cliBus.GetBus(), devicePolicy)
	defer releaseDeviceRemoteShell()

	if takeover {
		if err := takeoverDaemonSocket(ctx, le, sockPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return err
	}

	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockPath, Net: "unix"})
	if err != nil {
		return errors.Wrapf(err, "listen on daemon socket %s; use --takeover only if you intend to ask another runtime to yield", sockPath)
	}
	defer lis.Close()

	if err := os.Chmod(sockPath, 0o600); err != nil {
		le.WithError(err).Warn("failed to chmod socket")
	}

	le.Infof("listening on %s", sockPath)
	idleTracker := newDaemonIdleTracker(idleTimeout, func() {
		le.Info("daemon idle timeout reached, shutting down")
		serveCancel()
		lis.Close()
	})
	defer idleTracker.close()
	releaseWebKeepalive := resource_root.SetWebListenerKeepaliveFunc(func(listenerID string) func() {
		le.WithField("listener", listenerID).Debug("web listener holding daemon lifetime")
		return idleTracker.serviceAttached()
	})
	defer releaseWebKeepalive()

	mux := srpc.NewMux(invoker)
	if err := mux.Register(newDaemonControlHandler(func() {
		serveCancel()
		lis.Close()
	})); err != nil {
		return err
	}
	if err := mux.Register(newDevicePolicyControlHandler(devicePolicy.Reload)); err != nil {
		return err
	}
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		return err
	}
	go func() {
		<-serveCtx.Done()
		lis.Close()
	}()

	srv := srpc.NewServer(mux)
	if err := startupNotifier.reportReady(); err != nil {
		return err
	}
	err = acceptDaemonListener(serveCtx, lis, srv, idleTracker)
	if err != nil && (serveCtx.Err() != nil || stderrors.Is(err, net.ErrClosed)) {
		return nil
	}
	return err
}

func startDaemonPluginRuntime(
	ctx context.Context,
	stateRoot string,
	cliBus cli_entrypoint.CliBus,
) (func(), error) {
	pluginRoot := filepath.Join(stateRoot, "plugin")
	pluginStateRoot := filepath.Join(pluginRoot, "state")
	pluginDistRoot := filepath.Join(pluginRoot, "dist")
	for _, dir := range []string{pluginStateRoot, pluginDistRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	var rels []func()
	rel := func() {
		for _, v := range slices.Backward(rels) {
			v()
		}
	}

	lookupOpCtrl := db_world.NewLookupOpController(
		"bldr-manifest-ops",
		cliBus.GetWorldEngineID(),
		bldr_manifest_world.LookupOp,
	)
	relLookupCtrl, err := cliBus.GetBus().AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		return nil, err
	}
	rels = append(rels, relLookupCtrl)

	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(
		ctx,
		cliBus.GetWorldEngine(),
		daemonPluginHostObjectKey,
	); err != nil {
		rel()
		return nil, err
	}

	_, relPluginSched, err := plugin_host_default.StartPluginScheduler(
		ctx,
		cliBus.GetBus(),
		cliBus.GetWorldEngineID(),
		daemonPluginHostObjectKey,
		cliBus.GetVolume().GetID(),
		cliBus.GetVolume().GetPeerID().String(),
		true,
		true,
		true,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relPluginSched)

	_, relPluginHost, err := plugin_host_default.StartPluginHost(
		ctx,
		cliBus.GetBus(),
		pluginStateRoot,
		pluginDistRoot,
		"",
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relPluginHost)

	return rel, nil
}

// waitForResourceService waits for the resource service to appear, and on dist
// runtimes surfaces launcher bootstrap failures when no usable DistConfig exists.
func waitForResourceService(
	ctx context.Context,
	busCtx cli_entrypoint.CliBus,
	watchLauncher bool,
) (srpc.Invoker, directive.Reference, error) {
	b := busCtx.GetBus()
	serviceID := resource.SRPCResourceServiceServiceID
	resourceCh := make(chan srpc.Invoker, 1)
	resourceHandler := directive.NewTypedCallbackHandler[srpc.Invoker](
		func(v directive.TypedAttachedValue[srpc.Invoker]) {
			select {
			case resourceCh <- v.GetValue():
			default:
			}
		},
		nil,
		nil,
		nil,
	)
	_, resourceRef, err := b.AddDirective(
		bifrost_rpc.NewLookupRpcService(serviceID, ""),
		resourceHandler,
	)
	if err != nil {
		return nil, nil, err
	}

	if !watchLauncher {
		select {
		case <-ctx.Done():
			resourceRef.Release()
			return nil, nil, ctx.Err()
		case invoker := <-resourceCh:
			return invoker, resourceRef, nil
		}
	}

	launcherErrCh := make(chan error, 1)
	fetchHandler := directive.NewTypedCallbackHandler[*spacewave_launcher.FetchStatus](
		func(v directive.TypedAttachedValue[*spacewave_launcher.FetchStatus]) {
			st := v.GetValue()
			if st == nil || st.Fetching || st.HasConfig || st.LastErr == "" {
				return
			}
			err := errors.Errorf(
				"launcher bootstrap failed: %s",
				strings.TrimSpace(st.LastErr),
			)
			select {
			case launcherErrCh <- err:
			default:
			}
		},
		nil,
		nil,
		nil,
	)
	_, fetchRef, err := b.AddDirective(
		spacewave_launcher.NewWatchLauncherFetchStatus(projectID),
		fetchHandler,
	)
	if err != nil {
		resourceRef.Release()
		return nil, nil, errors.Wrap(err, "watch launcher fetch status")
	}
	defer fetchRef.Release()

	select {
	case <-ctx.Done():
		resourceRef.Release()
		return nil, nil, ctx.Err()
	case err := <-launcherErrCh:
		resourceRef.Release()
		return nil, nil, err
	case invoker := <-resourceCh:
		return invoker, resourceRef, nil
	}
}
