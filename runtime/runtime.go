package runtime

import (
	"context"
	"os"

	"github.com/aperturerobotics/bldr/runtime/core"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/example/boilerplate"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	boilerplate_v1 "github.com/aperturerobotics/controllerbus/example/boilerplate/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Runtime is the environment-specific implementation of IPC and browser window management.
type Runtime interface {
	// GetContext returns the root context of the environment.
	GetContext() context.Context
	// GetLogger returns the root log entry.
	GetLogger() *logrus.Entry
	// GetBus returns the root controller bus to use in this process.
	GetBus() bus.Bus
	// GetStorage returns the set of available storage providers.
	GetStorage() []Storage
	// GetWebViews returns the current snapshot of active WebViews.
	GetWebViews() []WebView
	// CreateWebView creates a new web view and waits for it to become active.
	//
	// Returns ErrWebViewUnavailable if WebView is not available or cannot be created.
	CreateWebView(ctx context.Context) (WebView, error)
	// Execute executes the runtime.
	// Returns any errors, nil if Execute is not required.
	Execute(ctx context.Context) error
	// Close closes the runtime and waits for Execute to finish if wait is set.
	// if ctx is nil, don't wait for the close to complete.
	Close(ctx context.Context) error
}

// Run constructs and executes the runtime with defaults.
//
// WebViews is the default set of WebView pre-created by the environment.
func Run(ctx context.Context, le *logrus.Entry, rt Runtime) error {
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}

	// TODO: select 1 cache and 1 main storage (or 1 storage with no cache)
	// construct one of all the available storages
	storageProviders := rt.GetStorage()
	for _, st := range storageProviders {
		vc := st.BuildVolumeConfig("aperture")
		_, _, volRef, err := loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(vc),
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "start volume controller")
		}
		defer volRef.Release()
	}

	exePath, _ := os.Executable()
	le.
		WithField("current-path", exePath).
		Info("environment bound, executing runtime")

	// cross platform demo
	sr.AddFactory(boilerplate_controller.NewFactory(b))
	execDir := resolver.NewLoadControllerWithConfig(&boilerplate_controller.Config{
		ExampleField: "hello cross-platform",
	})
	_, ctrlRef, err := bus.ExecOneOff(ctx, b, execDir, nil)
	if err != nil {
		le.WithError(err).Warn("failed to exec boilerplate controller")
	}
	if ctrlRef != nil {
		defer ctrlRef.Release()
	}

	res, resRef, err := bus.ExecOneOff(ctx, b, &boilerplate_v1.Boilerplate{
		MessageText: "hello from a directive",
	}, nil)
	if err != nil {
		le.WithError(err).Warn("failed to exec boilerplate controller directive")
	}
	resRef.Release()
	plen := res.GetValue().(boilerplate.BoilerplateResult).GetPrintedLen()
	le.Infof("successfully executed directive, logged %d chars", plen)

	errCh := make(chan error, 1)
	go func() {
		if err := rt.Execute(ctx); err != nil {
			errCh <- err
		}
	}()

	// TODO remove this test
	le.Info("runtime: creating a new webview")
	wv, err := rt.CreateWebView(ctx)
	if err != nil {
		le.WithError(err).Warn("failed to create webview")
	} else {
		defer wv.Close()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return nil
	}
}
