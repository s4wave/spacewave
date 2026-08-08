//go:build !js && !windows

// Package nativehost supervises a native Spacewave viewer child.
package nativehost

import (
	"os"

	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

// Config describes one immutable native viewer launch.
type Config struct {
	// executable is the absolute native viewer path.
	Executable string
	// launchRecord is the validated launch identity.
	LaunchRecord *native.NativeViewerLaunchRecord
	// stdin is the child input stream.
	Stdin *os.File
	// stdout is the child output stream.
	Stdout *os.File
	// stderr is the child diagnostic stream.
	Stderr *os.File
	// restartLimit is the maximum number of retries after the first attempt.
	RestartLimit uint
	// endpointFactory creates the child endpoint set.
	EndpointFactory EndpointFactory
}
