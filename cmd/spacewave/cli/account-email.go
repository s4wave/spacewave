//go:build !js

package spacewave_cli

import (
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// newAccountEmailCommand builds the account email command group.
func newAccountEmailCommand() *cli.Command {
	return &cli.Command{
		Name:  "email",
		Usage: "manage the cloud account's email addresses",
		Subcommands: []*cli.Command{
			newAccountEmailAddCommand(),
		},
	}
}

// newAccountEmailAddCommand builds the account email add command.
func newAccountEmailAddCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:      "add",
		Usage:     "add an email address and send its verification email",
		ArgsUsage: "<address>",
		Flags:     clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			// Require the address argument.
			address := strings.TrimSpace(c.Args().First())
			if address == "" {
				return errors.New("email address required")
			}

			// Mount the selected cloud Session.
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()
			sess, err := client.mountSession(ctx, sessionIndex32(sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Add the address to the Session's account.
			sessionClient, err := sess.GetResourceRef().GetClient()
			if err != nil {
				return errors.Wrap(err, "session client")
			}
			svc := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)
			resp, err := svc.AddEmail(ctx, &s4wave_provider_spacewave.AddEmailRequest{Email: address})
			if err != nil {
				return errors.Wrap(err, "add email")
			}

			// Report whether a verification email went out.
			if resp.GetSent() {
				os.Stdout.WriteString("Added " + address + "; a verification email was sent.\n")
				return nil
			}
			os.Stdout.WriteString("Added " + address + ".\n")
			return nil
		},
	}
}
