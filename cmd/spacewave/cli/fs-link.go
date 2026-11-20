//go:build !js

package spacewave_cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// FsLinkArgs selects a local folder and the UnixFS directory to link it at.
type FsLinkArgs struct {
	// statePath selects the daemon state directory.
	statePath string
	// spaceID overrides the Space in the URI.
	spaceID string
	// sessionIdx overrides the session in the URI.
	sessionIdx int
}

// BuildFlags builds the targeting flags.
func (a *FsLinkArgs) BuildFlags() []cli.Flag {
	return commonFsFlags(&a.statePath, &a.spaceID, &a.sessionIdx)
}

// Run links the folder and reports the files and bytes indexed.
func (a *FsLinkArgs) Run(c *cli.Context) error {
	// Open the folder's destination.
	folder, uri, spaceSvc, release, err := a.openLinkTarget(c)
	if err != nil {
		return err
	}
	defer release()

	// Link the folder, printing each batch's progress.
	strm, err := spaceSvc.LinkFolder(c.Context, &s4wave_space.LinkFolderRequest{ObjectKey: uri.objectKey, Path: uri.path, Folder: folder})
	if err != nil {
		return errors.Wrap(err, "link folder")
	}
	defer strm.Close()
	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			return errors.New("link folder: stream ended before it finished")
		}
		if err != nil {
			return errors.Wrap(err, "link folder")
		}
		if msg.GetDone() {
			fmt.Fprintf(os.Stdout, "linked %d files, %d bytes\n", msg.GetFiles(), msg.GetBytes())
			return nil
		}
		fmt.Fprintf(os.Stderr, "linked %d files, %d bytes so far\n", msg.GetFiles(), msg.GetBytes())
	}
}

// openLinkTarget parses the FOLDER spacewave:DESTINATION arguments, requires
// the folder to be a directory, and mounts the destination's Space. It
// returns the folder's absolute path, the destination, the Space resource,
// and the release of the mounts.
func (a *FsLinkArgs) openLinkTarget(c *cli.Context) (string, fsURI, s4wave_space.SRPCSpaceResourceServiceClient, func(), error) {
	// Parse the local folder and the spacewave: destination.
	if c.NArg() != 2 {
		return "", fsURI{}, nil, nil, errors.New("expected FOLDER spacewave:DESTINATION")
	}
	folder, destination := c.Args().Get(0), c.Args().Get(1)
	if !strings.HasPrefix(destination, "spacewave:") {
		return "", fsURI{}, nil, nil, errors.New("the destination must have the spacewave: prefix")
	}
	uri, err := parseFsURI(strings.TrimPrefix(destination, "spacewave:"), a.spaceID, a.sessionIdx)
	if err != nil {
		return "", fsURI{}, nil, nil, err
	}

	// Require the folder to be a directory, by its absolute path.
	folder, err = filepath.Abs(folder)
	if err != nil {
		return "", fsURI{}, nil, nil, err
	}
	info, err := os.Stat(folder)
	if err != nil {
		return "", fsURI{}, nil, nil, err
	}
	if !info.IsDir() {
		return "", fsURI{}, nil, nil, errors.New("link source must be a directory")
	}

	// Mount the session.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return "", fsURI{}, nil, nil, err
	}
	sess, err := client.mountSession(ctx, uri.sessionIdx)
	if err != nil {
		client.close()
		return "", fsURI{}, nil, nil, err
	}
	releaseSession := func() {
		sess.Release()
		client.close()
	}

	// Mount the Space.
	spaceID, err := client.resolveSpaceID(ctx, sess, uri.spaceID)
	if err != nil {
		releaseSession()
		return "", fsURI{}, nil, nil, err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		releaseSession()
		return "", fsURI{}, nil, nil, err
	}
	return folder, uri, spaceSvc, func() {
		spaceCleanup()
		releaseSession()
	}, nil
}

// newFsLinkCommand builds the folder link command.
func newFsLinkCommand() *cli.Command {
	args := &FsLinkArgs{}
	return &cli.Command{
		Name:      "link",
		Usage:     "link a folder on this device into a UnixFS directory without copying its bytes",
		ArgsUsage: "FOLDER spacewave:DESTINATION",
		Description: fsURIDescription(`Each file's blocks stay in the file and are read from it on this device;
only the directory tree and each file's block list go to the Space. A file
changed after linking reads as missing on this device. Symlinks and special
files are skipped. The link commits in batches, so a failed link can leave a
partial tree: run it again to finish.`, "  spacewave fs link ./photos spacewave:drive/photos"),
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
