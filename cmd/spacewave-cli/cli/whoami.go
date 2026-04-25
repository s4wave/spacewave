//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	session_pb "github.com/s4wave/spacewave/core/session"
)

// newWhoamiCommand builds the whoami command.
func newWhoamiCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "whoami",
		Usage: "show current session identity",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runWhoami(c, statePath, c.String("output"), uint32(sessionIdx))
		},
	}
}

// runWhoami implements the whoami command logic.
func runWhoami(c *cli.Context, statePath, outputFormat string, sessionIdx uint32) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

	ref := info.GetSessionRef().GetProviderResourceRef()

	// Get lock state.
	lockMode := "auto"
	lockStr := "unlocked (auto)"
	lockStrm, err := sess.WatchLockState(ctx)
	if err == nil {
		lockResp, lerr := lockStrm.Recv()
		if lerr == nil {
			if lockResp.GetMode() == session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
				lockMode = "pin"
			}
			if lockResp.GetLocked() {
				lockStr = "locked (" + lockMode + ")"
			} else {
				lockStr = "unlocked (" + lockMode + ")"
			}
		}
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionId")
		ms.WriteString(ref.GetId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("peerId")
		ms.WriteString(info.GetPeerId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(ref.GetProviderId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerAccountId")
		ms.WriteString(ref.GetProviderAccountId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("lock")
		ms.WriteString(lockStr)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	writeFields(os.Stdout, [][2]string{
		{"Session", ref.GetId()},
		{"Peer", info.GetPeerId()},
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Lock", lockStr},
	})
	return nil
}
