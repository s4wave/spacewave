//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_git_core "github.com/s4wave/spacewave/core/git"
	space_world "github.com/s4wave/spacewave/core/space/world"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_deploy "github.com/s4wave/spacewave/sdk/deploy"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// newSpaceCommand builds the space command group.
func newSpaceCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	flags := clientFlags(&statePath, &sessionIdx)
	return &cli.Command{
		Name:  "space",
		Usage: "manage spaces",
		Flags: flags,
		Subcommands: []*cli.Command{
			newSpaceListCommand(&statePath, &sessionIdx),
			newSpaceCreateCommand(&statePath, &sessionIdx),
			newSpaceDeleteCommand(&statePath, &sessionIdx),
			newSpaceInfoCommand(&statePath, &sessionIdx),
			newSpaceResolveCommand(&statePath, &sessionIdx),
			newSpaceSettingsCommand(&statePath, &sessionIdx),
			newSpaceImportGitCommand(&statePath, &sessionIdx),
			newSpaceDeployCommand(&statePath, &sessionIdx),
			newObjectCommand(&statePath, &sessionIdx),
		},
	}
}

// newSpaceListCommand builds the space list subcommand.
func newSpaceListCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var watch bool
	return &cli.Command{
		Name:  "list",
		Usage: "list spaces in the current session",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "watch",
				Aliases:     []string{"w"},
				Usage:       "watch for changes (append mode)",
				EnvVars:     []string{"SPACEWAVE_WATCH"},
				Destination: &watch,
			},
		},
		Action: func(c *cli.Context) error {
			return runSpaceList(c, *statePath, c.String("output"), uint32(*sessionIdx), watch)
		},
	}
}

// runSpaceList executes the space list command.
func runSpaceList(c *cli.Context, statePath, outputFormat string, sessionIdx uint32, watch bool) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv resources list")
	}

	switch outputFormat {
	case "json", "yaml":
		if data, jerr := resp.MarshalJSON(); jerr != nil {
			return jerr
		} else if err := formatOutput(data, outputFormat); err != nil {
			return err
		}
	default:
		printSpacesList(resp)
	}

	if !watch {
		return nil
	}

	for {
		resp, err = strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv resources list")
		}
		w := os.Stdout
		w.WriteString("\n--- " + time.Now().Format(time.RFC3339) + " ---\n")
		switch outputFormat {
		case "json", "yaml":
			if data, jerr := resp.MarshalJSON(); jerr != nil {
				return jerr
			} else if err := formatOutput(data, outputFormat); err != nil {
				return err
			}
		default:
			printSpacesList(resp)
		}
	}
}

// newSpaceCreateCommand builds the space create subcommand.
func newSpaceCreateCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "create a new space",
		ArgsUsage: "<name>",
		Action: func(c *cli.Context) error {
			name := c.Args().First()
			if name == "" {
				return errors.New("space name required")
			}

			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			resp, err := sess.CreateSpace(ctx, name, "", "")
			if err != nil {
				return errors.Wrap(err, "create space")
			}

			id := resp.GetSharedObjectRef().GetProviderResourceRef().GetId()
			switch c.String("output") {
			case "json", "yaml":
				data, err := resp.MarshalJSON()
				if err != nil {
					return err
				}
				return formatOutput(data, c.String("output"))
			default:
				os.Stdout.WriteString(id + "\n")
				return nil
			}
		},
	}
}

// newSpaceDeleteCommand builds the space delete subcommand.
func newSpaceDeleteCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "delete a space by ID",
		ArgsUsage: "<space-id>",
		Action: func(c *cli.Context) error {
			spaceID := c.Args().First()
			if spaceID == "" {
				return errors.New("space ID required")
			}

			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			if _, err := sess.DeleteSpace(ctx, spaceID); err != nil {
				return errors.Wrap(err, "delete space")
			}

			os.Stdout.WriteString("space deleted\n")
			return nil
		},
	}
}

// newSpaceInfoCommand builds the space info subcommand.
func newSpaceInfoCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "show space details",
		ArgsUsage: "<space-id>",
		Action: func(c *cli.Context) error {
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			strm, err := spaceSvc.WatchSpaceState(ctx, &s4wave_space.WatchSpaceStateRequest{})
			if err != nil {
				return errors.Wrap(err, "watch space state")
			}
			defer strm.Close()

			state, err := strm.Recv()
			if err != nil {
				return errors.Wrap(err, "recv space state")
			}

			switch c.String("output") {
			case "json", "yaml":
				data, err := state.MarshalJSON()
				if err != nil {
					return err
				}
				return formatOutput(data, c.String("output"))
			default:
				printSpaceState(spaceID, state)
				return nil
			}
		},
	}
}

