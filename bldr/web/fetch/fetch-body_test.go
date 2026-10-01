package web_fetch

import (
	"bytes"
	"io"
	"testing"
)

// fakeFetchBodyStream returns queued request data packets from Recv.
type fakeFetchBodyStream struct {
	SRPCFetchService_FetchStream
	pkts []*FetchRequest
}

// Recv returns the next queued packet.
func (s *fakeFetchBodyStream) Recv() (*FetchRequest, error) {
	if len(s.pkts) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	pkt := s.pkts[0]
	s.pkts = s.pkts[1:]
	return pkt, nil
}

func newRequestDataPkt(data []byte, done bool) *FetchRequest {
	return &FetchRequest{Body: &FetchRequest_RequestData{
		RequestData: &FetchRequestData{Data: data, Done: done},
	}}
}

// TestFetchBodyReaderSmallBuffer checks chunks larger than the read buffer
// are retained for subsequent reads.
func TestFetchBodyReaderSmallBuffer(t *testing.T) {
	// Queue request data packets on a fake fetch stream.
	strm := &fakeFetchBodyStream{pkts: []*FetchRequest{
		newRequestDataPkt([]byte("hello world"), false),
		newRequestDataPkt(nil, false),
		newRequestDataPkt([]byte("!!"), true),
	}}
	rdr := NewFetchBodyReader(strm)

	// Read the body in small chunks until the stream ends.
	var out bytes.Buffer
	buf := make([]byte, 3)
	for {
		n, err := rdr.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Check the reassembled body matches the sent chunks.
	if got := out.String(); got != "hello world!!" {
		t.Fatalf("unexpected body: %q", got)
	}
}

// TestFetchBodyReaderReadAll checks the reader terminates with io.EOF.
func TestFetchBodyReaderReadAll(t *testing.T) {
	// Queue request data packets on a fake fetch stream.
	strm := &fakeFetchBodyStream{pkts: []*FetchRequest{
		newRequestDataPkt([]byte("abc"), false),
		newRequestDataPkt([]byte("def"), true),
	}}

	// Read the whole body and check the reassembled content.
	data, err := io.ReadAll(NewFetchBodyReader(strm))
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(data) != "abcdef" {
		t.Fatalf("unexpected body: %q", string(data))
	}
}
