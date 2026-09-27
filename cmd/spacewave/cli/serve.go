//go:build !js

package spacewave_cli

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	resource "github.com/s4wave/spacewave/bldr/resource"
	"github.com/s4wave/spacewave/core/daemon"
	desktopcontrol "github.com/s4wave/spacewave/core/daemon/desktopcontrol"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	terminal_remoteshell "github.com/s4wave/spacewave/core/terminal/remoteshell"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

// serveSocketPath selects the exact listener requested by the command and
// falls back to the state-local daemon socket.
func serveSocketPath(c *cli.Context, statePath string) string {
	return effectiveSocketPath(c, filepath.Join(statePath, socketName))
}

// newServeCommand builds the serve command that starts the daemon
// with a resource service socket listener.
func newServeCommand(getBus func() cli_entrypoint.CliBus, yieldBroker *yield_policy.Broker) *cli.Command {
	// Keep serve flags scoped to this command's startup and idle policy.
	var startupPipeID string
	var runtimeTracePath string
	var takeover bool
	idleTimeout := defaultDaemonIdleTimeout
	return &cli.Command{
		Name:  "serve",
		Usage: "start the daemon and listen for CLI connections",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "takeover",
				Usage:       "ask any existing runtime on the socket to yield",
				Destination: &takeover,
			},
			&cli.DurationFlag{
				Name:        "idle-timeout",
				Usage:       "duration controls shutdown after the last active client/service; zero disables idle shutdown",
				Value:       defaultDaemonIdleTimeout,
				Destination: &idleTimeout,
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
				EnvVars:     []string{daemon.TracePathEnvVar},
				Destination: &runtimeTracePath,
			},
		},
		Action: func(c *cli.Context) (retErr error) {
			return runWithRuntimeTrace(runtimeTracePath, func() error {
				return runServeCommand(c, getBus, yieldBroker, startupPipeID, takeover, idleTimeout)
			})
		},
	}
}

// runServeCommand owns the state lease, Resource readiness and common daemon lifetime.
func runServeCommand(
	c *cli.Context,
	getBus func() cli_entrypoint.CliBus,
	yieldBroker *yield_policy.Broker,
	startupPipeID string,
	takeover bool,
	idleTimeout time.Duration,
) (retErr error) {
	// Resolve the command target before any writable bus is requested.
	ctx := c.Context
	resolved, err := resolveStatePathFromContext(c, "")
	if err != nil {
		return err
	}
	startupNotifier, err := daemon.NewStartupNotifier(ctx, resolved, startupPipeID)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			startupNotifier.Error(retErr)
			return
		}
		startupNotifier.Close()
	}()
	if !c.IsSet("idle-timeout") {
		idleTimeout, err = getDaemonIdleTimeout()
		if err != nil {
			return err
		}
	}

	// Suppress the bus-hosted listener while serve owns the public socket.
	sockPath := serveSocketPath(c, resolved)
	handoffBroker := yieldBroker
	handoffBroker.BeginHandoff("spacewave serve", sockPath)
	defer handoffBroker.Reclaim()

	// Hold exclusion through bus teardown, including initialization failures.
	statePathLease, err := prepareDaemonRuntime(ctx, nil, resolved, sockPath, takeover)
	if err != nil {
		return err
	}
	leaseOwned := true
	defer func() {
		if leaseOwned {
			if err := statePathLease.release(); err != nil && retErr == nil {
				retErr = errors.Wrap(err, "release writable state path lease")
			}
		}
	}()

	// Bound the daemon's retained Go heap after busy periods while allowing
	// an explicit GOMEMLIMIT to select a different budget.
	_, hasMemoryLimit := os.LookupEnv("GOMEMLIMIT")
	if !hasMemoryLimit && debug.SetMemoryLimit(-1) == math.MaxInt64 {
		defer debug.SetMemoryLimit(debug.SetMemoryLimit(1 << 30))
	}

	// Transfer lease cleanup to the bus only after writable construction succeeds.
	cliBus := getBus()
	if cliBus == nil {
		return errors.New("bus not initialized")
	}
	le := cliBus.GetLogger()
	cliBus.AddRelease(func() {
		if err := statePathLease.release(); err != nil {
			le.WithError(err).Error("failed to release writable state path lease")
		}
	})
	leaseOwned = false
	serveCtx, serveCancel := context.WithCancel(ctx)
	defer serveCancel()

	// Native core runs on the CLI bus; Dist core runs in the spacewave-core
	// plugin process.
	nativeCore := cliBus.GetPluginHostObjectKey() == ""
	invoker := newDaemonResourceInvoker(cliBus.GetBus())
	if nativeCore {
		var invokerRef directive.Reference
		invoker, invokerRef, err = lookupLocalResourceInvoker(serveCtx, cliBus.GetBus())
		if err != nil {
			return err
		}
		defer invokerRef.Release()

		// Native core owns the same Resource authority as the core plugin.
		// Present it as that plugin before starting plugins that register types.
		releaseCorePlugin, err := cliBus.GetBus().AddController(serveCtx, newNativeCorePlugin(invoker), nil)
		if err != nil {
			return err
		}
		defer releaseCorePlugin()
	}

	// Native hosts supply the same plugin-host services as a distribution bus.
	var releasePluginHost func()
	if nativeCore {
		pluginRoot := filepath.Join(resolved, "plugin")
		pluginStateRoot := filepath.Join(pluginRoot, "state")
		pluginDistRoot := filepath.Join(pluginRoot, "dist")
		for _, dir := range []string{pluginStateRoot, pluginDistRoot} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		_, releasePluginHost, err = plugin_host_default.StartPluginHost(
			serveCtx,
			cliBus.GetBus(),
			pluginStateRoot,
			pluginDistRoot,
			"",
		)
		if err != nil {
			return err
		}
		defer releasePluginHost()
	}

	// Tie background projections and remote services to the serving lifetime.
	devicePolicy, err := device_policy.NewPolicyStore(resolved)
	if err != nil {
		return err
	}
	startLocalSessionKeeper(serveCtx, le, invoker)
	startDeviceLauncherUpdateProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker)
	startDevicePolicyCapabilityProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker, devicePolicy)
	releaseDeviceRemoteShell := terminal_remoteshell.StartHandler(serveCtx, le, cliBus.GetBus(), devicePolicy)
	defer releaseDeviceRemoteShell()

	// Protect and bind the Resource socket before publishing daemon readiness.
	explicitSocket := effectiveSocketPath(c, "") != ""
	lis, err := resource_listener.ListenProtectedUnix(sockPath, !explicitSocket)
	if err != nil {
		return errors.Wrapf(err, "listen on daemon socket %s", sockPath)
	}
	defer lis.Close()

	// Retain startup demand until readiness; then every daemon uses idle expiry.
	le.Infof("listening on %s", sockPath)
	idleTracker := newDaemonIdleTracker(idleTimeout, func() {
		le.Info("daemon idle timeout reached, shutting down")
		serveCancel()
		lis.Close()
	})
	defer idleTracker.close()
	releaseStartupDemand := idleTracker.serviceAttached()
	defer releaseStartupDemand()

	// Persistent services participate in the same idle count as public clients.
	startWebListenerKeepalive(serveCtx, le, invoker, idleTracker)

	// Retain desktop demand independently of the connection that opens it.
	mux := srpc.NewMux(invoker)
	desktopControl := newDaemonDesktopControl(serveCtx, cliBus.GetBus(), idleTracker)
	defer desktopControl.close()
	if err := desktopcontrol.SRPCRegisterDesktopControlService(mux, desktopControl); err != nil {
		return err
	}

	// Register local controls before allowing clients onto the listener.
	shutdownCtx, shutdownCancel := context.WithCancel(serveCtx)
	defer shutdownCancel()
	shutdownCh := shutdownCtx.Done()
	controlHandler := newDaemonControlHandler(func() {
		shutdownCancel()
		lis.Close()
	})
	if err := mux.Register(controlHandler); err != nil {
		return err
	}
	if err := mux.Register(newDevicePolicyControlHandler(devicePolicy.Reload)); err != nil {
		return err
	}
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		return err
	}

	// Verify the actual native or forwarded Resource service before handing
	// custody to the launcher. Public accepts begin only after acknowledgement.
	readyClient, err := buildSDKClientFromInvoker(serveCtx, mux)
	if err != nil {
		return errors.Wrap(err, "initialize daemon Resource service")
	}
	defer readyClient.close()
	if err := startupNotifier.Ready(serveCtx); err != nil {
		return err
	}

	// Wake lease losers even when they consumed the socket's bind event before
	// listen completed. Both core shapes publish the same post-custody event.
	if err := daemon.PublishReady(sockPath); err != nil {
		return errors.Wrap(err, "publish daemon readiness")
	}

	// Public service starts only after the launcher relinquishes custody.
	srv := srpc.NewServer(mux)
	startDeviceCapacityObserver(serveCtx, le, resolved, invoker, devicePolicy)
	releaseStartupDemand()
	return serveDaemonListener(serveCtx, serveCancel, lis, srv, controlHandler, shutdownCh, idleTracker)
}

