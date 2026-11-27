//go:build !js

package spacewave_cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/db/world"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// promTargetKeyPrefix prefixes the object key of a target created from a bare
// ID.
const promTargetKeyPrefix = "prom/target/"

// promDashboardKeyPrefix prefixes the object key of a Dashboard created from a
// bare ID.
const promDashboardKeyPrefix = "prom/dashboard/"

// promArgs selects the daemon, session and Space of a prom command.
type promArgs struct {
	statePath  string
	spaceID    string
	sessionIdx int
	output     string
}

// BuildFlags returns the flags shared by every prom subcommand.
func (a *promArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		statePathFlag(&a.statePath),
		socketPathFlag(),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space ID or name (default: the only Space)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: &a.spaceID,
		},
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session index",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: &a.sessionIdx,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output format (text/json/yaml)",
			EnvVars:     []string{"SPACEWAVE_OUTPUT"},
			Value:       "text",
			Destination: &a.output,
		},
	}
}

// withSpace runs cb with the selected Space and its World engine.
func (a *promArgs) withSpace(c *cli.Context, cb func(*sdkClient, s4wave_space.SRPCSpaceResourceServiceClient, world.Engine) error) error {
	idx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}
	return withVmSpace(c, a.statePath, idx, a.spaceID, cb)
}

// newPromCommand builds the prom command group.
func newPromCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "prom",
		Usage: "scrape Prometheus exporters into the Space and query them with PromQL",
		Subcommands: []*cli.Command{
			{
				Name:    "target",
				Aliases: []string{"targets"},
				Usage:   "manage scrape targets",
				Subcommands: []*cli.Command{
					buildPromTargetCreateCommand(),
					buildPromTargetListCommand(),
					buildPromTargetApproveCommand(true),
					buildPromTargetApproveCommand(false),
				},
			},
			{
				Name:    "dashboard",
				Aliases: []string{"dashboards"},
				Usage:   "manage Dashboards",
				Subcommands: []*cli.Command{
					buildPromDashboardCreateCommand(),
				},
			},
			buildPromQueryCommand(),
			buildPromLabelsCommand(),
		},
	}
}

// promTargetCreateArgs creates a scrape target.
type promTargetCreateArgs struct {
	promArgs
	url       string
	job       string
	instance  string
	labels    cli.StringSlice
	interval  time.Duration
	timeout   time.Duration
	retention time.Duration
}

// BuildFlags returns the create flags.
func (a *promTargetCreateArgs) BuildFlags() []cli.Flag {
	return append(a.promArgs.BuildFlags(),
		&cli.StringFlag{Name: "url", Usage: "exporter metrics URL, or " + s4wave_prom.DeviceMetricsURL + " for the Device's own Spacewave process", Required: true, Destination: &a.url},
		&cli.StringFlag{Name: "job", Usage: "job label of every series", Required: true, Destination: &a.job},
		&cli.StringFlag{Name: "instance", Usage: "instance label of every series (default: the URL host)", Destination: &a.instance},
		&cli.StringSliceFlag{Name: "label", Usage: "extra NAME=VALUE label of every series", Destination: &a.labels},
		&cli.DurationFlag{Name: "interval", Usage: "time between scrapes", Value: s4wave_prom.DefaultScrapeInterval, Destination: &a.interval},
		&cli.DurationFlag{Name: "timeout", Usage: "bound of one scrape (default: the interval)", Destination: &a.timeout},
		&cli.DurationFlag{Name: "retention", Usage: "how long the target keeps samples", Value: s4wave_prom.DefaultRetention, Destination: &a.retention},
	)
}

