//go:build native_provider_harness && !js

package main

import (
	"context"
	"net/http"
	"testing"

	aperture_cli "github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func TestNativeProviderHarnessRequiresEndpointBeforeLookup(t *testing.T) {
	original := nativeProviderHarnessEndpoint
	nativeProviderHarnessEndpoint = ""
	t.Cleanup(func() { nativeProviderHarnessEndpoint = original })
	t.Setenv("SPACEWAVE_CLOUD_BASE_URL", "https://cloud.example.invalid")

	lookupCalled := false
	commands := newNativeProviderHarnessCommand(func() cli_entrypoint.CliBus {
		lookupCalled = true
		return nil
	})
	ctx := aperture_cli.NewContext(aperture_cli.NewApp(), nil, nil)
	if err := commands[0].Action(ctx); err == nil {
		t.Fatal("missing endpoint unexpectedly succeeded")
	}
	if lookupCalled {
		t.Fatal("bus lookup started without an explicit endpoint")
	}
}

func TestNativeProviderHarnessConfigSetLoopbackEndpoint(t *testing.T) {
	const endpoint = "http://127.0.0.1:43127"
	original := nativeProviderHarnessEndpoint
	nativeProviderHarnessEndpoint = endpoint
	t.Cleanup(func() { nativeProviderHarnessEndpoint = original })

	sets, err := nativeProviderHarnessConfigSet(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("build native provider harness config set: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("config set count = %d, want 1", len(sets))
	}
	entry := sets[0]["provider-spacewave"]
	if entry == nil {
		t.Fatal("provider-spacewave config missing")
	}
	conf, ok := entry.GetConfig().(*provider_spacewave.Config)
	if !ok {
		t.Fatalf("provider-spacewave config has type %T", entry.GetConfig())
	}
	if conf.GetEndpoint() != endpoint ||
		conf.GetAccountEndpoint() != endpoint ||
		conf.GetPublicBaseUrl() != endpoint {
		t.Fatalf("provider endpoints = %q, %q, %q; want %q", conf.GetEndpoint(), conf.GetAccountEndpoint(), conf.GetPublicBaseUrl(), endpoint)
	}
}

func TestNativeProviderHarnessEndpointShapes(t *testing.T) {
	for _, endpoint := range []string{
		"http://127.0.0.1:43127",
		"http://[::1]:43127",
		"http://localhost:43127",
	} {
		if _, err := validateNativeProviderHarnessEndpoint(endpoint); err != nil {
			t.Errorf("valid endpoint %q rejected: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"",
		"https://127.0.0.1:43127",
		"http://192.0.2.1:43127",
		"http://127.0.0.1",
		"http://127.0.0.1:43127/path",
		"http://user@127.0.0.1:43127",
		"http://127.0.0.1:43127?cloud=true",
	} {
		if _, err := validateNativeProviderHarnessEndpoint(endpoint); err == nil {
			t.Errorf("hostile endpoint %q unexpectedly accepted", endpoint)
		}
	}
}

func TestNativeProviderHarnessEndpointIsolation(t *testing.T) {
	const endpoint = "http://127.0.0.1:43127"
	original := nativeProviderHarnessEndpoint
	nativeProviderHarnessEndpoint = endpoint
	t.Cleanup(func() { nativeProviderHarnessEndpoint = original })
	for _, key := range nativeProviderHarnessEndpointEnv {
		t.Setenv(key, "https://cloud.example.invalid")
	}

	requests := 0
	lookupCalled := false
	commands := newNativeProviderHarnessCommand(func() cli_entrypoint.CliBus {
		lookupCalled = true
		conf := &provider_spacewave.Config{
			Endpoint:         endpoint,
			AccountEndpoint:  endpoint,
			PublicBaseUrl:    endpoint,
			SigningEnvPrefix: "spacewave",
		}
		prov := provider_spacewave.NewProvider(
			logrus.NewEntry(logrus.New()),
			nil,
			conf,
			provider_spacewave.NewProviderInfo("spacewave"),
			nil,
			nil,
		)
		prov.GetHTTPClient().Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests++
			t.Fatal("provider attempted HTTP during endpoint resolution")
			return nil, nil
		})
		if prov.GetEndpoint() != endpoint ||
			prov.GetAccountEndpoint() != endpoint ||
			prov.GetPublicBaseURL() != endpoint {
			t.Fatalf("provider endpoints displaced by environment: %q, %q, %q", prov.GetEndpoint(), prov.GetAccountEndpoint(), prov.GetPublicBaseURL())
		}
		return nil
	})
	ctx := aperture_cli.NewContext(aperture_cli.NewApp(), nil, nil)
	if err := commands[0].Action(ctx); err == nil {
		t.Fatal("harness unexpectedly succeeded without a bus")
	}
	if !lookupCalled {
		t.Fatal("harness did not reach the provider lookup boundary")
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0", requests)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
