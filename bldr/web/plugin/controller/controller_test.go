package bldr_web_plugin_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
)

func TestAddControllerSendReadyAndWaitIgnoresNilControllerExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := &nilExitBus{released: make(chan struct{})}
	ctrl := NewController(nil, bus, nil)
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)

	go func() {
		done <- ctrl.addControllerSendReadyAndWait(ctx, noopController{}, func() error {
			ready <- struct{}{}
			return nil
		})
	}()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("ready was not sent")
	}

	select {
	case err := <-done:
		t.Fatalf("addControllerSendReadyAndWait returned after nil controller exit: %v", err)
	case <-bus.released:
		t.Fatal("controller was released after nil controller exit")
	case <-time.After(20 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("addControllerSendReadyAndWait returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("addControllerSendReadyAndWait did not return after context cancellation")
	}
}

func TestAddControllerSendReadyAndWaitReturnsControllerError(t *testing.T) {
	ctx := t.Context()

	wantErr := errors.New("controller failed")
	bus := &nilExitBus{exitErr: wantErr, released: make(chan struct{})}
	ctrl := NewController(nil, bus, nil)

	err := ctrl.addControllerSendReadyAndWait(ctx, noopController{}, func() error {
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("addControllerSendReadyAndWait returned %v, want %v", err, wantErr)
	}
}

type nilExitBus struct {
	noopBus

	exitErr  error
	released chan struct{}
}

func (b *nilExitBus) AddController(ctx context.Context, ctrl controller.Controller, cb func(error)) (func(), error) {
	if cb != nil {
		cb(b.exitErr)
	}
	return func() {
		close(b.released)
	}, nil
}

type noopBus struct{}

func (noopBus) GetControllers() []controller.Controller {
	return nil
}

func (noopBus) GetControllersBroadcast() *broadcast.Broadcast {
	return nil
}

func (noopBus) ExecuteController(context.Context, controller.Controller) error {
	return nil
}

func (noopBus) RemoveController(controller.Controller) {}

func (noopBus) GetDirectives() []directive.Instance {
	return nil
}

func (noopBus) GetDirectivesBroadcast() *broadcast.Broadcast {
	return nil
}

func (noopBus) AddDirective(directive.Directive, directive.ReferenceHandler) (directive.Instance, directive.Reference, error) {
	return nil, nil, nil
}

func (noopBus) AddHandler(directive.Handler) (func(), error) {
	return func() {}, nil
}

type noopController struct{}

func (noopController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("noop", controller.MustParseVersion("0.0.1"), "noop")
}

func (noopController) Execute(context.Context) error {
	return nil
}

func (noopController) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

func (noopController) Close() error {
	return nil
}
