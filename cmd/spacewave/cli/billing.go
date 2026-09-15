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

// billingBytesPerGB preserves the CLI's binary storage-unit conversion.
const billingBytesPerGB = 1024 * 1024 * 1024

// billingSessionHandle retains the Session needed for one billing snapshot.
type billingSessionHandle interface {
	// Release releases the mounted Session and its resources.
	Release()
	// GetSessionInfo identifies the Session's provider before querying billing.
	GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error)
	// AccessSpacewaveSession returns the Session's cloud billing client.
	AccessSpacewaveSession() (billingSpacewaveSessionService, error)
}

// billingSpacewaveSessionService opens the selected account's billing watch.
type billingSpacewaveSessionService interface {
	// WatchBillingState watches billing until its context or Session is released.
	WatchBillingState(
		context.Context,
		*s4wave_provider_spacewave.WatchBillingStateRequest,
	) (billingStateStream, error)
}

// billingStateStream receives the initial snapshot used by the usage command.
type billingStateStream interface {
	// Recv waits for billing state or a terminal stream error.
	Recv() (*s4wave_provider_spacewave.WatchBillingStateResponse, error)
}

// billingSpacewaveSessionClient adapts the generated cloud Session client.
type billingSpacewaveSessionClient struct {
	// client shares the mounted Session's resource lifetime.
	client s4wave_session.SRPCSpacewaveSessionResourceServiceClient
}

// mountedBillingSession owns the command's mounted Session reference.
type mountedBillingSession struct {
	// session supplies provider identity and cloud resource access.
	session *s4wave_session.Session
}

// Release releases the command's mounted Session reference.
func (s *mountedBillingSession) Release() {
	s.session.Release()
}

// GetSessionInfo returns the mounted Session's provider identity.
func (s *mountedBillingSession) GetSessionInfo(ctx context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	return s.session.GetSessionInfo(ctx)
}

// AccessSpacewaveSession binds billing RPCs to the retained Session resource.
func (s *mountedBillingSession) AccessSpacewaveSession() (billingSpacewaveSessionService, error) {
	client, err := s.session.GetResourceRef().GetClient()
	if err != nil {
		return nil, errors.Wrap(err, "session client")
	}
	return &billingSpacewaveSessionClient{
		client: s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(client),
	}, nil
}

// WatchBillingState forwards the selected account's billing subscription.
func (c *billingSpacewaveSessionClient) WatchBillingState(
	ctx context.Context,
	req *s4wave_provider_spacewave.WatchBillingStateRequest,
) (billingStateStream, error) {
	return c.client.WatchBillingState(ctx, req)
}

