//go:build native_provider_harness && !js

package main

import (
	"context"
	"math"
	"os"

	aperture_cli "github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// nativeProviderHarnessEndpoint is set with the Go linker for isolated native
// provider tests. Production builds exclude this file.
var nativeProviderHarnessEndpoint string

func init() {
	if nativeProviderHarnessEndpoint != "" {
		configSets = append(configSets, nativeProviderHarnessConfigSet)
	}
	cliCommands = append(cliCommands, newNativeProviderHarnessCommand)
}

func nativeProviderHarnessConfigSet(
	_ context.Context,
	_ bus.Bus,
	_ *logrus.Entry,
) ([]configset.ConfigSet, error) {
	conf := &provider_spacewave.Config{
		Endpoint:         nativeProviderHarnessEndpoint,
		AccountEndpoint:  nativeProviderHarnessEndpoint,
		PublicBaseUrl:    nativeProviderHarnessEndpoint,
		SigningEnvPrefix: "spacewave",
	}
	return []configset.ConfigSet{{
		"provider-spacewave": configset.NewControllerConfig(math.MaxUint64, conf),
	}}, nil
}

func newNativeProviderHarnessCommand(getBus func() cli_entrypoint.CliBus) []*aperture_cli.Command {
	return []*aperture_cli.Command{{
		Name:  "native-provider-harness",
		Usage: "inspect the native Spacewave provider in an isolated test build",
		Flags: []aperture_cli.Flag{
			&aperture_cli.BoolFlag{Name: "fetch-cloud-config"},
		},
		Action: func(c *aperture_cli.Context) error {
			cb := getBus()
			prov, provRef, err := provider.ExLookupProvider(c.Context, cb.GetBus(), "spacewave", false, nil)
			if err != nil {
				return errors.Wrap(err, "lookup native Spacewave provider")
			}
			defer provRef.Release()

			swProv, ok := prov.(*provider_spacewave.Provider)
			if !ok {
				return errors.Errorf("native Spacewave provider has type %T", prov)
			}
			if _, err := os.Stdout.WriteString(
				"endpoint=" + swProv.GetEndpoint() + "\n" +
					"account_endpoint=" + swProv.GetAccountEndpoint() + "\n" +
					"public_base_url=" + swProv.GetPublicBaseURL() + "\n",
			); err != nil {
				return err
			}
			if !c.Bool("fetch-cloud-config") {
				return nil
			}
			_, release, err := swProv.GetCloudConfig(c.Context)
			if err != nil {
				return errors.Wrap(err, "fetch native provider cloud config")
			}
			if release != nil {
				defer release()
			}
			_, err = os.Stdout.WriteString("cloud_config=ok\n")
			return err
		},
	}}
}
