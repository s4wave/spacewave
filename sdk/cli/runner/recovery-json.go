package runner

import (
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

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
