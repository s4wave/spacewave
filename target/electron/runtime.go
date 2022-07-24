package electron

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bldr/storage"
	web_document "github.com/aperturerobotics/bldr/web/document"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/util/ccontainer"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// Runtime is the electron runtime.
//
// Communicates with the electron Renderer via IPC.
type Runtime struct {
	le  *logrus.Entry
	bus bus.Bus

	electronPath string
	rendererPath string
	runtimeUuid  string

	handler  web_runtime.WebRuntimeHandler
	storage  []storage.Storage
	execSema *semaphore.Weighted

	electronCtr *ccontainer.CContainer
	runtimeCtr  *ccontainer.CContainer
}

// NewRuntime constructs a new browser runtime which starts Electron.
// sessionUuid is used to make the unix pipe path unique.
func NewRuntime(
	le *logrus.Entry,
	b bus.Bus,
	handler web_runtime.WebRuntimeHandler,
	st []storage.Storage,
	electronPath, rendererPath,
	runtimeUuid string,
) (*Runtime, error) {
	return &Runtime{
		le:  le,
		bus: b,

		electronPath: electronPath,
		rendererPath: rendererPath,
		runtimeUuid:  runtimeUuid,

		storage:  st,
		execSema: semaphore.NewWeighted(1),
		handler:  handler,

		electronCtr: ccontainer.NewCContainer(nil),
		runtimeCtr:  ccontainer.NewCContainer(nil),
	}, nil
}

// GetLogger returns the root log entry.
func (r *Runtime) GetLogger() *logrus.Entry {
	return r.le
}

// GetBus returns the root controller bus to use in this process.
func (r *Runtime) GetBus() bus.Bus {
	return r.bus
}

// GetStorage returns the set of available storage providers.
func (r *Runtime) GetStorage(ctx context.Context) ([]storage.Storage, error) {
	st := make([]storage.Storage, len(r.storage))
	copy(st, r.storage)
	return st, nil
}

// GetWebDocuments returns the current snapshot of active WebDocuments.
func (r *Runtime) GetWebDocuments(ctx context.Context) (map[string]web_document.WebDocument, error) {
	webRuntime, err := r.WaitWebRuntime(ctx, nil)
	if err != nil {
		return nil, err
	}
	return webRuntime.GetWebDocuments(ctx)
}

// CreateWebDocument creates a new WebDocument and waits for it to become active.
func (r *Runtime) CreateWebDocument(ctx context.Context, webViewID string) (web_document.WebDocument, error) {
	webRuntime, err := r.WaitWebRuntime(ctx, nil)
	if err != nil {
		return nil, err
	}
	return webRuntime.CreateWebDocument(ctx, webViewID)
}

// Execute executes the runtime.
// Returns any errors, nil if Execute is not required.
func (r *Runtime) Execute(ctx context.Context) error {
	err := r.execSema.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	defer r.execSema.Release(1)

	e, err := RunElectron(ctx, r.le, r.electronPath, r.rendererPath, r.runtimeUuid)
	if err != nil {
		return err
	}
	defer e.Close()

	remote, err := web_runtime.NewRemote(r.le, r.bus, r.handler, r.runtimeUuid, e.GetIpc())
	if err != nil {
		return err
	}

	var webRuntime web_runtime.WebRuntime = remote
	r.electronCtr.SetValue(e)
	defer r.electronCtr.SetValue(nil)
	r.runtimeCtr.SetValue(webRuntime)
	defer r.runtimeCtr.SetValue(nil)

	err = remote.Execute(ctx)
	if err != nil && err != context.Canceled {
		r.le.WithError(err).Error("electron remote runtime exited with error")
	} else {
		r.le.Info("exiting")
	}

	return err
}

// WaitWebRuntime waits for the WebRuntime to be ready and returns it.
// if errCh is set, checks it for errors to return early.
func (r *Runtime) WaitWebRuntime(ctx context.Context, errCh <-chan error) (web_runtime.WebRuntime, error) {
	webRuntimeCtr, err := r.runtimeCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	webRuntime, ok := webRuntimeCtr.(web_runtime.WebRuntime)
	if !ok {
		return nil, errors.New("invalid web runtime object in container")
	}
	return webRuntime, nil
}

// WaitElectron waits for the Electron object to be ready and returns it.
// if errCh is set, checks it for errors to return early.
func (r *Runtime) WaitElectron(ctx context.Context, errCh <-chan error) (*Electron, error) {
	electronCtr, err := r.electronCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	electron, ok := electronCtr.(*Electron)
	if !ok {
		return nil, errors.New("invalid electron object in container")
	}
	return electron, nil
}

// Close closes the runtime and waits for Execute to finish if wait is set.
// if ctx is nil, don't wait for the close to complete.
func (r *Runtime) Close(ctx context.Context) error {
	// ctx will already have been canceled;
	if ctx == nil {
		return nil
	}
	// wait for electron to exit
	err := r.execSema.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	r.execSema.Release(1)
	return nil
}

// _ is a type assertion
var _ web_runtime.WebRuntime = ((*Runtime)(nil))
