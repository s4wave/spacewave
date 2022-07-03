package web_fetch

import (
	context "context"
	"io"
	"net/http"
)

// ToHttpRequest constructs a http request from the FetchRequest.
func (r *FetchRequestInfo) ToHttpRequest(ctx context.Context, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, r.GetMethod(), r.GetUrl(), body)
	if err != nil {
		return nil, err
	}
	SetHeaders(r.GetHeaders(), req.Header)
	return req, nil
}
