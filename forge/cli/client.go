//go:build !js && !wasip1

package cli

import (
	"errors"
	"net"

	"github.com/aperturerobotics/cli"
	cbus_cli "github.com/aperturerobotics/controllerbus/cli"
	"github.com/aperturerobotics/starpc/srpc"
	hydra_cli "github.com/s4wave/spacewave/db/cli"
	hydra_api "github.com/s4wave/spacewave/db/daemon/api"
	api "github.com/s4wave/spacewave/forge/daemon/api"
	bifrost_cli "github.com/s4wave/spacewave/net/cli"
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
)

// ClientArgs binds the Forge client commands to a shared daemon connection.
type ClientArgs struct {
	// cbusArgs binds the ControllerBus commands.
	cbusArgs cbus_cli.ClientArgs
	// bifrostArgs binds the Bifrost commands.
	bifrostArgs bifrost_cli.ClientArgs
	// hydraArgs binds the Hydra commands.
	hydraArgs hydra_cli.ClientArgs
	// client is the shared RPC client, opened on first use.
	client api.ForgeDaemonClient

	// DialAddr is the address to dial.
	DialAddr string
}

// BuildCommand binds the client command tree and its shared dial address.
func (a *ClientArgs) BuildCommand() *cli.Command {
	// Supply the Forge connection when the ControllerBus command runs.
	cbusCmd := a.cbusArgs.BuildControllerBusCommand()
	cbusCmd.Before = func(_ *cli.Context) error {
		client, err := a.buildClient()
		if err != nil {
			return err
		}
		a.cbusArgs.SetClient(client)
		return nil
	}

	// Bind each child to the Forge service it uses.
	return &cli.Command{
		Name:  "client",
		Usage: "client sub-commands",
		Subcommands: []*cli.Command{
			cbusCmd,
			a.bifrostArgs.BuildBifrostCommandWithClient(func() (bifrost_api.BifrostAPIClient, error) {
				return a.buildClient()
			}),
			a.hydraArgs.BuildHydraCommandWithClient(func() (hydra_api.HydraDaemonClient, error) {
				return a.buildClient()
			}),
		},
		Flags: a.BuildFlags(),
	}
}

// BuildFlags attaches the flags to a flag set.
func (a *ClientArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "dial-addr",
			Usage:       "address to dial API on",
			Destination: &a.DialAddr,
			Value:       "127.0.0.1:5110",
		},
	}
}

// buildClient dials the Forge daemon once and reuses the resulting client.
func (a *ClientArgs) buildClient() (api.ForgeDaemonClient, error) {
	// Reuse the open Forge client across subcommands.
	if a.client != nil {
		return a.client, nil
	}

	// Require an address before opening a transport.
	if a.DialAddr == "" {
		return nil, errors.New("dial address is not set")
	}

	// Connect to Forge and build its multiplexed RPC client.
	nconn, err := net.Dial("tcp", a.DialAddr)
	if err != nil {
		return nil, err
	}
	muxedConn, err := srpc.NewMuxedConn(nconn, false, nil)
	if err != nil {
		return nil, err
	}
	client := srpc.NewClientWithMuxedConn(muxedConn)
	a.client = api.NewForgeDaemonClient(client)
	return a.client, nil
}
