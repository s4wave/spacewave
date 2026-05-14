//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
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
			buildPluginAddCommand(),
			buildPluginRemoveCommand(),
			buildPluginImportManifestCommand(getBus),
		},
	}
}

// buildPluginImportManifestCommand builds the plugin import-manifest subcommand.
func buildPluginImportManifestCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	var dbPath string
	var manifestID string
	var objectKey string
	var targetDBPath string
	return &cli.Command{
		Name:  "import-manifest",
		Usage: "import a built manifest into the local plugin host store",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "db",
				Usage:       "path to .bldr/ directory containing the devtool DB",
				Required:    true,
				Destination: &dbPath,
			},
			&cli.StringFlag{
				Name:        "manifest-id",
				Usage:       "manifest identifier",
				Required:    true,
				Destination: &manifestID,
			},
			&cli.StringFlag{
				Name:        "object-key",
				Usage:       "plugin host object key to import into",
				Value:       daemonPluginHostObjectKey,
				Destination: &objectKey,
			},
			&cli.StringFlag{
				Name:        "target-db",
				Usage:       "optional path to target .bldr/ devtool DB; defaults to the running daemon world",
				Destination: &targetDBPath,
			},
		},
		Action: func(c *cli.Context) error {
			return runPluginImportManifest(c.Context, getBus, dbPath, targetDBPath, manifestID, objectKey)
		},
	}
}

func runPluginImportManifest(
	ctx context.Context,
	getBus func() cli_entrypoint.CliBus,
	dbPath string,
	targetDBPath string,
	manifestID string,
	objectKey string,
) error {
	cliBus := getBus()
	if cliBus == nil {
		return errors.New("bus not initialized")
	}
	if objectKey == "" {
		return errors.New("object-key is required")
	}
	le := cliBus.GetLogger()
	src, err := openDevtoolVolume(ctx, le, dbPath)
	if err != nil {
		return errors.Wrap(err, "open devtool storage")
	}
	defer src.Close()
	collectedManifests, err := lookupDevtoolManifests(ctx, le, src, manifestID)
	if err != nil {
		return errors.Wrap(err, "lookup manifest")
	}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_s2.NewStepFactory())
	transformConf, err := block_transform.NewConfig([]config.Config{&transform_s2.Config{}})
	if err != nil {
		return errors.Wrap(err, "build transform config")
	}
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		transformConf,
	)
	if err != nil {
		return errors.Wrap(err, "build block transformer")
	}
	var dest volume.Volume
	var destEngine world.Engine
	var destClose func() error
	destBucketID := "bldr/cli"
	if targetDBPath == "" {
		dest = cliBus.GetVolume()
		destEngine = cliBus.GetWorldEngine()
	} else {
		dest, err = openDevtoolVolume(ctx, le, targetDBPath)
		if err != nil {
			return errors.Wrap(err, "open target devtool storage")
		}
		defer dest.Close()
		targetEng, err := openDevtoolWorldEngine(ctx, le, dest)
		if err != nil {
			return errors.Wrap(err, "open target devtool world")
		}
		destEngine = targetEng
		destClose = targetEng.Close
		destBucketID = devtoolEngineBucketID
	}
	if destClose != nil {
		defer destClose()
	}
	for _, collected := range collectedManifests {
		rootRef := collected.ManifestRef.GetRootRef()
		if err := copyManifestBlockDAG(ctx, rootRef, src, dest, xfrm); err != nil {
			return errors.Wrap(err, "copy manifest blocks")
		}
	}
	tx, err := destEngine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tx, objectKey); err != nil {
		return errors.Wrap(err, "create plugin host manifest store")
	}
	imported := make([]string, 0, len(collectedManifests))
	for _, collected := range collectedManifests {
		manifestKey := bldr_manifest.NewManifestKey(objectKey, collected.Manifest.GetMeta())
		objRef := collected.ManifestRef.Clone()
		objRef.BucketId = destBucketID
		objRef.TransformConf = transformConf
		if _, _, err := bldr_manifest_world.SetManifest(ctx, tx, "", manifestKey, objRef); err != nil {
			return errors.Wrap(err, "set manifest")
		}
		if err := tx.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objectKey, manifestKey, manifestID)); err != nil {
			return errors.Wrap(err, "link manifest")
		}
		imported = append(imported, manifestKey)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit manifest import")
	}
	for _, manifestKey := range imported {
		os.Stdout.WriteString("imported: " + manifestKey + "\n")
	}
	return nil
}

