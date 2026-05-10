//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// newLogoutCommand builds the logout command.
func newLogoutCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	var yes bool
	var sessionID string
	var accountID string
	return &cli.Command{
		Name:      "logout",
		Usage:     "sign out a local session",
		ArgsUsage: "[session-index|session-id|account-id]",
		Flags:     sessionLogoutFlags(&statePath, &sessionIdx, &sessionID, &accountID, &yes),
		Action: func(c *cli.Context) error {
			return runSessionLogout(c, statePath, uint32(sessionIdx), sessionLogoutTarget{
				Positional: c.Args().First(),
				SessionID:  sessionID,
				AccountID:  accountID,
			}, yes)
		},
	}
}
