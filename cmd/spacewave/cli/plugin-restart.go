//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// pluginRestartArgs are the arguments of the plugin restart command.
type pluginRestartArgs struct {
	statePath  string
	sessionIdx uint
	spaceID    string
}

// newPluginRestartCommand builds the command that restarts a Space plugin.
func newPluginRestartCommand() *cli.Command {
	args := &pluginRestartArgs{}
	return &cli.Command{
		Name:      "restart",
		Usage:     "end a plugin's running worker and start its selected manifest",
		ArgsUsage: "<manifest-id>",
		Description: "Ends the running worker of a plugin in the Space runtime, so the scheduler starts the plugin's " +
			"selected manifest at once. Use it when a swap waits on a worker that neither answers nor exits.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the flags that select the session and Space.
func (a *pluginRestartArgs) BuildFlags() []cli.Flag {
	return append(clientFlags(&a.statePath, &a.sessionIdx), &cli.StringFlag{
		Name:        "space",
		Usage:       "space ID (auto-detected if only one space)",
		EnvVars:     []string{"SPACEWAVE_SPACE"},
		Destination: &a.spaceID,
	})
}

// Run restarts the plugin through the mounted Space contents.
func (a *pluginRestartArgs) Run(c *cli.Context) error {
	// Require a manifest ID.
	manifestID := c.Args().First()
	if c.NArg() != 1 || manifestID == "" {
		return errors.New("usage: plugin restart [--space ID] <manifest-id>")
	}

	// Connect to the daemon and mount the selected session.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIndex32(a.sessionIdx))
	if err != nil {
		return err
	}
	defer sess.Release()

	// Mount the Space and its plugin runtime.
	spaceID, err := client.resolveSpaceID(ctx, sess, a.spaceID)
	if err != nil {
		return err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer spaceCleanup()
	contents, contentsCleanup, err := client.mountSpaceContents(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer contentsCleanup()

	// Restart the plugin's execution.
	if _, err := contents.RestartPlugin(ctx, &s4wave_space.RestartSpacePluginRequest{PluginId: manifestID}); err != nil {
		return errors.Wrap(err, "restart plugin")
	}
	os.Stdout.WriteString("restarted: " + manifestID + "\n")
	return nil
}
