//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newPluginCommand builds the plugin command group.
func newPluginCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:    "plugin",
		Aliases: []string{"plugins"},
		Usage:   "manage space plugins",
		Subcommands: []*cli.Command{
			buildPluginListCommand(),
			buildPluginApproveCommand(),
			buildPluginDenyCommand(),
			buildPluginAddCommand(),
			buildPluginRemoveCommand(),
		},
	}
}

// buildPluginListCommand builds the plugin list subcommand.
func buildPluginListCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:  "list",
		Usage: "list plugins and their approval state",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
			&cli.BoolFlag{
				Name:    "watch",
				Usage:   "watch for changes (append mode)",
				EnvVars: []string{"SPACEWAVE_WATCH"},
			},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			spaceID := c.String("space")
			watch := c.Bool("watch")
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, 1)
			if err != nil {
				return err
			}
			defer sess.Release()

			spaceID, err = client.resolveSpaceID(ctx, sess, spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			contentsSvc, contentsCleanup, err := client.mountSpaceContents(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer contentsCleanup()

			strm, err := contentsSvc.WatchState(ctx, &s4wave_space.WatchSpaceContentsStateRequest{})
			if err != nil {
				return errors.Wrap(err, "watch state")
			}
			defer strm.Close()

			w := os.Stdout
			for {
				state, err := strm.Recv()
				if err != nil {
					return errors.Wrap(err, "recv state")
				}

				plugins := state.GetPlugins()
				if len(plugins) == 0 {
					w.WriteString("no plugins\n")
				} else {
					for _, p := range plugins {
						w.WriteString(p.GetPluginId())
						w.WriteString("  ")
						w.WriteString(p.GetApprovalState().String())
						if p.GetLoaded() {
							w.WriteString("  loaded")
						}
						desc := p.GetDescription()
						if desc != "" {
							w.WriteString("  " + desc)
						}
						w.WriteString("\n")
					}
				}

				if !watch {
					return nil
				}
				w.WriteString("--- " + time.Now().Format(time.RFC3339) + " ---\n")
			}
		},
	}
}

// buildPluginApproveCommand builds the plugin approve subcommand.
func buildPluginApproveCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:      "approve",
		Usage:     "approve a plugin (fire-and-forget)",
		ArgsUsage: "<name-or-id>",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
		},
		Action: func(c *cli.Context) error {
			nameOrID := c.Args().First()
			if nameOrID == "" {
				return errors.New("plugin name or manifest ID required")
			}
			return setPluginApproval(c, statePath, nameOrID, true)
		},
	}
}

// buildPluginDenyCommand builds the plugin deny subcommand.
func buildPluginDenyCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:      "deny",
		Usage:     "deny a plugin (fire-and-forget)",
		ArgsUsage: "<name-or-id>",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
		},
		Action: func(c *cli.Context) error {
			nameOrID := c.Args().First()
			if nameOrID == "" {
				return errors.New("plugin name or manifest ID required")
			}
			return setPluginApproval(c, statePath, nameOrID, false)
		},
	}
}

// setPluginApproval resolves a plugin name or ID and sets its approval state.
func setPluginApproval(c *cli.Context, statePath, nameOrID string, approved bool) error {
	ctx := c.Context
	spaceID := c.String("space")
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, 1)
	if err != nil {
		return err
	}
	defer sess.Release()

	spaceID, err = client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer spaceCleanup()

	// Resolve name-or-id to a plugin ID by checking contents state.
	pluginID, err := resolvePluginID(ctx, client, spaceSvc, nameOrID)
	if err != nil {
		return err
	}

	contentsSvc, contentsCleanup, err := client.mountSpaceContents(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer contentsCleanup()

	_, err = contentsSvc.SetPluginApproval(ctx, &s4wave_space.SetPluginApprovalRequest{
		PluginId: pluginID,
		Approved: approved,
	})
	if err != nil {
		return errors.Wrap(err, "set plugin approval")
	}

	w := os.Stdout
	if approved {
		w.WriteString("approved: " + pluginID + "\n")
	} else {
		w.WriteString("denied: " + pluginID + "\n")
	}
	return nil
}

// resolvePluginID resolves a plugin name or manifest ID to the canonical plugin ID.
// It mounts SpaceContents, fetches the current state, and matches by ID or description.
func resolvePluginID(
	ctx context.Context,
	client *sdkClient,
	spaceSvc s4wave_space.SRPCSpaceResourceServiceClient,
	nameOrID string,
) (string, error) {
	contentsSvc, contentsCleanup, err := client.mountSpaceContents(ctx, spaceSvc)
	if err != nil {
		return "", err
	}
	defer contentsCleanup()

	strm, err := contentsSvc.WatchState(ctx, &s4wave_space.WatchSpaceContentsStateRequest{})
	if err != nil {
		return "", errors.Wrap(err, "watch state")
	}
	defer strm.Close()

	state, err := strm.Recv()
	if err != nil {
		return "", errors.Wrap(err, "recv state")
	}

	// Try exact match on plugin ID first.
	for _, p := range state.GetPlugins() {
		if p.GetPluginId() == nameOrID {
			return nameOrID, nil
		}
	}

	// Try case-insensitive match on description or partial ID match.
	lower := strings.ToLower(nameOrID)
	for _, p := range state.GetPlugins() {
		if strings.ToLower(p.GetDescription()) == lower {
			return p.GetPluginId(), nil
		}
		if strings.Contains(strings.ToLower(p.GetPluginId()), lower) {
			return p.GetPluginId(), nil
		}
	}

	// If no match found, use it as-is (could be a not-yet-loaded plugin).
	return nameOrID, nil
}

// buildPluginAddCommand builds the plugin add subcommand.
func buildPluginAddCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:      "add",
		Usage:     "add a plugin to space settings",
		ArgsUsage: "<manifest-id>",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
		},
		Action: func(c *cli.Context) error {
			manifestID := c.Args().First()
			if manifestID == "" {
				return errors.New("manifest ID required")
			}

			ctx := c.Context
			spaceID := c.String("space")
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, 1)
			if err != nil {
				return err
			}
			defer sess.Release()

			spaceID, err = client.resolveSpaceID(ctx, sess, spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			_, err = spaceSvc.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{
				PluginId: manifestID,
			})
			if err != nil {
				return errors.Wrap(err, "add space plugin")
			}

			os.Stdout.WriteString("added: " + manifestID + "\n")
			return nil
		},
	}
}

// buildPluginRemoveCommand builds the plugin remove subcommand.
func buildPluginRemoveCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a plugin from space settings",
		ArgsUsage: "<manifest-id>",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
		},
		Action: func(c *cli.Context) error {
			manifestID := c.Args().First()
			if manifestID == "" {
				return errors.New("manifest ID required")
			}

			ctx := c.Context
			spaceID := c.String("space")
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, 1)
			if err != nil {
				return err
			}
			defer sess.Release()

			spaceID, err = client.resolveSpaceID(ctx, sess, spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			_, err = spaceSvc.RemoveSpacePlugin(ctx, &s4wave_space.RemoveSpacePluginRequest{
				PluginId: manifestID,
			})
			if err != nil {
				return errors.Wrap(err, "remove space plugin")
			}

			os.Stdout.WriteString("removed: " + manifestID + "\n")
			return nil
		},
	}
}
