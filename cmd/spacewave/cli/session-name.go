//go:build !js

package spacewave_cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// sessionNameArgs are the arguments of the session name command.
type sessionNameArgs struct {
	statePath  string
	sessionIdx uint
}

// newSessionNameCommand builds the command that names a local account.
func newSessionNameCommand() *cli.Command {
	args := &sessionNameArgs{}
	return &cli.Command{
		Name:      "name",
		Usage:     "name the local account of a session",
		ArgsUsage: "<name>",
		Description: "Sets the display name the account picker and session list show for a local account. " +
			"Every session of the account shows the new name. A Spacewave Cloud account shows its username instead.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the flags that select the session to name.
func (a *sessionNameArgs) BuildFlags() []cli.Flag {
	return clientFlags(&a.statePath, &a.sessionIdx)
}

// Run sets the local account display name through the mounted session.
func (a *sessionNameArgs) Run(c *cli.Context) error {
	// Require one nonblank name.
	if c.NArg() != 1 {
		return errors.New("usage: session name [--session-index N] <name>")
	}
	name := strings.Join(strings.Fields(c.Args().First()), " ")
	if name == "" {
		return errors.New("session name must not be blank")
	}

	// Connect to the daemon and mount the selected session.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return err
	}
	defer client.close()
	idx := sessionIndex32(a.sessionIdx)
	sess, err := client.mountSession(ctx, idx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Only a local account keeps a display name of its own.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	if providerID := info.GetSessionRef().GetProviderResourceRef().GetProviderId(); providerID != provider_local.ProviderID {
		return errors.Errorf("session %d is a %q account, whose name is its username; only a local account can be named", idx, providerID)
	}

	// Set the name through the local session service.
	sessionClient, err := sess.GetResourceRef().GetClient()
	if err != nil {
		return errors.Wrap(err, "session client")
	}
	service := s4wave_session.NewSRPCLocalSessionResourceServiceClient(sessionClient)
	if _, err := service.SetDisplayName(ctx, &s4wave_session.SetLocalDisplayNameRequest{DisplayName: name}); err != nil {
		return errors.Wrap(err, "set display name")
	}
	os.Stdout.WriteString("named session " + strconv.FormatUint(uint64(idx), 10) + " " + strconv.Quote(name) + "\n")
	return nil
}
