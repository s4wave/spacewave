package resource_session

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/core/provider"
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
