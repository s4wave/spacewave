package web_fetch

import (
	context "context"
	"io"
	"net/http"
)

// NewFetchRequestWithInfo constructs a Info FetchRequest from a HTTP request.
func NewFetchRequestWithInfo(req *http.Request, clientID string, hasBody bool) *FetchRequest {
	return &FetchRequest{
		Body: &FetchRequest_RequestInfo{
			RequestInfo: NewFetchRequestInfo(req, clientID, hasBody),
		},
	}
}

// NewFetchRequestInfo constructs a FetchRequestInfo from a HTTP request.
func NewFetchRequestInfo(req *http.Request, clientID string, hasBody bool) *FetchRequestInfo {
	headersMap := BuildHeadersMap(req.Header, false)
	return &FetchRequestInfo{
		Method:   req.Method,
		Url:      req.URL.String(),
		Headers:  headersMap,
		HasBody:  hasBody,
		ClientId: clientID,
		Redirect: "follow",
	}
}

// NewFetchRequestWithData constructs a FetchRequest containing some data.
func NewFetchRequestWithData(data []byte, done bool) *FetchRequest {
	return &FetchRequest{
		Body: &FetchRequest_RequestData{
			&FetchRequestData{
				Data: data,
				Done: done,
			},
		},
	}
}

// ToHttpRequest constructs a http request from the FetchRequest.
func (r *FetchRequestInfo) ToHttpRequest(ctx context.Context, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, r.GetMethod(), r.GetUrl(), body)
	if err != nil {
		return nil, err
	}
	SetHeaders(r.GetHeaders(), req.Header)
	return req, nil
}
