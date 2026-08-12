//go:build native_provider_harness && !js

package main

import (
	"context"
	"testing"

	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
)

func TestNativeProviderHarnessConfigSet(t *testing.T) {
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
