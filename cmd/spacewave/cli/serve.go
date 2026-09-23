//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	resource "github.com/s4wave/spacewave/bldr/resource"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	resource_root_controller "github.com/s4wave/spacewave/core/resource/root/controller"
	terminal_remoteshell "github.com/s4wave/spacewave/core/terminal/remoteshell"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
	"github.com/sirupsen/logrus"
)

// serveSocketPath selects the exact listener requested by the command and
// falls back to the state-local daemon socket.
func serveSocketPath(c *cli.Context, statePath string) string {
	return effectiveSocketPath(c, filepath.Join(statePath, socketName))
}

// newServeCommand builds the serve command that starts the daemon
// with a resource service socket listener.
func newServeCommand(getBus func() cli_entrypoint.CliBus, yieldBroker *yield_policy.Broker) *cli.Command {
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
				EnvVars:     []string{daemonTracePathEnvVar},
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

func runServeCommand(
	c *cli.Context,
	getBus func() cli_entrypoint.CliBus,
	yieldBroker *yield_policy.Broker,
	startupPipeID string,
	takeover bool,
	idleTimeout time.Duration,
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
	if !c.IsSet("idle-timeout") {
		idleTimeout, err = getDaemonIdleTimeout()
		if err != nil {
			return err
		}
	}

	sockPath := serveSocketPath(c, resolved)
	handoffBroker := yieldBroker
	handoffBroker.BeginHandoff("spacewave serve", sockPath)
	defer handoffBroker.Reclaim()

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
	var invoker srpc.Invoker
	if nativeCore {
		var invokerRef directive.Reference
		invoker, invokerRef, err = lookupLocalResourceInvoker(serveCtx, cliBus.GetBus())
		if err != nil {
			return err
		}
		defer invokerRef.Release()
		// Native core owns the same Resource authority as the core plugin.
		// Publish its qualified route before starting plugins that register types.
		releaseCoreRoute, err := registerNativeCoreResource(serveCtx, cliBus.GetBus(), invoker)
		if err != nil {
			return err
		}
		defer releaseCoreRoute()
	} else {
		// Each Dist Resource RPC waits for the current spacewave-core generation
		// after its stream is opened.
		invoker = newDaemonResourceInvoker(cliBus.GetBus())
	}
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
	devicePolicy, err := device_policy.NewPolicyStore(resolved)
	if err != nil {
		return err
	}
	startLocalSessionKeeper(serveCtx, le, invoker)
	startDeviceLauncherUpdateProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker)
	startDevicePolicyCapabilityProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker, devicePolicy)
	releaseDeviceRemoteShell := terminal_remoteshell.StartHandler(serveCtx, le, cliBus.GetBus(), devicePolicy)
	defer releaseDeviceRemoteShell()

	_, explicitSocket := lineageFlagSet(c, "socket-path")
	// Protect and bind the Resource socket before publishing daemon readiness.
	lis, err := resource_listener.ListenProtectedUnix(sockPath, !explicitSocket)
	if err != nil {
		return errors.Wrapf(err, "listen on daemon socket %s", sockPath)
	}
	defer lis.Close()

	le.Infof("listening on %s", sockPath)
	idleTracker := newDaemonIdleTracker(idleTimeout, func() {
		le.Info("daemon idle timeout reached, shutting down")
		serveCancel()
		lis.Close()
	})
	defer idleTracker.close()

	if nativeCore {
		releaseRootCtrl, err := holdWebListenerKeepalive(serveCtx, cliBus.GetBus(), le, idleTracker)
		if err != nil {
			return err
		}
		defer releaseRootCtrl()
	}

	mux := srpc.NewMux(invoker)
	shutdownCh := make(chan struct{})
	var shutdownOnce sync.Once
	controlHandler := newDaemonControlHandler(func() {
		shutdownOnce.Do(func() { close(shutdownCh) })
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

	srv := srpc.NewServer(mux)
	startDeviceCapacityObserver(serveCtx, le, resolved, sockPath, devicePolicy)
	if err := startupNotifier.reportReady(); err != nil {
		return err
	}
	return serveDaemonListener(serveCtx, serveCancel, lis, srv, controlHandler, shutdownCh, idleTracker)
}

// holdWebListenerKeepalive waits for the native root resource controller and
// lets its web listeners hold the daemon idle tracker. The returned release
// drops the controller reference.
//
// Only native core runs the root resource controller on the CLI bus. The Dist
// CLI bus has no factory for it, so waiting there would block forever.
func holdWebListenerKeepalive(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	idleTracker *daemonIdleTracker,
) (func(), error) {
	rootCtrl, _, rootCtrlRef, err := loader.WaitExecControllerRunningTyped[*resource_root_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&resource_root_controller.Config{}),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "wait for root resource controller")
	}
	rootCtrl.SetWebListenerKeepaliveFunc(func(listenerID string) func() {
		le.WithField("listener", listenerID).Debug("web listener holding daemon lifetime")
		return idleTracker.serviceAttached()
	})
	return rootCtrlRef.Release, nil
}

// lookupLocalResourceInvoker waits for the Resource service already registered
// on the CLI bus and returns the reference that keeps it alive.
func lookupLocalResourceInvoker(
	ctx context.Context,
	b bus.Bus,
) (srpc.Invoker, directive.Reference, error) {
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

type daemonResourceInvoker struct {
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

func (i *daemonResourceInvoker) InvokeMethod(
	serviceID, methodID string,
	strm srpc.Stream,
) (bool, error) {
	if serviceID != resource.SRPCResourceServiceServiceID {
		return false, nil
	}
	client, clientRef, err := i.loadClient(strm.Context())
	if err != nil || clientRef == nil {
		return false, err
	}
	defer clientRef.Release()
	return srpc.NewClientInvoker(client).InvokeMethod(serviceID, methodID, strm)
}
