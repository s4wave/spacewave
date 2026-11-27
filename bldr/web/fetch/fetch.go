package web_fetch

import (
	context "context"
	"io"
	"net/http"
	"runtime/trace"
	"strings"

	"github.com/pkg/errors"
)

// FetchCaller is a function which starts the Fetch call.
type FetchCaller func(ctx context.Context) (SRPCFetchService_FetchClient, error)

// Fetch executes a Fetch RPC stream with a remote and writes the response
// to rw. Returns once the response body is complete.
func Fetch(
	ctx context.Context,
	caller FetchCaller,
	req *http.Request,
	rw http.ResponseWriter,
) error {
	// Send the request and wait for the response headers.
	resp, err := roundTrip(ctx, caller, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Apply the response status and headers to the writer.
	for k, vs := range resp.Header {
		rw.Header()[k] = append(rw.Header()[k], vs...)
	}
	rw.WriteHeader(resp.StatusCode)

	// Headers are forwarded, so the response can no longer be repaired with an
	// error status. A failure below poisons the response body via Abort so
	// the remote observes an error instead of a truncated body that ends
	// cleanly with a done packet.
	if _, err := io.Copy(rw, resp.Body); err != nil {
		err = errors.Wrap(err, "fetch response body")
		if aborter, ok := rw.(BodyAborter); ok {
			aborter.Abort(err)
		}
		return err
	}
	return nil
}

// HandleFetch handles an incoming Fetch RPC stream with a http handler.
func HandleFetch(
	strm SRPCFetchService_FetchStream,
	handler http.HandlerFunc,
) error {
	// construct the http request
	ctx := strm.Context()

	// receive the request headers
	reqFirstPkt, err := strm.Recv()
	if err != nil {
		return err
	}
	reqInfo := reqFirstPkt.GetRequestInfo()

	// streaming request body (if necessary)
	var fetchBodyReader io.Reader
	if reqInfo.GetHasBody() {
		fetchBodyReader = NewFetchBodyReader(strm)
	}
	httpRequest, err := reqInfo.ToHttpRequest(ctx, fetchBodyReader)
	if err != nil {
		return err
	}

	// construct response writer
	rw := NewFetchResponseWriter(strm)

	// Trace the complete handler window and propagate its task to downstream work.
	var handlerTask *trace.Task
	if trace.IsEnabled() {
		handlerCtx, task := trace.NewTask(httpRequest.Context(), "bldr/web/fetch/serve-http")
		handlerTask = task
		rw.traceCtx = handlerCtx
		httpRequest = httpRequest.WithContext(handlerCtx)
	}
	err = serveFetchHTTP(rw, httpRequest, handler)
	if handlerTask != nil {
		handlerTask.End()
	}
	if err != nil {
		return err
	}

	// flush implicit 200 headers when the handler wrote nothing.
	rw.WriteHeader(http.StatusOK)

	// The final done packet asserts the body is complete. A poisoned body
	// (failed packet send, handler Abort, or Content-Length mismatch) must
	// instead close the stream with an error so remotes cannot accept a
	// truncated body.
	if err := rw.BodyError(httpRequest.Method); err != nil {
		return err
	}

	// send done packet
	err = strm.Send(&FetchResponse{
		Body: &FetchResponse_ResponseData{
			ResponseData: &ResponseData{Done: true},
		},
	})
	if err != nil {
		return err
	}
	return nil
}

// serveFetchHTTP runs the handler, recovering panics into an error. Under
// GoScript a JavaScript throw can surface as a Go panic from the handler,
// so the stream must then close with an error instead of a clean done packet.
func serveFetchHTTP(rw http.ResponseWriter, req *http.Request, handler http.HandlerFunc) (err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			err = errors.Errorf("fetch handler panic: %v", rerr)
		}
	}()
	handler.ServeHTTP(rw, req)
	return nil
}

// BuildHeadersMap builds the headers proto map from the Headers object.
func BuildHeadersMap(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for k, vs := range headers {
		out[k] = strings.Join(vs, ", ")
	}
	return out
}

// hasContentType reports whether the headers map sets a Content-Type.
func hasContentType(headers map[string]string) bool {
	for k, v := range headers {
		if v != "" && strings.EqualFold(k, "content-type") {
			return true
		}
	}
	return false
}

// SetHeaders copies headers from a map to a http.Header.
func SetHeaders(headerMap map[string]string, setTo http.Header) {
	for k, v := range headerMap {
		vals := strings.Split(v, ",")
		for i := range vals {
			vals[i] = strings.TrimSpace(vals[i])
			if len(vals[i]) != 0 {
				setTo.Add(k, vals[i])
			}
		}
	}
}

// BuildFetchResponse_Info builds a FetchResponse from http response info.
func BuildFetchResponse_Info(header http.Header, statusCode int) *FetchResponse {
	// Default the status, and the Content-Type of a response that has a body.
	// A bodiless 204 or 304 carries none, as with net/http: an HTTP cache
	// merges a 304's headers into its stored response.
	if statusCode == 0 {
		statusCode = 200
	}
	headers := BuildHeadersMap(header)
	if statusAllowsBody(statusCode) && !hasContentType(headers) {
		headers["Content-Type"] = "application/octet-stream"
	}

	// Build the response info packet.
	return &FetchResponse{
		Body: &FetchResponse_ResponseInfo{
			ResponseInfo: &ResponseInfo{
				Ok:           true,
				Redirected:   false,
				Headers:      headers,
				Status:       uint32(statusCode), //nolint:gosec
				StatusText:   http.StatusText(statusCode),
				ResponseType: "default",
			},
		},
	}
}

// BuildFetchResponse_Data builds a FetchResponse from http response data.
func BuildFetchResponse_Data(data []byte, done bool) *FetchResponse {
	return &FetchResponse{
		Body: &FetchResponse_ResponseData{
			ResponseData: &ResponseData{Data: cloneFetchPacketData(data), Done: done},
		},
	}
}

func cloneFetchPacketData(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}