func copyManifestBlockDAG(
	ctx context.Context,
	rootRef *block.BlockRef,
	src block.StoreOps,
	dest block.StoreOps,
	xfrm block.Transformer,
) error {
	if rootRef.GetEmpty() {
		return nil
	}
	visited := make(map[string]bool)
	return copyManifestBlock(ctx, rootRef, bldr_manifest.NewManifestBlock, src, dest, xfrm, visited)
}

func copyManifestBlock(
	ctx context.Context,
	ref *block.BlockRef,
	ctor block.Ctor,
	src block.StoreOps,
	dest block.StoreOps,
	xfrm block.Transformer,
	visited map[string]bool,
) error {
	if ref.GetEmpty() {
		return nil
	}
	refStr := ref.MarshalString()
	if visited[refStr] {
		return nil
	}
	visited[refStr] = true
	exists, err := dest.GetBlockExists(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "check block exists: %s", refStr)
	}
	var data []byte
	if exists {
		var found bool
		data, found, err = dest.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "get existing block: %s", refStr)
		}
		if !found {
			return errors.Wrapf(block.ErrNotFound, "existing block: %s", refStr)
		}
	} else {
		var found bool
		data, found, err = src.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "get block: %s", refStr)
		}
		if !found {
			return errors.Wrapf(block.ErrNotFound, "block: %s", refStr)
		}
		if _, _, err := dest.PutBlock(ctx, data, nil); err != nil {
			return errors.Wrapf(err, "put block: %s", refStr)
		}
	}
	if ctor == nil {
		return nil
	}
	decoded := data
	if xfrm != nil {
		decoded, err = xfrm.DecodeBlock(data)
		if err != nil {
			return errors.Wrapf(err, "decode block: %s", refStr)
		}
	}
	blk := ctor()
	if err := blk.UnmarshalBlock(decoded); err != nil {
		return errors.Wrapf(err, "unmarshal block: %s", refStr)
	}
	return followManifestBlockRefs(ctx, blk, src, dest, xfrm, visited)
}

func followManifestBlockRefs(
	ctx context.Context,
	blk any,
	src block.StoreOps,
	dest block.StoreOps,
	xfrm block.Transformer,
	visited map[string]bool,
) error {
	if withRefs, ok := blk.(block.BlockWithRefs); ok {
		refs, err := withRefs.GetBlockRefs()
		if err != nil {
			return errors.Wrap(err, "get block refs")
		}
		for id, childRef := range refs {
			childCtor := withRefs.GetBlockRefCtor(id)
			if err := copyManifestBlock(ctx, childRef, childCtor, src, dest, xfrm, visited); err != nil {
				return err
			}
		}
	}
	if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
		for _, sub := range withSubBlocks.GetSubBlocks() {
			if sub == nil || sub.IsNil() {
				continue
			}
			if err := followManifestBlockRefs(ctx, sub, src, dest, xfrm, visited); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildPluginListCommand builds the plugin list subcommand.
func buildPluginListCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "list",
		Usage: "list plugins and their load state",
		Flags: append(clientFlags(&statePath, &sessionIdx),
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
		),
		Action: func(c *cli.Context) error {
			ctx := c.Context
			spaceID := c.String("space")
			watch := c.Bool("watch")
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(sessionIdx))
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
						state := "loading"
						if p.GetLoaded() {
							state = "loaded"
						}
						w.WriteString("  " + state)
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

// buildPluginAddCommand builds the plugin add subcommand.
func buildPluginAddCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:      "add",
		Usage:     "add a plugin to space settings",
		ArgsUsage: "<manifest-id>",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
		),
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

			sess, err := client.mountSession(ctx, uint32(sessionIdx))
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
	var sessionIdx uint
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a plugin from space settings",
		ArgsUsage: "<manifest-id>",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:    "space",
				Usage:   "space ID (auto-detected if only one space)",
				EnvVars: []string{"SPACEWAVE_SPACE"},
			},
		),
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

			sess, err := client.mountSession(ctx, uint32(sessionIdx))
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
