//go:build !js

package wasm

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

const e2eCloudAuthConfigPath = "/api/auth/config"

func startE2ECloudAuthConfigEndpoint() (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	addr := listener.Addr().(*net.TCPAddr)
	endpoint := "http://127.0.0.1:" + strconv.Itoa(addr.Port)
	mux := http.NewServeMux()
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
		if err := srv.Shutdown(context.Background()); err != nil {
			if closeErr := srv.Close(); closeErr != nil {
				panic(closeErr)
			}
		}
	}
	return endpoint, stop, nil
}

func applyE2ECloudAuthConfigEndpoint(projectConfig *bldr_project.ProjectConfig, endpoint string) error {
	manifest := projectConfig.GetManifests()["spacewave-core"]
	if manifest == nil {
		return errors.New("spacewave-core manifest not found")
	}
	builder := manifest.GetBuilder()
	if builder == nil {
		return errors.New("spacewave-core manifest builder not found")
	}
	if builder.GetId() != bldr_plugin_compiler_go.ConfigID {
		return errors.Errorf("spacewave-core builder is %q, expected %q", builder.GetId(), bldr_plugin_compiler_go.ConfigID)
	}

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

	providerData, err := swConf.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "marshal provider-spacewave config")
	}
	providerEntry.Config = providerData

	builderData, err := goConf.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "marshal spacewave-core builder config")
	}
	builder.Config = builderData
	return nil
}

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
