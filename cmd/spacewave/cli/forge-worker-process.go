//go:build !js

package spacewave_cli

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// buildForgeWorkerProcessCommand controls the existing Space process binding.
// The Space runtime owns execution after this command releases its connection.
func buildForgeWorkerProcessCommand(statePath *string, sessionIdx *uint, spaceID *string, flags []cli.Flag, approved bool) *cli.Command {
	name, usage, result := "stop-worker", "disable a Forge Worker's local process binding", "Disabled"
	if approved {
		name, usage, result = "start-worker", "enable a Forge Worker's local process binding", "Enabled"
	}
	return &cli.Command{
		Name: name, Usage: usage, ArgsUsage: "<worker-key>", Flags: flags,
		Description: "The Worker must already exist and belong to this session peer. The Space runtime starts or stops execution and retains the binding across CLI invocations.",
		Action: func(c *cli.Context) error {
			if c.Args().Len() != 1 {
				return errors.New("one Worker key is required")
			}
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()
			session, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer session.Release()
			sid, err := client.resolveSpaceID(ctx, session, *spaceID)
			if err != nil {
				return err
			}
			space, releaseSpace, err := client.mountSpace(ctx, session, sid)
			if err != nil {
				return err
			}
			defer releaseSpace()
			contents, releaseContents, err := client.mountSpaceContents(ctx, space)
			if err != nil {
				return err
			}
			defer releaseContents()
			_, err = contents.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
				ObjectKey: c.Args().First(), TypeId: forge_worker.WorkerTypeID, Approved: approved,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(os.Stdout, "%s worker %q on this device.\n", result, c.Args().First())
			return err
		},
	}
}
