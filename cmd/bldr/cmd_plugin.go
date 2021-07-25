package main

import (
	plugin_cli "github.com/aperturerobotics/controllerbus/plugin/cli"
)

var pluginCompilerFlags plugin_cli.CompilerArgs

func init() {
	commands = append(commands, pluginCompilerFlags.BuildDevtoolCommand())
}
