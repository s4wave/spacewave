package resource_sobject

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/bstore"
	provider "github.com/s4wave/spacewave/core/provider"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	"github.com/s4wave/spacewave/testbed"
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
	b         bus.Bus
	healthCtr *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

type testSpaceBody struct {
	ref *sobject.SharedObjectRef
	so  sobject.SharedObject
}

type testBodyMountHandler struct {
	bodyType string
	ref      *sobject.SharedObjectRef
	so       sobject.SharedObject
	body     space.SpaceSharedObjectBody

	resolveCt int
	releaseCt int
	releaseCh chan struct{}
}

type testResourceClientContext struct {
	nextID    uint32
	values    map[uint32]any
	releases  map[uint32]func()
	addErr    error
	releaseCt int
}

func (c *testResourceClientContext) Context() context.Context {
	return context.Background()
}

func (c *testResourceClientContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *testResourceClientContext) AddResourceValue(_ srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	// Preserve the configured resource registration failure.
	if c.addErr != nil {
		return 0, c.addErr
	}

	// Register the resource value and its release function under a new ID.
	c.nextID++
	if c.values == nil {
		c.values = make(map[uint32]any)
	}
	if c.releases == nil {
		c.releases = make(map[uint32]func())
	}
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	return c.nextID, nil
}

func (c *testResourceClientContext) ReleaseResource(resourceID uint32) bool {
	// Find the registered resource before releasing it.
	releaseFn, ok := c.releases[resourceID]
	if !ok {
		return false
	}

	// Remove the resource and invoke its retained release function.
	delete(c.releases, resourceID)
	delete(c.values, resourceID)
	if releaseFn != nil {
		releaseFn()
	}
	c.releaseCt++
	return true
}

func (c *testResourceClientContext) GetResourceValue(resourceID uint32) (any, error) {
	value, ok := c.values[resourceID]
	if !ok {
		return nil, errors.New("resource not found")
	}
	return value, nil
}

func (c *testResourceClientContext) GetAttachedResource(uint32) (srpc.Client, error) {
	return nil, errors.New("attached resource not found")
}

func (h *testBodyMountHandler) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	// Resolve only body mounting directives for the configured body type.
	dir, ok := di.GetDirective().(sobject.MountSharedObjectBody)
	if !ok || dir.MountSharedObjectBodyType() != h.bodyType {
		return nil, nil
	}

	// Require the configured SharedObject reference and mounted source.
	if !dir.MountSharedObjectBodyRef().EqualVT(h.ref) {
		return nil, errors.New("unexpected shared object ref")
	}
	if dir.MountSharedObjectBodySource() != h.so {
		return nil, errors.New("expected mounted shared object source")
	}

	return directive.R(directive.NewAccessResolver(func(context.Context, func()) (space.MountSharedObjectBodyValue, func(), error) {
		h.resolveCt++
		return sobject.NewMountSharedObjectBodyValue(
				h.ref,
				h.bodyType,
				h.so,
				h.body,
			), func() {
				h.releaseCt++
				if h.releaseCh != nil {
					select {
					case h.releaseCh <- struct{}{}:
					default:
					}
				}
			}, nil
	}), nil)
}

func (s *testMountedSharedObject) GetBus() bus.Bus {
	return s.b
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

func (b *testSpaceBody) GetWorldEngine() world.Engine {
	return nil
}

func (b *testSpaceBody) GetWorldEngineID() string {
	return "test-native"
}

func (b *testSpaceBody) GetWorldEngineBucketID() string {
	return b.ref.GetBlockStoreId()
}

func (b *testSpaceBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return b.ref
}

func (b *testSpaceBody) GetSharedObject() sobject.SharedObject {
	return b.so
}

func recvMountedSharedObjectHealth(
	t *testing.T,
	msgs <-chan *s4wave_sobject.WatchSharedObjectHealthResponse,
) *sobject.SharedObjectHealth {
	// Receive the mounted SharedObject health or fail with the caller location.
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
	// Give the parallel health stream test a cancelable lifetime.
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mount a SharedObject whose health begins ready.
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

	// Start streaming the mounted SharedObject health.
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.WatchSharedObjectHealth(
			&s4wave_sobject.WatchSharedObjectHealthRequest{},
			strm,
		)
	}()

	// Verify that the stream begins with the current ready health.
	ready := recvMountedSharedObjectHealth(t, strm.msgs)
	if ready.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready status, got %v", ready.GetStatus())
	}

	// Publish a body configuration failure through the SharedObject health watch.
	healthCtr.SetValue(sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"unsupported shared object type: weird.body",
	))

	// Verify that the stream preserves the failure status, layer and reason.
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

	// Stop the health stream and verify that it returns cancellation.
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchSharedObjectHealth() = %v, want context canceled", err)
	}
}

