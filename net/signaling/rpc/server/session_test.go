package signaling_rpc_server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	"github.com/sirupsen/logrus"
)

type testSessionPeerIDKey struct{}

type testSessionStream struct {
	ctx       context.Context
	requests  chan *signaling.SessionRequest
	responses chan *signaling.SessionResponse
}

func newTestSessionStream(ctx context.Context, req *signaling.SessionRequest) *testSessionStream {
	// Queue initialization before exposing the stream to the server.
	requests := make(chan *signaling.SessionRequest, 1)
	requests <- req
	return &testSessionStream{
		ctx:       ctx,
		requests:  requests,
		responses: make(chan *signaling.SessionResponse, 8),
	}
}

func (s *testSessionStream) Context() context.Context {
	return s.ctx
}

func (s *testSessionStream) MsgSend(msg srpc.Message) error {
	// Preserve the generated stream's response type contract.
	response, ok := msg.(*signaling.SessionResponse)
	if !ok {
		return errors.New("unexpected session response type")
	}

	// Retain each response for the test's ordered protocol assertions.
	s.responses <- response
	return nil
}

func (s *testSessionStream) MsgRecv(msg srpc.Message) error {
	// Read one request through the context-aware test transport.
	request, err := s.Recv()
	if err != nil {
		return err
	}

	// Populate the generated destination through its normal codec.
	data, err := request.MarshalVT()
	if err != nil {
		return err
	}
	return msg.UnmarshalVT(data)
}

func (s *testSessionStream) CloseSend() error {
	return nil
}

func (s *testSessionStream) Close() error {
	return nil
}

func (s *testSessionStream) Send(response *signaling.SessionResponse) error {
	return s.MsgSend(response)
}

