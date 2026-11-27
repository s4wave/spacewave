//go:build !js

package spacewave_cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// FsLinksArgs selects the Drive whose linked folders to watch.
type FsLinksArgs struct {
	// statePath selects the daemon state directory.
	statePath string
	// spaceID overrides the Space in the URI.
	spaceID string
	// sessionIdx overrides the session in the URI.
	sessionIdx int
}

// BuildFlags builds the targeting flags.
func (a *FsLinksArgs) BuildFlags() []cli.Flag {
	return commonFsFlags(&a.statePath, &a.spaceID, &a.sessionIdx)
}

// Run prints the Drive's linked folders and each change to them, one block
// per update, until interrupted.
func (a *FsLinksArgs) Run(c *cli.Context) error {
	// Parse the Drive and mount its Space.
	if c.NArg() != 1 {
		return errors.New("expected spacewave:DRIVE")
	}
	uri, err := parseFsURI(strings.TrimPrefix(c.Args().Get(0), "spacewave:"), a.spaceID, a.sessionIdx)
	if err != nil {
		return err
	}
	spaceSvc, release, err := mountURISpace(c, a.statePath, uri)
	if err != nil {
		return err
	}
	defer release()

	// Print each update with the time it arrived.
	strm, err := spaceSvc.WatchDriveLinks(c.Context, &s4wave_space.WatchDriveLinksRequest{ObjectKey: uri.objectKey})
	if err != nil {
		return errors.Wrap(err, "watch drive links")
	}
	defer strm.Close()
	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "watch drive links")
		}
		now := time.Now().Format(time.RFC3339Nano)
		fmt.Fprintf(os.Stdout, "%s other peers reached: %d\n", now, msg.GetOtherPeersReached())
		for _, l := range msg.GetLinks() {
			where := "elsewhere"
			if l.GetLocal() != nil {
				where = "here"
			}
			fmt.Fprintf(os.Stdout, "%s %q /%s %s %s %s\n", now, l.GetDeviceLabel(), l.GetPath(), l.GetState(), l.GetAvailability(), where)
		}
	}
}

// newFsLinksCommand builds the command that watches a Drive's linked folders.
func newFsLinksCommand() *cli.Command {
	args := &FsLinksArgs{}
	return &cli.Command{
		Name:      "links",
		Usage:     "watch the folders linked into a Drive and whether this session reaches their Devices",
		ArgsUsage: "spacewave:DRIVE",
		Description: fsURIDescription(`Prints each linking Device with its path, its link state, whether this
session reaches it now and whether it is this Device, then a line per change,
until interrupted. The count of other peers reached names the peers besides
the linking Devices that may also serve the Drive's files.`, "  spacewave fs links spacewave:drive"),
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
