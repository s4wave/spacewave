package s4wave_secret

import (
	"context"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
)

func TestSecretPayloadStoreOperationStopsProcessorAfterStore(t *testing.T) {
	ctx := t.Context()
	payload := &SecretPayload{Value: []byte("stored"), ContentType: "text/plain", Version: 1}
	data, err := payload.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	so := newTestSecretPayloadStoreSharedObject()
	op := newSecretPayloadStoreOperation(so, so.stateCtr, payload)

	if err := op.Store(ctx, data); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !so.processExited() {
		t.Fatal("expected processor routine to exit before Store returned")
	}

	stored, err := ReadSecretPayloadFromSnapshot(ctx, so.stateCtr.GetValue())
	if err != nil {
		t.Fatalf("ReadSecretPayloadFromSnapshot: %v", err)
	}
	if !stored.EqualVT(payload) {
		t.Fatalf("stored payload mismatch: got %v want %v", stored, payload)
	}
}

func TestSecretPayloadStoreOperationReturnsProcessorError(t *testing.T) {
	ctx := t.Context()
	payload := &SecretPayload{Value: []byte("stored"), Version: 1}
	data, err := payload.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	processErr := stderrors.New("processor failed")
	so := newTestSecretPayloadStoreSharedObject()
	so.processErr = processErr
	so.failBeforeState = true
	op := newSecretPayloadStoreOperation(so, so.stateCtr, payload)

	if err := op.Store(ctx, data); !stderrors.Is(err, processErr) {
		t.Fatalf("expected processor error, got %v", err)
	}
}

func TestSecretPayloadStoreOperationReturnsPostStoreProcessorError(t *testing.T) {
	ctx := t.Context()
	payload := &SecretPayload{Value: []byte("stored"), Version: 1}
	data, err := payload.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	processErr := stderrors.New("processor failed after state publish")
	so := newTestSecretPayloadStoreSharedObject()
	so.processErr = processErr
	op := newSecretPayloadStoreOperation(so, so.stateCtr, payload)

	if err := op.Store(ctx, data); !stderrors.Is(err, processErr) {
		t.Fatalf("expected processor error, got %v", err)
	}
	stored, err := ReadSecretPayloadFromSnapshot(ctx, so.stateCtr.GetValue())
	if err != nil {
		t.Fatalf("ReadSecretPayloadFromSnapshot: %v", err)
	}
	if !stored.EqualVT(payload) {
		t.Fatalf("stored payload mismatch: got %v want %v", stored, payload)
	}
}

func TestSecretPayloadStoreOperationCancelStopsRoutines(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	payload := &SecretPayload{Value: []byte("stored"), Version: 1}
	data, err := payload.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	so := newTestSecretPayloadStoreSharedObject()
	so.blockBeforeState = true
	op := newSecretPayloadStoreOperation(so, so.stateCtr, payload)

	done := make(chan error, 1)
	go func() {
		done <- op.Store(ctx, data)
	}()
	select {
	case <-so.processEntered:
	case <-time.After(time.Second):
		t.Fatal("processor routine did not start")
	}
	select {
	case <-so.processBlocked:
	case <-time.After(time.Second):
		t.Fatal("processor routine did not block before state publish")
	}
	cancel()
	select {
	case err := <-done:
		if !stderrors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Store did not return after cancellation")
	}
	if !so.processExited() {
		t.Fatal("expected processor routine to exit after cancellation")
	}
}

type testSecretPayloadStoreSharedObject struct {
	stateCtr         *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	ops              chan []byte
	queueEntered     chan struct{}
	processEntered   chan struct{}
	processBlocked   chan struct{}
	closeQueueOnce   sync.Once
	closeProcessOnce sync.Once
	closeBlockedOnce sync.Once
	mtx              sync.Mutex
	exited           bool
	processErr       error
	failBeforeState  bool
	blockQueue       bool
	blockBeforeState bool
}

func newTestSecretPayloadStoreSharedObject() *testSecretPayloadStoreSharedObject {
	return &testSecretPayloadStoreSharedObject{
		stateCtr:       ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
		ops:            make(chan []byte, 1),
		queueEntered:   make(chan struct{}),
		processEntered: make(chan struct{}),
		processBlocked: make(chan struct{}),
	}
}

