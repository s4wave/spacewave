//go:build !js

package spacewave_cli

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// FsResetRootArgs selects a linked folder and the UnixFS directory it links
// to.
type FsResetRootArgs struct {
	// FsLinkArgs selects the folder and its destination.
	FsLinkArgs
}

// Run records the file system the folder's root is on now as the one it is
// linked on, and prints the identities before and after.
func (a *FsResetRootArgs) Run(c *cli.Context) error {
	// Open the folder's destination.
	folder, _, spaceSvc, release, err := a.openLinkTarget(c)
	if err != nil {
		return err
	}
	defer release()

	// Record the root's file system.
	resp, err := spaceSvc.ResetFolderRoot(c.Context, &s4wave_space.ResetFolderRootRequest{Folder: folder})
	if err != nil {
		return errors.Wrap(err, "reset folder root")
	}
	if resp.GetPrevious() == resp.GetCurrent() {
		fmt.Fprintf(os.Stdout, "%s is on file system %d, as recorded\n", folder, resp.GetCurrent())
		return nil
	}
	fmt.Fprintf(os.Stdout, "%s: recorded file system %d, was %d; the link resumes with a full sync\n", folder, resp.GetCurrent(), resp.GetPrevious())
	return nil
}

// newFsResetRootCommand builds the command that accepts a linked folder's new
// file system.
func newFsResetRootCommand() *cli.Command {
	args := &FsResetRootArgs{}
	return &cli.Command{
		Name:      "reset-root",
		Usage:     "accept the file system a linked folder is on now, as after its disk returns",
		ArgsUsage: "FOLDER spacewave:DESTINATION",
		Description: fsURIDescription(`A linked folder stops syncing while its root is missing or on another file
system than the one it was linked on, as when its disk is unmounted, so an
empty mount point never empties the Drive. A disk mounted again can come back
with another file-system identity; this command records the one the folder is
on now, and the link resumes with a full sync that removes every Drive entry
the folder lacks. Run it only when FOLDER holds the linked files.`, "  spacewave fs reset-root /Volumes/photos spacewave:drive/photos"),
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
