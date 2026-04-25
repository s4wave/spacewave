//go:build !js

package spacewave_cli

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// newWebCommand builds the web command that exposes the native runtime on localhost.
func newWebCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var host string
	var listenMultiaddr string
	var port uint
	var background bool
	return &cli.Command{
		Name:  "web",
		Usage: "start a localhost web listener for the native runtime",
		Subcommands: []*cli.Command{
			newWebListCommand(),
			newWebStopCommand(),
		},
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:        "host",
				Usage:       "localhost hostname or loopback address to bind",
				Value:       "127.0.0.1",
				Destination: &host,
			},
			&cli.UintFlag{
				Name:        "port",
				Usage:       "tcp port to bind; 0 chooses a random free port",
				Value:       0,
				Destination: &port,
			},
			&cli.StringFlag{
				Name:        "listen",
				Usage:       "listen multiaddr, overriding --host and --port",
				Destination: &listenMultiaddr,
			},
			&cli.BoolFlag{
				Name:        "background",
				Aliases:     []string{"bg"},
				Usage:       "keep the listener in the daemon after this command exits",
				Destination: &background,
			},
		},
		Action: func(c *cli.Context) error {
			return runWeb(c, statePath, host, port, listenMultiaddr, background)
		},
	}
}

func newWebListCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:  "list",
		Usage: "list background localhost web listeners",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
		},
		Action: func(c *cli.Context) error {
			return runWebList(c, statePath)
		},
	}
}

func newWebStopCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:      "stop",
		Usage:     "stop a background localhost web listener",
		Args:      true,
		ArgsUsage: "<listener-id>",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
		},
		Action: func(c *cli.Context) error {
			listenerID := c.Args().First()
			if listenerID == "" {
				return errors.New("listener id is required")
			}
			return runWebStop(c, statePath, listenerID)
		},
	}
}

func runWeb(
	c *cli.Context,
	statePath string,
	host string,
	port uint,
	listenMultiaddr string,
	background bool,
) error {
	ctx := c.Context
	if port > 65535 {
		return errors.New("port must be <= 65535")
	}
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	reqMultiaddr := listenMultiaddr
	if reqMultiaddr == "" {
		reqMultiaddr = buildWebListenMultiaddr(host, uint32(port))
	}
	resp, err := client.root.AccessWebListener(ctx, reqMultiaddr, background)
	if err != nil {
		return errors.Wrap(err, "access web listener")
	}

	var release func()
	if resp.GetResourceId() != 0 {
		ref := client.resClient.CreateResourceReference(resp.GetResourceId())
		release = ref.Release
		defer release()
	}

	url := resp.GetUrl() + "/#otp=" + resp.GetBootstrapSecret()
	if background {
		if resp.GetReused() {
			os.Stdout.WriteString("Reusing background Spacewave web session:\n  " + url + "\n")
		} else {
			os.Stdout.WriteString("Spacewave is running in the background:\n  " + url + "\n")
		}
		os.Stdout.WriteString("Use `spacewave web list` to see listeners or `spacewave web stop <listener-id>` to stop one.\n")
		return nil
	}
	os.Stdout.WriteString("Spacewave is ready in your browser:\n  " + url + "\n")
	os.Stdout.WriteString("Press Ctrl-C to stop this listener.\n")
	select {
	case <-ctx.Done():
		return nil
	}
}

func runWebList(c *cli.Context, statePath string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	listeners, err := client.root.ListWebListeners(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "unimplemented") {
			return errors.New("the running Spacewave daemon is from an older build; run `spacewave stop` with the same --state-path, then rerun this command")
		}
		return errors.Wrap(err, "list web listeners")
	}
	if len(listeners) == 0 {
		os.Stdout.WriteString("No background Spacewave web listeners.\n")
		return nil
	}
	for _, listener := range listeners {
		os.Stdout.WriteString(listener.GetListenerId() + "\t" + listener.GetUrl() + "\t" + listener.GetListenMultiaddr() + "\n")
	}
	return nil
}

func runWebStop(c *cli.Context, statePath string, listenerID string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	stopped, err := client.root.StopWebListener(ctx, listenerID)
	if err != nil {
		return errors.Wrap(err, "stop web listener")
	}
	if !stopped {
		return errors.Errorf("web listener not found: %s", listenerID)
	}
	os.Stdout.WriteString("Stopped Spacewave web listener: " + listenerID + "\n")
	return nil
}

func buildWebListenMultiaddr(host string, port uint32) string {
	portStr := strconv.FormatUint(uint64(port), 10)
	normalized := strings.Trim(host, "[]")
	ip := net.ParseIP(normalized)
	if ip == nil {
		return "/dns4/" + normalized + "/tcp/" + portStr
	}
	if ip.To4() != nil {
		return "/ip4/" + ip.String() + "/tcp/" + portStr
	}
	return "/ip6/" + ip.String() + "/tcp/" + portStr
}
