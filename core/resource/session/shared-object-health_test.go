package resource_session

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
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type testWatchSharedObjectHealthStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_session.WatchSharedObjectHealthResponse
}

func newTestWatchSharedObjectHealthStream(
	ctx context.Context,
) *testWatchSharedObjectHealthStream {
	return &testWatchSharedObjectHealthStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_session.WatchSharedObjectHealthResponse, 16),
	}
}

func (m *testWatchSharedObjectHealthStream) Context() context.Context {
	return m.ctx
}

func (m *testWatchSharedObjectHealthStream) Send(
	resp *s4wave_session.WatchSharedObjectHealthResponse,
) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testWatchSharedObjectHealthStream) SendAndClose(
	resp *s4wave_session.WatchSharedObjectHealthResponse,
) error {
	return m.Send(resp)
}

func (m *testWatchSharedObjectHealthStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testWatchSharedObjectHealthStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testWatchSharedObjectHealthStream) CloseSend() error {
	return nil
}

func (m *testWatchSharedObjectHealthStream) Close() error {
	return nil
}

type testHealthSharedObject struct {
	id        string
	healthCtr *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

func (s *testHealthSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testHealthSharedObject) GetPeerID() peer.ID {
	return ""
}

func (s *testHealthSharedObject) GetSharedObjectID() string {
	return s.id
}

func (s *testHealthSharedObject) GetBlockStore() bstore.BlockStore {
	return nil
}

func (s *testHealthSharedObject) AccessLocalStateStore(
	context.Context,
	string,
	func(),
) (kvtx.Store, func(), error) {
	return nil, nil, errors.New("not implemented")
}

func (s *testHealthSharedObject) GetSharedObjectState(
	context.Context,
) (sobject.SharedObjectStateSnapshot, error) {
	return nil, nil
}

func (s *testHealthSharedObject) AccessSharedObjectState(
	context.Context,
	func(),
) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, errors.New("not implemented")
}

func (s *testHealthSharedObject) QueueOperation(context.Context, []byte) (string, error) {
	return "", errors.New("not implemented")
}

func (s *testHealthSharedObject) WaitOperation(
	context.Context,
	string,
) (uint64, bool, error) {
	return 0, false, errors.New("not implemented")
}

func (s *testHealthSharedObject) ClearOperationResult(
	context.Context,
	string,
) error {
	return errors.New("not implemented")
}

func (s *testHealthSharedObject) ProcessOperations(
	context.Context,
	bool,
	sobject.ProcessOpsFunc,
) error {
	return errors.New("not implemented")
}

func (s *testHealthSharedObject) AccessSharedObjectHealth(
	context.Context,
	func(),
) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	return s.healthCtr, func() {}, nil
}

func recvSharedObjectHealth(
	t *testing.T,
	msgs <-chan *s4wave_session.WatchSharedObjectHealthResponse,
) *sobject.SharedObjectHealth {
	t.Helper()

	select {
	case msg := <-msgs:
		if msg == nil || msg.GetHealth() == nil {
			t.Fatal("expected health payload")
		}
		return msg.GetHealth()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shared object health")
		return nil
	}
}

func TestWatchSharedObjectHealthCdnLookupStreamsLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	readyHealth := sobject.NewSharedObjectReadyHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	)
	healthCtr := ccontainer.NewCContainer[*sobject.SharedObjectHealth](readyHealth)
	so := &testHealthSharedObject{
		id:        "cdn-space",
		healthCtr: healthCtr,
	}
	r := &SessionResource{
		cdnLookup: func(sharedObjectID string) (sobject.SharedObject, *sobject.SharedObjectMeta) {
			if sharedObjectID != so.id {
				return nil, nil
			}
			return so, nil
		},
	}
	strm := newTestWatchSharedObjectHealthStream(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.WatchSharedObjectHealth(
			&s4wave_session.WatchSharedObjectHealthRequest{SharedObjectId: so.id},
			strm,
		)
	}()

	initial := recvSharedObjectHealth(t, strm.msgs)
	if initial.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected initial loading status, got %v", initial.GetStatus())
	}

	ready := recvSharedObjectHealth(t, strm.msgs)
	if ready.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready status, got %v", ready.GetStatus())
	}

	closedHealth := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"block not found",
	)
	healthCtr.SetValue(closedHealth)

	closed := recvSharedObjectHealth(t, strm.msgs)
	if closed.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", closed.GetStatus())
	}
	if closed.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND {
		t.Fatalf("expected block-not-found reason, got %v", closed.GetCommonReason())
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchSharedObjectHealth() = %v, want context canceled", err)
	}
}

// _ is a type assertion
var _ s4wave_session.SRPCSessionResourceService_WatchSharedObjectHealthStream = (*testWatchSharedObjectHealthStream)(nil)

// _ is a type assertion
var _ sobject.SharedObject = (*testHealthSharedObject)(nil)

// _ is a type assertion
var _ sobject.SharedObjectHealthAccessor = (*testHealthSharedObject)(nil)