// lookupLocalResourceInvoker waits for the Resource service already registered
// on the CLI bus and returns the reference that keeps it alive.
func lookupLocalResourceInvoker(
	ctx context.Context,
	b bus.Bus,
) (srpc.Invoker, directive.Reference, error) {
	// Retain the local Resource service directive for the daemon's lifetime.
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		b,
		resource.SRPCResourceServiceServiceID,
		"",
		true,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(invokers) == 0 {
		return nil, nil, errors.New("resource service not found")
	}
	return invokers[0], invokerRef, nil
}

// daemonPluginClientLoader waits for the current spacewave-core generation.
type daemonPluginClientLoader func(context.Context) (srpc.Client, directive.Reference, error)

// daemonResourceInvoker forwards each stream to the current core plugin.
type daemonResourceInvoker struct {
	// loadClient acquires the plugin client and its stream-scoped reference.
	loadClient daemonPluginClientLoader
}

// newDaemonResourceInvoker routes each Resource RPC stream to the current
// spacewave-core plugin generation.
func newDaemonResourceInvoker(b bus.Bus) srpc.Invoker {
	return &daemonResourceInvoker{
		loadClient: func(ctx context.Context) (srpc.Client, directive.Reference, error) {
			return bldr_plugin.ExPluginLoadWaitClient(ctx, b, "spacewave-core", nil)
		},
	}
}

// InvokeMethod retains the selected core generation until the stream ends.
func (i *daemonResourceInvoker) InvokeMethod(
	serviceID, methodID string,
	strm srpc.Stream,
) (bool, error) {
	// Leave non-Resource services to the daemon's local control handlers.
	if serviceID != resource.SRPCResourceServiceServiceID {
		return false, nil
	}

	// Keep the selected plugin alive until this forwarded stream completes.
	client, clientRef, err := i.loadClient(strm.Context())
	if err != nil || clientRef == nil {
		return false, err
	}
	defer clientRef.Release()
	return srpc.NewClientInvoker(client).InvokeMethod(serviceID, methodID, strm)
}

// _ is a type assertion.
var _ srpc.Invoker = (*daemonResourceInvoker)(nil)
