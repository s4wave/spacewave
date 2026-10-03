//go:build !js

package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// RunListSpaces lists spaces in the current session.
func (a *ClientArgs) RunListSpaces(c *cli.Context) error {
	// Mount the selected Session and retain it until the command returns.
	ctx := c.Context
	sess, cleanup, err := a.MountSession(ctx, sessionIndex32(a.SessionIdx))
	if err != nil {
		return err
	}
	defer cleanup()

	// Watch the Session resource list for its current Spaces.
	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	// Receive the Session resource snapshot for the Space listing.
	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv resources list")
	}

	// Finish the listing when the Session contains no Spaces.
	spaces := resp.GetSpacesList()
	if len(spaces) == 0 {
		os.Stdout.WriteString("no spaces found\n")
		return nil
	}

	// Print each Space ID and name in aligned columns.
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
	// Mount the selected Session and retain it until the command returns.
	ctx := c.Context
	sess, cleanup, err := a.MountSession(ctx, sessionIndex32(a.SessionIdx))
	if err != nil {
		return err
	}
	defer cleanup()

	// Create the named Space in the mounted Session.
	resp, err := sess.CreateSpace(ctx, &s4wave_session.CreateSpaceRequest{SpaceName: a.SpaceName})
	if err != nil {
		return errors.Wrap(err, "create space")
	}

	// Report the newly created Space ID and name.
	id := resp.GetSharedObjectRef().GetProviderResourceRef().GetId()
	os.Stdout.WriteString("created space: " + id + " (name=" + a.SpaceName + ")\n")
	return nil
}
