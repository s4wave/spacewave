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
func (r *FetchBodyReader) Read(p []byte) (n int, err error) {
	toRead := p
	// while we can still read more data
	for len(toRead) != 0 {
		// if the buffer is empty and we have unbuffered none, read more.
		if r.buf.Len() == 0 {
			if n != 0 {
				break
			}
			if r.done {
				break
			}
			pkt, err := r.strm.Recv()
			if err != nil {
				return n, err
			}
			if pkt.GetRequestData().GetDone() {
				r.done = true
			}
			data := pkt.GetRequestData().GetData()
			if len(data) == 0 {
				continue
			}
			// if len(toRead) <= len(data), read fully w/o buffering
			if len(toRead) <= len(data) {
				copy(toRead, data)
				n += len(data)
				toRead = toRead[len(data):]
			} else {
				// otherwise buffer it & continue
				_, err = r.buf.Write(data)
				if err != nil {
					return n, err
				}
			}
		}
		// read from the buffer to toRead
		rn, err := r.buf.Read(toRead)
		if err != nil {
			return n, err
		}
		// advance toRead by rn
		n += rn
		toRead = toRead[rn:]
	}
	return n, nil
}

// _ is a type assertion
var _ io.Reader = ((*FetchBodyReader)(nil))
