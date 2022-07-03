package web_fetch

import (
	context "context"
	"errors"
	"net/http"
	"strings"
)

// FetchCaller is a function which starts the Fetch call.
type FetchCaller func(ctx context.Context, in *FetchRequest) (SRPCFetchService_FetchClient, error)

// Fetch executes a Fetch RPC stream with a remote.
//
// Returns once headers are received. Buffers response data.
func Fetch(ctx context.Context, caller FetchCaller, req *http.Request) (*http.Response, error) {
	return nil, errors.New("TODO Fetch")
}

// HandleFetch handles an incoming Fetch RPC stream with a http handler.
func HandleFetch(
	req *FetchRequest,
	strm SRPCFetchService_FetchStream,
	handler http.HandlerFunc,
) error {
	// construct the http request
	ctx := strm.Context()
	httpRequest, err := req.ToHttpRequest(ctx)
	if err != nil {
		return err
	}
	// construct response writer
	rw := NewFetchResponseWriter(strm)
	handler.ServeHTTP(rw, httpRequest)
	_ = strm.CloseSend()
	return nil
}

// BuildHeadersMap builds the headers proto map from the Headers object.
func BuildHeadersMap(headers http.Header, setDefaults bool) map[string]string {
	out := make(map[string]string, len(headers))
	for k, vs := range headers {
		out[k] = strings.Join(vs, ", ")
	}
	if setDefaults {
		var hasContentType bool
		for k, v := range out {
			if v != "" && strings.EqualFold(k, "content-type") {
				hasContentType = true
				break
			}
		}
		if !hasContentType {
			out["Content-Type"] = "application/octet-stream"
		}
	}
	return out
}

// BuildFetchResponse_Info builds a FetchResponse from http response info.
func BuildFetchResponse_Info(header http.Header, statusCode int) *FetchResponse {
	if statusCode == 0 {
		statusCode = 200
	}
	return &FetchResponse{
		Body: &FetchResponse_ResponseInfo{
			ResponseInfo: &ResponseInfo{
				Ok:           true,
				Redirected:   false,
				Headers:      BuildHeadersMap(header, true),
				Status:       uint32(statusCode),
				StatusText:   http.StatusText(statusCode),
				ResponseType: "default",
			},
		},
	}
}

// BuildFetchResponse_Data builds a FetchResponse from http response data.
func BuildFetchResponse_Data(data []byte) *FetchResponse {
	return &FetchResponse{
		Body: &FetchResponse_ResponseData{
			ResponseData: &ResponseData{Data: data},
		},
	}
}

// Build
