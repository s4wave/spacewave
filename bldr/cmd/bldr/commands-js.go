//go:build js

package main

import "github.com/aperturerobotics/cli"

// buildCommands leaves native development commands out of JavaScript builds.
func buildCommands() ([]*cli.Command, []cli.Flag, []func()) {
	return nil, nil, nil
}
