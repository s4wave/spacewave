package electron

import (
	"context"
	"io"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver/v4"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	runtime_controller "github.com/s4wave/spacewave/bldr/web/runtime/controller"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// ControllerID is the browser runtime controller ID.
const ControllerID = "bldr/web/plugin/electron"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

const quitWaitTimeout = 2 * time.Second

// RuntimeID is the runtime identifier
const RuntimeID = "electron"

// Controller is the electron runtime controller.
//
// Communicates with the electron Renderer via IPC.
type Controller struct {
	le  *logrus.Entry
	bus bus.Bus

	electronPath string
	workdirPath  string
	rendererPath string
	runtimeUuid  string

	extraElectronArgs []string
	electronInit      *ElectronInit

	execSema    *semaphore.Weighted
	electronCtr *ccontainer.CContainer[*Electron]
}

// NewController constructs a new browser runtime which starts Electron.
// sessionUuid is used to make the unix pipe path unique.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	electronPath, workdirPath, rendererPath,
	runtimeUuid string,
	extraElectronArgs []string,
	electronInit *ElectronInit,
) (*Controller, error) {
	return &Controller{
		le:  le,
		bus: b,

		electronPath:      electronPath,
		workdirPath:       workdirPath,
		rendererPath:      rendererPath,
		runtimeUuid:       runtimeUuid,
		extraElectronArgs: extraElectronArgs,
		electronInit:      electronInit,

		execSema:    semaphore.NewWeighted(1),
		electronCtr: ccontainer.NewCContainer[*Electron](nil),
	}, nil
}

// GetControllerInfo returns information about the controller.
func (r *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"Electron "+r.runtimeUuid,
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

	e, err := RunElectron(
		ctx,
		r.le,
		r.electronPath,
		r.workdirPath,
		r.rendererPath,
		r.runtimeUuid,
		r.extraElectronArgs,
		r.electronInit,
	)
	if err != nil {
		return err
	}
	defer e.Close()
	defer r.electronCtr.SetValue(nil)

	// construct the runtime controller and execute it on the bus.
	rc := runtime_controller.NewController(
		r.le,
		r.bus,
		func(
			ctx context.Context,
			le *logrus.Entry,
			handler web_runtime.WebRuntimeHandler,
		) (web_runtime.WebRuntime, error) {
			mc := e.GetMuxedConn()
			srpcClient := srpc.NewClientWithMuxedConn(mc)
			remote, err := web_runtime.NewRemote(
				r.le,
				r.bus,
				handler,
				r.runtimeUuid,
				srpcClient,
				func(ctx context.Context, r *web_runtime.Remote) error {
					return r.GetRpcServer().AcceptMuxedConn(ctx, mc)
				},
			)
			if err != nil {
				return nil, err
			}
			var webController web_runtime.WebRuntime = remote
			r.electronCtr.SetValue(e)
			return webController, nil
		},
		ControllerID,
		Version,
	)

	err = r.bus.ExecuteController(ctx, rc)
	if r.shouldExitWithoutRestart(err, e) {
		r.le.Info("electron exited cleanly; stopping without restart")
		return nil
	}
	if err != nil && err != context.Canceled && err.Error() != "stream reset" {
		r.le.WithError(err).Error("electron remote runtime exited with error")
	} else {
		r.le.Info("exiting")
	}

	return err
}

func (r *Controller) shouldExitWithoutRestart(err error, e *Electron) bool {
	if err != io.EOF {
		return false
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), quitWaitTimeout)
	defer waitCancel()
	waitErr := e.Wait(waitCtx)
	return shouldExitWithoutRestart(err, waitErr, r.electronInit.GetQuitPolicy())
}

func shouldExitWithoutRestart(
	runtimeErr error,
	processErr error,
	quitPolicy QuitPolicy,
) bool {
	if runtimeErr != io.EOF {
		return false
	}
	if quitPolicy != QuitPolicy_QUIT_POLICY_EXIT {
		return false
	}
	return processErr == nil
}

// WaitElectron waits for the Electron object to be ready and returns it.
// if errCh is set, checks it for errors to return early.
func (r *Controller) WaitElectron(ctx context.Context, errCh <-chan error) (*Electron, error) {
	electronCtr, err := r.electronCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return electronCtr, nil
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
