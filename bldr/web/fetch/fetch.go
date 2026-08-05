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

// Fetch executes a Fetch RPC stream with a remote.
//
// Returns once headers are received. Buffers response data.
func Fetch(
	ctx context.Context,
	caller FetchCaller,
	req *http.Request,
	rw http.ResponseWriter,
) error {
	// initialize the call
	strm, err := caller(ctx)
	if err != nil {
		return err
	}
	defer strm.Close()

	// send the request info
	hasBody := req.Body != nil
	err = strm.Send(NewFetchRequestWithInfo(req, "", hasBody))
	if err != nil {
		return err
	}

	// if we have a body, send it.
	if hasBody {
		buf := make([]byte, 2048)
		for {
			n, err := req.Body.Read(buf)
			if err != nil && err != io.EOF {
				return err
			}
			isEOF := err == io.EOF
			if n != 0 || isEOF {
				if werr := strm.Send(NewFetchRequestWithData(buf[:n], isEOF)); werr != nil {
					return werr
				}
			}
			if isEOF {
				break
			}
		}
	}

	// wait for the response info
	fetchResp, err := strm.Recv()
	if err != nil {
		return err
	}

	info := fetchResp.GetResponseInfo()
	statusCode := info.GetStatus()
	if statusCode == 0 {
		statusCode = 500
	}
	SetHeaders(info.GetHeaders(), rw.Header())
	rw.WriteHeader(int(statusCode))

	// Headers are forwarded; the response can no longer be repaired with an
	// error status. Any failure below poisons the response body via Abort so
	// the remote observes an error instead of a truncated body that ends
	// cleanly with a done packet.
	abortBody := func(err error) error {
		err = errors.Wrap(err, "fetch response body")
		if aborter, ok := rw.(BodyAborter); ok {
			aborter.Abort(err)
		}
		return err
	}
	for {
		fetchResp, err := strm.Recv()
		if err != nil {
			// The remote always sends a final ResponseData packet with done=true
			// after the last body bytes. Stream EOF before that packet means the
			// response body is incomplete.
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return abortBody(err)
		}
		switch body := fetchResp.GetBody().(type) {
		case *FetchResponse_ResponseData:
			data := body.ResponseData.GetData()
			written := 0
			for written < len(data) {
				nw, err := rw.Write(data[written:])
				written += nw
				if err != nil {
					return abortBody(err)
				}
			}
			if body.ResponseData.GetDone() {
				return nil
			}
		default:
			return abortBody(errors.New("unexpected non-data packet after info packet"))
		}
	}
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
// GoScript a JavaScript throw can surface as a Go panic from the handler;
// the stream must then close with an error instead of a clean done packet.
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
	if statusCode == 0 {
		statusCode = 200
	}
	return &FetchResponse{
		Body: &FetchResponse_ResponseInfo{
			ResponseInfo: &ResponseInfo{
				Ok:           true,
				Redirected:   false,
				Headers:      BuildHeadersMap(header, true),
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
