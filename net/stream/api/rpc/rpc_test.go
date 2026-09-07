package stream_api_rpc

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
)

// finalReadStream returns the final bytes and EOF in the same read.
type finalReadStream struct {
	done   chan struct{}
	closed atomic.Bool
}

func (s *finalReadStream) Read(p []byte) (int, error)  { return copy(p, "final reply"), io.EOF }
func (s *finalReadStream) Write(p []byte) (int, error) { return len(p), nil }
func (s *finalReadStream) Close() error {
	if !s.closed.Swap(true) {
		close(s.done)
	}
	return nil
}

type finalReadRPC struct {
	stream *finalReadStream
	sent   chan string
}

func (r *finalReadRPC) Context() context.Context { return context.Background() }
func (r *finalReadRPC) Send(d *Data) error       { r.sent <- string(d.GetData()); return nil }
func (r *finalReadRPC) Recv() (*Data, error)     { <-r.stream.done; return nil, io.EOF }

func TestAttachRPCForwardsFinalBytesBeforeEOF(t *testing.T) {
	stream := &finalReadStream{done: make(chan struct{})}
	rpc := &finalReadRPC{stream: stream, sent: make(chan string, 1)}
	if err := AttachRPCToStream(rpc, stream, nil); err != io.EOF {
		t.Fatalf("forward error = %v, want EOF", err)
	}
	select {
	case got := <-rpc.sent:
		if got != "final reply" {
			t.Fatalf("forwarded %q, want final reply", got)
		}
	default:
		t.Fatal("final reply was dropped")
	}
}
