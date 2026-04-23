package volume_controller

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/reconciler"
	reconciler_controller "github.com/aperturerobotics/hydra/reconciler/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	common_kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

type testReconcilerFactory struct {
	b bus.Bus
}

func (f *testReconcilerFactory) GetConfigID() string {
	return reconciler_example.ControllerID
}

func (f *testReconcilerFactory) GetControllerID() string {
	return "hydra/volume/controller/test-reconciler"
}

func (f *testReconcilerFactory) ConstructConfig() config.Config {
	return &reconciler_example.Config{}
}

func (f *testReconcilerFactory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	_ = ctx
	_ = conf
	return reconciler_controller.NewController(
		opts.GetLogger(),
		f.b,
		controller.NewInfo(
			f.GetControllerID(),
			f.GetVersion(),
			"test reconciler",
		),
		&noopReconciler{},
	), nil
}

func (f *testReconcilerFactory) GetVersion() semver.Version {
	return semver.MustParse("0.0.1")
}

type noopReconciler struct{}

func (r *noopReconciler) Execute(ctx context.Context, handle reconciler.Handle) error {
	<-ctx.Done()
	return nil
}

func (r *noopReconciler) Close() error {
	return nil
}

type testQueue struct {
	msgs [][]byte
}

func (q *testQueue) Peek(ctx context.Context) (mqueue.Message, bool, error) {
	_ = ctx
	if len(q.msgs) == 0 {
		return nil, false, nil
	}
	return &testMessage{id: 1, data: q.msgs[0]}, true, nil
}

func (q *testQueue) Ack(ctx context.Context, id uint64) error {
	_ = ctx
	_ = id
	if len(q.msgs) != 0 {
		q.msgs = q.msgs[1:]
	}
	return nil
}

func (q *testQueue) Push(ctx context.Context, data []byte) (mqueue.Message, error) {
	_ = ctx
	q.msgs = append(q.msgs, append([]byte(nil), data...))
	return &testMessage{id: uint64(len(q.msgs)), data: data}, nil
}

func (q *testQueue) Wait(ctx context.Context, ack bool) (mqueue.Message, error) {
	msg, ok, err := q.Peek(ctx)
	if err != nil || !ok {
		return msg, err
	}
	if ack {
		_ = q.Ack(ctx, msg.GetId())
	}
	return msg, nil
}

func (q *testQueue) DeleteQueue(ctx context.Context) error {
	_ = ctx
	q.msgs = nil
	return nil
}

type testMessage struct {
	id   uint64
	data []byte
}

func (m *testMessage) GetId() uint64 {
	return m.id
}

func (m *testMessage) GetTimestamp() time.Time {
	return time.Time{}
}

func (m *testMessage) GetData() []byte {
	return m.data
}

func newTestVolume(t *testing.T, ctx context.Context) *common_kvtx.Volume {
	t.Helper()
	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("NewKVKey failed: %v", err)
	}
	vol, err := common_kvtx.NewVolume(
		ctx,
		"hydra/test-volume",
		kvKey,
		store_kvtx_inmem.NewStore(),
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolume failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	return vol
}

func newTestController(le *logrus.Entry, b bus.Bus) *Controller {
	ctrl := &Controller{
		le:             le,
		bus:            b,
		reconcilerKeys: make(map[bucket_store.BucketReconcilerPair]struct{}),
	}
	ctrl.reconcilers = keyed.NewKeyed(ctrl.newRunningReconciler)
	return ctrl
}

func buildReconcilerConfig(
	t *testing.T,
	bucketID, reconcilerID string,
	rev uint32,
) *bucket.Config {
	t.Helper()
	ctrlConf, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(
			uint64(rev),
			&reconciler_example.Config{
				BucketId:     bucketID,
				BlockStoreId: "placeholder-store",
				ReconcilerId: reconcilerID,
			},
		),
		false,
	)
	if err != nil {
		t.Fatalf("build controller config: %v", err)
	}
	return &bucket.Config{
		Id:  bucketID,
		Rev: rev,
		Reconcilers: []*bucket.ReconcilerConfig{{
			Id:         reconcilerID,
			Controller: ctrlConf,
		}},
	}
}

