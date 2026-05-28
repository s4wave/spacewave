//go:build js

package cdn_bstore

import (
	"context"
	"io"
	"net/http"

	fetch "github.com/aperturerobotics/util/js/fetch"
)

type jsRootPointerResponse struct {
	resp *fetch.Response
}

type httpRootPointerResponse struct {
	resp *http.Response
}

func fetchRootPointerResponse(ctx context.Context, httpCli *http.Client, url string) (rootPointerResponse, error) {
	if httpCli != nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpCli.Do(req)
		if err != nil {
			return nil, err
		}
		return httpRootPointerResponse{resp: resp}, nil
	}

	resp, err := fetch.Fetch(url, &fetch.Opts{
		Method: http.MethodGet,
		Signal: ctx,
	})
	if err != nil {
		return nil, err
	}
	return jsRootPointerResponse{resp: resp}, nil
}

func (r jsRootPointerResponse) StatusCode() int {
	return r.resp.StatusCode
}

func (r jsRootPointerResponse) Body() io.Reader {
	return r.resp.Body
}

func (r jsRootPointerResponse) Close() {
	if r.resp.Body == nil {
		return
	}
	_ = r.resp.Body.Close()
}

func (r httpRootPointerResponse) StatusCode() int {
	return r.resp.StatusCode
}

func (r httpRootPointerResponse) Body() io.Reader {
	return r.resp.Body
}

func (r httpRootPointerResponse) Close() {
	if r.resp.Body == nil {
		return
	}
	_ = r.resp.Body.Close()
}
