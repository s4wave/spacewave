//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// newLogoutCommand builds the logout command.
func newLogoutCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:  "logout",
		Usage: "revoke the current cloud session",
		Flags: append(clientFlags(&statePath, &sessionIdx), pemFileFlag(&pemFile)),
		Action: func(c *cli.Context) error {
			return runLogout(c, statePath, uint32(sessionIdx), pemFile)
		},
	}
}

// runLogout implements the logout command logic.
func runLogout(c *cli.Context, statePath string, sessionIdx uint32, pemFile string) error {
	ctx := c.Context
	resolved, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return err
	}

	cred, err := promptCredential(pemFile)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

	peerID := info.GetPeerId()
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	_, err = acctSvc.RevokeSession(ctx, &s4wave_account.RevokeSessionRequest{
		SessionPeerId: peerID,
		Credential:    cred,
	})
	if err != nil {
		return errors.Wrap(err, "revoke session")
	}

	os.Stdout.WriteString("Logged out.\n")
	return nil
}
