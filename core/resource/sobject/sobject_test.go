package resource_sobject

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

type testSharedObjectHealthStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_sobject.WatchSharedObjectHealthResponse
}

func newTestSharedObjectHealthStream(
	ctx context.Context,
) *testSharedObjectHealthStream {
	return &testSharedObjectHealthStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_sobject.WatchSharedObjectHealthResponse, 16),
	}
}

func (m *testSharedObjectHealthStream) Context() context.Context {
	return m.ctx
}

func (m *testSharedObjectHealthStream) Send(
	resp *s4wave_sobject.WatchSharedObjectHealthResponse,
) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testSharedObjectHealthStream) SendAndClose(
	resp *s4wave_sobject.WatchSharedObjectHealthResponse,
) error {
	return m.Send(resp)
}

func (m *testSharedObjectHealthStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testSharedObjectHealthStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testSharedObjectHealthStream) CloseSend() error {
	return nil
}

func (m *testSharedObjectHealthStream) Close() error {
	return nil
}

type testMountedSharedObject struct {
	id        string
	healthCtr *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

func (s *testMountedSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testMountedSharedObject) GetPeerID() peer.ID {
	return ""
}

func (s *testMountedSharedObject) GetSharedObjectID() string {
	return s.id
}

func (s *testMountedSharedObject) GetBlockStore() bstore.BlockStore {
	return nil
}

func (s *testMountedSharedObject) AccessLocalStateStore(
	context.Context,
	string,
	func(),
) (kvtx.Store, func(), error) {
	return nil, nil, errors.New("not implemented")
}

func (s *testMountedSharedObject) GetSharedObjectState(
	context.Context,
) (sobject.SharedObjectStateSnapshot, error) {
	return nil, nil
}

func (s *testMountedSharedObject) AccessSharedObjectState(
	context.Context,
	func(),
) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, errors.New("not implemented")
}

func (s *testMountedSharedObject) QueueOperation(context.Context, []byte) (string, error) {
	return "", errors.New("not implemented")
}

func (s *testMountedSharedObject) WaitOperation(
	context.Context,
	string,
) (uint64, bool, error) {
	return 0, false, errors.New("not implemented")
}

func (s *testMountedSharedObject) ClearOperationResult(
	context.Context,
	string,
) error {
	return errors.New("not implemented")
}

func (s *testMountedSharedObject) ProcessOperations(
	context.Context,
	bool,
	sobject.ProcessOpsFunc,
) error {
	return errors.New("not implemented")
}

func (s *testMountedSharedObject) AccessSharedObjectHealth(
	context.Context,
	func(),
) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	return s.healthCtr, func() {}, nil
}

func recvMountedSharedObjectHealth(
	t *testing.T,
	msgs <-chan *s4wave_sobject.WatchSharedObjectHealthResponse,
) *sobject.SharedObjectHealth {
	t.Helper()

	select {
	case msg := <-msgs:
		if msg == nil || msg.GetHealth() == nil {
			t.Fatal("expected health payload")
		}
		return msg.GetHealth()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mounted shared object health")
		return nil
	}
}

func TestWatchSharedObjectHealthStreamsMountedLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	healthCtr := ccontainer.NewCContainer[*sobject.SharedObjectHealth](
		sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	)
	r := &SharedObjectResource{
		sharedObject: &testMountedSharedObject{
			id:        "so-mounted",
			healthCtr: healthCtr,
		},
	}
	strm := newTestSharedObjectHealthStream(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.WatchSharedObjectHealth(
			&s4wave_sobject.WatchSharedObjectHealthRequest{},
			strm,
		)
	}()

	ready := recvMountedSharedObjectHealth(t, strm.msgs)
	if ready.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready status, got %v", ready.GetStatus())
	}

	healthCtr.SetValue(sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"unsupported shared object type: weird.body",
	))

	closed := recvMountedSharedObjectHealth(t, strm.msgs)
	if closed.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", closed.GetStatus())
	}
	if closed.GetLayer() != sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
		t.Fatalf("expected body layer, got %v", closed.GetLayer())
	}
	if closed.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED {
		t.Fatalf("expected body-config reason, got %v", closed.GetCommonReason())
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchSharedObjectHealth() = %v, want context canceled", err)
	}
}

// _ is a type assertion
var _ s4wave_sobject.SRPCSharedObjectResourceService_WatchSharedObjectHealthStream = (*testSharedObjectHealthStream)(nil)

// _ is a type assertion
var _ sobject.SharedObject = (*testMountedSharedObject)(nil)

// _ is a type assertion
var _ sobject.SharedObjectHealthAccessor = (*testMountedSharedObject)(nil)