func TestWatchSharedObjectHealthStreamsMountedCurrentTypedHealthFirst(t *testing.T) {
	// Give the parallel health stream test a cancelable lifetime.
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mount a SharedObject with a retained body configuration failure.
	typedHealth := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"unsupported shared object type: weird.body",
	)
	healthCtr := ccontainer.NewCContainer[*sobject.SharedObjectHealth](typedHealth)
	r := &SharedObjectResource{
		sharedObject: &testMountedSharedObject{
			id:        "so-mounted",
			healthCtr: healthCtr,
		},
	}
	strm := newTestSharedObjectHealthStream(ctx)

	// Start streaming the mounted SharedObject health.
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.WatchSharedObjectHealth(
			&s4wave_sobject.WatchSharedObjectHealthRequest{},
			strm,
		)
	}()

	// Verify that the initial health preserves every typed failure detail.
	current := recvMountedSharedObjectHealth(t, strm.msgs)
	if current.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", current.GetStatus())
	}
	if current.GetLayer() != sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
		t.Fatalf("expected body layer, got %v", current.GetLayer())
	}
	if current.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED {
		t.Fatalf("expected body-config reason, got %v", current.GetCommonReason())
	}
	if current.GetRemediationHint() != sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA {
		t.Fatalf("expected repair-source-data remediation hint, got %v", current.GetRemediationHint())
	}
	if current.GetError() != "unsupported shared object type: weird.body" {
		t.Fatalf("expected typed health detail to survive, got %q", current.GetError())
	}

	// Stop the health stream and verify that it returns cancellation.
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchSharedObjectHealth() = %v, want context canceled", err)
	}
}

func TestMountSharedObjectBodyReturnsTypedHealthResponseForBodyConfigFailures(t *testing.T) {
	if !testMountSharedObjectBodyAvailable {
		t.Skip("native body mounting is unavailable under goscript")
	}
	t.Parallel()

	tests := []struct {
		name     string
		bodyType string
		detail   string
	}{
		{
			name:     "empty",
			bodyType: "",
			detail:   sobject.ErrEmptyBodyType.Error(),
		},
		{
			name:     "unsupported",
			bodyType: "weird.body",
			detail:   "unsupported shared object type: weird.body",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a resource context for the parallel body configuration case.
			t.Parallel()
			resourceCtx := &testResourceClientContext{}
			ctx := resource_server.WithResourceClientContext(context.Background(), resourceCtx)
			r := &SharedObjectResource{
				meta: &sobject.SharedObjectMeta{BodyType: tc.bodyType},
			}

			// Attempt to mount the SharedObject with the invalid body configuration.
			resp, err := r.MountSharedObjectBody(
				ctx,
				&s4wave_sobject.MountSharedObjectBodyRequest{},
			)
			if err != nil {
				t.Fatalf("MountSharedObjectBody() error = %v", err)
			}

			// Verify that the failed body mount allocates no resource.
			if resp.GetResourceId() != 0 {
				t.Fatalf("expected no body resource id, got %d", resp.GetResourceId())
			}

			// Verify that the mount response preserves the typed body failure details.
			health := resp.GetHealth()
			if health == nil {
				t.Fatal("expected typed health response")
			}
			if health.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
				t.Fatalf("expected closed status, got %v", health.GetStatus())
			}
			if health.GetLayer() != sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
				t.Fatalf("expected body layer, got %v", health.GetLayer())
			}
			if health.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED {
				t.Fatalf("expected body-config reason, got %v", health.GetCommonReason())
			}
			if health.GetRemediationHint() != sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA {
				t.Fatalf("expected repair-source-data hint, got %v", health.GetRemediationHint())
			}
			if health.GetError() != tc.detail {
				t.Fatalf("expected detail %q, got %q", tc.detail, health.GetError())
			}

			// Verify that the failed body mount registers no child resources.
			if resourceCtx.nextID != 0 {
				t.Fatalf("expected no child resources to be registered, got %d", resourceCtx.nextID)
			}
		})
	}
}

