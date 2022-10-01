package main

import plugin_entrypoint "github.com/aperturerobotics/bldr/plugin/entrypoint"

func main() {
	plugin_entrypoint.Main(
		[]plugin_entrypoint.AddFactoryFunc{},
		[]plugin_entrypoint.BuildConfigSetFunc{},
	)
}