// newSpaceResolveCommand builds the space resolve subcommand.
func newSpaceResolveCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "resolve",
		Usage:     "resolve a space name to its ID",
		ArgsUsage: "<name>",
		Action: func(c *cli.Context) error {
			name := c.Args().First()
			if name == "" {
				return errors.New("space name required")
			}

			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			id, err := client.getSpaceByName(ctx, sess, name)
			if err != nil {
				return err
			}

			os.Stdout.WriteString(id + "\n")
			return nil
		},
	}
}

// newSpaceSettingsCommand builds the space settings subcommand.
func newSpaceSettingsCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "settings",
		Usage:     "show space settings",
		ArgsUsage: "[space-id]",
		Action: func(c *cli.Context) error {
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			strm, err := spaceSvc.WatchSpaceState(ctx, &s4wave_space.WatchSpaceStateRequest{})
			if err != nil {
				return errors.Wrap(err, "watch space state")
			}
			defer strm.Close()

			state, err := strm.Recv()
			if err != nil {
				return errors.Wrap(err, "recv space state")
			}

			settings := state.GetSettings()
			switch c.String("output") {
			case "json", "yaml":
				data, err := settings.MarshalJSON()
				if err != nil {
					return err
				}
				return formatOutput(data, c.String("output"))
			default:
				printSpaceSettings(settings)
				return nil
			}
		},
	}
}

// newSpaceImportGitCommand builds the space import-git subcommand.
func newSpaceImportGitCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var spaceID, objectKey, ref string
	var singleBranch, recursive, disableCheckout bool
	return &cli.Command{
		Name:      "import-git",
		Usage:     "import a git repository into a space's world",
		ArgsUsage: "<git-url>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "space",
				Usage:       "space ID (auto-detected if only one space)",
				EnvVars:     []string{"SPACEWAVE_SPACE"},
				Destination: &spaceID,
			},
			&cli.StringFlag{
				Name:        "object-key",
				Usage:       "world object key for the repo",
				EnvVars:     []string{"SPACEWAVE_OBJECT_KEY"},
				Required:    true,
				Destination: &objectKey,
			},
			&cli.BoolFlag{
				Name:        "disable-checkout",
				Usage:       "skip creating a worktree after clone",
				EnvVars:     []string{"SPACEWAVE_DISABLE_CHECKOUT"},
				Value:       true,
				Destination: &disableCheckout,
			},
			&cli.StringFlag{
				Name:        "ref",
				Usage:       "git reference to clone (branch/tag)",
				EnvVars:     []string{"SPACEWAVE_GIT_REF"},
				Destination: &ref,
			},
			&cli.BoolFlag{
				Name:        "single-branch",
				Usage:       "only fetch the specified ref",
				EnvVars:     []string{"SPACEWAVE_SINGLE_BRANCH"},
				Destination: &singleBranch,
			},
			&cli.BoolFlag{
				Name:        "recursive",
				Aliases:     []string{"r"},
				Usage:       "recursively clone submodules",
				EnvVars:     []string{"SPACEWAVE_RECURSIVE"},
				Destination: &recursive,
			},
		},
		Action: func(c *cli.Context) error {
			url := c.Args().First()
			if url == "" {
				return errors.New("git URL required as first argument")
			}

			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err := client.resolveSpaceID(ctx, sess, spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			w := os.Stdout
			readTx, err := engine.NewTransaction(ctx, false)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			_, exists, err := readTx.GetObject(ctx, objectKey)
			if err != nil {
				readTx.Discard()
				return errors.Wrap(err, "check object")
			}
			readTx.Discard()

			if exists {
				tx, err := engine.NewTransaction(ctx, true)
				if err != nil {
					return errors.Wrap(err, "new transaction")
				}
				defer tx.Discard()
				w.WriteString("object " + objectKey + " exists, fetching...\n")
				fetchOp := git_world.NewGitFetchOp(objectKey, &git_block.FetchOpts{
					RemoteUrl: url,
				})
				_, _, err = tx.ApplyWorldOp(ctx, fetchOp, "")
				if err != nil {
					return errors.Wrap(err, "fetch")
				}
				w.WriteString("fetched " + url + " into " + objectKey + "\n")
				return tx.Commit(ctx)
			}

			w.WriteString("cloning " + url + " as " + objectKey + "...\n")
			repoRef, err := s4wave_git_core.CloneGitRepoToRef(ctx, engine, &git_block.CloneOpts{
				Url:             url,
				Ref:             ref,
				SingleBranch:    singleBranch,
				Recursive:       recursive,
				DisableCheckout: disableCheckout && !recursive,
			}, nil, nil)
			if err != nil {
				return err
			}
			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()
			cloneOp := git_world.NewGitInitOp(
				objectKey,
				repoRef,
				disableCheckout && !recursive,
				nil,
				unixfs_block.ToTimestamp(time.Now(), false),
			)
			_, _, err = tx.ApplyWorldOp(ctx, cloneOp, "")
			if err != nil {
				return errors.Wrap(err, "publish git repo")
			}
			w.WriteString("cloned " + url + " as " + objectKey + "\n")
			return tx.Commit(ctx)
		},
	}
}

