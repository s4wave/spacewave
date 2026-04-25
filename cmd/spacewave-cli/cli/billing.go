//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

const billingBytesPerGB = 1024 * 1024 * 1024

type billingSessionHandle interface {
	Release()
	GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error)
	AccessSpacewaveSession() (billingSpacewaveSessionService, error)
}

type billingSpacewaveSessionService interface {
	WatchBillingState(
		context.Context,
		*s4wave_provider_spacewave.WatchBillingStateRequest,
	) (billingStateStream, error)
}

type billingStateStream interface {
	Recv() (*s4wave_provider_spacewave.WatchBillingStateResponse, error)
}

type billingSpacewaveSessionClient struct {
	client s4wave_session.SRPCSpacewaveSessionResourceServiceClient
}

type mountedBillingSession struct {
	session *s4wave_session.Session
}

func (s *mountedBillingSession) Release() {
	s.session.Release()
}

func (s *mountedBillingSession) GetSessionInfo(ctx context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	return s.session.GetSessionInfo(ctx)
}

func (s *mountedBillingSession) AccessSpacewaveSession() (billingSpacewaveSessionService, error) {
	client, err := s.session.GetResourceRef().GetClient()
	if err != nil {
		return nil, errors.Wrap(err, "session client")
	}
	return &billingSpacewaveSessionClient{
		client: s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(client),
	}, nil
}

func (c *billingSpacewaveSessionClient) WatchBillingState(
	ctx context.Context,
	req *s4wave_provider_spacewave.WatchBillingStateRequest,
) (billingStateStream, error) {
	return c.client.WatchBillingState(ctx, req)
}

var (
	billingResolveStatePath = resolveStatePathFromContext
	billingConnectDaemon    = connectDaemon
	billingCloseClient      = func(client *sdkClient) { client.close() }
	billingMountSession     = func(ctx context.Context, client *sdkClient, idx uint32) (billingSessionHandle, error) {
		sess, err := client.mountSession(ctx, idx)
		if err != nil {
			return nil, err
		}
		return &mountedBillingSession{session: sess}, nil
	}
)

// newBillingCommand builds the billing command group.
func newBillingCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "billing",
		Usage: "inspect billing and usage",
		Subcommands: []*cli.Command{
			newBillingUsageCommand(),
		},
	}
}

// newBillingUsageCommand builds the billing usage command.
func newBillingUsageCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var billingAccountID string
	return &cli.Command{
		Name:  "usage",
		Usage: "show current billing usage",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:        "billing-account-id",
				Usage:       "billing account id to inspect",
				Destination: &billingAccountID,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output format (text/json/yaml)",
				EnvVars: []string{"SPACEWAVE_OUTPUT"},
				Value:   "text",
			},
		),
		Action: func(c *cli.Context) error {
			return runBillingUsage(c, statePath, c.String("output"), uint32(sessionIdx), billingAccountID)
		},
	}
}

// runBillingUsage implements the billing usage command.
func runBillingUsage(
	c *cli.Context,
	statePath string,
	outputFormat string,
	sessionIdx uint32,
	billingAccountID string,
) error {
	ctx := c.Context
	resolved, err := billingResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := billingConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer billingCloseClient(client)

	sess, err := billingMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	ref := info.GetSessionRef().GetProviderResourceRef()
	if ref.GetProviderId() == provider_local.ProviderID {
		return writeBillingUsageNotApplicable(os.Stdout, outputFormat, sessionIdx, ref.GetProviderId(), "billing usage is only available for Spacewave cloud sessions")
	}

	svc, err := sess.AccessSpacewaveSession()
	if err != nil {
		return err
	}
	strm, err := svc.WatchBillingState(ctx, &s4wave_provider_spacewave.WatchBillingStateRequest{
		BillingAccountId: billingAccountID,
	})
	if err != nil {
		return errors.Wrap(err, "watch billing state")
	}
	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "receive billing state")
	}

	return writeBillingUsageOutput(os.Stdout, outputFormat, sessionIdx, billingAccountID, resp.GetUsage())
}

