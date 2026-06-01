package resource_session

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/core/provider"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	"github.com/sirupsen/logrus"
)

func TestReportRecoveryStatusPublishesRendererFacts(t *testing.T) {
	b := inmem.NewBus(cdc.NewController(context.Background(), logrus.NewEntry(logrus.New())))
	statusRes := NewStatusResource(b, nil)

	initial := statusRes.buildRecoveryStatus()
	if initial.GetBoot().GetStatus() != "not-reported" {
		t.Fatalf("initial boot status = %q, want not-reported", initial.GetBoot().GetStatus())
	}
	if initial.GetRuntimeAsset().GetStatus() != "not-reported" {
		t.Fatalf("initial asset status = %q, want not-reported", initial.GetRuntimeAsset().GetStatus())
	}

	_, err := statusRes.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "reset-complete",
		},
		RuntimeAsset: &s4wave_status.RuntimeAssetRecoveryStatus{
			ScriptPath:     "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs",
			StatusCode:     404,
			Classification: "missing",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	status := statusRes.buildRecoveryStatus()
	if status.GetBoot().GetCompatibilityVersion() != "1000000" ||
		status.GetBoot().GetLastResetDecision() != "reset-complete" ||
		status.GetBoot().GetStatus() != "reported" {
		t.Fatalf("unexpected boot recovery status: %#v", status.GetBoot())
	}
	if status.GetRuntimeAsset().GetScriptPath() != "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs" ||
		status.GetRuntimeAsset().GetStatusCode() != 404 ||
		status.GetRuntimeAsset().GetClassification() != "missing" ||
		status.GetRuntimeAsset().GetStatus() != "reported" {
		t.Fatalf("unexpected runtime asset recovery status: %#v", status.GetRuntimeAsset())
	}
}

func TestRecoveryStatusRegistrySharesRendererFactsAcrossResources(t *testing.T) {
	b := inmem.NewBus(cdc.NewController(context.Background(), logrus.NewEntry(logrus.New())))
	registry := NewRecoveryStatusRegistry()
	ref := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "session-1",
			ProviderId:        "local",
			ProviderAccountId: "account-1",
		},
	}
	publisher := NewStatusResource(b, registry.getSessionRecoveryStatusCtrForRef(ref))
	reader := NewStatusResource(b, registry.getSessionRecoveryStatusCtrForRef(ref))

	_, err := publisher.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "reset-complete",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	status := reader.buildRecoveryStatus()
	if status.GetBoot().GetCompatibilityVersion() != "1000000" ||
		status.GetBoot().GetLastResetDecision() != "reset-complete" ||
		status.GetBoot().GetStatus() != "reported" {
		t.Fatalf("reader did not see shared renderer status: %#v", status.GetBoot())
	}
}