// newSpaceDeployCommand builds the space deploy subcommand.
func newSpaceDeployCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var spaceID, dbPath, manifestID, objectKey string
	return &cli.Command{
		Name:  "deploy",
		Usage: "deploy a manifest from a .bldr devtool DB into a space",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "space",
				Usage:       "space ID (auto-detected if only one space)",
				EnvVars:     []string{"SPACEWAVE_SPACE"},
				Destination: &spaceID,
			},
			&cli.StringFlag{
				Name:        "db",
				Usage:       "path to .bldr/ directory containing the devtool DB",
				EnvVars:     []string{"SPACEWAVE_DB"},
				Required:    true,
				Destination: &dbPath,
			},
			&cli.StringFlag{
				Name:        "manifest-id",
				Usage:       "manifest identifier (e.g., glados-core)",
				EnvVars:     []string{"SPACEWAVE_MANIFEST_ID"},
				Required:    true,
				Destination: &manifestID,
			},
			&cli.StringFlag{
				Name:        "object-key",
				Usage:       "object key to store the manifest under (default: manifest-id)",
				EnvVars:     []string{"SPACEWAVE_OBJECT_KEY"},
				Destination: &objectKey,
			},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context

			key := objectKey
			if key == "" {
				key = manifestID
			}

			le := logrus.NewEntry(logrus.New())

			// Open the devtool sqlite volume from the .bldr/ directory.
			vol, err := openDevtoolVolume(ctx, le, dbPath)
			if err != nil {
				return errors.Wrap(err, "open devtool storage")
			}
			defer vol.Close()

			// Look up the manifest by ID in the devtool world.
			collected, err := lookupDevtoolManifest(ctx, le, vol, manifestID)
			if err != nil {
				return errors.Wrap(err, "lookup manifest")
			}

			// Build the full ObjectRef with transform config so the server can decode blocks.
			transformConf, err := block_transform.NewConfig([]config.Config{
				&transform_s2.Config{},
			})
			if err != nil {
				return errors.Wrap(err, "build transform config")
			}
			manifestRef := collected.ManifestRef.Clone()
			manifestRef.TransformConf = transformConf
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err := client.resolveSpaceID(ctx, sess, spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			w := os.Stdout
			w.WriteString("deploying manifest " + manifestID + " to space " + sid + " (key=" + key + ")\n")
			w.WriteString("source: " + dbPath + "\n")
			w.WriteString("manifest rev=" + strconv.FormatUint(collected.GetRev(), 10) +
				" ref=" + manifestRef.GetRootRef().MarshalString() + "\n")

			strm, err := spaceSvc.DeployManifest(ctx)
			if err != nil {
				return errors.Wrap(err, "open deploy stream")
			}

			// Send initial deploy request with manifest ref.
			err = strm.Send(&s4wave_deploy.DeployManifestMessage{
				Body: &s4wave_deploy.DeployManifestMessage_Request{
					Request: &s4wave_deploy.DeployManifestRequest{
						SpaceId:     spaceID,
						ManifestRef: manifestRef,
						ObjectKey:   key,
						ManifestId:  manifestID,
					},
				},
			})
			if err != nil {
				return errors.Wrap(err, "send deploy request")
			}

			return runDeployBlockExchange(ctx, strm, vol, w)
		},
	}
}

