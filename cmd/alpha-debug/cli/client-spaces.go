//go:build !js

package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
)

// RunListSpaces lists spaces in the current session.
func (a *ClientArgs) RunListSpaces(c *cli.Context) error {
	ctx := c.Context

	sess, cleanup, err := a.MountSession(ctx, uint32(a.SessionIdx))
	if err != nil {
		return err
	}
	defer cleanup()

	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv resources list")
	}

	spaces := resp.GetSpacesList()
	if len(spaces) == 0 {
		os.Stdout.WriteString("no spaces found\n")
		return nil
	}
	w := os.Stdout
	for _, sp := range spaces {
		id := sp.GetEntry().GetRef().GetProviderResourceRef().GetId()
		name := sp.GetSpaceMeta().GetName()
		w.WriteString(id)
		// pad to 40 chars
		for i := len(id); i < 40; i++ {
			w.WriteString(" ")
		}
		w.WriteString("  " + name + "\n")
	}
	return nil
}

// RunCreateSpace creates a new space.
func (a *ClientArgs) RunCreateSpace(c *cli.Context) error {
	ctx := c.Context

	sess, cleanup, err := a.MountSession(ctx, uint32(a.SessionIdx))
	if err != nil {
		return err
	}
	defer cleanup()

	resp, err := sess.CreateSpace(ctx, a.SpaceName, "", "")
	if err != nil {
		return errors.Wrap(err, "create space")
	}
	id := resp.GetSharedObjectRef().GetProviderResourceRef().GetId()
	os.Stdout.WriteString("created space: " + id + " (name=" + a.SpaceName + ")\n")
	return nil
}
