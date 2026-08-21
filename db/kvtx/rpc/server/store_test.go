package kvtx_rpc_server

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

func TestTxHandleCloseOpsWaitsForActiveStreams(t *testing.T) {
	h := &txHandle{
		active: make(map[uint64]func()),
		idle:   make(chan struct{}),
	}

	released := make(chan struct{})
	_, release, err := h.acquire(func() {
		close(released)
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	closed := make(chan struct{})
	go func() {
		h.closeOps()
		close(closed)
	}()

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("closeOps did not release active stream")
	}

	select {
	case <-closed:
		t.Fatal("closeOps returned before active stream released")
	default:
	}

	release()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("closeOps did not return after active stream released")
	}

	_, _, err = h.acquire(nil)
	if !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatalf("acquire after close error = %v, want %v", err, kvtx.ErrDiscarded)
	}
}

func TestRetryClassForError(t *testing.T) {
	if got := retryClassForError(errors.Join(errors.New("backend detail"), kvtx.ErrInvalidSnapshot)); got != kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_INVALID_SNAPSHOT {
		t.Fatalf("retry class = %v, want invalid snapshot", got)
	}
	if got := retryClassForError(errors.New("backend detail")); got != kvtx_rpc.KvtxRetryClass_KVTX_RETRY_CLASS_UNSPECIFIED {
		t.Fatalf("retry class = %v, want unspecified", got)
	}
}

type transactionTestStream struct {
	ctx       context.Context
	requests  chan *kvtx_rpc.KvtxTransactionRequest
	responses chan *kvtx_rpc.KvtxTransactionResponse
}

func (s *transactionTestStream) Context() context.Context   { return s.ctx }
func (s *transactionTestStream) MsgSend(srpc.Message) error { return errors.New("unexpected MsgSend") }

func (s *transactionTestStream) MsgRecv(srpc.Message) error { return errors.New("unexpected MsgRecv") }
func (s *transactionTestStream) CloseSend() error           { return nil }
func (s *transactionTestStream) Close() error               { return nil }
func (s *transactionTestStream) Send(resp *kvtx_rpc.KvtxTransactionResponse) error {
	s.responses <- resp
	return nil
}

func (s *transactionTestStream) SendAndClose(resp *kvtx_rpc.KvtxTransactionResponse) error {
	return s.Send(resp)
}

func (s *transactionTestStream) Recv() (*kvtx_rpc.KvtxTransactionRequest, error) {
	select {
	case req := <-s.requests:
		return req, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *transactionTestStream) RecvTo(req *kvtx_rpc.KvtxTransactionRequest) error {
	got, err := s.Recv()
	if err == nil {
		*req = *got
	}
	return err
}

type blockingScanStream struct {
	ctx          context.Context
	request      *kvtx_rpc.KvtxScanPrefixRequest
	sendStarted  chan struct{}
	allowReturn  chan struct{}
	requestTaken bool
}

func (s *blockingScanStream) Context() context.Context { return s.ctx }
func (s *blockingScanStream) MsgSend(srpc.Message) error {
	select {
	case <-s.sendStarted:
	default:
		close(s.sendStarted)
	}
	<-s.ctx.Done()
	<-s.allowReturn
	return s.ctx.Err()
}

func (s *blockingScanStream) MsgRecv(msg srpc.Message) error {
	if s.requestTaken {
		return io.EOF
	}
	req, ok := msg.(*kvtx_rpc.KvtxScanPrefixRequest)
	if !ok {
		return errors.New("unexpected scan request type")
	}
	*req = *s.request
	s.requestTaken = true
	return nil
}
func (s *blockingScanStream) CloseSend() error { return nil }
func (s *blockingScanStream) Close() error     { return nil }

func TestKvtxTransactionDiscardWaitsForActiveScanPrefix(t *testing.T) {
	// Open and seed the real bbolt transaction store.
	boltStore, err := kvtx_bolt.Open(t.TempDir()+"/kvtx.db", 0o600, nil, []byte("kvtx"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = boltStore.GetDB().Close() })

	seed, err := boltStore.NewTransaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Set(context.Background(), []byte("prefix/key"), []byte("value")); err != nil {
		seed.Discard()
		t.Fatal(err)
	}
	if err := seed.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Start one retained RPC transaction and capture its operations route.
	store := NewStore(boltStore)
	txStream := &transactionTestStream{
		ctx:       context.Background(),
		requests:  make(chan *kvtx_rpc.KvtxTransactionRequest, 2),
		responses: make(chan *kvtx_rpc.KvtxTransactionResponse, 2),
	}
	txDone := make(chan error, 1)
	go func() { txDone <- store.KvtxTransaction(txStream) }()
	txStream.requests <- &kvtx_rpc.KvtxTransactionRequest{
		Body: &kvtx_rpc.KvtxTransactionRequest_Init{Init: &kvtx_rpc.KvtxTransactionInit{}},
	}
	ack := <-txStream.responses
	txID := ack.GetAck().GetTransactionId()
	if txID == "" {
		t.Fatalf("transaction ack = %v, want transaction id", ack)
	}

	// Hold ScanPrefix inside its response send so discard must join it.
	scanCtx, cancelScan := context.WithCancel(context.Background())
	scanStream := &blockingScanStream{
		ctx:         scanCtx,
		request:     &kvtx_rpc.KvtxScanPrefixRequest{Prefix: []byte("prefix/")},
		sendStarted: make(chan struct{}),
		allowReturn: make(chan struct{}),
	}
	invoker, release, err := store.GetKvtxOpsMux(context.Background(), txID, cancelScan)
	if err != nil {
		t.Fatal(err)
	}
	scanDone := make(chan error, 1)
	go func() {
		defer release()
		found, err := invoker.InvokeMethod(kvtx_rpc.SRPCKvtxOpsServiceID, "ScanPrefix", scanStream)
		if !found && err == nil {
			err = errors.New("ScanPrefix method not found")
		}
		scanDone <- err
	}()
	select {
	case <-scanStream.sendStarted:
	case <-time.After(time.Second):
		t.Fatal("ScanPrefix did not start sending the bbolt result")
	}

	// Request discard and require cancellation without an early acknowledgement.
	txStream.requests <- &kvtx_rpc.KvtxTransactionRequest{
		Body: &kvtx_rpc.KvtxTransactionRequest_Discard{Discard: true},
	}
	select {
	case <-scanCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("discard did not cancel the active ScanPrefix")
	}
	select {
	case resp := <-txStream.responses:
		t.Fatalf("discard completed before ScanPrefix returned: %v", resp)
	default:
	}

	// Let the scan return, then require discard and the transaction stream to finish.
	close(scanStream.allowReturn)
	select {
	case <-scanDone:
	case <-time.After(time.Second):
		t.Fatal("ScanPrefix did not return after cancellation")
	}
	select {
	case resp := <-txStream.responses:
		if !resp.GetComplete().GetDiscarded() {
			t.Fatalf("transaction response = %v, want discarded", resp)
		}
	case <-time.After(time.Second):
		t.Fatal("discard did not complete after ScanPrefix returned")
	}
	select {
	case err := <-txDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("KvtxTransaction did not return")
	}
}
