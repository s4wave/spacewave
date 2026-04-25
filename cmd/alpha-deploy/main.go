//go:build !js

package main

import (
	"os"
	"os/signal"

	appcli "github.com/aperturerobotics/cli"

	deploy_cli "github.com/s4wave/spacewave/cmd/alpha-deploy/cli"
)

var args deploy_cli.DeployArgs

func main() {
	ctx, stop := signal.NotifyContext(args.GetContext(), os.Interrupt)
	defer stop()
	args.SetContext(ctx)

	app := appcli.NewApp()
	app.Name = "alpha-deploy"
	app.HideVersion = true
	app.Usage = "deploy manifests to a running Spacewave Alpha Space"
	app.Commands = []*appcli.Command{
		args.BuildDeployCommand(),
	}
	if err := app.RunContext(ctx, os.Args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