func TestMountSharedObjectBodyUsesBodyDirectiveForNativeBody(t *testing.T) {
	// Require native body mounting for the directive lifecycle test.
	if !testMountSharedObjectBodyAvailable {
		t.Skip("native body mounting is unavailable under goscript")
	}

	// Start a testbed that resolves the native body mounting directive.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Describe the mounted SharedObject and its native body.
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "so-native",
			ProviderId:        "local",
			ProviderAccountId: "acct",
		},
		BlockStoreId: "store",
	}
	so := &testMountedSharedObject{
		id: "so-native",
		b:  tb.Bus,
	}
	bodyType := "test-native"
	body := &testSpaceBody{ref: ref, so: so}
	handler := &testBodyMountHandler{
		bodyType:  bodyType,
		ref:       ref,
		so:        so,
		body:      body,
		releaseCh: make(chan struct{}, 2),
	}

	// Register the native body resolver for the test lifetime.
	removeHandler, err := tb.Bus.AddHandler(handler)
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	// Create the SharedObject resource that registers mounted body children.
	resourceCtx := &testResourceClientContext{}
	ctx = resource_server.WithResourceClientContext(ctx, resourceCtx)
	r := &SharedObjectResource{
		le:            tb.Logger,
		b:             tb.Bus,
		sharedObject:  so,
		meta:          &sobject.SharedObjectMeta{BodyType: bodyType},
		ref:           ref,
		sessionPeerID: "session-peer",
	}

	// Mount the native body through its directive resolver.
	resp, err := r.MountSharedObjectBody(
		ctx,
		&s4wave_sobject.MountSharedObjectBodyRequest{},
	)
	if err != nil {
		t.Fatalf("MountSharedObjectBody() error = %v", err)
	}

	// Verify that the mount retains one resolved SpaceResource child.
	if handler.resolveCt != 1 {
		t.Fatalf("expected body directive resolver to run once, got %d", handler.resolveCt)
	}
	if resp.GetResourceId() == 0 {
		t.Fatal("expected body resource id")
	}
	value := resourceCtx.values[resp.GetResourceId()]
	if _, ok := value.(*resource_space.SpaceResource); !ok {
		t.Fatalf("expected SpaceResource child, got %T", value)
	}
	if handler.releaseCt != 0 {
		t.Fatalf("expected child release to be retained, got %d", handler.releaseCt)
	}

	// Release the first body resource before mounting it again.
	if !resourceCtx.ReleaseResource(resp.GetResourceId()) {
		t.Fatal("expected child resource release")
	}

	// Mount the native body again after releasing the first child.
	rejoined, err := r.MountSharedObjectBody(
		ctx,
		&s4wave_sobject.MountSharedObjectBodyRequest{},
	)
	if err != nil {
		t.Fatalf("rejoin MountSharedObjectBody() error = %v", err)
	}

	// Verify that the second mount resolves a fresh directive value.
	if handler.resolveCt != 2 {
		t.Fatalf("rejoin reused the released body directive value: resolver calls = %d", handler.resolveCt)
	}

	// Release the second child and verify both directive values are released.
	if !resourceCtx.ReleaseResource(rejoined.GetResourceId()) {
		t.Fatal("expected rejoined child resource release")
	}
	for range 2 {
		select {
		case <-handler.releaseCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected both directive value releases, got %d", handler.releaseCt)
		}
	}
}

// _ is a type assertion
var _ s4wave_sobject.SRPCSharedObjectResourceService_WatchSharedObjectHealthStream = (*testSharedObjectHealthStream)(nil)

// _ is a type assertion
var _ sobject.SharedObject = (*testMountedSharedObject)(nil)

// _ is a type assertion
var _ sobject.SharedObjectHealthAccessor = (*testMountedSharedObject)(nil)

// _ is a type assertion
var _ resource_server.ResourceClientContext = (*testResourceClientContext)(nil)

// _ is a type assertion
var _ directive.Handler = (*testBodyMountHandler)(nil)

// _ is a type assertion
var _ space.SpaceSharedObjectBody = (*testSpaceBody)(nil)
