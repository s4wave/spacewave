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
	return &cli.Command{
		Name:        "export-root",
		Usage:       "save a recovery root after local persistence and remote uploads complete",
		ArgsUsage:   "NEW_FILE",
		Description: "Writes a binary WorldRootSnapshot with mode 0600, refusing to overwrite a file.\nThe file contains decryption material: encrypt it before storing it offsite.\nKeep the Space's S3 endpoint, bucket, and block-store prefix with the recovery file.\nThe referenced packfiles must remain available; this file does not contain them.\nExporting does not register a permanent garbage-collection retention root.",
		Action: func(c *cli.Context) error {
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
			if _, err := engine.Sync(ctx); err != nil {
				return err
			}
			if err := withSession(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
				return waitSpaceStorageSynced(ctx, sess, resolvedSpaceID)
			}); err != nil {
				return err
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
