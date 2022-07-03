package web_fetch

import (
	"net/http"
	"sync"
)

// FetchResponseWriter implements ResponseWriter with a Fetch stream.
type FetchResponseWriter struct {
	strm            SRPCFetchService_FetchStream
	header          http.Header
	writeHeaderOnce sync.Once
}

// NewFetchResponseWriter constructs the FetchResponseWriter.
func NewFetchResponseWriter(strm SRPCFetchService_FetchStream) *FetchResponseWriter {
	return &FetchResponseWriter{
		strm:   strm,
		header: http.Header{},
	}
}

// Header returns the header map that will be sent by WriteHeader.
func (w *FetchResponseWriter) Header() http.Header {
	return w.header
}

// WriteHeader sends an HTTP response header with the provided status code.
func (w *FetchResponseWriter) WriteHeader(statusCode int) {
	w.writeHeaderOnce.Do(func() {
		// send response message
		_ = w.strm.Send(BuildFetchResponse_Info(w.header, statusCode))
	})
}

// Write writes the data to the connection as part of an HTTP reply.
func (w *FetchResponseWriter) Write(p []byte) (int, error) {
	// write header if not already written
	w.WriteHeader(200)
	// write data
	err := w.strm.Send(BuildFetchResponse_Data(p))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// _ is a type assertion
var _ http.ResponseWriter = ((*FetchResponseWriter)(nil))