// Run creates the target at the key argument, or at a new key.
func (a *promTargetCreateArgs) Run(c *cli.Context) error {
	// Choose the key and build the meta.
	key := c.Args().First()
	switch {
	case key == "":
		key = promTargetKeyPrefix + ulid.NewULID()
	case !strings.Contains(key, "/"):
		key = promTargetKeyPrefix + key
	}
	meta, err := a.buildMeta()
	if err != nil {
		return err
	}

	// Create the target in one transaction.
	err = a.withSpace(c, func(_ *sdkClient, _ s4wave_space.SRPCSpaceResourceServiceClient, engine world.Engine) error {
		return world.ExecTransaction(c.Context, engine, true, func(ctx context.Context, tx world.WorldState) error {
			if err := requireNewObject(ctx, tx, key, "target"); err != nil {
				return err
			}
			return s4wave_prom.CreateTarget(ctx, tx, key, meta)
		})
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, key+"\n")
	return err
}

// buildMeta builds the target's labels and scrape config from the flags.
func (a *promTargetCreateArgs) buildMeta() (*s4wave_prom.TargetMeta, error) {
	// Check the scrape config.
	scrape := &s4wave_prom.ScrapeConfig{Url: a.url, IntervalMs: durationMs(a.interval), TimeoutMs: durationMs(a.timeout)}
	if err := scrape.Validate(); err != nil {
		return nil, err
	}

	// Collect the labels, defaulting the instance to the URL host.
	instance := a.instance
	if instance == "" {
		u, err := url.Parse(a.url)
		if err != nil || u.Host == "" {
			return nil, errors.Errorf("--instance is required for %q", a.url)
		}
		instance = u.Host
	}
	byName := map[string]string{"job": a.job, "instance": instance}
	for _, kv := range a.labels.Value() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, errors.Errorf("invalid --label %q, want NAME=VALUE", kv)
		}
		byName[k] = v
	}

	// Store the labels sorted by name, as a label set requires.
	names := make([]string, 0, len(byName))
	for k := range byName {
		names = append(names, k)
	}
	slices.Sort(names)
	set := &s4wave_prom.LabelSet{}
	for _, k := range names {
		set.Labels = append(set.Labels, &s4wave_prom.Label{Name: k, Value: byName[k]})
	}
	return &s4wave_prom.TargetMeta{Labels: set, Scrape: scrape, RetentionMs: uint64(max(a.retention.Milliseconds(), 0))}, nil
}

// promDashboardCreateArgs creates a Dashboard.
type promDashboardCreateArgs struct {
	promArgs
}

