//go:build !js

// Package dev provides the spacewave-dev command surface for downstream
// applications that build on spacewave. It wraps the bldr devtool commands so a
// consuming app gets one pinned tool (setup, start, build) without depending on
// the bldr CLI binary directly, and defaults the local source replacement that
// downstream apps always need.
package dev

import (
	"github.com/aperturerobotics/cli"
	bldr_devtool "github.com/s4wave/spacewave/bldr/devtool"
)

// CLIName is the spacewave-dev binary name.
const CLIName = "spacewave-dev"

// downstreamSrcPath is the default bldr-src-path for a downstream app. bldr-dist
// must replace the app's own (unpublished) module with the local checkout so it
// can import the app's bldr plugins. The path is relative to the dist sources
// root (<state>/src); "../.." resolves to the app repo root for the default
// .bldr/ state dir. Overridable with --bldr-src-path.
const downstreamSrcPath = "../.."

// BuildApp constructs the spacewave-dev CLI application.
func BuildApp() *cli.App {
	args := bldr_devtool.NewDevtoolArgs()
	args.BldrSrcPath = downstreamSrcPath

	app := cli.NewApp()
	app.Name = CLIName
	app.HideVersion = true
	app.Usage = "downstream spacewave application development"
	app.Commands = args.BuildSubCommands()
	app.Flags = args.BuildFlags()
	app.Before = func(c *cli.Context) error {
		switch c.Args().First() {
		case "setup", "start", "build", "static":
			ensureWebSources(c.Context, args)
		}
		return nil
	}
	app.After = func(c *cli.Context) error {
		args.CloseLogFiles()
		return nil
	}
	return app
}
