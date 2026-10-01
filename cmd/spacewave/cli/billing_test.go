//go:build !js

package spacewave_cli

import (
	"context"
	"testing"

	"github.com/aperturerobotics/cli"
	s4wave_provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestRunBillingUsageTextOutput requests the selected account and explains its bill.
func TestRunBillingUsageTextOutput(t *testing.T) {
	// Supply a cloud Session whose watch records the requested billing account.
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

	// Run the same command action used by the interactive CLI.
	c := cli.NewContext(nil, emptyFlagSet(t), nil)
	c.Context = context.Background()
	out, err := captureStdout(t, func() error {
		return runBillingUsage(c, ".spacewave", "text", 2, "ba-selected")
	})
	if err != nil {
		t.Fatalf("run billing usage: %v", err)
	}

	// Verify the account, accepted prices, exact rates, and current allowances.
	if requestedBA != "ba-selected" {
		t.Fatalf("expected selected billing account, got %q", requestedBA)
	}

	// Check the account, storage, and pricing output fields.
	assertContains(t, out, "Billing Account:")
	assertContains(t, out, "ba-selected")
	assertContains(t, out, "Storage:")
	assertContains(t, out, "110.00 GB / 100.00 GB included")
	assertContains(t, out, "Monthly Price:")
	assertContains(t, out, "$5.00")

	// Check the extra spending limit and usage charge fields.
	assertContains(t, out, "Extra Spending Limit:")
	assertContains(t, out, "$10.00")
	assertContains(t, out, "Extra Usage Charges:")
	assertContains(t, out, "$1.25")
	assertContains(t, out, "Pending Extra Charges:")
	assertContains(t, out, "<$0.01")

	// Check the per-operation rates and billing period.
	assertContains(t, out, "$0.000004 per write")
	assertContains(t, out, "$0.000001 per uncached read")
	assertContains(t, out, "Billing Period:")
	assertContains(t, out, "2026-04-22 22:00 UTC to 2026-05-22 22:00 UTC")

	// Check the operation usage allowances.
	assertContains(t, out, "Write Ops:")
	assertContains(t, out, "250 / 100 included")
	assertContains(t, out, "Read Ops:")
	assertContains(t, out, "900 / 500 included")
	assertContains(t, out, "2026-04-22 22:00 UTC")
}

// TestWriteBillingUsageJSONOutput preserves field names and exact integer encoding.
func TestWriteBillingUsageJSONOutput(t *testing.T) {
	// Capture the machine-readable snapshot.
	out, err := captureStdout(t, func() error {
		return writeBillingUsageOutput(nil, "json", 3, "ba-json", billingUsageResponse().GetUsage())
	})
	if err != nil {
		t.Fatalf("write json: %v", err)
	}

	// Check the current offer and usage contract.
	assertContains(t, out, `"applicable":true`)
	assertContains(t, out, `"sessionIndex":3`)
	assertContains(t, out, `"billingAccountId":"ba-json"`)
	assertContains(t, out, `"storageBytes":118111600640`)
	assertContains(t, out, `"overageLimitCents":1000`)
	assertContains(t, out, `"accruedOverageMicrodollars":"1250000"`)
	assertContains(t, out, `"reservedOverageMicrodollars":"500"`)

	// Check the offer, policy, and period contract fields.
	assertContains(t, out, `"offerVersion":"offer-test"`)
	assertContains(t, out, `"policyVersion":"policy-test"`)
	assertContains(t, out, `"writeMicrodollars":4`)
	assertContains(t, out, `"readMicrodollars":1`)
	assertContains(t, out, `"currentPeriodEnd":"1779487200000"`)
	assertContains(t, out, `"usageMeteredThroughAt":"1776895200000"`)
}

// TestWriteBillingUsageYAMLOutput preserves protobuf integer strings in YAML.
func TestWriteBillingUsageYAMLOutput(t *testing.T) {
	// Capture the converted machine-readable snapshot.
	out, err := captureStdout(t, func() error {
		return writeBillingUsageOutput(nil, "yaml", 4, "ba-yaml", billingUsageResponse().GetUsage())
	})
	if err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	// Verify identity, charges, and operation allowances survive conversion.
	assertContains(t, out, "applicable: true")
	assertContains(t, out, "sessionIndex: 4")
	assertContains(t, out, "billingAccountId: ba-yaml")
	assertContains(t, out, `accruedOverageMicrodollars: "1250000"`)
	assertContains(t, out, "overageLimitCents: 1000")
	assertContains(t, out, `readOpsBaseline: "500"`)
}

// TestRunBillingUsageLocalSessionNotApplicable avoids cloud RPCs for local Sessions.
func TestRunBillingUsageLocalSessionNotApplicable(t *testing.T) {
	// Supply local identity without a cloud billing service.
	restore := stubBillingTestHooks(t)
	defer restore()
	billingMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (billingSessionHandle, error) {
		return &fakeBillingSessionHandle{info: localBillingSessionInfo()}, nil
	}

	// Run the command and preserve the reason billing does not apply.
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

// TestWriteBillingUsageNotApplicableJSON identifies an unavailable cloud bill.
func TestWriteBillingUsageNotApplicableJSON(t *testing.T) {
	// Capture the structured unavailable result.
	out, err := captureStdout(t, func() error {
		return writeBillingUsageNotApplicable(nil, "json", 5, provider_local.ProviderID, "cloud billing unavailable")
	})
	if err != nil {
		t.Fatalf("write json: %v", err)
	}

	// Preserve the provider and reason for machine consumers.
	assertContains(t, out, `"applicable":false`)
	assertContains(t, out, `"providerId":"local"`)
	assertContains(t, out, `"reason":"cloud billing unavailable"`)
}

// stubBillingTestHooks replaces daemon access and returns restoration for the caller.
func stubBillingTestHooks(t *testing.T) func() {
	// Retain the production hooks until this test releases its replacements.
	t.Helper()

	// Retain the production hooks until this test releases its replacements.
	oldResolveStatePath := billingResolveStatePath
	oldConnectDaemon := billingConnectDaemon
	oldCloseClient := billingCloseClient
	oldMountSession := billingMountSession

	// Check state-path routing while keeping the command independent of a daemon.
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

	// Restore all shared hooks before another command test runs.
	return func() {
		billingResolveStatePath = oldResolveStatePath
		billingConnectDaemon = oldConnectDaemon
		billingCloseClient = oldCloseClient
		billingMountSession = oldMountSession
	}
}

// billingUsageResponse covers accepted pricing, pending charges, and metered usage.
func billingUsageResponse() *s4wave_provider_spacewave.WatchBillingStateResponse {
	return &s4wave_provider_spacewave.WatchBillingStateResponse{
		Usage: &s4wave_provider_spacewave.BillingUsageInfo{
			StorageBytes:                110 * billingBytesPerGB,
			StorageBaselineBytes:        100 * billingBytesPerGB,
			WriteOps:                    250,
			WriteOpsBaseline:            100,
			ReadOps:                     900,
			ReadOpsBaseline:             500,
			OverageLimitCents:           1000,
			AccruedOverageMicrodollars:  1250000,
			ReservedOverageMicrodollars: 500,
			CurrentPeriodStart:          1776895200000,
			CurrentPeriodEnd:            1779487200000,
			OfferVersion:                "offer-test",
			PolicyVersion:               "policy-test",
			MonthlyPriceCents:           500,
			WriteMicrodollars:           4,
			ReadMicrodollars:            1,
			UsageMeteredThroughAt:       1776895200000,
		},
	}
}

// spacewaveBillingSessionInfo identifies a Session eligible for cloud billing.
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

// localBillingSessionInfo identifies a Session without cloud billing.
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

// fakeBillingSessionHandle supplies identity and billing results to command tests.
type fakeBillingSessionHandle struct {
	// info supplies the Session's provider identity.
	info *s4wave_session.GetSessionInfoResponse
	// infoErr fails the identity lookup when selected.
	infoErr error
	// svc supplies the billing watch.
	svc billingSpacewaveSessionService
	// svcErr fails access to the billing service when selected.
	svcErr error
}

// Release satisfies the mount contract without owning external resources.
func (s *fakeBillingSessionHandle) Release() {}

// GetSessionInfo returns the selected provider identity or lookup failure.
func (s *fakeBillingSessionHandle) GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	return s.info, nil
}

// AccessSpacewaveSession returns the selected cloud service or access failure.
func (s *fakeBillingSessionHandle) AccessSpacewaveSession() (billingSpacewaveSessionService, error) {
	if s.svcErr != nil {
		return nil, s.svcErr
	}
	return s.svc, nil
}

// fakeBillingSpacewaveSessionService records requests and supplies one snapshot.
type fakeBillingSpacewaveSessionService struct {
	// resp supplies the initial billing snapshot.
	resp *s4wave_provider_spacewave.WatchBillingStateResponse
	// err fails opening the watch when selected.
	err error
	// captureBillingAccountID observes the requested account selection.
	captureBillingAccountID func(string)
}

// WatchBillingState records account selection before returning its configured result.
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

// fakeBillingStateStream provides the one snapshot read by the command.
type fakeBillingStateStream struct {
	// resp is the selected billing snapshot.
	resp *s4wave_provider_spacewave.WatchBillingStateResponse
}

// Recv returns the configured billing snapshot without starting a watch.
func (s *fakeBillingStateStream) Recv() (*s4wave_provider_spacewave.WatchBillingStateResponse, error) {
	return s.resp, nil
}

// _ verifies the command test adapters' billing contracts.
var (
	_ billingSessionHandle           = (*fakeBillingSessionHandle)(nil)
	_ billingSpacewaveSessionService = (*fakeBillingSpacewaveSessionService)(nil)
	_ billingStateStream             = (*fakeBillingStateStream)(nil)
)
