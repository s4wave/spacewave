//go:build !js && !windows

// Package nativehost supervises a native Spacewave viewer child.
package nativehost

import (
	"os"

	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

// Config describes one immutable native viewer launch.
type Config struct {
	// Executable is the absolute native viewer path.
	Executable string
	// LaunchRecord is the validated launch identity.
	LaunchRecord *native.NativeViewerLaunchRecord
	// Stdin is the child input stream.
	Stdin *os.File
	// Stdout is the child output stream.
	Stdout *os.File
	// Stderr is the child diagnostic stream.
	Stderr *os.File
	// RestartLimit is the maximum number of retries after the first attempt.
	RestartLimit uint
	// EndpointFactory creates the child endpoint set.
	EndpointFactory EndpointFactory
}
