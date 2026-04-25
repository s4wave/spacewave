package provider_spacewave

import (
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// GetCloudConfig resolves the shared pre-auth cloud provider configuration.
func (p *Provider) GetCloudConfig(
	ctx context.Context,
) (*api.AuthConfigResponse, func(), error) {
	return p.cloudCfgRc.Resolve(ctx)
}

func (p *Provider) resolveCloudConfig(
	ctx context.Context,
	_ func(),
) (*api.AuthConfigResponse, func(), error) {
	reqURL, err := url.JoinPath(p.endpoint, "/api/auth/config")
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

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, parseCloudResponseError(resp, respBody)
	}

	var result api.AuthConfigResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal auth config")
	}
	return &result, nil, nil
}
