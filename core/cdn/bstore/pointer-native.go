//go:build !js

package cdn_bstore

import (
	"context"
	"io"
	"net/http"

	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
)

type nativeRootPointerResponse struct {
	resp *http.Response
}

func fetchRootPointerResponse(ctx context.Context, httpCli *http.Client, url string) (rootPointerResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "building root pointer request")
	}
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	return nativeRootPointerResponse{resp: resp}, nil
}

func (r nativeRootPointerResponse) StatusCode() int {
	return r.resp.StatusCode
}

func (r nativeRootPointerResponse) Body() io.Reader {
	return r.resp.Body
}

func (r nativeRootPointerResponse) Close() {
	alpha_nethttp.DrainAndCloseResponseBody(r.resp)
}
