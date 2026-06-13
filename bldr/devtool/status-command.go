//go:build !js

package devtool

import (
	"context"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func (d *DevtoolBus) setCommandStartingWithLogFile(name, summary, logFile string) {
	d.SetCommandStatus(devtool_status.BldrDevtoolCommandStatus{
		Name:    name,
		State:   devtool_status.BldrDevtoolCommandStateStarting,
		Summary: summary,
		LogFile: logFile,
	})
}

func (d *DevtoolBus) setCommandRunningWithLogFile(name, summary, logFile string) {
	d.SetCommandStatus(devtool_status.BldrDevtoolCommandStatus{
		Name:    name,
		State:   devtool_status.BldrDevtoolCommandStateRunning,
		Summary: summary,
		LogFile: logFile,
	})
}

func (d *DevtoolBus) finishCommandWithLogFile(ctx context.Context, name, logFile string, err error) {
	command := devtool_status.BldrDevtoolCommandStatus{
		Name:    name,
		State:   devtool_status.BldrDevtoolCommandStateDone,
		LogFile: logFile,
	}
	if err != nil {
		command.State = devtool_status.BldrDevtoolCommandStateError
		command.Error = err.Error()
	}
	if ctx.Err() != nil {
		command.State = devtool_status.BldrDevtoolCommandStateCanceled
		if command.Error == "" {
			command.Error = ctx.Err().Error()
		}
	}
	d.SetCommandStatus(command)
}

func (d *DevtoolBus) finishCommandThenStopTUI(ctx context.Context, name, logFile string, err error, stopTUI func()) {
	d.finishCommandWithLogFile(ctx, name, logFile, err)
	if stopTUI != nil {
		stopTUI()
	}
}
