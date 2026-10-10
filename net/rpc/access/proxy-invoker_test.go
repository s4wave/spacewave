package bifrost_rpc_access

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// responseFirstStream is the invoking side of an Echo call whose request ends
// only after the remote service has answered.
type responseFirstStream struct {
	// Stream is nil: the proxy uses only the methods overridden here.
	srpc.Stream
	// ctx is the invoking call's context.
	ctx context.Context
	// requests counts the request messages received so far.
	requests int
	// answered is closed when the remote response reaches the invoking stream.
	answered chan struct{}
}

// Context returns the invoking call's context.
func (s *responseFirstStream) Context() context.Context {
	return s.ctx
}

// MsgSend delivers the remote response and releases the end of the request.
func (s *responseFirstStream) MsgSend(srpc.Message) error {
	close(s.answered)
	return nil
}

// MsgRecv returns the request, then waits for the response to end the request.
func (s *responseFirstStream) MsgRecv(msg srpc.Message) error {
	s.requests++
	if s.requests == 1 {
		data, err := (&echo.EchoMsg{Body: "hello world"}).MarshalVT()
		if err != nil {
			return err
		}
		return msg.UnmarshalVT(data)
	}
	select {
	case <-s.answered:
		return io.EOF
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// TestProxyInvokerRemoteCompletesFirst checks that a call whose remote service
// completes before the request ends reports the remote's result.
func TestProxyInvokerRemoteCompletesFirst(t *testing.T) {
	// Bound the test so a lost completion fails instead of hanging.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	log := logrus.New()
	client := newAccessTestServer(ctx, t, logrus.NewEntry(log))
	invoker := NewProxyInvoker(client, &LookupRpcServiceRequest{ServiceId: echo.SRPCEchoerServiceID}, true)

	// Invoke Echo repeatedly through the proxy.
	for i := range 2000 {
		strm := &responseFirstStream{ctx: ctx, answered: make(chan struct{})}
		found, err := invoker.InvokeMethod(echo.SRPCEchoerServiceID, "Echo", strm)
		if !found || err != nil {
			t.Fatalf("call %d: found=%v err=%v", i, found, err)
		}
	}
}
