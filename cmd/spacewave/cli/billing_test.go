//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
	s4wave_provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

func TestRunBillingUsageTextOutput(t *testing.T) {
	restore := stubBillingTestHooks(t)
	defer restore()

	var requestedBA string
	billingMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (billingSessionHandle, error) {
		if idx != 2 {
			t.Fatalf("unexpected session index: %d", idx)
		}
		return &fakeBillingSessionHandle{
			info: spacewaveBillingSessionInfo(),
			svc: &fakeBillingSpacewaveSessionService{
				resp: billingUsageResponse(),
				captureBillingAccountID: func(baID string) {
					requestedBA = baID
				},
			},
		}, nil
	}

	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()
	out, err := captureStdout(t, func() error {
		return runBillingUsage(c, ".spacewave", "text", 2, "ba-selected")
	})
	if err != nil {
		t.Fatalf("run billing usage: %v", err)
	}

	if requestedBA != "ba-selected" {
		t.Fatalf("expected selected billing account, got %q", requestedBA)
	}
	assertContains(t, out, "Billing Account:")
	assertContains(t, out, "ba-selected")
	assertContains(t, out, "Storage:")
	assertContains(t, out, "110.00 GB / 100.00 GB included")
	assertContains(t, out, "Extra Storage:")
	assertContains(t, out, "$0.20/mo if held")
	assertContains(t, out, "Month-to-date overage:")
	assertContains(t, out, "0.023871 GB-months = <$0.01 estimated")
	assertContains(t, out, "Already-deleted data:")
	assertContains(t, out, "0.005000 GB-months = +<$0.01 estimated")
	assertContains(t, out, "Write Ops:")
	assertContains(t, out, "250 / 100 included")
	assertContains(t, out, "Read Ops:")
	assertContains(t, out, "900 / 500 included")
	assertContains(t, out, "2026-04-22 22:00 UTC")
}

func TestWriteBillingUsageJSONOutput(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return writeBillingUsageOutput(nil, "json", 3, "ba-json", billingUsageResponse().GetUsage())
	})
	if err != nil {
		t.Fatalf("write json: %v", err)
	}

	assertContains(t, out, `"applicable":true`)
	assertContains(t, out, `"sessionIndex":3`)
	assertContains(t, out, `"billingAccountId":"ba-json"`)
	assertContains(t, out, `"storageBytes":118111600640`)
	assertContains(t, out, `"storageOverageMonthToDateGbMonths":0.023871`)
	assertContains(t, out, `"storageOverageDeletedCostEstimateUsd":0.0001`)
	assertContains(t, out, `"usageMeteredThroughAt":"1776895200000"`)
}

func TestWriteBillingUsageYAMLOutput(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return writeBillingUsageOutput(nil, "yaml", 4, "ba-yaml", billingUsageResponse().GetUsage())
	})
	if err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	assertContains(t, out, "applicable: true")
	assertContains(t, out, "sessionIndex: 4")
	assertContains(t, out, "billingAccountId: ba-yaml")
	assertContains(t, out, "storageOverageBytes: 10737418240")
	assertContains(t, out, `readOpsBaseline: "500"`)
}

func TestRunBillingUsageLocalSessionNotApplicable(t *testing.T) {
	restore := stubBillingTestHooks(t)
	defer restore()

	billingMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (billingSessionHandle, error) {
		return &fakeBillingSessionHandle{info: localBillingSessionInfo()}, nil
	}

	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()
	out, err := captureStdout(t, func() error {
		return runBillingUsage(c, ".spacewave", "text", 1, "")
	})
	if err != nil {
		t.Fatalf("run billing usage: %v", err)
	}

	assertContains(t, out, "Billing Usage:")
	assertContains(t, out, "not applicable")
	assertContains(t, out, "billing usage is only available for Spacewave cloud sessions")
}

func TestWriteBillingUsageTextHidesDeletedDataWhenZero(t *testing.T) {
	usage := billingUsageResponse().GetUsage()
	usage.StorageOverageDeletedGbMonths = 0
	usage.StorageOverageDeletedCostEstimateUsd = 0

	out, err := captureStdout(t, func() error {
		return writeBillingUsageOutput(os.Stdout, "text", 1, "", usage)
	})
	if err != nil {
		t.Fatalf("write text: %v", err)
	}
	if strings.Contains(out, "Already-deleted data") {
		t.Fatalf("expected deleted-data line to be hidden\n%s", out)
	}
}

func TestWriteBillingUsageNotApplicableJSON(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return writeBillingUsageNotApplicable(nil, "json", 5, provider_local.ProviderID, "cloud billing unavailable")
	})
	if err != nil {
		t.Fatalf("write json: %v", err)
	}

	assertContains(t, out, `"applicable":false`)
	assertContains(t, out, `"providerId":"local"`)
	assertContains(t, out, `"reason":"cloud billing unavailable"`)
}

