//go:build !js

package bldr_cli_compiler

import "strconv"

// CliImport describes a discovered NewCliCommands in a Go package.
type CliImport struct {
	// Alias is the import alias for the package.
	Alias string
	// TakesYieldBroker is true when NewCliCommands declares a second yield
	// broker parameter. Generated wrappers pass the process-shared broker
	// and guard the first bus access with a handoff.
	TakesYieldBroker bool
}

// CommandBuilder formats a command builder using the composition root's broker.
func (c CliImport) CommandBuilder(appName, broker string) string {
	if !c.TakesYieldBroker {
		return c.Alias + ".NewCliCommands"
	}
	return `func(getBus func() cli_entrypoint.CliBus) []*aperture_cli.Command {
	handedOff := false
	protectedGetBus := func() cli_entrypoint.CliBus {
		if !handedOff {
			` + broker + `.BeginHandoff(` + strconv.Quote(appName+" CLI") + `, "")
			handedOff = true
		}
		return getBus()
	}
	return ` + c.Alias + `.NewCliCommands(protectedGetBus, ` + broker + `)
}`
}
