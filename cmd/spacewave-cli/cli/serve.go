//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	resource "github.com/s4wave/spacewave/bldr/resource"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// newServeCommand builds the serve command that starts the daemon
// with a resource service socket listener.
func newServeCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	var startupPipeID string
	return &cli.Command{
		Name:  "serve",
		Usage: "start the daemon and listen for CLI connections",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "daemon-startup-pipe-id",
				Usage:       "internal startup pipe identifier",
				Destination: &startupPipeID,
				Hidden:      true,
			},
		},
		Action: func(c *cli.Context) (retErr error) {
			ctx := c.Context
			le := logrus.NewEntry(logrus.New())

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
			if err := takeoverDaemonSocket(ctx, le, sockPath); err != nil {
				return err
			}

			cliBus := getBus()
			if cliBus == nil {
				return errors.New("bus not initialized")
			}
			le = cliBus.GetLogger()
			serveCtx, serveCancel := context.WithCancel(ctx)
			defer serveCancel()
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

			_ = os.Remove(sockPath)
			if err := os.MkdirAll(resolved, 0o755); err != nil {
				return err
			}

			lis, err := net.Listen("unix", sockPath)
			if err != nil {
				return err
			}
			defer func() {
				lis.Close()
				_ = os.Remove(sockPath)
			}()

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
		},
	}
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