func stubBillingTestHooks(t *testing.T) func() {
	t.Helper()

	oldResolveStatePath := billingResolveStatePath
	oldConnectDaemon := billingConnectDaemon
	oldCloseClient := billingCloseClient
	oldMountSession := billingMountSession

	billingResolveStatePath = func(_ *cli.Context, statePath string) (string, error) {
		if statePath != ".spacewave" {
			t.Fatalf("unexpected state path: %s", statePath)
		}
		return "/tmp/state", nil
	}
	billingConnectDaemon = func(ctx context.Context, statePath string) (*sdkClient, error) {
		if statePath != "/tmp/state" {
			t.Fatalf("unexpected resolved state path: %s", statePath)
		}
		return &sdkClient{}, nil
	}
	billingCloseClient = func(*sdkClient) {}
	billingMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (billingSessionHandle, error) {
		t.Fatal("billingMountSession not stubbed")
		return nil, nil
	}

	return func() {
		billingResolveStatePath = oldResolveStatePath
		billingConnectDaemon = oldConnectDaemon
		billingCloseClient = oldCloseClient
		billingMountSession = oldMountSession
	}
}

func billingUsageResponse() *s4wave_provider_spacewave.WatchBillingStateResponse {
	return &s4wave_provider_spacewave.WatchBillingStateResponse{
		Usage: &s4wave_provider_spacewave.BillingUsageInfo{
			StorageBytes:                             110 * billingBytesPerGB,
			StorageBaselineBytes:                     100 * billingBytesPerGB,
			WriteOps:                                 250,
			WriteOpsBaseline:                         100,
			ReadOps:                                  900,
			ReadOpsBaseline:                          500,
			StorageOverageBytes:                      10 * billingBytesPerGB,
			StorageOverageMonthlyCostEstimateUsd:     0.2,
			StorageOverageMonthToDateGbMonths:        0.023871,
			StorageOverageMonthToDateCostEstimateUsd: 0.00047742,
			StorageOverageDeletedGbMonths:            0.005,
			StorageOverageDeletedCostEstimateUsd:     0.0001,
			UsageMeteredThroughAt:                    1776895200000,
		},
	}
}

func spacewaveBillingSessionInfo() *s4wave_session.GetSessionInfoResponse {
	return &s4wave_session.GetSessionInfoResponse{
		SessionRef: &session_pb.SessionRef{
			ProviderResourceRef: &s4wave_provider.ProviderResourceRef{
				ProviderId:        "spacewave",
				ProviderAccountId: "cloud-account",
				Id:                "cloud-session",
			},
		},
	}
}

func localBillingSessionInfo() *s4wave_session.GetSessionInfoResponse {
	return &s4wave_session.GetSessionInfoResponse{
		SessionRef: &session_pb.SessionRef{
			ProviderResourceRef: &s4wave_provider.ProviderResourceRef{
				ProviderId:        provider_local.ProviderID,
				ProviderAccountId: "local-account",
				Id:                "local-session",
			},
		},
	}
}

type fakeBillingSessionHandle struct {
	info    *s4wave_session.GetSessionInfoResponse
	infoErr error
	svc     billingSpacewaveSessionService
	svcErr  error
}

func (s *fakeBillingSessionHandle) Release() {}

func (s *fakeBillingSessionHandle) GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	return s.info, nil
}

func (s *fakeBillingSessionHandle) AccessSpacewaveSession() (billingSpacewaveSessionService, error) {
	if s.svcErr != nil {
		return nil, s.svcErr
	}
	return s.svc, nil
}

type fakeBillingSpacewaveSessionService struct {
	resp                    *s4wave_provider_spacewave.WatchBillingStateResponse
	err                     error
	captureBillingAccountID func(string)
}

func (s *fakeBillingSpacewaveSessionService) WatchBillingState(
	ctx context.Context,
	req *s4wave_provider_spacewave.WatchBillingStateRequest,
) (billingStateStream, error) {
	if s.captureBillingAccountID != nil {
		s.captureBillingAccountID(req.GetBillingAccountId())
	}
	if s.err != nil {
		return nil, s.err
	}
	return &fakeBillingStateStream{resp: s.resp}, nil
}

type fakeBillingStateStream struct {
	resp *s4wave_provider_spacewave.WatchBillingStateResponse
}

func (s *fakeBillingStateStream) Recv() (*s4wave_provider_spacewave.WatchBillingStateResponse, error) {
	return s.resp, nil
}

// _ is a type assertion
var (
	_ billingSessionHandle           = (*fakeBillingSessionHandle)(nil)
	_ billingSpacewaveSessionService = (*fakeBillingSpacewaveSessionService)(nil)
	_ billingStateStream             = (*fakeBillingStateStream)(nil)
)
