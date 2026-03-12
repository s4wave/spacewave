package saucer

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net/http"

	web_fetch "github.com/aperturerobotics/bldr/web/fetch"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// MaxFrameSize is the maximum size of a length-prefixed frame.
var MaxFrameSize uint32 = 10 * 1024 * 1024 // 10MB

// RequestHandler accepts yamux streams from C++ and routes FetchRequest/FetchResponse.
type RequestHandler struct {
	le      *logrus.Entry
	docMgr  *DocumentManager
	handler http.HandlerFunc

	bootstrapHTML string
	entrypointJS  string
}

// NewRequestHandler constructs a new RequestHandler.
func NewRequestHandler(
	le *logrus.Entry,
	docMgr *DocumentManager,
	handler http.HandlerFunc,
	bootstrapHTML string,
	entrypointJS string,
) *RequestHandler {
	return &RequestHandler{
		le:            le,
		docMgr:        docMgr,
		handler:       handler,
		bootstrapHTML: bootstrapHTML,
		entrypointJS:  entrypointJS,
	}
}

// AcceptStreams accepts yamux streams from the muxed connection and handles each.
func (h *RequestHandler) AcceptStreams(ctx context.Context, mc srpc.MuxedConn) error {
	for {
		stream, err := mc.AcceptStream()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
		go h.handleStream(ctx, stream)
	}
}

// handleStream handles a single yamux stream carrying FetchRequest/FetchResponse.
func (h *RequestHandler) handleStream(ctx context.Context, rwc io.ReadWriteCloser) {
	defer rwc.Close()

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// Read the first frame: FetchRequestInfo.
	reqMsg, err := readFrame(rwc)
	if err != nil {
		h.le.WithError(err).Debug("failed to read request frame")
		return
	}

	req := &web_fetch.FetchRequest{}
	if err := req.UnmarshalVT(reqMsg); err != nil {
		h.le.WithError(err).Debug("failed to unmarshal fetch request")
		return
	}

	info := req.GetRequestInfo()
	if info == nil {
		h.le.Debug("first fetch request frame missing request_info")
		return
	}

	// Build a streaming body reader if the request has a body.
	var body io.Reader
	if info.GetHasBody() {
		body = &fetchBodyReader{rwc: rwc}
	}

	httpReq, err := info.ToHttpRequest(subCtx, body)
	if err != nil {
		h.le.WithError(err).Debug("failed to build http request from fetch info")
		return
	}

	// Build a response writer that sends FetchResponse frames.
	rw := newFramedResponseWriter(rwc)

	// Add CORS headers.
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	rw.Header().Set("Access-Control-Allow-Headers", "*")

	// Route the request.
	path := httpReq.URL.Path

	switch {
	case path == "/" || path == "" || path == "/index.html":
		h.serveBootstrapHTML(rw, httpReq)
	case path == "/entrypoint.mjs":
		h.serveEntrypointJS(rw, httpReq)
	case httpReq.Method == "OPTIONS":
		rw.WriteHeader(204)
	default:
		// Delegate to the http handler (includes /b/saucer/* and ServiceWorkerHost routes).
		h.handler.ServeHTTP(rw, httpReq)
	}

	// Send final done frame.
	rw.finish()
}

// serveBootstrapHTML serves the bootstrap HTML at / or /index.html.
func (h *RequestHandler) serveBootstrapHTML(rw http.ResponseWriter, req *http.Request) {
	if h.bootstrapHTML == "" {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("Bootstrap HTML not configured"))
		return
	}
	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(200)
	_, _ = rw.Write([]byte(h.bootstrapHTML))
}

// serveEntrypointJS serves the entrypoint JS module at /entrypoint.mjs.
func (h *RequestHandler) serveEntrypointJS(rw http.ResponseWriter, req *http.Request) {
	if h.entrypointJS == "" {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("Entrypoint not configured"))
		return
	}
	rw.Header().Set("Content-Type", "text/javascript")
	rw.WriteHeader(200)
	_, _ = rw.Write([]byte(h.entrypointJS))
}

// fetchBodyReader reads FetchRequestData frames from the stream as an io.Reader.
type fetchBodyReader struct {
	rwc    io.Reader
	buf    []byte
	done   bool
	bufOff int
}

// Read implements io.Reader.
func (r *fetchBodyReader) Read(p []byte) (int, error) {
	// Drain buffer first.
	if r.bufOff < len(r.buf) {
		n := copy(p, r.buf[r.bufOff:])
		r.bufOff += n
		return n, nil
	}

	if r.done {
		return 0, io.EOF
	}

	// Read next frame.
	frame, err := readFrame(r.rwc)
	if err != nil {
		return 0, err
	}

	msg := &web_fetch.FetchRequest{}
	if err := msg.UnmarshalVT(frame); err != nil {
		return 0, err
	}

	data := msg.GetRequestData()
	if data == nil {
		return 0, io.EOF
	}

	if data.GetDone() {
		r.done = true
	}

	r.buf = data.GetData()
	r.bufOff = 0

	n := copy(p, r.buf)
	r.bufOff = n
	if n == 0 && r.done {
		return 0, io.EOF
	}
	return n, nil
}

// framedResponseWriter sends FetchResponse frames over a stream.
type framedResponseWriter struct {
	w      io.Writer
	header http.Header
	sent   bool
	err    error
}

// newFramedResponseWriter constructs a framedResponseWriter.
func newFramedResponseWriter(w io.Writer) *framedResponseWriter {
	return &framedResponseWriter{
		w:      w,
		header: http.Header{},
	}
}

// Header returns the header map for the response.
func (rw *framedResponseWriter) Header() http.Header {
	return rw.header
}

// WriteHeader sends the response headers as a FetchResponse_ResponseInfo frame.
func (rw *framedResponseWriter) WriteHeader(statusCode int) {
	if rw.sent {
		return
	}
	rw.sent = true

	resp := web_fetch.BuildFetchResponse_Info(rw.header, statusCode)
	data, err := resp.MarshalVT()
	if err != nil {
		rw.err = err
		return
	}
	if err := writeFrame(rw.w, data); err != nil {
		rw.err = err
	}
}

// Write writes response body data as FetchResponse_ResponseData frames.
func (rw *framedResponseWriter) Write(p []byte) (int, error) {
	if !rw.sent {
		rw.WriteHeader(200)
	}
	if rw.err != nil {
		return 0, rw.err
	}

	resp := web_fetch.BuildFetchResponse_Data(p, false)
	data, err := resp.MarshalVT()
	if err != nil {
		return 0, err
	}
	if err := writeFrame(rw.w, data); err != nil {
		return 0, err
	}
	return len(p), nil
}

// finish sends the final done frame.
func (rw *framedResponseWriter) finish() {
	if !rw.sent {
		rw.WriteHeader(200)
	}

	resp := web_fetch.BuildFetchResponse_Data(nil, true)
	data, err := resp.MarshalVT()
	if err != nil {
		return
	}
	_ = writeFrame(rw.w, data)
}

// readFrame reads a LittleEndian uint32 length-prefixed frame.
func readFrame(r io.Reader) ([]byte, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lenBuf)
	if msgLen > MaxFrameSize {
		return nil, io.ErrShortBuffer
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

// writeFrame writes a LittleEndian uint32 length-prefixed frame.
func writeFrame(w io.Writer, data []byte) error {
	if len(data) > math.MaxUint32 {
		return io.ErrShortBuffer
	}
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data))) //nolint:gosec
	if _, err := w.Write(lenBuf); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// _ is a type assertion
var _ http.ResponseWriter = ((*framedResponseWriter)(nil))
