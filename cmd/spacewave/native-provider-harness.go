//go:build native_provider_harness && !js

package main

import (
	"context"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

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

type nativeProviderHarnessEnvValue struct {
	value string
	set   bool
}

var nativeProviderHarnessEndpointEnv = []string{
	"SPACEWAVE_CLOUD_BASE_URL",
	"SPACEWAVE_CLOUD_ACCOUNT_BASE_URL",
	"SPACEWAVE_CLOUD_PUBLIC_BASE_URL",
	"SPACEWAVE_CLOUD_SIGNING_ENV_PREFIX",
}

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
	endpoint, err := validateNativeProviderHarnessEndpoint(nativeProviderHarnessEndpoint)
	if err != nil {
		return nil, err
	}
	conf := &provider_spacewave.Config{
		Endpoint:         endpoint,
		AccountEndpoint:  endpoint,
		PublicBaseUrl:    endpoint,
		SigningEnvPrefix: "spacewave",
	}
	return []configset.ConfigSet{{
		"provider-spacewave": configset.NewControllerConfig(math.MaxUint64, conf),
	}}, nil
}

func validateNativeProviderHarnessEndpoint(endpoint string) (string, error) {
	if endpoint == "" {
		return "", errors.New("native provider harness endpoint is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", errors.Wrap(err, "parse native provider harness endpoint")
	}
	if u.Scheme != "http" {
		return "", errors.New("native provider harness endpoint must use http")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("native provider harness endpoint must not contain credentials, query, or fragment")
	}
	host := u.Hostname()
	if host == "" || u.Port() == "" {
		return "", errors.New("native provider harness endpoint must include a host and port")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() {
			return "", errors.New("native provider harness endpoint must be loopback or localhost")
		}
	} else if !strings.EqualFold(host, "localhost") {
		return "", errors.New("native provider harness endpoint must be loopback or localhost")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("native provider harness endpoint must not contain a path")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("native provider harness endpoint must contain a valid port")
	}
	return endpoint, nil
}

// withNativeProviderHarnessEndpoint makes the linker-selected endpoint win over
// inherited provider environment variables for the duration of one lookup.
func withNativeProviderHarnessEndpoint(endpoint string, fn func() error) error {
	endpoint, err := validateNativeProviderHarnessEndpoint(endpoint)
	if err != nil {
		return err
	}

	old := make(map[string]nativeProviderHarnessEnvValue, len(nativeProviderHarnessEndpointEnv))
	for _, key := range nativeProviderHarnessEndpointEnv {
		value, set := os.LookupEnv(key)
		old[key] = nativeProviderHarnessEnvValue{value: value, set: set}
		value = endpoint
		if key == "SPACEWAVE_CLOUD_SIGNING_ENV_PREFIX" {
			value = "spacewave"
		}
		if err := os.Setenv(key, value); err != nil {
			restoreNativeProviderHarnessEnv(old)
			return errors.Wrapf(err, "set %s", key)
		}
	}
	defer restoreNativeProviderHarnessEnv(old)
	return fn()
}

func restoreNativeProviderHarnessEnv(old map[string]nativeProviderHarnessEnvValue) {
	for key, env := range old {
		if env.set {
			_ = os.Setenv(key, env.value)
		} else {
			_ = os.Unsetenv(key)
		}
	}
}

func newNativeProviderHarnessCommand(getBus func() cli_entrypoint.CliBus) []*aperture_cli.Command {
	return []*aperture_cli.Command{{
		Name:  "native-provider-harness",
		Usage: "inspect the native Spacewave provider in an isolated test build",
		Flags: []aperture_cli.Flag{
			&aperture_cli.BoolFlag{Name: "fetch-cloud-config"},
		},
		Action: func(c *aperture_cli.Context) error {
			endpoint, err := validateNativeProviderHarnessEndpoint(nativeProviderHarnessEndpoint)
			if err != nil {
				return err
			}
			return withNativeProviderHarnessEndpoint(endpoint, func() error {
				cb := getBus()
				if cb == nil {
					return errors.New("native provider harness bus is unavailable")
				}
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
			})
		},
	}}
}
