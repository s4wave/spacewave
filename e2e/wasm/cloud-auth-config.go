//go:build !js

package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/e2e/wasm/internal/configjson"
)

// e2eCloudAuthConfigPath serves discovery for the loopback cloud fixture.
const e2eCloudAuthConfigPath = "/api/auth/config"

// stableE2ECloudAuthConfigAddr keeps cached startup manifests reusable.
func stableE2ECloudAuthConfigAddr(stateRoot string) string {
	sum := sha256.Sum256([]byte("e2e-cloud-auth|" + stateRoot))
	port := 20000 + int(binary.BigEndian.Uint16(sum[:2])%30000)
	return "127.0.0.1:" + strconv.Itoa(port)
}

// startE2ECloudAuthConfigEndpoint serves auth discovery and session signaling.
func startE2ECloudAuthConfigEndpoint(bindAddr string) (string, func(), error) {
	// Bind the cached endpoint when available, otherwise allocate a free port.
	if bindAddr == "" {
		bindAddr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil && bindAddr != "127.0.0.1:0" {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return "", nil, err
	}

	// Keep signaling connections within the fixture's lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	addr := listener.Addr().(*net.TCPAddr)
	endpoint := "http://127.0.0.1:" + strconv.Itoa(addr.Port)
	mux := http.NewServeMux()
	if err := registerE2ESignaling(ctx, mux); err != nil {
		cancel()
		listener.Close()
		return "", nil, err
	}

	// Advertise only local endpoints to browser clients.
	mux.HandleFunc(e2eCloudAuthConfigPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", http.MethodGet+", "+http.MethodOptions)
		w.Header().Set("Access-Control-Allow-Headers", "Accept")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		resp := &api.AuthConfigResponse{
			SsoBaseUrl:       endpoint + "/api/auth/sso/start",
			ExchangeUrl:      endpoint + "/api/auth/sso/code/exchange",
			ConfirmUrl:       endpoint + "/api/auth/sso/confirm",
			AccountBaseUrl:   endpoint,
			PublicBaseUrl:    endpoint,
			GoogleSsoEnabled: false,
			GithubSsoEnabled: false,
			TurnstileSiteKey: "",
		}
		data, err := resp.MarshalVT()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := w.Write(data); err != nil {
			return
		}
	})

	// Serve until cleanup cancels sockets and closes the listener.
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	stop := func() {
		cancel()
		if err := srv.Shutdown(context.Background()); err != nil {
			if closeErr := srv.Close(); closeErr != nil {
				panic(closeErr)
			}
		}
	}
	return endpoint, stop, nil
}

// applyE2ECloudAuthConfigEndpoint routes cloud discovery and local rendezvous to the fixture.
func applyE2ECloudAuthConfigEndpoint(projectConfig *bldr_project.ProjectConfig, endpoint string) error {
	// Downstream applications may omit the Spacewave provider plugin.
	manifest := projectConfig.GetManifests()["spacewave-core"]
	if manifest == nil {
		// Downstream Bldr apps can reuse the WASM harness without bundling the
		// Spacewave product provider; there is no cloud auth config to rewrite.
		return nil
	}
	builder := manifest.GetBuilder()
	if builder == nil {
		return errors.New("spacewave-core manifest builder not found")
	}
	if builder.GetId() != bldr_plugin_compiler_go.ConfigID {
		return errors.Errorf("spacewave-core builder is %q, expected %q", builder.GetId(), bldr_plugin_compiler_go.ConfigID)
	}

	// Rewrite provider configuration before compiling startup manifests.
	goConf, err := decodeGoPluginConfig(builder.GetConfig())
	if err != nil {
		return errors.Wrap(err, "decode spacewave-core builder config")
	}
	providerEntry := goConf.GetConfigSet()["provider-spacewave"]
	if providerEntry == nil {
		return errors.New("provider-spacewave config not found")
	}
	swConf, err := decodeSpacewaveProviderConfig(providerEntry.GetConfig())
	if err != nil {
		return errors.Wrap(err, "decode provider-spacewave config")
	}
	swConf.Endpoint = endpoint
	swConf.AccountEndpoint = endpoint
	swConf.PublicBaseUrl = endpoint

	providerData, err := configjson.MarshalCanonical(swConf)
	if err != nil {
		return errors.Wrap(err, "marshal provider-spacewave config")
	}
	providerEntry.Config = providerData

	// Standalone sessions use this endpoint again after their browser restarts.
	localEntry := goConf.GetConfigSet()["provider-local"]
	if localEntry != nil {
		conf := &provider_local.Config{}
		data := localEntry.GetConfig()
		if len(data) != 0 {
			if data[0] == '{' {
				err = conf.UnmarshalJSON(data)
			} else {
				err = conf.UnmarshalVT(data)
			}
			if err != nil {
				return errors.Wrap(err, "decode provider-local config")
			}
		}
		conf.SignalingUrl = endpoint
		localEntry.Config, err = configjson.MarshalCanonical(conf)
		if err != nil {
			return errors.Wrap(err, "marshal provider-local config")
		}
	}

	// Publish the updated builder configuration as one manifest value.
	builderData, err := configjson.MarshalCanonical(goConf)
	if err != nil {
		return errors.Wrap(err, "marshal spacewave-core builder config")
	}
	builder.Config = builderData
	return nil
}

// decodeGoPluginConfig accepts JSON or binary manifest configuration.
func decodeGoPluginConfig(data []byte) (*bldr_plugin_compiler_go.Config, error) {
	conf := &bldr_plugin_compiler_go.Config{}
	if len(data) == 0 {
		return conf, nil
	}
	if data[0] == '{' {
		return conf, conf.UnmarshalJSON(data)
	}
	return conf, conf.UnmarshalVT(data)
}

// decodeSpacewaveProviderConfig accepts JSON or binary provider configuration.
func decodeSpacewaveProviderConfig(data []byte) (*provider_spacewave.Config, error) {
	conf := &provider_spacewave.Config{}
	if len(data) == 0 {
		return conf, nil
	}
	if data[0] == '{' {
		return conf, conf.UnmarshalJSON(data)
	}
	return conf, conf.UnmarshalVT(data)
}