var (
	// billingResolveStatePath resolves the daemon's persisted configuration.
	billingResolveStatePath = resolveStatePathFromContext
	// billingConnectDaemon opens the command's daemon connection.
	billingConnectDaemon = connectDaemon
	// billingCloseClient releases the command's daemon connection.
	billingCloseClient = func(client *sdkClient) { client.close() }
	// billingMountSession retains the requested Session for the command.
	billingMountSession = func(ctx context.Context, client *sdkClient, idx uint32) (billingSessionHandle, error) {
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
			return runBillingUsage(c, statePath, c.String("output"), sessionIndex32(sessionIdx), billingAccountID)
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
	// Retain the configured daemon and Session for the whole request.
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

	// Local Sessions have no cloud billing account to inspect.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	ref := info.GetSessionRef().GetProviderResourceRef()
	if ref.GetProviderId() == provider_local.ProviderID {
		return writeBillingUsageNotApplicable(os.Stdout, outputFormat, sessionIdx, ref.GetProviderId(), "billing usage is only available for Spacewave cloud sessions")
	}

	// Read one authoritative snapshot through the Session's billing watch.
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

// writeBillingUsageOutput renders the current offer and metered operation usage.
func writeBillingUsageOutput(
	w io.Writer,
	outputFormat string,
	sessionIdx uint32,
	billingAccountID string,
	usage *s4wave_provider_spacewave.BillingUsageInfo,
) error {
	// Keep machine output aligned with the protobuf field names and integer encoding.
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
		ms.WriteObjectField("overageLimitCents")
		ms.WriteUint32(usage.GetOverageLimitCents())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("accruedOverageMicrodollars")
		ms.WriteInt64(usage.GetAccruedOverageMicrodollars())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("reservedOverageMicrodollars")
		ms.WriteInt64(usage.GetReservedOverageMicrodollars())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("currentPeriodStart")
		ms.WriteInt64(usage.GetCurrentPeriodStart())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("currentPeriodEnd")
		ms.WriteInt64(usage.GetCurrentPeriodEnd())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("offerVersion")
		ms.WriteString(usage.GetOfferVersion())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("monthlyPriceCents")
		ms.WriteUint32(usage.GetMonthlyPriceCents())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("writeMicrodollars")
		ms.WriteUint32(usage.GetWriteMicrodollars())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("readMicrodollars")
		ms.WriteUint32(usage.GetReadMicrodollars())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("policyVersion")
		ms.WriteString(usage.GetPolicyVersion())
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

	// Present spending, allowances, rates, and freshness with explicit units.
	baLabel := "default"
	if billingAccountID != "" {
		baLabel = billingAccountID
	}
	fields := [][2]string{
		{"Billing Account", baLabel},
		{"Monthly Price", billingFormatCurrency(float64(usage.GetMonthlyPriceCents()) / 100)},
		{"Extra Spending Limit", billingFormatCurrency(float64(usage.GetOverageLimitCents()) / 100)},
		{"Extra Usage Charges", billingFormatCurrency(float64(usage.GetAccruedOverageMicrodollars()) / 1e6)},
		{"Pending Extra Charges", billingFormatCurrency(float64(usage.GetReservedOverageMicrodollars()) / 1e6)},
		{"Storage", billingFormatBytes(usage.GetStorageBytes()) + " / " + billingFormatBytes(usage.GetStorageBaselineBytes()) + " included"},
		{"Write Ops", strconv.FormatInt(usage.GetWriteOps(), 10) + " / " + strconv.FormatInt(usage.GetWriteOpsBaseline(), 10) + " included"},
		{"Read Ops", strconv.FormatInt(usage.GetReadOps(), 10) + " / " + strconv.FormatInt(usage.GetReadOpsBaseline(), 10) + " included"},
		{"Extra Write Rate", "$" + strconv.FormatFloat(float64(usage.GetWriteMicrodollars())/1e6, 'f', 6, 64) + " per write"},
		{"Extra Read Rate", "$" + strconv.FormatFloat(float64(usage.GetReadMicrodollars())/1e6, 'f', 6, 64) + " per uncached read"},
		{"Metered Through", billingFormatTimestamp(usage.GetUsageMeteredThroughAt())},
	}
	if usage.GetCurrentPeriodStart() > 0 && usage.GetCurrentPeriodEnd() > 0 {
		fields = append(fields, [2]string{"Billing Period", billingFormatTimestamp(usage.GetCurrentPeriodStart()) + " to " + billingFormatTimestamp(usage.GetCurrentPeriodEnd())})
	}
	writeFields(w, fields)
	return nil
}

// writeBillingUsageNotApplicable explains why the selected provider has no bill.
func writeBillingUsageNotApplicable(
	w io.Writer,
	outputFormat string,
	sessionIdx uint32,
	providerID string,
	reason string,
) error {
	// Preserve a structured unavailable result for machine consumers.
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

	// Give interactive users the provider and the reason billing is unavailable.
	writeFields(w, [][2]string{
		{"Billing Usage", "not applicable"},
		{"Provider", providerID},
		{"Reason", reason},
	})
	return nil
}

// billingFormatBytes renders storage using the CLI's existing unit convention.
func billingFormatBytes(bytes float64) string {
	return strconv.FormatFloat(bytes/billingBytesPerGB, 'f', 2, 64) + " GB"
}

// billingFormatCurrency keeps a positive subcent charge distinguishable from zero.
func billingFormatCurrency(amount float64) string {
	if amount > 0 && amount < 0.01 {
		return "<$0.01"
	}
	return "$" + strconv.FormatFloat(amount, 'f', 2, 64)
}

// billingFormatTimestamp renders metering times in UTC and identifies missing data.
func billingFormatTimestamp(ms int64) string {
	if ms <= 0 {
		return "not yet metered"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04 UTC")
}

// _ verifies the mounted Session adapter's billing contract.
var _ billingSessionHandle = (*mountedBillingSession)(nil)
