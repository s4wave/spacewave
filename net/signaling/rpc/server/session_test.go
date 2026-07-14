package signaling_rpc_server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/crypto"
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
	response, ok := msg.(*signaling.SessionResponse)
	if !ok {
		return errors.New("unexpected session response type")
	}
	s.responses <- response
	return nil
}

func (s *testSessionStream) MsgRecv(msg srpc.Message) error {
	request, err := s.Recv()
	if err != nil {
		return err
	}
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
	ctx := t.Context()
	_, pubA, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
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

	sessKey, _ := newSessionKey(peerA.String(), peerB.String())
	server.mtx.Lock()
	sess, _ := server.getSession(sessKey)
	firstRegistrationWait := sess.getWaitCh()
	server.mtx.Unlock()

	ctxA, cancelA := context.WithCancel(context.WithValue(ctx, testSessionPeerIDKey{}, peerA))
	defer cancelA()
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

	server.mtx.Lock()
	secondRegistrationWait := sess.getWaitCh()
	server.mtx.Unlock()

	ctxB, cancelB := context.WithCancel(context.WithValue(ctx, testSessionPeerIDKey{}, peerB))
	defer cancelB()
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

	waitOpened := func(name string, stream *testSessionStream) {
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
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s opened response", name)
		}
	}
	waitOpened("peer A", streamA)
	waitOpened("peer B", streamB)

	cancelA()
	cancelB()
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
}

var _ signaling.SRPCSignaling_SessionStream = (*testSessionStream)(nil)
