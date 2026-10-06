//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// webArgs are the arguments of the web command.
type webArgs struct {
	statePath        string
	sessionIdx       uint
	space            string
	host             string
	port             uint
	listenMultiaddr  string
	background       bool
	printURL         bool
	displayPath      string
	displayComponent string
}

// newWebCommand builds the web command that exposes the native runtime on localhost.
func newWebCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	args := &webArgs{}
	return &cli.Command{
		Name:  "web",
		Usage: "start a localhost web listener for the native runtime",
		Subcommands: []*cli.Command{
			newWebListCommand(),
			newWebStopCommand(),
		},
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the flags of the web command.
func (a *webArgs) BuildFlags() []cli.Flag {
	return append(
		clientFlags(&a.statePath, &a.sessionIdx),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "bind the listener to this Space ID or name and serve it read-only",
			Destination: &a.space,
		},
		&cli.StringFlag{
			Name:        "host",
			Usage:       "localhost hostname or loopback address to bind",
			Value:       "127.0.0.1",
			Destination: &a.host,
		},
		&cli.UintFlag{
			Name:        "port",
			Usage:       "tcp port to bind; 0 chooses a random free port",
			Value:       0,
			Destination: &a.port,
		},
		&cli.StringFlag{
			Name:        "listen",
			Usage:       "listen multiaddr, overriding --host and --port",
			Destination: &a.listenMultiaddr,
		},
		&cli.BoolFlag{
			Name:        "background",
			Aliases:     []string{"bg"},
			Usage:       "keep the listener in the daemon after this command exits",
			Destination: &a.background,
		},
		&cli.BoolFlag{
			Name:        "print-url",
			Usage:       "print only the resolved browser URL to stdout",
			Destination: &a.printURL,
		},
		&cli.StringFlag{
			Name:        "display",
			Usage:       "open web in kiosk display mode for the given object path",
			Destination: &a.displayPath,
		},
		&cli.StringFlag{
			Name:        "display-component",
			Usage:       "preferred object viewer component id for kiosk display mode",
			Destination: &a.displayComponent,
		},
	)
}

// Run starts the web listener and prints its browser URL.
func (a *webArgs) Run(c *cli.Context) error {
	// Reject an invalid port and connect to the daemon.
	ctx := c.Context
	if a.port > 65535 {
		return errors.New("port must be <= 65535")
	}
	if a.displayPath == "" && a.displayComponent != "" {
		return errors.New("--display-component requires --display")
	}
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// Build the listen address and the optional Space binding.
	req := &s4wave_root.AccessWebListenerRequest{
		ListenMultiaddr: a.listenMultiaddr,
		Background:      a.background,
	}
	if req.ListenMultiaddr == "" {
		req.ListenMultiaddr = buildWebListenMultiaddr(a.host, uint32(a.port))
	}
	if a.space != "" {
		req.SessionIdx = sessionIndex32(a.sessionIdx)
		req.SpaceId, err = resolveWebSpaceID(ctx, client, req.SessionIdx, a.space)
		if err != nil {
			return err
		}
	}

	// Access the web listener and release its resource when the command returns.
	resp, err := client.root.AccessWebListener(ctx, req)
	if err != nil {
		return errors.Wrap(err, "access web listener")
	}
	if resp.GetResourceId() != 0 {
		ref := client.resClient.CreateResourceReference(resp.GetResourceId())
		defer ref.Release()
	}

	// Build the browser URL from the display path and component.
	queryParts := make([]string, 0, 2)
	if a.displayPath != "" {
		queryParts = append(queryParts, "path="+url.QueryEscape(a.displayPath))
	}
	if a.displayComponent != "" {
		queryParts = append(queryParts, "component="+url.QueryEscape(a.displayComponent))
	}
	webPath := "/"
	if len(queryParts) != 0 {
		webPath = "/display?" + strings.Join(queryParts, "&")
	}
	browserURL := resp.GetUrl() + webPath + "#otp=" + resp.GetBootstrapSecret()

	// Print the browser URL and wait unless the listener is backgrounded.
	if a.printURL {
		os.Stdout.WriteString(browserURL + "\n")
		if a.background {
			return nil
		}
		<-ctx.Done()
		return nil
	}
	if a.background {
		if resp.GetReused() {
			os.Stdout.WriteString("Reusing background Spacewave web session:\n  " + browserURL + "\n")
		} else {
			os.Stdout.WriteString("Spacewave is running in the background:\n  " + browserURL + "\n")
		}
		os.Stdout.WriteString("Use `spacewave web list` to see listeners or `spacewave web stop <listener-id>` to stop one.\n")
		return nil
	}
	os.Stdout.WriteString("Spacewave is ready in your browser:\n  " + browserURL + "\n")
	os.Stdout.WriteString("Press Ctrl-C to stop this listener.\n")
	<-ctx.Done()
	return nil
}

// resolveWebSpaceID resolves a Space ID or name in session sessionIdx.
func resolveWebSpaceID(ctx context.Context, client *sdkClient, sessionIdx uint32, space string) (string, error) {
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	return client.resolveSpaceID(ctx, sess, space)
}

func newWebListCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:  "list",
		Usage: "list background localhost web listeners",
		Flags: daemonClientFlags(&statePath),
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
		Flags:     daemonClientFlags(&statePath),
		Action: func(c *cli.Context) error {
			listenerID := c.Args().First()
			if listenerID == "" {
				return errors.New("listener id is required")
			}
			return runWebStop(c, statePath, listenerID)
		},
	}
}

func runWebList(c *cli.Context, statePath string) error {
	// Connect to the daemon.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// List web listeners and print each one.
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
	// Connect to the daemon.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// Stop the web listener and report it.
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
	// Build an IP listen multiaddr from the host and port.
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
