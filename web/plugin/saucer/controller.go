package saucer

import (
	"context"
	"io"
	"net/http"
	"strings"

	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	runtime_controller "github.com/aperturerobotics/bldr/web/runtime/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// ControllerID is the saucer runtime controller ID.
const ControllerID = "bldr/web/plugin/saucer"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// RuntimeID is the runtime identifier.
const RuntimeID = "saucer"

// Controller is the saucer runtime controller.
// Communicates with the Saucer webview via IPC.
type Controller struct {
	le  *logrus.Entry
	bus bus.Bus

	saucerPath  string
	workdirPath string
	runtimeUuid string

	extraSaucerArgs []string
	saucerInit      *SaucerInit

	execSema  *semaphore.Weighted
	saucerCtr *ccontainer.CContainer[*Saucer]
}

// NewController constructs a new Saucer runtime controller.
// sessionUuid is used to make the unix pipe path unique.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	saucerPath, workdirPath,
	runtimeUuid string,
	extraSaucerArgs []string,
	saucerInit *SaucerInit,
) (*Controller, error) {
	return &Controller{
		le:  le,
		bus: b,

		saucerPath:      saucerPath,
		workdirPath:     workdirPath,
		runtimeUuid:     runtimeUuid,
		extraSaucerArgs: extraSaucerArgs,
		saucerInit:      saucerInit,

		execSema:  semaphore.NewWeighted(1),
		saucerCtr: ccontainer.NewCContainer[*Saucer](nil),
	}, nil
}

// GetControllerInfo returns information about the controller.
func (r *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"Saucer "+r.runtimeUuid,
	)
}

// GetLogger returns the root log entry.
func (r *Controller) GetLogger() *logrus.Entry {
	return r.le
}

// GetBus returns the root controller bus to use in this process.
func (r *Controller) GetBus() bus.Bus {
	return r.bus
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Controller) Execute(ctx context.Context) error {
	err := r.execSema.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	defer r.execSema.Release(1)

	s, err := RunSaucer(
		ctx,
		r.le,
		r.saucerPath,
		r.workdirPath,
		r.runtimeUuid,
		r.extraSaucerArgs,
		r.saucerInit,
	)
	if err != nil {
		return err
	}
	defer s.Close()
	defer r.saucerCtr.SetValue(nil)

	// Start the debug bridge Unix socket in the background.
	go func() {
		dle := r.le.WithField("component", "debug-bridge")
		if err := runDebugSocket(ctx, dle, s.conn, r.workdirPath); err != nil {
			if ctx.Err() == nil {
				dle.WithError(err).Warn("debug bridge exited")
			}
		}
	}()

	// Construct the runtime controller and execute it on the bus.
	rc := runtime_controller.NewController(
		r.le,
		r.bus,
		func(
			ctx context.Context,
			le *logrus.Entry,
			handler web_runtime.WebRuntimeHandler,
		) (web_runtime.WebRuntime, error) {
			mc := s.GetMuxedConn()

			// Create the DocumentManager to handle document lifecycle.
			docMgr := NewDocumentManager(le)

			// Create a local SRPC mux with DocumentManager as the WebRuntime service.
			// This allows Remote to call WatchWebRuntimeStatus/WebDocumentRpc on the DocumentManager.
			mux := srpc.NewMux()
			if err := web_runtime.SRPCRegisterWebRuntime(mux, docMgr); err != nil {
				return nil, err
			}
			srv := srpc.NewServer(mux)
			loopbackClient := srpc.NewClient(srpc.NewServerPipe(srv))

			// Build a combined HTTP handler that routes /b/saucer/* to the
			// DocumentManager and everything else to the runtime controller.
			rtCtrl, ok := handler.(*runtime_controller.Controller)
			if !ok {
				return nil, errors.Errorf("expected runtime controller handler, got %T", handler)
			}
			httpHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				if strings.HasPrefix(req.URL.Path, "/b/saucer/") {
					docMgr.ServeSaucerHTTP(rw, req)
					return
				}
				rtCtrl.ServeServiceWorkerHTTP(rw, req)
			})

			// Create the RequestHandler that accepts yamux streams from C++.
			var bootstrapHTML, entrypointJS string
			if r.saucerInit != nil {
				bootstrapHTML = r.saucerInit.BootstrapHtml
				entrypointJS = r.saucerInit.EntrypointJs
			}
			reqHandler := NewRequestHandler(le, docMgr, httpHandler, bootstrapHTML, entrypointJS)

			// Construct the Remote with the loopback client and accept loop.
			remote, err := web_runtime.NewRemote(
				le,
				r.bus,
				handler,
				r.runtimeUuid,
				loopbackClient,
				func(ctx context.Context, _ *web_runtime.Remote) error {
					le.Debug("starting yamux accept loop")
					err := reqHandler.AcceptStreams(ctx, mc)
					le.WithError(err).Debug("yamux accept loop exited")
					if err == io.EOF || err == context.Canceled {
						// Block until context is canceled to prevent the
						// Remote from exiting when the yamux session closes.
						<-ctx.Done()
						return context.Canceled
					}
					return err
				},
			)
			if err != nil {
				return nil, err
			}

			// Set the Remote's SRPC server on the DocumentManager so
			// JS-initiated RPC streams are handled locally (WebRuntimeHost).
			docMgr.SetServer(remote.GetRpcServer())

			r.saucerCtr.SetValue(s)
			return remote, nil
		},
		ControllerID,
		Version,
	)

	err = r.bus.ExecuteController(ctx, rc)
	if err != nil && err != context.Canceled && err.Error() != "stream reset" {
		r.le.WithError(err).Error("saucer remote runtime exited with error")
		return err
	}
	r.le.Info("exiting")
	return err
}

// WaitSaucer waits for the Saucer object to be ready and returns it.
// if errCh is set, checks it for errors to return early.
func (r *Controller) WaitSaucer(ctx context.Context, errCh <-chan error) (*Saucer, error) {
	saucerCtr, err := r.saucerCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return saucerCtr, nil
}

// HandleDirective asks if the handler can resolve the directive.
func (r *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close closes the runtime.
func (r *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