func (s *testSessionStream) SendAndClose(response *signaling.SessionResponse) error {
	// Deliver an optional final response before closing the sending side.
	if response != nil {
		if err := s.Send(response); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

func (s *testSessionStream) Recv() (*signaling.SessionRequest, error) {
	select {
	case request := <-s.requests:
		return request, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *testSessionStream) RecvTo(request *signaling.SessionRequest) error {
	// Receive one request before copying it through the generated codec.
	data, err := s.Recv()
	if err != nil {
		return err
	}
	encoded, err := data.MarshalVT()
	if err != nil {
		return err
	}
	return request.UnmarshalVT(encoded)
}

// TestSessionOpenedAfterRegistrationBroadcast verifies a monitor reconciles an
// already-open session after its registration broadcast has been consumed.
func TestSessionOpenedAfterRegistrationBroadcast(t *testing.T) {
	// Create distinct authenticated peers for the real server state machine.
	ctx := t.Context()
	privA, pubA, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	peerA, err := peer.IDFromPublicKey(pubA)
	if err != nil {
		t.Fatal(err)
	}
	_, pubB, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPublicKey(pubB)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServerWithIdentify(logrus.NewEntry(logrus.New()), func(ctx context.Context) (peer.ID, error) {
		pid, ok := ctx.Value(testSessionPeerIDKey{}).(peer.ID)
		if !ok {
			return "", errors.New("missing test peer id")
		}
		return pid, nil
	})

	// Observe registration before the corresponding monitor begins waiting.
	sessKey, _ := newSessionKey(peerA.String(), peerB.String())
	server.mtx.Lock()
	sess, _ := server.getSession(sessKey)
	firstRegistrationWait := sess.getWaitCh()
	server.mtx.Unlock()

	// Register the first endpoint and retain its stream across peer replacement.
	ctxA, cancelA := context.WithCancel(context.WithValue(ctx, testSessionPeerIDKey{}, peerA))
	t.Cleanup(cancelA)
	streamA := newTestSessionStream(ctxA, &signaling.SessionRequest{
		Body: &signaling.SessionRequest_Init{Init: &signaling.SessionInit{PeerId: peerB.String()}},
	})
	doneA := make(chan error, 1)
	go func() {
		doneA <- server.Session(streamA)
	}()

	select {
	case <-firstRegistrationWait:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first session registration")
	}

	// Register the second endpoint after the first registration was observed.
	server.mtx.Lock()
	secondRegistrationWait := sess.getWaitCh()
	server.mtx.Unlock()

	ctxB, cancelB := context.WithCancel(context.WithValue(ctx, testSessionPeerIDKey{}, peerB))
	t.Cleanup(cancelB)
	streamB := newTestSessionStream(ctxB, &signaling.SessionRequest{
		Body: &signaling.SessionRequest_Init{Init: &signaling.SessionInit{PeerId: peerA.String()}},
	})
	doneB := make(chan error, 1)
	go func() {
		doneB <- server.Session(streamB)
	}()

	select {
	case <-secondRegistrationWait:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second session registration")
	}

	// Each endpoint must learn the same nonzero generation before sending.
	waitOpened := func(name string, stream *testSessionStream) uint64 {
		t.Helper()
		select {
		case response := <-stream.responses:
			opened, ok := response.GetBody().(*signaling.SessionResponse_Opened)
			if !ok {
				t.Fatalf("%s received %T, want opened response", name, response.GetBody())
			}
			if opened.Opened == 0 {
				t.Fatalf("%s received empty opened sequence number", name)
			}
			return opened.Opened
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s opened response", name)
		}
		return 0
	}
	initial := waitOpened("peer A", streamA)
	if got := waitOpened("peer B", streamB); got != initial {
		t.Fatalf("peer B generation = %d, want %d", got, initial)
	}

	// Replace B while its old stream is still registered. A never observes a
	// closed state, but must learn the replacement generation to send again.
	ctxNextB, cancelNextB := context.WithCancel(context.WithValue(ctx, testSessionPeerIDKey{}, peerB))
	t.Cleanup(cancelNextB)
	streamNextB := newTestSessionStream(ctxNextB, &signaling.SessionRequest{
		Body: &signaling.SessionRequest_Init{Init: &signaling.SessionInit{PeerId: peerA.String()}},
	})
	doneNextB := make(chan error, 1)
	go func() {
		doneNextB <- server.Session(streamNextB)
	}()
	next := waitOpened("peer A after replacement", streamA)
	if next <= initial {
		t.Fatalf("replacement generation = %d, want greater than %d", next, initial)
	}
	if got := waitOpened("replacement peer B", streamNextB); got != next {
		t.Fatalf("replacement peer B generation = %d, want %d", got, next)
	}

	// Deliver a signed message through the replacement generation, then cancel
	// it before acknowledgment. Cancellation must wake the receiver on its own.
	msg, err := signaling.NewSessionMsg(privA, hash.RecommendedHashType, []byte("replacement"), 1)
	if err != nil {
		t.Fatal(err)
	}
	streamA.requests <- &signaling.SessionRequest{
		SessionSeqno: next,
		Body:         &signaling.SessionRequest_SendMsg{SendMsg: msg},
	}
	select {
	case response := <-streamNextB.responses:
		if !response.GetRecvMsg().EqualVT(msg) {
			t.Fatalf("replacement received %v, want signed message", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out receiving replacement message")
	}
	streamA.requests <- &signaling.SessionRequest{
		SessionSeqno: next,
		Body:         &signaling.SessionRequest_ClearMsg{ClearMsg: 1},
	}
	select {
	case response := <-streamNextB.responses:
		if response.GetClearMsg() != 1 {
			t.Fatalf("replacement received %v, want cancellation", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out receiving message cancellation")
	}

	// Stop and join all generations before releasing the test context.
	cancelA()
	cancelB()
	cancelNextB()
	select {
	case <-doneA:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out stopping peer A session")
	}
	select {
	case <-doneB:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out stopping peer B session")
	}
	select {
	case <-doneNextB:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out stopping replacement peer B session")
	}
}

var _ signaling.SRPCSignaling_SessionStream = (*testSessionStream)(nil)