// Run creates the Dashboard at the key argument, or at a new key, with the
// starter panels of the jobs the Space's targets carry.
func (a *promDashboardCreateArgs) Run(c *cli.Context) error {
	// Choose the key.
	key := c.Args().First()
	switch {
	case key == "":
		key = promDashboardKeyPrefix + ulid.NewULID()
	case !strings.Contains(key, "/"):
		key = promDashboardKeyPrefix + key
	}

	// Create the Dashboard with starter panels in one transaction.
	err := a.withSpace(c, func(_ *sdkClient, _ s4wave_space.SRPCSpaceResourceServiceClient, engine world.Engine) error {
		return world.ExecTransaction(c.Context, engine, true, func(ctx context.Context, tx world.WorldState) error {
			if err := requireNewObject(ctx, tx, key, "dashboard"); err != nil {
				return err
			}
			jobs, err := s4wave_prom.TargetJobs(ctx, tx)
			if err != nil {
				return err
			}
			return s4wave_prom.CreateDashboard(ctx, tx, key, s4wave_prom.StarterDashboard(jobs))
		})
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, key+"\n")
	return err
}

// buildPromDashboardCreateCommand builds the dashboard create subcommand.
func buildPromDashboardCreateCommand() *cli.Command {
	args := &promDashboardCreateArgs{}
	return &cli.Command{
		Name:      "create",
		Usage:     "create a Dashboard with starter panels for the Space's targets",
		ArgsUsage: "[id-or-object-key]",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// buildPromTargetCreateCommand builds the target create subcommand.
func buildPromTargetCreateCommand() *cli.Command {
	args := &promTargetCreateArgs{}
	return &cli.Command{
		Name:      "create",
		Usage:     "create a scrape target; approve it on the Device that scrapes it",
		ArgsUsage: "[id-or-object-key]",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// promTargetApproveArgs approves or revokes a target's process on this Device.
type promTargetApproveArgs struct {
	promArgs
	approve bool
}

// Run records the binding decision in this daemon's account.
func (a *promTargetApproveArgs) Run(c *cli.Context) error {
	// Require the target key.
	if c.NArg() != 1 {
		return errors.New("one target object key is required")
	}
	key := c.Args().First()

	// Record the decision for this Device's Space runtime.
	return a.withSpace(c, func(client *sdkClient, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient, _ world.Engine) error {
		// Set the binding through the Space contents resource.
		contents, release, err := client.mountSpaceContents(c.Context, spaceSvc)
		if err != nil {
			return err
		}
		defer release()
		_, err = contents.SetProcessBinding(c.Context, &s4wave_space.SetProcessBindingRequest{
			ObjectKey: key,
			TypeId:    s4wave_prom.TargetTypeID,
			Approved:  a.approve,
		})
		return errors.Wrap(err, "set target process binding")
	})
}

// buildPromTargetApproveCommand builds the approve or revoke subcommand.
func buildPromTargetApproveCommand(approve bool) *cli.Command {
	args := &promTargetApproveArgs{approve: approve}
	name, usage := "approve", "scrape the target from this Device"
	if !approve {
		name, usage = "revoke", "stop scraping the target from this Device"
	}
	return &cli.Command{
		Name:      name,
		Usage:     usage,
		ArgsUsage: "<object-key>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// promTargetListArgs lists the Space's targets with their status.
type promTargetListArgs struct {
	promArgs
}

// Run prints each target's labels, scrape config and latest status.
func (a *promTargetListArgs) Run(c *cli.Context) error {
	// Read every target in one transaction.
	type row struct {
		key    string
		meta   *s4wave_prom.TargetMeta
		status *s4wave_prom.TargetStatus
	}
	var rows []row
	err := a.withSpace(c, func(_ *sdkClient, _ s4wave_space.SRPCSpaceResourceServiceClient, engine world.Engine) error {
		return world.ExecTransaction(c.Context, engine, false, func(ctx context.Context, tx world.WorldState) error {
			keys, err := s4wave_prom.ListTargets(ctx, tx)
			if err != nil {
				return err
			}
			for _, key := range keys {
				meta, status, err := s4wave_prom.ReadTarget(ctx, tx, key)
				if err != nil {
					return err
				}
				rows = append(rows, row{key: key, meta: meta, status: status})
			}
			return nil
		})
	})
	if err != nil {
		return err
	}

	// Print the rows.
	table := [][]string{{"KEY", "LABELS", "URL", "UP", "LAST SCRAPE", "SERIES", "DEVICE", "ERROR"}}
	for _, r := range rows {
		up, last, series, device := "-", "never", "-", "-"
		if st := r.status; st != nil {
			up = strconv.FormatBool(st.GetUp())
			last = time.Since(time.UnixMilli(st.GetLastScrapeMs())).Truncate(time.Second).String() + " ago"
			series = strconv.FormatUint(st.GetSeries(), 10)
			device = st.GetDevicePeerId()
		}
		table = append(table, []string{
			r.key, formatPromLabels("", r.meta.GetLabels()), r.meta.GetScrape().GetUrl(),
			up, last, series, device, r.status.GetLastError(),
		})
	}
	writeTable(os.Stdout, "", table)
	return nil
}

// buildPromTargetListCommand builds the target list subcommand.
func buildPromTargetListCommand() *cli.Command {
	args := &promTargetListArgs{}
	return &cli.Command{
		Name:   "list",
		Usage:  "list scrape targets and their latest scrape",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// promQueryArgs evaluates PromQL over the Space's targets.
type promQueryArgs struct {
	promArgs
	target string
	at     string
	rng    time.Duration
	step   time.Duration
}

// BuildFlags returns the query flags.
func (a *promQueryArgs) BuildFlags() []cli.Flag {
	return append(a.promArgs.BuildFlags(),
		&cli.StringFlag{Name: "target", Usage: "target whose plugin resource answers (default: any target)", Destination: &a.target},
		&cli.StringFlag{Name: "time", Usage: "RFC 3339 instant of an instant query (default: now)", Destination: &a.at},
		&cli.DurationFlag{Name: "range", Usage: "make a range query over this long, ending now", Destination: &a.rng},
		&cli.DurationFlag{Name: "step", Usage: "step of a range query (default: range/10)", Destination: &a.step},
	)
}

// Run evaluates the expression argument and prints the result.
func (a *promQueryArgs) Run(c *cli.Context) error {
	// Build the request.
	if c.NArg() != 1 {
		return errors.New("one PromQL expression is required")
	}
	req := &s4wave_prom.QueryRequest{Expr: c.Args().First()}

	// Pick the instant or the range.
	if a.at != "" {
		at, err := time.Parse(time.RFC3339, a.at)
		if err != nil {
			return errors.Wrap(err, "parse --time")
		}
		req.TimeMs = at.UnixMilli()
	}
	if a.rng > 0 {
		step := a.step
		if step <= 0 {
			step = max(a.rng/10, time.Second)
		}
		end := time.Now()
		req.StartMs, req.EndMs, req.StepMs = end.Add(-a.rng).UnixMilli(), end.UnixMilli(), step.Milliseconds()
	}

	// Evaluate the expression.
	var resp *s4wave_prom.QueryResponse
	err := a.withQueryService(c, a.target, func(svc s4wave_prom.SRPCPromQueryResourceServiceClient) error {
		var err error
		resp, err = svc.Query(c.Context, req)
		return errors.Wrap(err, "query")
	})
	if err != nil {
		return err
	}

	// Print the result.
	if a.output == "json" || a.output == "yaml" {
		data, err := resp.MarshalJSON()
		if err != nil {
			return err
		}
		return formatOutput(data, a.output)
	}
	writePromResult(os.Stdout, resp)
	return nil
}

// withQueryService calls cb with the PromQL service on a target's typed
// resource, discovering a target when none is named. The service reads every
// target in the Space, so any target answers.
func (a *promArgs) withQueryService(c *cli.Context, target string, cb func(s4wave_prom.SRPCPromQueryResourceServiceClient) error) error {
	// Resolve the session.
	idx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}

	// Mount the target's typed resource.
	var discover func(context.Context, *sdk_engine.SDKEngine) (string, error)
	if target == "" {
		discover = discoverPromTarget
	}
	mount, err := mountObjectChain(c, a.statePath, fsURI{sessionIdx: idx, spaceID: a.spaceID, objectKey: target}, discover)
	if err != nil {
		return err
	}
	defer mount.release()
	return cb(s4wave_prom.NewSRPCPromQueryResourceServiceClient(mount.TypedClient))
}

// buildPromQueryCommand builds the query subcommand.
func buildPromQueryCommand() *cli.Command {
	args := &promQueryArgs{}
	return &cli.Command{
		Name:      "query",
		Usage:     "evaluate a PromQL expression over every target in the Space",
		ArgsUsage: "<expr>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// promLabelsArgs lists label names, or one label's values, over the Space's
// targets.
type promLabelsArgs struct {
	promArgs
	target string
	match  cli.StringSlice
}

// BuildFlags returns the labels flags.
func (a *promLabelsArgs) BuildFlags() []cli.Flag {
	return append(a.promArgs.BuildFlags(),
		&cli.StringFlag{Name: "target", Usage: "target whose plugin resource answers (default: any target)", Destination: &a.target},
		&cli.StringSliceFlag{Name: "match", Usage: "series selector to limit the lookup to, repeatable", Destination: &a.match},
	)
}

// Run prints the label names, or the values of the named label.
func (a *promLabelsArgs) Run(c *cli.Context) error {
	// Read the optional label name.
	if c.NArg() > 1 {
		return errors.New("at most one label name is allowed")
	}
	name, match := c.Args().First(), a.match.Value()

	// Look up the names or the values.
	var resp *s4wave_prom.LabelsResponse
	err := a.withQueryService(c, a.target, func(svc s4wave_prom.SRPCPromQueryResourceServiceClient) error {
		var err error
		if name == "" {
			resp, err = svc.LabelNames(c.Context, &s4wave_prom.LabelNamesRequest{Match: match})
		} else {
			resp, err = svc.LabelValues(c.Context, &s4wave_prom.LabelValuesRequest{Name: name, Match: match})
		}
		return errors.Wrap(err, "label lookup")
	})
	if err != nil {
		return err
	}

	// Print one per line, or the response as JSON or YAML.
	if a.output == "json" || a.output == "yaml" {
		data, err := resp.MarshalJSON()
		if err != nil {
			return err
		}
		return formatOutput(data, a.output)
	}
	for _, v := range resp.GetValues() {
		fmt.Println(v)
	}
	return nil
}

// buildPromLabelsCommand builds the labels subcommand.
func buildPromLabelsCommand() *cli.Command {
	args := &promLabelsArgs{}
	return &cli.Command{
		Name:      "labels",
		Usage:     "list label names, or one label's values, over every target in the Space",
		ArgsUsage: "[label-name]",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// discoverPromTarget returns the first target in the Space.
func discoverPromTarget(ctx context.Context, engine *sdk_engine.SDKEngine) (string, error) {
	// List the targets in one read transaction.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return "", err
	}
	defer tx.Discard()

	// Take the first target.
	keys, err := s4wave_prom.ListTargets(ctx, tx)
	if err != nil {
		return "", errors.Wrap(err, "list targets")
	}
	if len(keys) == 0 {
		return "", errors.New("the Space has no scrape targets")
	}
	return keys[0], nil
}

// writePromResult prints a query result: one row per series of a vector, the
// samples under each series of a matrix, or the bare value otherwise.
func writePromResult(w io.Writer, resp *s4wave_prom.QueryResponse) {
	switch resp.GetResultType() {
	case s4wave_prom.ResultType_RESULT_TYPE_VECTOR:
		rows := [][]string{{"SERIES", "VALUE"}}
		for _, s := range resp.GetSeries() {
			for _, smp := range s.GetSamples() {
				rows = append(rows, []string{formatPromLabels("__name__", s.GetLabels()), formatPromValue(smp.GetValue())})
			}
		}
		writeTable(w, "", rows)
	case s4wave_prom.ResultType_RESULT_TYPE_MATRIX:
		for i, s := range resp.GetSeries() {
			if i != 0 {
				io.WriteString(w, "\n")
			}
			io.WriteString(w, formatPromLabels("__name__", s.GetLabels())+"\n")
			rows := [][]string{{"TIME", "VALUE"}}
			for _, smp := range s.GetSamples() {
				rows = append(rows, []string{time.UnixMilli(smp.GetTimeMs()).Format(time.TimeOnly), formatPromValue(smp.GetValue())})
			}
			writeTable(w, "  ", rows)
		}
	case s4wave_prom.ResultType_RESULT_TYPE_SCALAR:
		for _, s := range resp.GetSeries() {
			for _, smp := range s.GetSamples() {
				io.WriteString(w, formatPromValue(smp.GetValue())+"\n")
			}
		}
	case s4wave_prom.ResultType_RESULT_TYPE_STRING:
		io.WriteString(w, resp.GetStringValue()+"\n")
	}
	for _, warning := range resp.GetWarnings() {
		io.WriteString(os.Stderr, "warning: "+warning+"\n")
	}
}

// formatPromLabels prints a label set as name{k="v",...}, taking the name
// from the label nameLabel when it is set.
func formatPromLabels(nameLabel string, set *s4wave_prom.LabelSet) string {
	// Split the name from the other labels.
	var name string
	var parts []string
	for _, l := range set.GetLabels() {
		if nameLabel != "" && l.GetName() == nameLabel {
			name = l.GetValue()
			continue
		}
		parts = append(parts, l.GetName()+"="+strconv.Quote(l.GetValue()))
	}

	// Print the bare name when it has no other labels.
	if len(parts) == 0 && name != "" {
		return name
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

// formatPromValue prints a sample value in its shortest exact form.
func formatPromValue(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// durationMs converts d to whole milliseconds, clamped to the uint32 range.
func durationMs(d time.Duration) uint32 {
	return uint32(min(max(d.Milliseconds(), 0), int64(^uint32(0)))) //nolint:gosec // clamped above.
}
