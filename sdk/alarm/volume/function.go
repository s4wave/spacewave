//go:build !goscript

package alarm_volume

import (
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
)

// watchInputType and watchResultType identify the message types an
// alarm/watch function reads and returns.
const (
	watchInputType  = "s4wave.alarm.watch.WatchInput"
	watchResultType = "s4wave.alarm.watch.WatchResult"
)

// Function compiles the Alarm's function as the function/pure object an
// alarm/watch node runs.
func (d *Definition) Function() (*s4wave_function.PureFunction, error) {
	module, err := s4wave_function.Compile(d.Source)
	if err != nil {
		return nil, err
	}
	return &s4wave_function.PureFunction{
		Source:     d.Source,
		Module:     module,
		InputType:  watchInputType,
		OutputType: watchResultType,
	}, nil
}