func TestReportRecoveryStatusReplacesRendererSnapshot(t *testing.T) {
	b := inmem.NewBus(cdc.NewController(context.Background(), logrus.NewEntry(logrus.New())))
	statusRes := NewStatusResource(b, nil)

	_, err := statusRes.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "reset-complete",
		},
		RuntimeAsset: &s4wave_status.RuntimeAssetRecoveryStatus{
			ScriptPath:     "/b/pa/spacewave-app/v/b/fe/app/App-Cod_FQyf.mjs",
			StatusCode:     404,
			Classification: "missing",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = statusRes.ReportRecoveryStatus(context.Background(), &s4wave_status.ReportRecoveryStatusRequest{
		Boot: &s4wave_status.BrowserBootRecoveryStatus{
			CompatibilityVersion: "1000000",
			LastResetDecision:    "current",
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	status := statusRes.buildRecoveryStatus()
	if status.GetBoot().GetLastResetDecision() != "current" {
		t.Fatalf("boot decision = %q, want current", status.GetBoot().GetLastResetDecision())
	}
	if status.GetRuntimeAsset().GetStatus() != "not-reported" ||
		status.GetRuntimeAsset().GetScriptPath() != "" {
		t.Fatalf("runtime asset status was not cleared: %#v", status.GetRuntimeAsset())
	}
}

func TestBuildLauncherRecoveryStatusIncludesEntrypointFacts(t *testing.T) {
	status := buildLauncherRecoveryStatus(
		&spacewave_launcher.LauncherInfo{
			DistConfig: &spacewave_launcher.DistConfig{
				ProjectId:  "spacewave",
				Rev:        42,
				ChannelKey: "staging",
			},
			UpdateState: &spacewave_launcher.UpdateState{
				Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
				Version:    "0.2.0",
				StagedPath: "/var/folders/spacewave/Spacewave.app",
			},
		},
		&spacewave_launcher.FetchStatus{
			SelectedConfigRev:             42,
			SelectedConfigSource:          "endpoint:https://release.example",
			FetchedConfigRev:              43,
			FetchedConfigSource:           "endpoint:https://release.example",
			ReleaseMetadataOutcome:        "staged",
			ReleaseWorldHeadRef:           "release-world-head",
			SelectedEntrypointManifestID:  "spacewave-dist",
			SelectedEntrypointPlatformID:  "desktop/darwin/arm64",
			SelectedEntrypointManifestRev: 7,
			SelectedEntrypointManifestRef: "manifest-ref",
		},
	)

	if status.GetSelectedChannelKey() != "staging" ||
		status.GetSelectedConfigRev() != 42 ||
		status.GetReleaseMetadataOutcome() != "staged" ||
		status.GetReleaseWorldHeadRef() != "release-world-head" {
		t.Fatalf("unexpected release status: %#v", status)
	}
	if status.GetSelectedEntrypointManifestId() != "spacewave-dist" ||
		status.GetSelectedEntrypointPlatformId() != "desktop/darwin/arm64" ||
		status.GetSelectedEntrypointManifestRev() != 7 ||
		status.GetSelectedEntrypointManifestRef() != "manifest-ref" {
		t.Fatalf("unexpected selected entrypoint status: %#v", status)
	}
	if status.GetUpdatePhase() != "staged" ||
		status.GetUpdateVersion() != "0.2.0" ||
		status.GetStagedPath() != "/var/folders/spacewave/Spacewave.app" {
		t.Fatalf("unexpected update status: %#v", status)
	}
}

func TestRecoveryStatusKeepsEntrypointAndPluginFactsSeparate(t *testing.T) {
	status := &s4wave_status.RecoveryStatus{
		Launcher: buildLauncherRecoveryStatus(
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        42,
					ChannelKey: "stable",
				},
				UpdateState: &spacewave_launcher.UpdateState{
					Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
					StagedPath: "/tmp/Spacewave.app",
				},
			},
			&spacewave_launcher.FetchStatus{
				SelectedEntrypointManifestID:  "spacewave-dist",
				SelectedEntrypointPlatformID:  "desktop/darwin/arm64",
				SelectedEntrypointManifestRef: "entrypoint-ref",
			},
		),
		Plugins: []*s4wave_status.PluginManifestRecoveryStatus{{
			PluginId:            "spacewave-app",
			ExecuteManifestRef:  "plugin-exec-ref",
			DownloadManifestRef: "plugin-download-ref",
		}},
		NativePackages: []*s4wave_status.NativePackageRecoveryStatus{{
			PluginId:     "spacewave-app",
			DistDir:      "/tmp/p/d/spacewave-app",
			Materialized: true,
			LastAction:   "materialized",
		}},
	}

	if status.GetLauncher().GetSelectedEntrypointManifestRef() != "entrypoint-ref" {
		t.Fatalf("launcher entrypoint ref missing: %#v", status.GetLauncher())
	}
	if status.GetPlugins()[0].GetExecuteManifestRef() != "plugin-exec-ref" ||
		status.GetNativePackages()[0].GetDistDir() != "/tmp/p/d/spacewave-app" {
		t.Fatalf("plugin-owned status missing: %#v", status)
	}
	if status.GetLauncher().GetSelectedEntrypointManifestRef() == status.GetPlugins()[0].GetExecuteManifestRef() {
		t.Fatalf("entrypoint and plugin refs were conflated: %#v", status)
	}
}
