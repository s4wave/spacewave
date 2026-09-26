package web_fetch

import (
	"bytes"
	"io"
)

// FetchBodyReader implements io.Reader with a FetchStream.
type FetchBodyReader struct {
	// strm is the rpc stream
	strm SRPCFetchService_FetchStream
	// buf is the incoming data buffer
	buf bytes.Buffer
	// done indicates there will be no more data.
	done bool
}

// NewFetchBodyReader constructs the FetchBodyReader.
func NewFetchBodyReader(strm SRPCFetchService_FetchStream) *FetchBodyReader {
	return &FetchBodyReader{strm: strm}
}

// Read reads data from the reader.
//
// Returns io.EOF once the stream is done and all buffered data is consumed.
func (r *FetchBodyReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// wait for at least one byte of data or the end of the stream.
	for r.buf.Len() == 0 {
		if r.done {
			return 0, io.EOF
		}
		pkt, err := r.strm.Recv()
		if err != nil {
			return 0, err
		}
		reqData := pkt.GetRequestData()
		if reqData.GetDone() {
			r.done = true
		}
		// buffer the full chunk so that data exceeding p is kept for later reads.
		_, _ = r.buf.Write(reqData.GetData())
	}
	return r.buf.Read(p)
}

// _ is a type assertion
var _ io.Reader = (*FetchBodyReader)(nil)
