//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// FsSyncArgs selects a local directory and a UnixFS directory to mirror.
type FsSyncArgs struct {
	// statePath selects the daemon state directory.
	statePath string
	// spaceID overrides the Space in the URI.
	spaceID string
	// sessionIdx overrides the session in the URI.
	sessionIdx int
	// keepExtra preserves destination entries absent from the source.
	keepExtra bool
}

// BuildFlags builds the targeting and deletion flags.
func (a *FsSyncArgs) BuildFlags() []cli.Flag {
	return append(commonFsFlags(&a.statePath, &a.spaceID, &a.sessionIdx),
		&cli.BoolFlag{Name: "keep-extra", Usage: "copy without deleting destination-only entries", Destination: &a.keepExtra},
	)
}

// Run mirrors the source directory and fences uploaded World blocks.
func (a *FsSyncArgs) Run(c *cli.Context) error {
	// Require an explicit remote prefix so local paths cannot select the wrong side.
	if c.NArg() != 2 {
		return errors.New("expected SOURCE DESTINATION; prefix the UnixFS URI with spacewave:")
	}
	source, destination := c.Args().Get(0), c.Args().Get(1)
	upload := strings.HasPrefix(destination, "spacewave:")
	if upload == strings.HasPrefix(source, "spacewave:") {
		return errors.New("exactly one path must have the spacewave: prefix")
	}
	localPath, remotePath := source, destination
	if !upload {
		localPath, remotePath = destination, source
	}
	uri, err := parseFsURI(strings.TrimPrefix(remotePath, "spacewave:"), a.spaceID, a.sessionIdx)
	if err != nil {
		return err
	}
	if upload {
		info, err := os.Stat(localPath)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("sync source must be a directory")
		}
	}

	// Mount the existing World interface; UnixFS owns traversal and file updates.
	ctx := c.Context
	engine, release, resolvedSpaceID, err := mountSpaceWorldEngine(ctx, c, a.statePath, uint(uri.sessionIdx), uri.spaceID)
	if err != nil {
		return err
	}
	defer release()
	ws := world.NewEngineWorldState(engine, upload)
	if !upload {
		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer tx.Discard()
		ws = tx
	}
	ref := &unixfs_world.UnixfsRef{ObjectKey: uri.objectKey}
	root, err := unixfs_world.BuildFSFromUnixfsRef(ctx, nil, ws, "", ref, false, upload, time.Now())
	if err != nil {
		return err
	}
	defer root.Release()
	handle := root
	if uri.path != "" {
		var missing []string
		handle, missing, err = root.LookupPath(ctx, uri.path)
		if err != nil {
			return err
		}
		defer handle.Release()
		if len(missing) != 0 {
			return errors.Errorf("UnixFS directory does not exist: %s", uri.path)
		}
	}
	nodeType, err := handle.GetNodeType(ctx)
	if err != nil {
		return err
	}
	if !nodeType.GetIsDirectory() {
		return errors.New("UnixFS sync path must be a directory")
	}

	// Delete only after the complete copy succeeds, as in rclone sync.
	mode := unixfs_sync.DeleteMode_DeleteMode_AFTER
	if a.keepExtra {
		mode = unixfs_sync.DeleteMode_DeleteMode_NONE
	}
	if upload {
		if err := unixfs_sync.SyncFromDisk(ctx, handle, localPath, mode, nil); err != nil {
			return err
		}
		if _, err := engine.Sync(ctx); err != nil {
			return errors.Wrap(err, "make synced World durable")
		}
		return withSession(c, a.statePath, uint(uri.sessionIdx), func(ctx context.Context, sess *s4wave_session.Session) error {
			return waitSpaceStorageSynced(ctx, sess, resolvedSpaceID)
		})
	}
	return unixfs_sync.Sync(ctx, localPath, handle, mode, nil)
}

// newFsSyncCommand builds the directory mirror command.
func newFsSyncCommand() *cli.Command {
	args := &FsSyncArgs{}
	return &cli.Command{
		Name:        "sync",
		Usage:       "mirror directory contents between disk and UnixFS, changing only the destination",
		ArgsUsage:   "SOURCE DESTINATION",
		Description: fsURIDescription("Prefix exactly one argument with spacewave:. The other is a local directory.\nThe UnixFS directory must exist. Destination-only entries are deleted after\na successful copy unless --keep-extra is set. Files match by size and mtime.\nA failed copy can leave partial updates; destination-only deletion is skipped.\nUploads wait for World storage durability before returning.", "  spacewave fs sync ./snapshot spacewave:backup\n  spacewave fs sync spacewave:backup ./restore"),
		Flags:       args.BuildFlags(),
		Action:      args.Run,
	}
}

// waitSpaceStorageSynced waits for the provider that owns the Space's uploads.
// Call after Engine.Sync has persisted the World and marked its blocks for upload.
func waitSpaceStorageSynced(ctx context.Context, sess *s4wave_session.Session, spaceID string) error {
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return err
	}
	if info.GetSessionRef().GetProviderResourceRef().GetProviderId() != provider_local.ProviderID {
		return waitSessionSynced(ctx, sess)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := sess.WatchSpaceStorage(ctx, spaceID)
	if err != nil {
		return errors.Wrap(err, "watch Space uploads")
	}
	defer stream.Close()
	for {
		status, err := stream.Recv()
		if err != nil {
			return errors.Wrap(err, "receive Space upload status")
		}
		if status.GetUploadError() != "" {
			return errors.Errorf("Space upload failed: %s", status.GetUploadError())
		}
		if status.GetPendingBlocks() == 0 {
			return nil
		}
	}
}
