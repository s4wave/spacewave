package web_fetch

import (
	context "context"
	"errors"
	"io"
	"net/http"
	"strings"
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
			if n != 0 {
				werr := strm.Send(NewFetchRequestWithData(buf[:n], isEOF))
				if werr != nil {
					return err
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
	statusTxt := info.GetStatusText()
	if statusCode == 0 {
		statusCode = 500
	}
	if statusTxt == "" {
		statusTxt = http.StatusText(int(statusCode))
	}
	SetHeaders(info.GetHeaders(), rw.Header())
	rw.WriteHeader(int(statusCode))

	for {
		fetchResp, err := strm.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch body := fetchResp.GetBody().(type) {
		case *FetchResponse_ResponseData:
			data := body.ResponseData.GetData()
			written := 0
			for written < len(data) {
				nw, err := rw.Write(data[written:])
				written += nw
				if err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected non-data packet after info packet")
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

	// serve http
	handler.ServeHTTP(rw, httpRequest)
	return strm.CloseSend()
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
		for i := 0; i < len(vals); i++ {
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
