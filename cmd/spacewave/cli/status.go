//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	session_pb "github.com/s4wave/spacewave/core/session"
)

// newStatusCommand builds the status CLI command.
func newStatusCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "status",
		Usage: "check daemon health and show summary",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runStatus(c, statePath, c.String("output"), uint32(sessionIdx))
		},
	}
}

// runStatus implements the status command logic.
func runStatus(c *cli.Context, statePath, outputFormat string, sessionIdx uint32) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()

	sockPath := effectiveSocketPath(c, "")
	if sockPath == "" {
		resolved, err := resolveStatePathFromContext(c, statePath)
		if err != nil {
			return err
		}
		sockPath = filepath.Join(resolved, socketName)
	}

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		if outputFormat == "json" || outputFormat == "yaml" {
			buf, ms := newMarshalBuf()
			ms.WriteObjectStart()
			var f bool
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("status")
			ms.WriteString("running")
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("socket")
			ms.WriteString(sockPath)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("sessionIndex")
			ms.WriteUint32(sessionIdx)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("error")
			ms.WriteString("no session: " + err.Error())
			ms.WriteObjectEnd()
			return formatOutput(buf.Bytes(), outputFormat)
		}
		writeFields(os.Stdout, [][2]string{
			{"Status", "running"},
			{"Socket", sockPath},
			{"Session Index", strconv.FormatUint(uint64(sessionIdx), 10)},
			{"Session", "none (" + err.Error() + ")"},
		})
		return nil
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

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
	ref := info.GetSessionRef().GetProviderResourceRef()
	sessID := ref.GetId()
	peerID := info.GetPeerId()
	provID := ref.GetProviderId()
	acctID := ref.GetProviderAccountId()
	spaceCount := strconv.Itoa(len(spaces))

	// Get lock state (best-effort, don't fail status if unavailable).
	lockStr := ""
	lockStrm, lockErr := sess.WatchLockState(ctx)
	if lockErr == nil {
		lockResp, lerr := lockStrm.Recv()
		if lerr == nil {
			mode := "auto"
			if lockResp.GetMode() == session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
				mode = "pin"
			}
			if lockResp.GetLocked() {
				lockStr = "locked (" + mode + ")"
			} else {
				lockStr = "unlocked (" + mode + ")"
			}
		}
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("status")
		ms.WriteString("running")
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("socket")
		ms.WriteString(sockPath)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionIndex")
		ms.WriteUint32(sessionIdx)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionId")
		ms.WriteString(sessID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("peerId")
		ms.WriteString(peerID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(provID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerAccountId")
		ms.WriteString(acctID)
		if lockStr != "" {
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("lock")
			ms.WriteString(lockStr)
		}
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("spaceCount")
		ms.WriteInt32(int32(len(spaces)))
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	fields := [][2]string{
		{"Status", "running"},
		{"Socket", sockPath},
		{"Session Index", strconv.FormatUint(uint64(sessionIdx), 10)},
		{"Session", truncateID(sessID, 8)},
		{"Peer", truncateID(peerID, 20)},
		{"Provider", provID},
		{"Account", acctID},
	}
	if lockStr != "" {
		fields = append(fields, [2]string{"Lock", lockStr})
	}
	fields = append(fields, [2]string{"Spaces", spaceCount})
	writeFields(os.Stdout, fields)
	return nil
}
