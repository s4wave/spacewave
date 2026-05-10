//go:build !js

package spacewave_cli

import (
	"slices"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestPluralCommandAliases(t *testing.T) {
	cases := []struct {
		name   string
		plural string
		cmd    *cli.Command
	}{
		{name: "space", plural: "spaces", cmd: newSpaceCommand(nil)},
		{name: "canvas", plural: "canvases", cmd: newCanvasCommand(nil)},
		{name: "account", plural: "accounts", cmd: newAccountCommand(nil)},
		{name: "session", plural: "sessions", cmd: newSessionCommand(nil)},
		{name: "provider", plural: "providers", cmd: newProviderCommand(nil)},
		{name: "plugin", plural: "plugins", cmd: newPluginCommand(nil)},
		{name: "vm", plural: "vms", cmd: newVmCommand(nil)},
	}
	for _, c := range cases {
		if !hasAlias(c.cmd.Aliases, c.plural) {
			t.Fatalf("%s alias missing from %s command", c.plural, c.name)
		}
	}
}

func hasAlias(aliases []string, want string) bool {
	return slices.Contains(aliases, want)
}
