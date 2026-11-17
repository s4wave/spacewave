package provider_spacewave

import (
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
)

// GetCloudConfig resolves the shared pre-auth cloud provider configuration.
func (p *Provider) GetCloudConfig(
	ctx context.Context,
) (*api.AuthConfigResponse, func(), error) {
	return p.cloudCfgRc.Resolve(ctx)
}

func cloudAuthConfigURL(endpoint string) (string, error) {
	return url.JoinPath(endpoint, "/api/auth/config")
}

func (p *Provider) resolveCloudConfig(
	ctx context.Context,
	_ func(),
) (*api.AuthConfigResponse, func(), error) {
	// Request the auth config.
	reqURL, err := cloudAuthConfigURL(p.endpoint)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build URL")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Accept", "application/octet-stream")
	resp, err := p.httpCli.Do(httpReq)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get auth config")
	}
	defer resp.Body.Close()

	// Read the body and decode a failure status.
	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, clouderror.ParseResponse(resp, respBody)
	}

	// Decode the config.
	var result api.AuthConfigResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal auth config")
	}
	return &result, nil, nil
}
