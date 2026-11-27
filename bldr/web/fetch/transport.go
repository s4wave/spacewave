package web_fetch

import (
	context "context"
	"io"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
)

// maxFetchRequestDataPacketBytes bounds one request body data packet.
const maxFetchRequestDataPacketBytes = 16 * 1024

// Transport is an http.RoundTripper that sends each request over its own
// Fetch RPC stream. The request body is sent in full before the response
// headers are read, so the transport does not carry protocol upgrades.
type Transport struct {
	// caller opens one Fetch stream per request.
	caller FetchCaller
}

// NewTransport constructs a Transport over a Fetch caller.
func NewTransport(caller FetchCaller) *Transport {
	return &Transport{caller: caller}
}

// RoundTrip sends req and returns once the response headers arrive. The
// response body reads the stream until its done packet. Closing the body
// closes the stream.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The RoundTripper contract closes the request body on every path.
	resp, err := roundTrip(req.Context(), t.caller, req)
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return resp, err
}

// roundTrip sends req on a new Fetch stream and returns the response whose
// body owns the stream.
func roundTrip(ctx context.Context, caller FetchCaller, req *http.Request) (*http.Response, error) {
	// Open the stream and send the request.
	strm, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := sendFetchRequest(strm, req); err != nil {
		_ = strm.Close()
		return nil, err
	}

	// Wait for the response info.
	fetchResp, err := strm.Recv()
	if err != nil {
		_ = strm.Close()
		return nil, err
	}
	info := fetchResp.GetResponseInfo()

	// Build the response around the remaining data packets.
	statusCode := int(info.GetStatus())
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	header := make(http.Header)
	SetHeaders(info.GetHeaders(), header)
	contentLength := int64(-1)
	if n, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64); err == nil {
		contentLength = n
	}
	return &http.Response{
		Status:        strconv.Itoa(statusCode) + " " + http.StatusText(statusCode),
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          &fetchResponseBody{strm: strm},
		ContentLength: contentLength,
		Request:       req,
	}, nil
}

// sendFetchRequest sends the request info and then its body in data packets.
func sendFetchRequest(strm SRPCFetchService_FetchClient, req *http.Request) error {
	// Send the request info.
	hasBody := req.Body != nil && req.Body != http.NoBody
	if err := strm.Send(NewFetchRequestWithInfo(req, "", hasBody)); err != nil {
		return err
	}
	if !hasBody {
		return nil
	}

	// Send the body, marking the last packet done.
	buf := make([]byte, maxFetchRequestDataPacketBytes)
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
			return nil
		}
	}
}

// fetchResponseBody reads response data packets until the done packet.
type fetchResponseBody struct {
	// strm is the Fetch stream the body owns.
	strm SRPCFetchService_FetchClient
	// buf holds unread bytes of the current packet.
	buf []byte
	// done is set once the done packet arrived.
	done bool
	// err is the sticky receive error.
	err error
}

// Read returns body bytes, io.EOF after the done packet, or
// io.ErrUnexpectedEOF when the stream ends before it.
func (b *fetchResponseBody) Read(p []byte) (int, error) {
	// Pull packets until one carries data or the body ends.
	for len(b.buf) == 0 {
		if b.done {
			return 0, io.EOF
		}
		if b.err == nil {
			b.err = b.recv()
		}
		if b.err != nil {
			return 0, b.err
		}
	}

	// Copy out of the current packet.
	n := copy(p, b.buf)
	b.buf = b.buf[n:]
	return n, nil
}

// recv receives the next data packet into buf.
func (b *fetchResponseBody) recv() error {
	// The remote always ends a complete body with a done packet.
	resp, err := b.strm.Recv()
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	if err != nil {
		return err
	}
	data, ok := resp.GetBody().(*FetchResponse_ResponseData)
	if !ok {
		return errors.New("unexpected non-data packet after info packet")
	}
	b.buf, b.done = data.ResponseData.GetData(), data.ResponseData.GetDone()
	return nil
}

// Close closes the Fetch stream.
func (b *fetchResponseBody) Close() error {
	return b.strm.Close()
}

// _ is a type assertion
var _ http.RoundTripper = (*Transport)(nil)
