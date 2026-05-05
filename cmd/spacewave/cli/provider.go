//go:build !js

package spacewave_cli

import (
	"os"
	"strconv"
	"strings"

	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	provider_pb "github.com/s4wave/spacewave/core/provider"
	s4wave_provider "github.com/s4wave/spacewave/sdk/provider"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// newProviderCommand builds the provider command group.
func newProviderCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:    "provider",
		Aliases: []string{"providers"},
		Usage:   "manage providers",
		Subcommands: []*cli.Command{
			newProviderListCommand(),
			newProviderInfoCommand(),
		},
	}
}

// newProviderListCommand builds the provider list command.
func newProviderListCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "list",
		Usage: "list registered providers",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runProviderList(c, statePath, c.String("output"))
		},
	}
}

// runProviderList implements the provider list command logic.
func runProviderList(c *cli.Context, statePath, outputFormat string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	providers, err := client.root.ListProviders(ctx)
	if err != nil {
		return errors.Wrap(err, "list providers")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		data, err := protojson.MarshalSlice(protojson.MarshalerConfig{}, providers)
		if err != nil {
			return err
		}
		return formatOutput(data, outputFormat)
	}

	if len(providers) == 0 {
		os.Stdout.WriteString("no providers\n")
		return nil
	}
	rows := [][]string{{"PROVIDER", "FEATURES"}}
	for _, p := range providers {
		var feats []string
		for _, f := range p.GetProviderFeatures() {
			feats = append(feats, featureShortName(f))
		}
		rows = append(rows, []string{p.GetProviderId(), strings.Join(feats, ", ")})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

// newProviderInfoCommand builds the provider info command.
func newProviderInfoCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:      "info",
		Usage:     "show provider details",
		ArgsUsage: "<provider-id>",
		Flags:     clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			providerID := c.Args().First()
			if providerID == "" {
				return errors.New("provider-id required as first argument")
			}
			return runProviderInfo(c, statePath, c.String("output"), providerID)
		},
	}
}

// runProviderInfo implements the provider info command logic.
func runProviderInfo(c *cli.Context, statePath, outputFormat, providerID string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	providerSvc, providerCleanup, err := client.lookupProvider(ctx, providerID)
	if err != nil {
		return err
	}
	defer providerCleanup()

	resp, err := providerSvc.GetProviderInfo(ctx, &s4wave_provider.GetProviderInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "get provider info")
	}

	info := resp.GetProviderInfo()
	if outputFormat == "json" || outputFormat == "yaml" {
		data, err := info.MarshalJSON()
		if err != nil {
			return err
		}
		return formatOutput(data, outputFormat)
	}

	w := os.Stdout
	writeFields(w, [][2]string{{"Provider", info.GetProviderId()}})
	features := info.GetProviderFeatures()
	if len(features) > 0 {
		w.WriteString("\nFeatures (" + strconv.Itoa(len(features)) + ")\n")
		for _, f := range features {
			w.WriteString("  " + featureShortName(f) + "\n")
		}
	}
	return nil
}

// featureShortName returns a short display name for a provider feature.
func featureShortName(f provider_pb.ProviderFeature) string {
	name := f.String()
	name = strings.TrimPrefix(name, "ProviderFeature_")
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}
