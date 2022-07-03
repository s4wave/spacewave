package web_fetch

import (
	context "context"
	"io"
	"net/http"
)

// ToHttpRequest constructs a http request from the FetchRequest.
func (r *FetchRequest) ToHttpRequest(ctx context.Context) (*http.Request, error) {
	var body io.Reader
	// TODO
	return http.NewRequestWithContext(ctx, r.GetMethod(), r.GetUrl(), body)
}
