//go:build !js

package devtool

import (
	"context"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// DevtoolTUIRunner starts interactive rendering for devtool command status.
type DevtoolTUIRunner interface {
	Start(context.Context, *devtool_status.BldrDevtoolStatusProducer) (func(), error)
}

// DevtoolTUIRunnerFunc adapts a function into a DevtoolTUIRunner.
type DevtoolTUIRunnerFunc func(context.Context, *devtool_status.BldrDevtoolStatusProducer) (func(), error)

// Start executes the runner function.
func (f DevtoolTUIRunnerFunc) Start(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
) (func(), error) {
	return f(ctx, producer)
}

func (a *DevtoolArgs) startTUIRunner(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
) (func(), error) {
	if !a.ShouldUseTUI() || a.TUIRunner == nil {
		return func() {}, nil
	}
	if runner, ok := a.TUIRunner.(*BldrDevtoolTUIRunner); ok {
		runner.cancelCommand = a.cancelStatusCommand
	}
	stop, err := a.TUIRunner.Start(ctx, producer)
	if err != nil {
		return nil, err
	}
	if stop == nil {
		return func() {}, nil
	}
	return stop, nil
}
