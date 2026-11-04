//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// spaceSecretReadArgs selects a Secret under the mounted Session's authority.
type spaceSecretReadArgs struct {
	// statePath selects the daemon state directory.
	statePath *string
	// sessionIdx selects the signed-in Session.
	sessionIdx *uint
	// spaceID selects the Space by name or ID.
	spaceID *string
	// expectedKind optionally restricts the Secret's use.
	expectedKind string
}

// BuildFlags exposes the optional Secret kind check.
func (a *spaceSecretReadArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "expected-kind", Usage: "require this Secret kind before reading", Destination: &a.expectedKind},
	}
}

// Run writes the raw Secret payload to stdout without adding a newline.
func (a *spaceSecretReadArgs) Run(c *cli.Context) error {
	// Reject ambiguous targets before opening a daemon connection.
	if c.NArg() != 1 || c.Args().First() == "" {
		return errors.New("secret read requires exactly one Secret object key")
	}
	key := c.Args().First()

	// Mount the selected Session with its existing authority.
	client, err := connectDaemonFromContext(c.Context, c, *a.statePath)
	if err != nil {
		return errors.Wrap(err, "read Secret "+key)
	}
	defer client.close()
	sess, err := client.mountSession(c.Context, sessionIndex32(*a.sessionIdx))
	if err != nil {
		return errors.Wrap(err, "read Secret "+key)
	}
	defer sess.Release()

	// Resolve and mount only the Space that holds the requested Secret.
	sid, err := client.resolveSpaceID(c.Context, sess, *a.spaceID)
	if err != nil {
		return errors.Wrap(err, "read Secret "+key)
	}
	spaceSvc, release, err := client.mountSpace(c.Context, sess, sid)
	if err != nil {
		return errors.Wrap(err, "read Secret "+key)
	}
	defer release()

	// Let the Secret service enforce the Session grant and expected kind.
	resp, err := spaceSvc.ReadSecretPayload(c.Context, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey: key, ExpectedKind: a.expectedKind,
	})
	if err != nil {
		return errors.Wrap(err, "read Secret "+key)
	}

	// Preserve binary payloads and propagate redirected-output failures.
	_, err = c.App.Writer.Write(resp.GetPayload().GetValue())
	return errors.Wrap(err, "write Secret "+key)
}

// newSpaceSecretReadCommand exposes the existing authenticated payload read.
func newSpaceSecretReadCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	// Bind the command to its parent Session and Space selectors.
	a := &spaceSecretReadArgs{statePath: statePath, sessionIdx: sessionIdx, spaceID: spaceID}
	return &cli.Command{
		Name: "read", Usage: "write a Secret's raw payload to stdout", ArgsUsage: "<object-key>",
		Description: "Read with the selected Session's existing Secret grant. The payload is written verbatim, without a trailing newline.\n\nExamples:\n  spacewave space secret --space my-space read secrets/provider\n  spacewave space secret --space my-space read --expected-kind provider_credential secrets/provider | jq -r .api_key",
		Flags:       a.BuildFlags(), Action: a.Run,
	}
}