func writeBillingUsageOutput(
	w io.Writer,
	outputFormat string,
	sessionIdx uint32,
	billingAccountID string,
	usage *s4wave_provider_spacewave.BillingUsageInfo,
) error {
	if usage == nil {
		usage = &s4wave_provider_spacewave.BillingUsageInfo{}
	}
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("applicable")
		ms.WriteBool(true)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionIndex")
		ms.WriteUint32(sessionIdx)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("billingAccountId")
		ms.WriteString(billingAccountID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageBytes")
		ms.WriteFloat64(usage.GetStorageBytes())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageBaselineBytes")
		ms.WriteFloat64(usage.GetStorageBaselineBytes())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageOverageBytes")
		ms.WriteFloat64(usage.GetStorageOverageBytes())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageOverageMonthlyCostEstimateUsd")
		ms.WriteFloat64(usage.GetStorageOverageMonthlyCostEstimateUsd())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageOverageMonthToDateGbMonths")
		ms.WriteFloat64(usage.GetStorageOverageMonthToDateGbMonths())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageOverageMonthToDateCostEstimateUsd")
		ms.WriteFloat64(usage.GetStorageOverageMonthToDateCostEstimateUsd())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageOverageDeletedGbMonths")
		ms.WriteFloat64(usage.GetStorageOverageDeletedGbMonths())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("storageOverageDeletedCostEstimateUsd")
		ms.WriteFloat64(usage.GetStorageOverageDeletedCostEstimateUsd())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("usageMeteredThroughAt")
		ms.WriteInt64(usage.GetUsageMeteredThroughAt())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("writeOps")
		ms.WriteInt64(usage.GetWriteOps())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("writeOpsBaseline")
		ms.WriteInt64(usage.GetWriteOpsBaseline())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("readOps")
		ms.WriteInt64(usage.GetReadOps())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("readOpsBaseline")
		ms.WriteInt64(usage.GetReadOpsBaseline())
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	if outputFormat != "text" && outputFormat != "table" {
		return formatOutput(nil, outputFormat)
	}

	baLabel := "default"
	if billingAccountID != "" {
		baLabel = billingAccountID
	}
	fields := [][2]string{
		{"Billing Account", baLabel},
		{"Storage", billingFormatBytes(usage.GetStorageBytes()) + " / " + billingFormatBytes(usage.GetStorageBaselineBytes()) + " included"},
		{"Extra Storage", billingFormatBytes(usage.GetStorageOverageBytes()) + " = " + billingFormatCurrency(usage.GetStorageOverageMonthlyCostEstimateUsd()) + "/mo if held"},
		{"Month-to-date overage", billingFormatGBMonths(usage.GetStorageOverageMonthToDateGbMonths()) + " = " + billingFormatCurrency(usage.GetStorageOverageMonthToDateCostEstimateUsd()) + " estimated"},
	}
	if usage.GetStorageOverageDeletedGbMonths() > 0 || usage.GetStorageOverageDeletedCostEstimateUsd() > 0 {
		fields = append(fields, [2]string{"Already-deleted data", billingFormatGBMonths(usage.GetStorageOverageDeletedGbMonths()) + " = +" + billingFormatCurrency(usage.GetStorageOverageDeletedCostEstimateUsd()) + " estimated"})
	}
	fields = append(fields,
		[2]string{"Write Ops", strconv.FormatInt(usage.GetWriteOps(), 10) + " / " + strconv.FormatInt(usage.GetWriteOpsBaseline(), 10) + " included"},
		[2]string{"Read Ops", strconv.FormatInt(usage.GetReadOps(), 10) + " / " + strconv.FormatInt(usage.GetReadOpsBaseline(), 10) + " included"},
		[2]string{"Metered Through", billingFormatTimestamp(usage.GetUsageMeteredThroughAt())},
	)
	writeFields(w, fields)
	return nil
}

func writeBillingUsageNotApplicable(
	w io.Writer,
	outputFormat string,
	sessionIdx uint32,
	providerID string,
	reason string,
) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("applicable")
		ms.WriteBool(false)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionIndex")
		ms.WriteUint32(sessionIdx)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(providerID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("reason")
		ms.WriteString(reason)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	if outputFormat != "text" && outputFormat != "table" {
		return formatOutput(nil, outputFormat)
	}

	writeFields(w, [][2]string{
		{"Billing Usage", "not applicable"},
		{"Provider", providerID},
		{"Reason", reason},
	})
	return nil
}

func billingFormatBytes(bytes float64) string {
	return strconv.FormatFloat(bytes/billingBytesPerGB, 'f', 2, 64) + " GB"
}

func billingFormatGBMonths(gbMonths float64) string {
	return strconv.FormatFloat(gbMonths, 'f', 6, 64) + " GB-months"
}

func billingFormatCurrency(amount float64) string {
	if amount > 0 && amount < 0.01 {
		return "<$0.01"
	}
	return "$" + strconv.FormatFloat(amount, 'f', 2, 64)
}

func billingFormatTimestamp(ms int64) string {
	if ms <= 0 {
		return "not yet metered"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04 UTC")
}

// _ is a type assertion
var _ billingSessionHandle = (*mountedBillingSession)(nil)
