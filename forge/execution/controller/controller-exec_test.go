package execution_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestExecuteTargetControllerLogging(t *testing.T) {
	t.Run("canceled shutdown", func(t *testing.T) {
		logger, hook := test.NewNullLogger()
		logger.SetLevel(logrus.WarnLevel)
		le := logrus.NewEntry(logger)
		ctx, cancel := context.WithCancel(t.Context())
		ctrl := &exitController{execute: func(context.Context) error {
			cancel()
			return errors.Wrap(context.Canceled, "controller exit")
		}}
		b := inmem.NewBus(directive_controller.NewController(ctx, le))

		err := (&Controller{le: le}).executeTargetController(ctx, b, ctrl)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("execute error = %v, want context canceled", err)
		}
		if entries := hook.AllEntries(); len(entries) != 0 {
			t.Fatalf("warning count = %d, want 0", len(entries))
		}
	})

	t.Run("real error", func(t *testing.T) {
		logger, hook := test.NewNullLogger()
		logger.SetLevel(logrus.WarnLevel)
		le := logrus.NewEntry(logger)
		wantErr := errors.New("controller failed")
		ctrl := &exitController{execute: func(context.Context) error { return wantErr }}
		b := inmem.NewBus(directive_controller.NewController(t.Context(), le))

		err := (&Controller{le: le}).executeTargetController(t.Context(), b, ctrl)
		if !errors.Is(err, wantErr) {
			t.Fatalf("execute error = %v, want %v", err, wantErr)
		}
		entries := hook.AllEntries()
		if len(entries) != 1 {
			t.Fatalf("warning count = %d, want 1", len(entries))
		}
		if msg := entries[0].Message; msg != "exec controller failed" {
			t.Fatalf("warning message = %q", msg)
		}
	})
}

type exitController struct {
	execute func(context.Context) error
}

func (*exitController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/exit", controller.MustParseVersion("0.0.1"), "test exit controller")
}

func (c *exitController) Execute(ctx context.Context) error {
	return c.execute(ctx)
}

func (*exitController) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

func (*exitController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*exitController)(nil)
