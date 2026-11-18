//go:build !js

package spacewave_cli

import (
	"errors"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/structpb"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/wrapperspb"
)

// debugColdPacksArgs locates the running daemon for cold pack administration.
type debugColdPacksArgs struct {
	// statePath locates the daemon's default socket unless --socket-path is set.
	statePath string
}

// newDebugColdPacksCommand builds the local cold pack control command.
func newDebugColdPacksCommand() *cli.Command {
	args := &debugColdPacksArgs{}
	return &cli.Command{
		Name:      "cold-packs",
		Usage:     "toggle configured cold pack reads or print cold pack counters",
		ArgsUsage: "on|off|stats",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// BuildFlags selects the target Unix socket without autostart or remote relay.
func (a *debugColdPacksArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{statePathFlag(&a.statePath), socketPathFlag()}
}

// Run issues one cold pack control request over the already-running socket.
func (a *debugColdPacksArgs) Run(c *cli.Context) error {
	// Require a single explicit action; a typo must not change the daemon.
	action := c.Args().First()
	if c.NArg() != 1 || (action != "on" && action != "off" && action != "stats") {
		return errors.New("expected one of: on, off, stats")
	}

	// Connect to the same protected socket used by the other debug commands.
	client, err := connectDebugTraceDaemon(c.Context, c, a.statePath, effectiveSocketPath(c, ""))
	if err != nil {
		return err
	}
	defer client.close()

	// Apply an explicit runtime toggle, without changing startup configuration.
	if action != "stats" {
		return client.srpc.ExecCall(c.Context, coldPackControlServiceID, coldPackSetReadsMethodID, wrapperspb.Bool(action == "on"), &emptypb.Empty{})
	}

	// Print the admin snapshot as JSON for scripts.
	var response structpb.Struct
	if err := client.srpc.ExecCall(c.Context, coldPackControlServiceID, coldPackStatsMethodID, &emptypb.Empty{}, &response); err != nil {
		return err
	}
	data, err := response.MarshalJSON()
	if err != nil {
		return err
	}
	_, err = c.App.Writer.Write(append(data, '\n'))
	return err
}
