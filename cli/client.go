package cli

import (
	"context"
	"errors"
	"net"

	bifrost_cli "github.com/aperturerobotics/bifrost/cli"
	cbus_cli "github.com/aperturerobotics/controllerbus/cli"
	api "github.com/aperturerobotics/forge/daemon/api"
	hydra_cli "github.com/aperturerobotics/hydra/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	ucli "github.com/urfave/cli/v2"
)

// ClientArgs contains the client arguments and functions.
type ClientArgs struct {
	// CbusConf is the controller-bus configuration.
	CbusConf cbus_cli.ClientArgs
	// BifrostConf is the bifrost configuration.
	BifrostConf bifrost_cli.ClientArgs
	// HydraConf is the hydra configuration.
	HydraConf hydra_cli.ClientArgs

	// le is the logger entry
	le *logrus.Entry
	// ctx is the context
	ctx context.Context
	// client is the client instance
	client api.ForgeDaemonClient

	// DialAddr is the address to dial.
	DialAddr string
}

// BuildFlags attaches the flags to a flag set.
func (a *ClientArgs) BuildFlags() []ucli.Flag {
	return []ucli.Flag{
		&ucli.StringFlag{
			Name:        "dial-addr",
			Usage:       "address to dial API on",
			Destination: &a.DialAddr,
			Value:       "127.0.0.1:5110",
		},
	}
}

// SetClient sets the client instance.
func (a *ClientArgs) SetClient(client api.ForgeDaemonClient) {
	a.client = client
}

// BuildClient builds the client or returns it if it has been set.
func (a *ClientArgs) BuildClient() (api.ForgeDaemonClient, error) {
	if a.client != nil {
		return a.client, nil
	}

	if a.DialAddr == "" {
		return nil, errors.New("dial address is not set")
	}

	nconn, err := net.Dial("tcp", a.DialAddr)
	if err != nil {
		return nil, err
	}
	muxedConn, err := srpc.NewMuxedConn(nconn, false)
	if err != nil {
		return nil, err
	}
	client := srpc.NewClientWithMuxedConn(muxedConn)
	a.client = api.NewForgeDaemonClient(client)
	return a.client, nil
}

// BuildForgeCommand returns the forge sub-command set.
func (a *ClientArgs) BuildForgeCommand() *cli.Command {
	forgeCmds := a.BuildCommands()
	return &cli.Command{
		Name:        "forge",
		Usage:       "Forge distributed build sub-commands.",
		Subcommands: forgeCmds,
	}
}

// BuildCommands attaches the commands.
func (a *ClientArgs) BuildCommands() []*ucli.Command {
	// TODO
	return nil
}

// SetContext sets the context.
func (a *ClientArgs) SetContext(c context.Context) {
	a.ctx = c
}

// GetContext returns the context.
func (a *ClientArgs) GetContext() context.Context {
	if c := a.ctx; c != nil {
		return c
	}
	return context.TODO()
}

// SetLogger sets the root log entry.
func (a *ClientArgs) SetLogger(le *logrus.Entry) {
	a.le = le
}

// GetLogger returns the log entry
func (a *ClientArgs) GetLogger() *logrus.Entry {
	if le := a.le; le != nil {
		return le
	}
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(log)
}
