//go:build !js

package spacewave_cli

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// newSpaceWorldExportCommand exports one durable root, including its decryption configuration.
func newSpaceWorldExportCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var retainName string
	return &cli.Command{
		Name:        "export-root",
		Usage:       "save a recovery root after local persistence and remote uploads complete",
		ArgsUsage:   "NEW_FILE",
		Description: "Writes a binary WorldRootSnapshot with mode 0600, refusing to overwrite a file.\nThe file contains decryption material: encrypt it before storing it offsite.\nKeep the Space's S3 endpoint, bucket, and block-store prefix with the recovery file.\nThe referenced packfiles must remain available; this file does not contain them.\nWith --retain, the Space keeps the root's blocks through storage reclaim until\nrelease-root releases the name. A Space holds at most 16 retained roots.\nWithout it, storage reclaim may delete blocks the root alone references.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "retain",
				Usage:       "retain the root in the Space under this name, replacing the root the name held",
				Destination: &retainName,
			},
		},
		Action: func(c *cli.Context) error {
			// Require one new output path and mount the Space's World.
			if c.NArg() != 1 {
				return errors.New("one new output file is required")
			}
			ctx := c.Context
			engine, release, resolvedSpaceID, err := mountSpaceWorldEngine(ctx, c, *statePath, *sessionIdx, *spaceID)
			if err != nil {
				return err
			}
			defer release()

			// Hold a reader pin until every block of this committed root is uploaded.
			tx, err := engine.NewTransaction(ctx, false)
			if err != nil {
				return err
			}
			defer tx.Discard()

			// Record the committed root with an inline transform so recovery needs no account state.
			snapshot := &s4wave_world.WorldRootSnapshot{}
			snapshot.Seqno, err = tx.GetSeqno(ctx)
			if err != nil {
				return err
			}
			if err := tx.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
				snapshot.RootRef = cursor.GetRef()
				snapshot.RootRef.TransformConf = cursor.GetTransformConf().CloneVT()
				snapshot.RootRef.TransformConfRef = nil
				return nil
			}); err != nil {
				return errors.Wrap(err, "resolve recovery transform")
			}

			// Make the root durable and wait until the Space's storage has its blocks.
			if _, err := engine.Sync(ctx); err != nil {
				return err
			}
			if err := withSession(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
				return waitSpaceStorageSynced(ctx, sess, resolvedSpaceID)
			}); err != nil {
				return err
			}

			// Keep the uploaded root's blocks through storage reclaim.
			if retainName != "" {
				if err := engine.SetRetainedRoot(ctx, retainName, snapshot.GetRootRef().GetRootRef()); err != nil {
					return errors.Wrap(err, "retain recovery root")
				}
			}

			// Write secrets only into a newly created private file.
			data, err := snapshot.MarshalVT()
			if err != nil {
				return err
			}
			file, err := os.OpenFile(c.Args().First(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return err
			}

			// Remove the partial file unless every write reaches disk.
			complete := false
			defer func() {
				if !complete {
					_ = file.Close()
					_ = os.Remove(c.Args().First())
				}
			}()
			if _, err := file.Write(data); err != nil {
				return err
			}
			if err := file.Sync(); err != nil {
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			complete = true
			return nil
		},
	}
}

// newSpaceWorldReleaseRootCommand releases a root export-root retained.
func newSpaceWorldReleaseRootCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:        "release-root",
		Usage:       "release a retained recovery root",
		ArgsUsage:   "NAME",
		Description: "Releases the root export-root --retain kept under NAME.\nThe next storage reclaim pass may delete blocks only that root referenced,\nso recovery files of the root stop restoring.",
		Action: func(c *cli.Context) error {
			// Require one name and mount the Space's World.
			if c.NArg() != 1 {
				return errors.New("one retained root name is required")
			}
			ctx := c.Context
			engine, release, _, err := mountSpaceWorldEngine(ctx, c, *statePath, *sessionIdx, *spaceID)
			if err != nil {
				return err
			}
			defer release()

			// Release the name.
			return engine.SetRetainedRoot(ctx, c.Args().First(), nil)
		},
	}
}
