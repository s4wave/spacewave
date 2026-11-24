//go:build !js

package spacewave_cli

import (
	"testing"

	"github.com/aperturerobotics/cli"
)

// TestDevicePolicyCommandExposesSubcommandsAndFlags checks the policy allow-list commands.
func TestDevicePolicyCommandExposesSubcommandsAndFlags(t *testing.T) {
	// Locate every device policy subcommand.
	deviceCmd := newDeviceCommand(nil)
	approveCmd := findTestSubcommand(t, deviceCmd, "approve")
	policyCmd := findTestSubcommand(t, deviceCmd, "policy")

	// Locate the node-type subcommands.
	nodeTypeCmd := findTestSubcommand(t, policyCmd, "node-type")
	nodeTypeAddCmd := findTestSubcommand(t, nodeTypeCmd, "add")
	nodeTypeRemoveCmd := findTestSubcommand(t, nodeTypeCmd, "remove")

	// Check the flags of each located subcommand.
	assertCommandFlags(t, approveCmd, "state-path", "socket-path", "session-index", "space", "ticket")
	assertCommandFlags(t, findTestSubcommand(t, deviceCmd, "show"), "state-path", "socket-path", "session-index", "space", "output")
	assertCommandFlags(t, nodeTypeAddCmd, "state-path", "socket-path")
	assertCommandFlags(t, nodeTypeRemoveCmd, "state-path", "socket-path")

	// Check the launch approval subcommands.
	launchCmd := findTestSubcommand(t, policyCmd, "launch")
	assertCommandFlags(t, findTestSubcommand(t, launchCmd, "approve"), "state-path", "socket-path", "session-index", "space", "host-execution", "allow-child-model")
	assertCommandFlags(t, findTestSubcommand(t, launchCmd, "revoke"), "state-path", "socket-path", "session-index", "space")
}

// findTestSubcommand returns the requested command or fails the test.
func findTestSubcommand(t *testing.T, cmd *cli.Command, name string) *cli.Command {
	// Find the requested command.
	t.Helper()
	for _, sub := range cmd.Subcommands {
		if sub.Name == name {
			return sub
		}
	}

	// Report a missing command.
	t.Fatalf("subcommand %q missing from %s", name, cmd.Name)
	return nil
}
