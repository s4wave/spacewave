//go:build !js

package bldr_project_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	controllerbus_controller "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/util/keyed"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/sirupsen/logrus"
)

func TestCloseJoinsPostExecuteTrackerWork(t *testing.T) {
	// Execute the project controller before adding tracker work.
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, &Config{})
	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Start a manifest tracker whose shutdown is held until the test releases it.
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	ctrl.manifestBuilders = keyed.NewKeyedRefCount(func(string) (keyed.Routine, *manifestBuilderTracker) {
		return ctrl.routines.Wrap(func(ctx context.Context) error {
			// Signal tracker startup and hold its shutdown after context cancellation.
			close(started)
			<-ctx.Done()
			<-release
			close(stopped)
			return nil
		}), &manifestBuilderTracker{}
	})
	ctrl.manifestBuilders.SetContext(context.Background(), true)
	ctrl.manifestBuilders.AddKeyRef("post-execute")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("manifest tracker did not start")
	}

	// Begin closing the controller and verify it waits for the running tracker.
	closeDone := make(chan struct{})
	go func() {
		_ = ctrl.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned before the tracker stopped")
	case <-time.After(100 * time.Millisecond):
	}

	// Release the tracker shutdown and wait for controller closure.
	close(release)
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the tracker stopped")
	}

	// Verify tracker shutdown, repeated closure, and rejection of new remote references.
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned before the tracker stop signal")
	}
	if err := ctrl.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := ctrl.AddRemoteRef("after-close")
	if err != errControllerClosed {
		t.Fatalf("AddRemoteRef after Close: got %v, want %v", err, errControllerClosed)
	}
}

type startupTestBus struct {
	*inmem.Bus
	addDirectiveCalled  chan struct{}
	addDirectiveRelease <-chan struct{}
	controllers         []controllerbus_controller.Controller
}

func (b *startupTestBus) AddDirective(
	dir directive.Directive,
	ref directive.ReferenceHandler,
) (directive.Instance, directive.Reference, error) {
	if b.addDirectiveCalled != nil {
		close(b.addDirectiveCalled)
	}
	if b.addDirectiveRelease != nil {
		<-b.addDirectiveRelease
	}
	return b.Bus.AddDirective(dir, ref)
}

func (b *startupTestBus) GetControllers() []controllerbus_controller.Controller {
	return b.controllers
}

func TestStartupRejectsAfterCloseBegins(t *testing.T) {
	// Execute a project controller with an observable startup directive bus.
	logger := logrus.NewEntry(logrus.New())
	baseBus := inmem.NewBus(directive_controller.NewController(context.Background(), logger))
	startupBus := &startupTestBus{
		Bus:                baseBus,
		addDirectiveCalled: make(chan struct{}),
	}
	ctrl := NewController(logger, startupBus, &Config{})
	if err := ctrl.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Start a manifest tracker that holds controller shutdown open.
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	ctrl.manifestBuilders = keyed.NewKeyedRefCount(func(string) (keyed.Routine, *manifestBuilderTracker) {
		return ctrl.routines.Wrap(func(ctx context.Context) error {
			// Signal tracker startup and hold its shutdown after context cancellation.
			close(started)
			<-ctx.Done()
			<-release
			close(stopped)
			return nil
		}), &manifestBuilderTracker{}
	})
	ctrl.manifestBuilders.SetContext(context.Background(), true)
	ctrl.manifestBuilders.AddKeyRef("startup-close-barrier")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("manifest tracker did not start")
	}

	// Begin closing the controller while its manifest tracker is blocked.
	closeDone := make(chan struct{})
	go func() {
		_ = ctrl.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned before the tracker stopped")
	case <-time.After(100 * time.Millisecond):
	}

	// Request startup during closure and verify the startup routine exits without loading plugins.
	ctrl.startup.SetContext(context.Background(), true)
	ctrl.startup.SetState(&bldr_project.StartConfig{Plugins: []string{""}})
	startupExited := make(chan error, 1)
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		startupExited <- ctrl.startup.WaitExited(waitCtx, false, nil)
	}()
	select {
	case <-startupBus.addDirectiveCalled:
		t.Fatal("executeStartup ran after Close began")
	case err := <-startupExited:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup exited with %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup routine did not exit after Close began")
	}

	// Release the blocked tracker and verify controller closure joins its shutdown.
	close(release)
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the tracker stopped")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned before the tracker stop signal")
	}
}

func TestCloseJoinsStartupWork(t *testing.T) {
	// Prepare a startup bus that holds plugin loading until the test releases it.
	logger := logrus.NewEntry(logrus.New())
	baseBus := inmem.NewBus(directive_controller.NewController(context.Background(), logger))
	started := make(chan struct{})
	release := make(chan struct{})
	startupBus := &startupTestBus{
		Bus:                 baseBus,
		addDirectiveCalled:  started,
		addDirectiveRelease: release,
	}
	startupBus.controllers = []controllerbus_controller.Controller{
		plugin_host_scheduler.NewController(
			logger,
			baseBus,
			plugin_host_scheduler.NewConfig("", "", "", "", "", false, false, false),
		),
	}

	// Start project startup and wait for plugin loading to reach the bus.
	ctrl := NewController(logger, startupBus, &Config{})
	ctrl.startup.SetContext(context.Background(), true)
	ctrl.startup.SetState(&bldr_project.StartConfig{Plugins: []string{""}})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("startup did not enter executeStartup")
	}

	// Begin closing the controller and verify it waits for active startup work.
	closeDone := make(chan struct{})
	go func() {
		_ = ctrl.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned while startup was inside executeStartup")
	case <-time.After(100 * time.Millisecond):
	}

	// Release plugin loading and verify controller closure joins startup work.
	close(release)
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after startup finished")
	}
}
