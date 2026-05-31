//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
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
	recovery, recoveryErr := watchRecoveryStatus(ctx, sess)

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
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("recovery")
		writeRecoveryStatusJSON(ms, recovery, recoveryErr)
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
	appendRecoveryStatusFields(&fields, recovery, recoveryErr)
	writeFields(os.Stdout, fields)
	return nil
}

func watchRecoveryStatus(ctx context.Context, sess *s4wave_session.Session) (*s4wave_status.RecoveryStatus, error) {
	client, err := sess.GetResourceRef().GetClient()
	if err != nil {
		return nil, err
	}
	strm, err := s4wave_status.NewSRPCSystemStatusServiceClient(client).WatchRecoveryStatus(
		ctx,
		&s4wave_status.WatchRecoveryStatusRequest{},
	)
	if err != nil {
		return nil, err
	}
	defer strm.Close()
	resp, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	return resp.GetStatus(), nil
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

func writeRecoveryStatusJSON(
	ms *protojson.MarshalState,
	recovery *s4wave_status.RecoveryStatus,
	recoveryErr error,
) {
	ms.WriteObjectStart()
	var f bool
	if recoveryErr != nil {
		writeJSONStringField(ms, &f, "error", recoveryErr.Error())
		ms.WriteObjectEnd()
		return
	}
	if recovery == nil {
		writeJSONStringField(ms, &f, "status", "unavailable")
		ms.WriteObjectEnd()
		return
	}
	if launcher := recovery.GetLauncher(); launcher != nil {
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("launcher")
		writeLauncherRecoveryJSON(ms, launcher)
	}
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("plugins")
	ms.WriteArrayStart()
	for i, plugin := range recovery.GetPlugins() {
		if i != 0 {
			ms.WriteMore()
		}
		writePluginRecoveryJSON(ms, plugin)
	}
	ms.WriteArrayEnd()
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("nativePackages")
	ms.WriteArrayStart()
	for i, pkg := range recovery.GetNativePackages() {
		if i != 0 {
			ms.WriteMore()
		}
		writeNativePackageRecoveryJSON(ms, pkg)
	}
	ms.WriteArrayEnd()
	if boot := recovery.GetBoot(); boot != nil {
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("boot")
		ms.WriteObjectStart()
		var bf bool
		writeJSONStringField(ms, &bf, "compatibilityVersion", boot.GetCompatibilityVersion())
		writeJSONStringField(ms, &bf, "lastResetDecision", boot.GetLastResetDecision())
		writeJSONStringField(ms, &bf, "status", boot.GetStatus())
		ms.WriteObjectEnd()
	}
	if asset := recovery.GetRuntimeAsset(); asset != nil {
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("runtimeAsset")
		ms.WriteObjectStart()
		var af bool
		writeJSONStringField(ms, &af, "scriptPath", asset.GetScriptPath())
		writeJSONUint64Field(ms, &af, "statusCode", uint64(asset.GetStatusCode()))
		writeJSONBoolField(ms, &af, "ok", asset.GetOk())
		writeJSONStringField(ms, &af, "classification", asset.GetClassification())
		writeJSONStringField(ms, &af, "fetchSource", asset.GetFetchSource())
		writeJSONStringField(ms, &af, "runtimeError", asset.GetRuntimeError())
		writeJSONStringField(ms, &af, "pluginAssetResult", asset.GetPluginAssetResult())
		writeJSONStringField(ms, &af, "contentType", asset.GetContentType())
		writeJSONStringField(ms, &af, "bodyPrefix", asset.GetBodyPrefix())
		writeJSONStringField(ms, &af, "status", asset.GetStatus())
		ms.WriteObjectEnd()
	}
	ms.WriteObjectEnd()
}

func writeLauncherRecoveryJSON(ms *protojson.MarshalState, launcher *s4wave_status.LauncherRecoveryStatus) {
	ms.WriteObjectStart()
	var f bool
	writeJSONUint64Field(ms, &f, "selectedConfigRev", launcher.GetSelectedConfigRev())
	writeJSONStringField(ms, &f, "selectedConfigSource", launcher.GetSelectedConfigSource())
	writeJSONUint64Field(ms, &f, "fetchedConfigRev", launcher.GetFetchedConfigRev())
	writeJSONStringField(ms, &f, "fetchedConfigSource", launcher.GetFetchedConfigSource())
	writeJSONStringField(ms, &f, "releaseMetadataOutcome", launcher.GetReleaseMetadataOutcome())
	ms.WriteObjectEnd()
}

func writePluginRecoveryJSON(ms *protojson.MarshalState, plugin *s4wave_status.PluginManifestRecoveryStatus) {
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "pluginId", plugin.GetPluginId())
	writeJSONStringField(ms, &f, "instanceKey", plugin.GetInstanceKey())
	writeJSONStringField(ms, &f, "executeManifestRef", plugin.GetExecuteManifestRef())
	writeJSONStringField(ms, &f, "downloadManifestRef", plugin.GetDownloadManifestRef())
	writeJSONUint64Field(ms, &f, "skippedCandidateCount", uint64(plugin.GetSkippedCandidateCount()))
	writeJSONStringField(ms, &f, "skippedCandidateSummary", plugin.GetSkippedCandidateSummary())
	writeJSONUint64Field(ms, &f, "ignoredCandidateCount", uint64(plugin.GetIgnoredCandidateCount()))
	writeJSONStringField(ms, &f, "ignoredCandidateSummary", plugin.GetIgnoredCandidateSummary())
	writeJSONUint64Field(ms, &f, "quarantinedCandidateCount", uint64(plugin.GetQuarantinedCandidateCount()))
	writeJSONStringField(ms, &f, "quarantinedCandidateSummary", plugin.GetQuarantinedCandidateSummary())
	ms.WriteObjectEnd()
}

func writeNativePackageRecoveryJSON(ms *protojson.MarshalState, pkg *s4wave_status.NativePackageRecoveryStatus) {
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "pluginId", pkg.GetPluginId())
	writeJSONStringField(ms, &f, "distDir", pkg.GetDistDir())
	writeJSONBoolField(ms, &f, "materialized", pkg.GetMaterialized())
	writeJSONBoolField(ms, &f, "invalidated", pkg.GetInvalidated())
	writeJSONStringField(ms, &f, "lastAction", pkg.GetLastAction())
	writeJSONStringField(ms, &f, "lastError", pkg.GetLastError())
	writeJSONStringField(ms, &f, "updatedAt", pkg.GetUpdatedAt())
	ms.WriteObjectEnd()
}
