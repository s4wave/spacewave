//go:build !js

package wasm

import (
	"io"
	"net/http"
	"testing"

	"github.com/aperturerobotics/util/gitroot"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestE2ECloudAuthConfigEndpoint(t *testing.T) {
	endpoint, stop, err := startE2ECloudAuthConfigEndpoint("")
	if err != nil {
		t.Fatalf("start endpoint: %v", err)
	}
	t.Cleanup(stop)

	resp, err := http.Get(endpoint + e2eCloudAuthConfigPath)
	if err != nil {
		t.Fatalf("get auth config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read auth config: %v", err)
	}
	cfg := &api.AuthConfigResponse{}
	if err := cfg.UnmarshalVT(body); err != nil {
		t.Fatalf("decode auth config: %v", err)
	}
	if cfg.GetPublicBaseUrl() != endpoint {
		t.Fatalf("public base URL = %q, want %q", cfg.GetPublicBaseUrl(), endpoint)
	}
}

func TestStableE2ECloudAuthConfigAddr(t *testing.T) {
	a := stableE2ECloudAuthConfigAddr("/repo/.bldr/e2e-wasm/wasm-a")
	b := stableE2ECloudAuthConfigAddr("/repo/.bldr/e2e-wasm/wasm-a")
	c := stableE2ECloudAuthConfigAddr("/repo/.bldr/e2e-wasm/wasm-b")
	if a != b {
		t.Fatalf("stable addr changed: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different state roots to use different addrs, got %q", a)
	}
}

func TestApplyE2ECloudAuthConfigEndpoint(t *testing.T) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	projectConfig, err := loadProjectConfig(repoRoot)
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}

	endpoint := "http://127.0.0.1:12345"
	if err := applyE2ECloudAuthConfigEndpoint(projectConfig, endpoint); err != nil {
		t.Fatalf("apply endpoint: %v", err)
	}

	builder := projectConfig.GetManifests()["spacewave-core"].GetBuilder()
	goConf, err := decodeGoPluginConfig(builder.GetConfig())
	if err != nil {
		t.Fatalf("decode go plugin config: %v", err)
	}
	providerEntry := goConf.GetConfigSet()["provider-spacewave"]
	if providerEntry == nil {
		t.Fatal("provider-spacewave config missing")
	}
	swConf, err := decodeSpacewaveProviderConfig(providerEntry.GetConfig())
	if err != nil {
		t.Fatalf("decode provider config: %v", err)
	}
	if swConf.GetEndpoint() != endpoint {
		t.Fatalf("endpoint = %q, want %q", swConf.GetEndpoint(), endpoint)
	}
	if swConf.GetAccountEndpoint() != endpoint {
		t.Fatalf("account endpoint = %q, want %q", swConf.GetAccountEndpoint(), endpoint)
	}
}