func TestWatchBucketHandleChangeSignalsRebind(t *testing.T) {
	trk := &bucketHandleTracker{
		handleCtr: ccontainer.NewCContainer[*bucketHandle](nil),
	}
	initial := &bucketHandle{bucketConf: &bucket.Config{Id: "bucket-1", Rev: 1}}
	trk.handleCtr.SetValue(initial)
	errCh := make(chan error, 1)
	ctx := t.Context()

	go (&runningReconciler{}).watchBucketHandleChange(ctx, trk, initial, errCh)
	trk.handleCtr.SetValue(&bucketHandle{bucketConf: &bucket.Config{Id: "bucket-1", Rev: 2}})

	select {
	case err := <-errCh:
		if err != errBucketHandleChanged {
			t.Fatalf("expected errBucketHandleChanged, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bucket-handle change")
	}
}

func TestExecuteBucketHandleReturnsRebindErrorOnHandleChange(t *testing.T) {
	ctx := context.Background()
	le := logrus.New().WithField("test", t.Name())
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatalf("NewCoreBus failed: %v", err)
	}
	csCtrl, err := configset_controller.NewController(le, b)
	if err != nil {
		t.Fatalf("NewController failed: %v", err)
	}
	rel, err := b.AddController(ctx, csCtrl, nil)
	if err != nil {
		t.Fatalf("AddController failed: %v", err)
	}
	defer rel()

	sr.AddFactory(&testReconcilerFactory{b: b})
	ctrl := newTestController(le, b)
	vol := newTestVolume(t, ctx)
	pair := bucket_store.BucketReconcilerPair{
		BucketID:     "bucket-1",
		ReconcilerID: "rec-1",
	}
	queue := &testQueue{}
	trk := &bucketHandleTracker{
		c:         ctrl,
		bucketID:  pair.BucketID,
		handleCtr: ccontainer.NewCContainer[*bucketHandle](nil),
	}
	initial := &bucketHandle{
		t:          trk,
		v:          vol,
		bucketConf: buildReconcilerConfig(t, pair.BucketID, pair.ReconcilerID, 1),
	}
	trk.handleCtr.SetValue(initial)

	go func() {
		time.Sleep(20 * time.Millisecond)
		trk.handleCtr.SetValue(&bucketHandle{
			t:          trk,
			v:          vol,
			bucketConf: buildReconcilerConfig(t, pair.BucketID, pair.ReconcilerID, 2),
		})
	}()

	r := &runningReconciler{c: ctrl, le: le, pair: pair}
	err = r.executeBucketHandle(ctx, func() {}, vol, queue, trk, initial)
	if err != errBucketHandleChanged {
		t.Fatalf("expected errBucketHandleChanged, got %v", err)
	}
}

func TestExecuteBucketHandleCleansUpWhenReconcilerConfigMissing(t *testing.T) {
	ctx := context.Background()
	le := logrus.New().WithField("test", t.Name())
	ctrl := newTestController(le, nil)
	vol := newTestVolume(t, ctx)
	pair := bucket_store.BucketReconcilerPair{
		BucketID:     "bucket-1",
		ReconcilerID: "rec-1",
	}
	ctrl.reconcilerKeys[pair] = struct{}{}
	queue, err := vol.GetReconcilerEventQueue(ctx, pair)
	if err != nil {
		t.Fatalf("GetReconcilerEventQueue failed: %v", err)
	}
	if _, err := queue.Push(ctx, []byte("msg")); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	r := &runningReconciler{c: ctrl, le: le, pair: pair}
	handle := &bucketHandle{
		v: vol,
		bucketConf: &bucket.Config{
			Id:  pair.BucketID,
			Rev: 1,
		},
	}

	if err := r.executeBucketHandle(ctx, func() {}, vol, queue, nil, handle); err != nil {
		t.Fatalf("executeBucketHandle failed: %v", err)
	}
	if _, ok := ctrl.reconcilerKeys[pair]; ok {
		t.Fatal("expected reconciler key removal when config is missing")
	}
	if _, ok, err := queue.Peek(ctx); err != nil || ok {
		t.Fatalf("expected reconciler queue purge when config is missing: ok=%v err=%v", ok, err)
	}
}

func TestExecuteBucketHandleCleansUpWhenBucketMissing(t *testing.T) {
	ctx := context.Background()
	le := logrus.New().WithField("test", t.Name())
	ctrl := newTestController(le, nil)
	vol := newTestVolume(t, ctx)
	pair := bucket_store.BucketReconcilerPair{
		BucketID:     "bucket-1",
		ReconcilerID: "rec-1",
	}
	ctrl.reconcilerKeys[pair] = struct{}{}
	queue, err := vol.GetReconcilerEventQueue(ctx, pair)
	if err != nil {
		t.Fatalf("GetReconcilerEventQueue failed: %v", err)
	}
	if _, err := queue.Push(ctx, []byte("msg")); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	r := &runningReconciler{c: ctrl, le: le, pair: pair}

	if err := r.executeBucketHandle(ctx, func() {}, vol, queue, nil, &bucketHandle{v: vol}); err != nil {
		t.Fatalf("executeBucketHandle failed: %v", err)
	}
	if _, ok := ctrl.reconcilerKeys[pair]; ok {
		t.Fatal("expected reconciler key removal when bucket is missing")
	}
	if _, ok, err := queue.Peek(ctx); err != nil || ok {
		t.Fatalf("expected reconciler queue purge when bucket is missing: ok=%v err=%v", ok, err)
	}
}
