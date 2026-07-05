package runner

import (
	"context"
	stderrors "errors"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

const statusMountSessionStage = "mount session"

var defaultStatusMountSessionTimeout = 10 * time.Second

// RunStatus executes the shared status command against the configured client factory.
func RunStatus(config Config, c *cli.Context, outputFormat string, sessionIdx uint32) error {
	config = config.defaults()
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := config.ClientFactory.NewClient(ctx, c)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.Close()

	endpoint, err := config.ClientFactory.StatusEndpoint(ctx, c)
	if err != nil {
		return err
	}

	mountTimeout, err := config.MountSessionTimeout()
	if err != nil {
		return err
	}
	mountCtx, mountCancel := context.WithTimeout(ctx, mountTimeout)
	defer mountCancel()

	sess, err := client.MountSession(mountCtx, sessionIdx)
	if err != nil {
		if stderrors.Is(mountCtx.Err(), context.DeadlineExceeded) {
			errMsg := statusMountSessionStage + " timed out after " + mountTimeout.String()
			if writeErr := writeStatusStageError(config, outputFormat, endpoint, sessionIdx, statusMountSessionStage, errMsg); writeErr != nil {
				return writeErr
			}
			return errors.New(errMsg)
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
			ms.WriteString(endpoint)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("sessionIndex")
			ms.WriteUint32(sessionIdx)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("error")
			ms.WriteString("no session: " + err.Error())
			ms.WriteObjectEnd()
			return formatOutput(config.Stdout, buf.Bytes(), outputFormat)
		}
		writeFields(config.Stdout, [][2]string{
			{"Status", "running"},
			{"Socket", endpoint},
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
	lockStr := readLockState(ctx, sess, "")
	recovery, recoveryErr := sess.WatchRecoveryStatus(ctx)

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("status")
		ms.WriteString("running")
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("socket")
		ms.WriteString(endpoint)
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
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("recovery")
		writeRecoveryStatusJSON(ms, recovery, recoveryErr)
		ms.WriteObjectEnd()
		return formatOutput(config.Stdout, buf.Bytes(), outputFormat)
	}

	fields := [][2]string{
		{"Status", "running"},
		{"Socket", endpoint},
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
	appendRecoveryStatusFields(&fields, recovery, recoveryErr)
	writeFields(config.Stdout, fields)
	return nil
}

func readLockState(ctx context.Context, sess Session, fallback string) string {
	lockStr := fallback
	lockStrm, err := sess.WatchLockState(ctx)
	if err != nil {
		return lockStr
	}
	defer lockStrm.Close()

	lockResp, err := lockStrm.Recv()
	if err != nil {
		return lockStr
	}
	mode := "auto"
	if lockResp.GetMode() == session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
		mode = "pin"
	}
	if lockResp.GetLocked() {
		return "locked (" + mode + ")"
	}
	return "unlocked (" + mode + ")"
}

func writeStatusStageError(
	config Config,
	outputFormat string,
	endpoint string,
	sessionIdx uint32,
	stage string,
	errMsg string,
) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("status")
		ms.WriteString("running")
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("socket")
		ms.WriteString(endpoint)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionIndex")
		ms.WriteUint32(sessionIdx)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("stage")
		ms.WriteString(stage)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("error")
		ms.WriteString(errMsg)
		ms.WriteObjectEnd()
		return formatOutput(config.Stdout, buf.Bytes(), outputFormat)
	}
	writeFields(config.Stdout, [][2]string{
		{"Status", "running"},
		{"Socket", endpoint},
		{"Session Index", strconv.FormatUint(uint64(sessionIdx), 10)},
		{"Stage", stage},
		{"Error", errMsg},
	})
	return nil
}

func appendRecoveryStatusFields(
	fields *[][2]string,
	recovery *s4wave_status.RecoveryStatus,
	recoveryErr error,
) {
	if recoveryErr != nil {
		*fields = append(*fields, [2]string{"Recovery", "unavailable (" + recoveryErr.Error() + ")"})
		return
	}
	if recovery == nil {
		*fields = append(*fields, [2]string{"Recovery", "unavailable"})
		return
	}
	if launcher := recovery.GetLauncher(); launcher != nil {
		*fields = append(*fields,
			[2]string{"Launcher Config", recoveryConfigSummary(launcher)},
			[2]string{"Release Metadata", launcher.GetReleaseMetadataOutcome()},
		)
	}
	*fields = append(*fields,
		[2]string{"Plugin Manifests", strconv.Itoa(len(recovery.GetPlugins()))},
		[2]string{"Native Packages", strconv.Itoa(len(recovery.GetNativePackages()))},
	)
	if boot := recovery.GetBoot(); boot != nil {
		*fields = append(*fields, [2]string{"Boot Recovery", recoveryBootSummary(boot)})
	}
	if asset := recovery.GetRuntimeAsset(); asset != nil {
		*fields = append(*fields, [2]string{"Runtime Asset", recoveryAssetSummary(asset)})
	}
}

func recoveryConfigSummary(status *s4wave_status.LauncherRecoveryStatus) string {
	return "selected rev " + strconv.FormatUint(status.GetSelectedConfigRev(), 10) +
		" from " + status.GetSelectedConfigSource() +
		"; fetched rev " + strconv.FormatUint(status.GetFetchedConfigRev(), 10) +
		" from " + status.GetFetchedConfigSource()
}

func recoveryBootSummary(status *s4wave_status.BrowserBootRecoveryStatus) string {
	if status.GetStatus() != "" && status.GetStatus() != "reported" {
		return status.GetStatus()
	}
	return "version " + status.GetCompatibilityVersion() +
		"; decision " + status.GetLastResetDecision()
}

func recoveryAssetSummary(status *s4wave_status.RuntimeAssetRecoveryStatus) string {
	if status.GetStatus() != "" && status.GetStatus() != "reported" {
		return status.GetStatus()
	}
	return status.GetScriptPath() + " " +
		strconv.FormatUint(uint64(status.GetStatusCode()), 10) +
		" " + status.GetClassification()
}
