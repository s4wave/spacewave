package resource_session_test

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// mockWatchStream implements the WatchTransferProgress server stream for testing.
type mockWatchStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_session.WatchTransferProgressResponse
	done chan struct{}
}

// newMockWatchStream creates a new mock stream.
func newMockWatchStream(ctx context.Context) *mockWatchStream {
	return &mockWatchStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_session.WatchTransferProgressResponse, 100),
		done: make(chan struct{}),
	}
}

// Context returns the stream context.
func (m *mockWatchStream) Context() context.Context {
	return m.ctx
}

// Send sends a response on the mock stream.
func (m *mockWatchStream) Send(resp *s4wave_session.WatchTransferProgressResponse) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

// SendAndClose sends a final response.
func (m *mockWatchStream) SendAndClose(resp *s4wave_session.WatchTransferProgressResponse) error {
	return m.Send(resp)
}

// MsgRecv reads a protobuf message from the stream.
func (m *mockWatchStream) MsgRecv(msg srpc.Message) error {
	return nil
}

// MsgSend writes a protobuf message to the stream.
func (m *mockWatchStream) MsgSend(msg srpc.Message) error {
	return nil
}

// CloseSend signals the writer is done sending.
func (m *mockWatchStream) CloseSend() error {
	return nil
}

// Close closes the stream.
func (m *mockWatchStream) Close() error {
	return nil
}

// _ is a type assertion
var _ s4wave_session.SRPCSessionResourceService_WatchTransferProgressStream = (*mockWatchStream)(nil)