func (s *testSecretPayloadStoreSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testSecretPayloadStoreSharedObject) GetPeerID() peer.ID {
	return peer.ID("secret-payload-store-test")
}

func (s *testSecretPayloadStoreSharedObject) GetSharedObjectID() string {
	return "secret-payload-store-test"
}

func (s *testSecretPayloadStoreSharedObject) GetBlockStore() bstore.BlockStore {
	return nil
}

func (s *testSecretPayloadStoreSharedObject) AccessLocalStateStore(
	context.Context,
	string,
	func(),
) (kvtx.Store, func(), error) {
	return nil, nil, stderrors.New("unexpected AccessLocalStateStore")
}

func (s *testSecretPayloadStoreSharedObject) GetSharedObjectState(
	context.Context,
) (sobject.SharedObjectStateSnapshot, error) {
	return s.stateCtr.GetValue(), nil
}

func (s *testSecretPayloadStoreSharedObject) AccessSharedObjectState(
	context.Context,
	func(),
) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return s.stateCtr, func() {}, nil
}

func (s *testSecretPayloadStoreSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	s.closeQueueOnce.Do(func() {
		close(s.queueEntered)
	})
	if s.blockQueue {
		<-ctx.Done()
		return "", ctx.Err()
	}
	select {
	case s.ops <- append([]byte(nil), op...):
		return "local-op", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *testSecretPayloadStoreSharedObject) WaitOperation(
	context.Context,
	string,
) (uint64, bool, error) {
	return 0, false, nil
}

func (s *testSecretPayloadStoreSharedObject) ClearOperationResult(context.Context, string) error {
	return nil
}

func (s *testSecretPayloadStoreSharedObject) ProcessOperations(
	ctx context.Context,
	watch bool,
	cb sobject.ProcessOpsFunc,
) error {
	s.closeProcessOnce.Do(func() {
		close(s.processEntered)
	})
	defer func() {
		s.mtx.Lock()
		s.exited = true
		s.mtx.Unlock()
	}()
	var data []byte
	select {
	case data = <-s.ops:
	case <-ctx.Done():
		return ctx.Err()
	}
	if s.failBeforeState && s.processErr != nil {
		return s.processErr
	}
	if s.blockBeforeState {
		s.closeBlockedOnce.Do(func() {
			close(s.processBlocked)
		})
		<-ctx.Done()
		return ctx.Err()
	}
	raw, _, err := cb(ctx, s.stateCtr.GetValue(), nil, []*sobject.SOOperationInner{{OpData: data}})
	if err != nil {
		return err
	}
	if raw != nil {
		s.stateCtr.SetValue(&testSecretPayloadStoreSnapshot{
			root: &sobject.SORootInner{StateData: append([]byte(nil), (*raw)...)},
		})
	}
	if s.processErr != nil {
		return s.processErr
	}
	if !watch {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *testSecretPayloadStoreSharedObject) processExited() bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.exited
}

type testSecretPayloadStoreSnapshot struct {
	root *sobject.SORootInner
}

func (s *testSecretPayloadStoreSnapshot) GetParticipantConfig(
	context.Context,
) (*sobject.SOParticipantConfig, error) {
	return nil, nil
}

func (s *testSecretPayloadStoreSnapshot) GetTransformer(
	context.Context,
) (*block_transform.Transformer, error) {
	return nil, nil
}

func (s *testSecretPayloadStoreSnapshot) GetTransformInfo(context.Context) (*sobject.TransformInfo, error) {
	return nil, nil
}

func (s *testSecretPayloadStoreSnapshot) GetOpQueue(
	context.Context,
) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, nil, nil
}

func (s *testSecretPayloadStoreSnapshot) GetRootInner(context.Context) (*sobject.SORootInner, error) {
	return s.root, nil
}

func (s *testSecretPayloadStoreSnapshot) ProcessOperations(
	context.Context,
	[]*sobject.SOOperation,
	sobject.SnapshotProcessOpsFunc,
) (*sobject.SORoot, []*sobject.SOOperationRejection, []*sobject.SOOperation, error) {
	return nil, nil, nil, nil
}
