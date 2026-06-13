package web_fetch

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/pkg/errors"
)

const maxFetchResponseDataPacketBytes = 16 * 1024

// FetchResponseWriter implements ResponseWriter with a Fetch stream.
type FetchResponseWriter struct {
	strm            SRPCFetchService_FetchStream
	header          http.Header
	writeHeaderOnce sync.Once
	status          int
	// declaredLen is the parsed Content-Length header captured when the
	// response headers are sent, or -1 when no valid length was declared.
	declaredLen int64
	written     int64
	err         error
}

// NewFetchResponseWriter constructs the FetchResponseWriter.
func NewFetchResponseWriter(strm SRPCFetchService_FetchStream) *FetchResponseWriter {
	return &FetchResponseWriter{
		strm:        strm,
		header:      http.Header{},
		declaredLen: -1,
	}
}

// Header returns the header map that will be sent by WriteHeader.
func (w *FetchResponseWriter) Header() http.Header {
	return w.header
}

// WriteHeader sends an HTTP response header with the provided status code.
func (w *FetchResponseWriter) WriteHeader(statusCode int) {
	w.writeHeaderOnce.Do(func() {
		w.status = statusCode
		if cl := w.header.Get("Content-Length"); cl != "" {
			if v, err := strconv.ParseInt(cl, 10, 64); err == nil && v >= 0 {
				w.declaredLen = v
			}
		}
		// send response message
		if err := w.strm.Send(BuildFetchResponse_Info(w.header, statusCode)); err != nil && w.err == nil {
			w.err = err
		}
	})
}

// Write writes the data to the connection as part of an HTTP reply.
func (w *FetchResponseWriter) Write(p []byte) (int, error) {
	// write header if not already written
	w.WriteHeader(200)
	if w.err != nil {
		return 0, w.err
	}

	written := 0
	for written < len(p) {
		end := min(written+maxFetchResponseDataPacketBytes, len(p))
		if err := w.strm.Send(BuildFetchResponse_Data(p[written:end], false)); err != nil {
			w.err = err
			w.written += int64(written)
			return written, err
		}
		written = end
	}
	w.written += int64(written)
	return written, nil
}

// BodyAborter is a ResponseWriter that can mark an in-flight response body as
// failed after headers were already sent. Proxies use it to prevent a
// truncated upstream body from completing with a clean done packet.
type BodyAborter interface {
	http.ResponseWriter
	// Abort poisons the response body with the given error.
	Abort(err error)
}

// Abort poisons the response body so BodyError reports the failure and the
// stream closes with an error instead of the clean final done packet.
func (w *FetchResponseWriter) Abort(err error) {
	if w.err == nil && err != nil {
		w.err = err
	}
}

// BodyError returns the error that poisoned the response body, if any.
//
// A non-nil result means the stream must close with an error instead of the
// clean final done packet: either a header/body packet send failed, or the
// handler declared a Content-Length and wrote a different number of body
// bytes, so the remote would otherwise accept a truncated body as complete.
func (w *FetchResponseWriter) BodyError(reqMethod string) error {
	if w.err != nil {
		return w.err
	}
	if reqMethod == http.MethodHead || !statusAllowsBody(w.status) {
		return nil
	}
	if w.declaredLen >= 0 && w.written != w.declaredLen {
		return errors.Errorf(
			"fetch response body incomplete: wrote %d of %d declared bytes",
			w.written, w.declaredLen,
		)
	}
	return nil
}

// statusAllowsBody reports whether the HTTP status code permits a body,
// mirroring net/http bodyAllowedForStatus.
func statusAllowsBody(status int) bool {
	return status >= 200 && status != http.StatusNoContent && status != http.StatusNotModified
}

// _ is a type assertion
var _ http.ResponseWriter = ((*FetchResponseWriter)(nil))