// runDeployBlockExchange handles the bidirectional block exchange for deploy.
// The server requests blocks and the CLI responds with data from the devtool volume.
func runDeployBlockExchange(
	ctx context.Context,
	strm s4wave_space.SRPCSpaceResourceService_DeployManifestClient,
	vol volume.Volume,
	w *os.File,
) error {
	for {
		msg, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv from server")
		}

		switch body := msg.GetBody().(type) {
		case *s4wave_deploy.DeployManifestMessage_BlockRequest:
			ref := body.BlockRequest.GetRef()
			w.WriteString("server requested block: " + ref.MarshalString() + "\n")

			data, found, err := vol.GetBlock(ctx, ref)
			if err != nil {
				return errors.Wrap(err, "read block from devtool volume")
			}

			resp := &s4wave_deploy.BlockResponse{
				Ref:      ref,
				NotFound: !found,
			}
			if found {
				resp.Data = data
			}
			err = strm.Send(&s4wave_deploy.DeployManifestMessage{
				Body: &s4wave_deploy.DeployManifestMessage_BlockResponse{
					BlockResponse: resp,
				},
			})
			if err != nil {
				return errors.Wrap(err, "send block response")
			}

		case *s4wave_deploy.DeployManifestMessage_Result:
			result := body.Result
			if result.GetError() != "" {
				return errors.Errorf("deploy failed: %s", result.GetError())
			}
			w.WriteString("deploy complete\n")
			return nil

		default:
			return errors.Errorf("unexpected message from server: %T", body)
		}
	}
}

// printSpaceState prints space state details to stdout.
func printSpaceState(spaceID string, state *s4wave_space.SpaceState) {
	w := os.Stdout
	stateStr := "loading"
	if state.GetReady() {
		stateStr = "ready"
	}
	writeFields(w, [][2]string{
		{"Space", spaceID},
		{"State", stateStr},
	})
	if state.GetReady() {
		if wc := state.GetWorldContents(); wc != nil {
			objs := wc.GetObjects()
			if len(objs) > 0 {
				w.WriteString("\nObjects (" + strconv.Itoa(len(objs)) + ")\n")
				rows := [][]string{{"KEY", "TYPE"}}
				for _, obj := range objs {
					rows = append(rows, []string{obj.GetObjectKey(), obj.GetObjectType()})
				}
				writeTable(w, "  ", rows)
			}
		}
		if settings := state.GetSettings(); settings != nil {
			plugins := settings.GetPluginIds()
			if len(plugins) > 0 {
				w.WriteString("\nPlugins (" + strconv.Itoa(len(plugins)) + ")\n")
				for _, pid := range plugins {
					w.WriteString("  " + pid + "\n")
				}
			}
		}
	}
}

// printSpaceSettings prints space settings to stdout.
func printSpaceSettings(settings *space_world.SpaceSettings) {
	w := os.Stdout
	if settings == nil {
		w.WriteString("no settings\n")
		return
	}
	var fields [][2]string
	if idx := settings.GetIndexPath(); idx != "" {
		fields = append(fields, [2]string{"Index Path", idx})
	}
	if len(fields) > 0 {
		writeFields(w, fields)
	}
	plugins := settings.GetPluginIds()
	if len(plugins) > 0 {
		if len(fields) > 0 {
			w.WriteString("\n")
		}
		w.WriteString("Plugins (" + strconv.Itoa(len(plugins)) + ")\n")
		for _, pid := range plugins {
			w.WriteString("  " + pid + "\n")
		}
	} else if len(fields) == 0 {
		w.WriteString("no settings\n")
	}
}

// printSpacesList prints the spaces list to stdout.
func printSpacesList(resp *s4wave_session.WatchResourcesListResponse) {
	spaces := resp.GetSpacesList()
	if len(spaces) == 0 {
		os.Stdout.WriteString("no spaces\n")
		return
	}
	rows := [][]string{{"ID", "NAME"}}
	for _, sp := range spaces {
		rows = append(rows, []string{
			truncateID(sp.GetEntry().GetRef().GetProviderResourceRef().GetId(), 8),
			sp.GetSpaceMeta().GetName(),
		})
	}
	writeTable(os.Stdout, "", rows)
}
