package resource_client

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

type mockResourceService struct {
	mu                sync.Mutex
	attachCalls       int
	nextResourceID    uint32
	clientEvents      chan *resource.ResourceClientResponse
	onAttachSend      func(*mockResourceAttachClient, *resource.ResourceAttachRequest)
	onResourceRPC     func(context.Context) (resource.SRPCResourceService_ResourceRpcClient, error)
	onControl         func(*resource.ResourceClientRequest)
	onClientCloseSend func()
}

func (m *mockResourceService) SRPCClient() srpc.Client { return nil }

func (m *mockResourceService) ResourceClient(ctx context.Context) (resource.SRPCResourceService_ResourceClientClient, error) {
	m.mu.Lock()
	if m.clientEvents == nil {
		m.clientEvents = make(chan *resource.ResourceClientResponse, 32)
	}
	events := m.clientEvents
	m.mu.Unlock()
	return &mockResourceClientClient{ctx: ctx, events: events, service: m}, nil
}

func (m *mockResourceService) ResourceRpc(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
	if m.onResourceRPC != nil {
		return m.onResourceRPC(ctx)
	}
	return nil, errors.New("unused")
}

func (m *mockResourceService) ResourceAttach(ctx context.Context) (resource.SRPCResourceService_ResourceAttachClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.attachCalls++
	strm := &mockResourceAttachClient{
		ctx:      ctx,
		recvCh:   make(chan *resource.ResourceAttachResponse, 16),
		onSend:   m.onAttachSend,
		service:  m,
		resource: m.nextResourceID,
		closed:   make(chan struct{}),
	}
	strm.recvCh <- &resource.ResourceAttachResponse{
		Body: &resource.ResourceAttachResponse_Ack{
			Ack: &resource.ResourceAttachAck{},
		},
	}
	return strm, nil
}

func (m *mockResourceService) nextAttachResourceID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextResourceID++
	return m.nextResourceID
}

type mockResourceClientClient struct {
	ctx      context.Context
	events   chan *resource.ResourceClientResponse
	service  *mockResourceService
	initOnce sync.Once
}

func (m *mockResourceClientClient) Context() context.Context { return m.ctx }

func (m *mockResourceClientClient) CloseSend() error {
	if m.service != nil && m.service.onClientCloseSend != nil {
		m.service.onClientCloseSend()
	}
	return nil
}

func (m *mockResourceClientClient) Close() error { return nil }

func (m *mockResourceClientClient) MsgSend(msg srpc.Message) error {
	req, ok := msg.(*resource.ResourceClientRequest)
	if !ok {
		return errors.New("unexpected msg type")
	}
	return m.Send(req)
}

func (m *mockResourceClientClient) Send(req *resource.ResourceClientRequest) error {
	if m.service.onControl != nil {
		m.service.onControl(req)
	}
	if controlID := req.GetControlId(); controlID != 0 {
		m.events <- &resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ControlAck{
			ControlAck: &resource.ResourceClientControlAck{ControlId: controlID},
		}}
	}
	return nil
}

func (m *mockResourceClientClient) MsgRecv(msg srpc.Message) error {
	resp, err := m.Recv()
	if err != nil {
		return err
	}
	out, ok := msg.(*resource.ResourceClientResponse)
	if !ok {
		return errors.New("unexpected msg type")
	}
	*out = *resp
	return nil
}

func (m *mockResourceClientClient) Recv() (*resource.ResourceClientResponse, error) {
	var resp *resource.ResourceClientResponse
	var sent bool
	m.initOnce.Do(func() {
		resp = &resource.ResourceClientResponse{
			Body: &resource.ResourceClientResponse_Init{
				Init: &resource.ResourceClientInit{
					ClientHandleId: 1,
					RootResourceId: 1,
				},
			},
		}
		sent = true
	})
	if sent {
		return resp, nil
	}

	select {
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	case resp := <-m.events:
		if resp != nil {
			return resp, nil
		}
		return nil, errors.New("resource client event stream closed")
	}
}

func (m *mockResourceClientClient) RecvTo(msg *resource.ResourceClientResponse) error {
	resp, err := m.Recv()
	if err != nil {
		return err
	}
	*msg = *resp
	return nil
}

type mockResourceAttachClient struct {
	ctx            context.Context
	recvCh         chan *resource.ResourceAttachResponse
	onSend         func(*mockResourceAttachClient, *resource.ResourceAttachRequest)
	service        *mockResourceService
	resource       uint32
	closed         chan struct{}
	onClose        func()
	closeOnce      sync.Once
	closeCalls     atomic.Int32
	closeSendCalls atomic.Int32
}

func (m *mockResourceAttachClient) Context() context.Context { return m.ctx }

func (m *mockResourceAttachClient) CloseSend() error {
	m.closeSendCalls.Add(1)
	return nil
}

func (m *mockResourceAttachClient) Close() error {
	m.closeCalls.Add(1)
	if m.onClose != nil {
		m.onClose()
	}
	m.closeOnce.Do(func() {
		if m.closed != nil {
			close(m.closed)
		}
	})
	return nil
}

func (m *mockResourceAttachClient) MsgSend(msg srpc.Message) error {
	req, ok := msg.(*resource.ResourceAttachRequest)
	if !ok {
		return errors.New("unexpected msg type")
	}
	return m.Send(req)
}

func (m *mockResourceAttachClient) MsgRecv(msg srpc.Message) error {
	resp, err := m.Recv()
	if err != nil {
		return err
	}
	out, ok := msg.(*resource.ResourceAttachResponse)
	if !ok {
		return errors.New("unexpected msg type")
	}
	*out = *resp
	return nil
}

func (m *mockResourceAttachClient) Send(req *resource.ResourceAttachRequest) error {
	if m.onSend != nil {
		m.onSend(m, req)
	}
	return nil
}

func (m *mockResourceAttachClient) Recv() (*resource.ResourceAttachResponse, error) {
	select {
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	case <-m.closed:
		return nil, context.Canceled
	case resp := <-m.recvCh:
		return resp, nil
	}
}

func (m *mockResourceAttachClient) RecvTo(msg *resource.ResourceAttachResponse) error {
	resp, err := m.Recv()
	if err != nil {
		return err
	}
	*msg = *resp
	return nil
}

type mockMuxedConn struct {
	closeCalls atomic.Int32
}

func (m *mockMuxedConn) Close() error {
	m.closeCalls.Add(1)
	return nil
}

func (m *mockMuxedConn) IsClosed() bool { return m.closeCalls.Load() != 0 }

func (m *mockMuxedConn) OpenStream(context.Context) (srpc.MuxedStream, error) {
	return nil, errors.New("unused")
}

func (m *mockMuxedConn) AcceptStream() (srpc.MuxedStream, error) {
	return nil, errors.New("unused")
}

type errorResourceServer struct {
	*resource_server.ResourceServer
	err error
}

func (s *errorResourceServer) ResourceClient(
	resource.SRPCResourceService_ResourceClientStream,
) error {
	return s.err
}

func TestNewClientReturnsResourceClientStreamError(t *testing.T) {
	serverMux := srpc.NewMux()
	server := &errorResourceServer{
		ResourceServer: resource_server.NewResourceServer(nil),
		err:            errors.New("resource client handler failed"),
	}
	if err := resource.SRPCRegisterResourceService(serverMux, server); err != nil {
		t.Fatalf("register resource service: %v", err)
	}

	service := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))),
	)
	if _, err := NewClient(t.Context(), service); err == nil {
		t.Fatal("NewClient succeeded after ResourceClient stream failure")
	}
}

func TestResourceRPCHonorsCallerContext(t *testing.T) {
	resourceRPCStarted := make(chan context.Context, 1)
	svc := &mockResourceService{
		onResourceRPC: func(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
			resourceRPCStarted <- ctx
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	client, err := NewClient(t.Context(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	rootClient, err := client.AccessRootResource().GetClient()
	if err != nil {
		client.Release()
		t.Fatalf("root client: %v", err)
	}

	callCtx, cancelCall := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- rootClient.ExecCall(
			callCtx,
			"test.Service",
			"Blocked",
			&resource.ResourceClientInitRequest{},
			&resource.ResourceClientInit{},
		)
	}()

	select {
	case <-resourceRPCStarted:
	case <-time.After(time.Second):
		client.Release()
		t.Fatal("ResourceRpc did not start")
	}

	cancelCall()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ExecCall error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		client.Release()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("ExecCall did not return after caller context cancellation")
	}

	client.Release()
}

func TestAttachResourceAddAckErrorReturnsError(t *testing.T) {
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId: add.GetAttachId(),
							Error:    "attach rejected",
						},
					},
				}
			}
		},
	}
	ctx := t.Context()

	c, err := NewClient(ctx, svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	_, err = c.AttachResource(context.Background(), "test", srpc.InvokerFunc(nil))
	if err == nil || err.Error() != "attach rejected" {
		t.Fatalf("expected attach rejected error, got %v", err)
	}

	sess := c.attach.currentSession()
	if sess == nil {
		t.Fatalf("expected attach session")
	}

	if got := sess.pending.len(); got != 0 {
		t.Fatalf("expected no pending attaches, got %d", got)
	}
}

func TestAttachResourceReusesSharedSession(t *testing.T) {
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId:   add.GetAttachId(),
							ResourceId: strm.service.nextAttachResourceID(),
						},
					},
				}
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	for i := range 2 {
		if _, err := c.AttachResource(context.Background(), "test", srpc.InvokerFunc(nil)); err != nil {
			t.Fatalf("AttachResource %d: %v", i, err)
		}
	}

	svc.mu.Lock()
	attachCalls := svc.attachCalls
	svc.mu.Unlock()
	if attachCalls != 1 {
		t.Fatalf("expected one shared attach session, got %d", attachCalls)
	}
}

func TestReleaseDrainsControlsBeforeCancelingGeneration(t *testing.T) {
	events := make(chan *resource.ResourceClientResponse)
	closeSend := make(chan struct{})
	var closeSendOnce sync.Once
	var releasesMu sync.Mutex
	var releases []uint32
	svc := &mockResourceService{
		clientEvents: events,
		onControl: func(req *resource.ResourceClientRequest) {
			if release := req.GetRelease(); release != nil {
				releasesMu.Lock()
				releases = append(releases, release.GetResourceId())
				releasesMu.Unlock()
			}
		},
		onClientCloseSend: func() {
			closeSendOnce.Do(func() { close(closeSend) })
		},
	}
	client, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatal(err)
	}
	child := client.CreateResourceReference(2)
	child.Release()
	client.Release()

	select {
	case <-closeSend:
	case <-time.After(time.Second):
		t.Fatal("ResourceClient controls did not finish")
	}
	releasesMu.Lock()
	got := append([]uint32(nil), releases...)
	releasesMu.Unlock()
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("release controls = %v, want [2]", got)
	}
	select {
	case <-client.ctx.Done():
		t.Fatal("generation canceled before the response stream closed")
	default:
	}

	close(events)
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("generation did not retire after the response stream closed")
	}
}

func TestAttachSessionClearedOnClientRelease(t *testing.T) {
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId:   add.GetAttachId(),
							ResourceId: strm.service.nextAttachResourceID(),
						},
					},
				}
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if _, err := c.AttachResource(context.Background(), "test", srpc.InvokerFunc(nil)); err != nil {
		t.Fatalf("AttachResource: %v", err)
	}

	c.Release()
	waitFor(t, time.Second, func() bool {
		return c.attach.currentSession() == nil
	})
}

func TestConcurrentFirstAttachUsesOneSession(t *testing.T) {
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId:   add.GetAttachId(),
							ResourceId: strm.service.nextAttachResourceID(),
						},
					},
				}
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	startCh := make(chan struct{})
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			<-startCh
			_, err := c.AttachResource(context.Background(), "test", srpc.InvokerFunc(nil))
			errCh <- err
		}()
	}
	close(startCh)

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("AttachResource: %v", err)
		}
	}

	svc.mu.Lock()
	attachCalls := svc.attachCalls
	svc.mu.Unlock()
	if attachCalls != 1 {
		t.Fatalf("expected one shared attach session, got %d", attachCalls)
	}
}

func TestAttachResourceReopensAfterSessionClose(t *testing.T) {
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId:   add.GetAttachId(),
							ResourceId: strm.service.nextAttachResourceID(),
						},
					},
				}
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	if _, err := c.AttachResource(context.Background(), "test", srpc.InvokerFunc(nil)); err != nil {
		t.Fatalf("AttachResource: %v", err)
	}

	sess := c.attach.currentSession()
	if sess == nil {
		t.Fatalf("expected attach session")
	}
	if err := sess.mc.Close(); err != nil {
		t.Fatalf("close attach session: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		return c.attach.currentSession() == nil
	})

	if _, err := c.AttachResource(context.Background(), "test", srpc.InvokerFunc(nil)); err != nil {
		t.Fatalf("AttachResource after close: %v", err)
	}

	svc.mu.Lock()
	attachCalls := svc.attachCalls
	svc.mu.Unlock()
	if attachCalls != 2 {
		t.Fatalf("expected attach session to reopen, got %d attach calls", attachCalls)
	}
}

func TestCanceledAttachDetachesLateSuccessfulAck(t *testing.T) {
	detachCh := make(chan uint32, 1)
	ctx, cancel := context.WithCancel(context.Background())
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				cancel()
				go func() {
					time.Sleep(10 * time.Millisecond)
					strm.recvCh <- &resource.ResourceAttachResponse{
						Body: &resource.ResourceAttachResponse_AddAck{
							AddAck: &resource.ResourceAttachAddAck{
								AttachId:   add.GetAttachId(),
								ResourceId: 42,
							},
						},
					}
				}()
			}
			if detach := req.GetDetach(); detach != nil {
				detachCh <- detach.GetResourceId()
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	_, err = c.AttachResource(ctx, "test", srpc.InvokerFunc(nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	select {
	case resourceID := <-detachCh:
		if resourceID != 42 {
			t.Fatalf("expected detach for resource 42, got %d", resourceID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected detach after late successful AddAck")
	}
}

func TestAttachPendingCancelAfterResolvedAckDetaches(t *testing.T) {
	pending := newAttachPendingAcks()
	attachID, ch := pending.add()

	if resourceID, detach := pending.resolve(&resource.ResourceAttachAddAck{
		AttachId:   attachID,
		ResourceId: 42,
	}); detach || resourceID != 0 {
		t.Fatalf("resolve returned detach=%v resource=%d, want no detach", detach, resourceID)
	}
	resourceID, detach := pending.cancel(attachID)
	if !detach || resourceID != 42 {
		t.Fatalf("cancel after resolved ack returned detach=%v resource=%d, want detach 42", detach, resourceID)
	}

	select {
	case result := <-ch:
		if result.err != nil || result.resourceID != 42 {
			t.Fatalf("resolved result = %+v, want resource 42", result)
		}
	default:
		t.Fatal("resolved ack did not notify waiter")
	}
	if got := pending.len(); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
}

func TestAttachPendingDuplicateAckDoesNotResend(t *testing.T) {
	pending := newAttachPendingAcks()
	attachID, ch := pending.add()
	ack := &resource.ResourceAttachAddAck{
		AttachId:   attachID,
		ResourceId: 42,
	}

	if resourceID, detach := pending.resolve(ack); detach || resourceID != 0 {
		t.Fatalf("first resolve returned detach=%v resource=%d, want no detach", detach, resourceID)
	}
	if resourceID, detach := pending.resolve(ack); detach || resourceID != 0 {
		t.Fatalf("duplicate resolve returned detach=%v resource=%d, want no detach", detach, resourceID)
	}

	select {
	case result := <-ch:
		if result.err != nil || result.resourceID != 42 {
			t.Fatalf("resolved result = %+v, want resource 42", result)
		}
	default:
		t.Fatal("first ack did not notify waiter")
	}
	select {
	case result := <-ch:
		t.Fatalf("duplicate ack resent result: %+v", result)
	default:
	}
	pending.complete(attachID)
	if got := pending.len(); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
}

func TestAttachSessionCloseFailsPendingAttachAndSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	strm := &mockResourceAttachClient{
		ctx:    context.Background(),
		recvCh: make(chan *resource.ResourceAttachResponse),
		closed: make(chan struct{}),
	}
	mc := &mockMuxedConn{}
	sess := &attachSession{
		ctx:        ctx,
		cancel:     cancel,
		strm:       strm,
		mc:         mc,
		router:     resource.NewRoutedInvoker(),
		pending:    newAttachPendingAcks(),
		muxes:      make(map[uint32]struct{}),
		releaseFns: make(map[uint32]func()),
		sendCh:     make(chan *attachSendRequest),
	}
	_, ch := sess.pending.add()

	sess.close()

	select {
	case result := <-ch:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("pending attach error = %v, want context canceled", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending attach was not failed on session close")
	}
	if got := sess.pending.len(); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
	if got := mc.closeCalls.Load(); got != 1 {
		t.Fatalf("muxed connection close calls = %d, want 1", got)
	}
	if got := strm.closeCalls.Load(); got != 1 {
		t.Fatalf("attach stream close calls = %d, want 1", got)
	}
	select {
	case <-strm.closed:
	default:
		t.Fatal("attach stream close did not unblock recv")
	}

	err := sess.send(&resource.ResourceAttachRequest{
		Body: &resource.ResourceAttachRequest_Detach{
			Detach: &resource.ResourceAttachDetach{ResourceId: 1},
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("send after close error = %v, want context canceled", err)
	}
}

func TestAttachSessionCloseRemovesMuxAndRejectsLateMux(t *testing.T) {
	sess := newAttachSession(
		context.Background(),
		nil,
		nil,
		resource.NewRoutedInvoker(),
	)
	called := false
	if err := sess.setMux(42, srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		called = true
		return true, nil
	})); err != nil {
		t.Fatalf("set mux before close: %v", err)
	}
	ok, err := sess.router.InvokeMethod("42/test.Service", "Call", nil)
	if err != nil || !ok || !called {
		t.Fatalf("route before close ok=%v called=%v err=%v, want successful dispatch", ok, called, err)
	}

	sess.close()

	called = false
	ok, err = sess.router.InvokeMethod("42/test.Service", "Call", nil)
	if ok || !errors.Is(err, resource.ErrResourceNotFound) || called {
		t.Fatalf("route after close ok=%v called=%v err=%v, want resource not found", ok, called, err)
	}
	if err := sess.setMux(43, srpc.InvokerFunc(nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("set mux after close error = %v, want context canceled", err)
	}
}

func TestAttachSessionCloseMarksReleasedBeforeTransportClose(t *testing.T) {
	sess := newAttachSession(
		context.Background(),
		nil,
		nil,
		resource.NewRoutedInvoker(),
	)
	errCh := make(chan error, 1)
	sess.strm = &mockResourceAttachClient{
		closed: make(chan struct{}),
		onClose: func() {
			errCh <- sess.setMux(42, srpc.InvokerFunc(nil))
		},
	}

	sess.close()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("set mux during transport close error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("transport close did not run")
	}
}

func TestCreateResourceReferenceAfterClientReleaseIsReleased(t *testing.T) {
	c, err := NewClient(context.Background(), &mockResourceService{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	c.Release()
	ref := c.CreateResourceReference(42)
	if _, err := ref.GetClient(); !errors.Is(err, resource.ErrResourceOrClientReleased) {
		t.Fatalf("released client GetClient error = %v, want released", err)
	}
}

func TestCreateResourceReferenceRacingClientReleaseReturnsReleasedRefs(t *testing.T) {
	c, err := NewClient(context.Background(), &mockResourceService{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	start := make(chan struct{})
	refs := make(chan ResourceRef, 16)
	var wg sync.WaitGroup
	for resourceID := uint32(1); resourceID <= 16; resourceID++ {
		wg.Add(1)
		go func(resourceID uint32) {
			defer wg.Done()
			<-start
			refs <- c.CreateResourceReference(resourceID)
		}(resourceID)
	}

	close(start)
	c.Release()
	wg.Wait()
	close(refs)

	for ref := range refs {
		if _, err := ref.GetClient(); !errors.Is(err, resource.ErrResourceOrClientReleased) {
			t.Fatalf("racing ref GetClient error = %v, want released", err)
		}
		ref.Release()
	}
}

func TestDetachResourceReleasesAttachedRootOnce(t *testing.T) {
	var releaseCalls atomic.Int32
	released := make(chan struct{}, 1)
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId:   add.GetAttachId(),
							ResourceId: strm.service.nextAttachResourceID(),
						},
					},
				}
			}
			if detach := req.GetDetach(); detach != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_DetachAck{
						DetachAck: &resource.ResourceAttachDetachAck{
							ResourceId: detach.GetResourceId(),
						},
					},
				}
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	rootID, err := c.AttachResource(context.Background(), "root", srpc.InvokerFunc(nil))
	if err != nil {
		t.Fatalf("AttachResource: %v", err)
	}
	c.setAttachedRelease(rootID, func() {
		releaseCalls.Add(1)
		select {
		case released <- struct{}{}:
		default:
		}
	})

	if err := c.DetachResource(context.Background(), rootID); err != nil {
		t.Fatalf("DetachResource: %v", err)
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("attached root release callback was not called")
	}
	if err := c.DetachResource(context.Background(), rootID); err != nil {
		t.Fatalf("second DetachResource: %v", err)
	}
	if releaseCalls.Load() != 1 {
		t.Fatalf("attached root release calls = %d, want 1", releaseCalls.Load())
	}
}

func TestAttachSessionCloseReleasesAttachedResources(t *testing.T) {
	var releaseCalls atomic.Int32
	svc := &mockResourceService{
		onAttachSend: func(strm *mockResourceAttachClient, req *resource.ResourceAttachRequest) {
			if add := req.GetAdd(); add != nil {
				strm.recvCh <- &resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId:   add.GetAttachId(),
							ResourceId: strm.service.nextAttachResourceID(),
						},
					},
				}
			}
		},
	}

	c, err := NewClient(context.Background(), svc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Release()

	rootID, err := c.AttachResource(context.Background(), "root", srpc.InvokerFunc(nil))
	if err != nil {
		t.Fatalf("AttachResource: %v", err)
	}
	c.setAttachedRelease(rootID, func() {
		releaseCalls.Add(1)
	})

	sess := c.attach.currentSession()
	if sess == nil {
		t.Fatal("expected attach session")
	}
	if err := sess.mc.Close(); err != nil {
		t.Fatalf("close attach session: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		return c.attach.currentSession() == nil && releaseCalls.Load() == 1
	})
}

func TestAttachSessionSetReleaseAfterCloseRunsRelease(t *testing.T) {
	sess := &attachSession{
		releaseFns: make(map[uint32]func()),
	}
	sess.releaseAllAttachedResources()

	released := make(chan struct{}, 1)
	sess.setRelease(42, func() {
		released <- struct{}{}
	})

	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release callback registered after close was not called")
	}
}

func TestResourceCallWaitsForPriorLifecycleControl(t *testing.T) {
	releaseStarted := make(chan struct{})
	allowRelease := make(chan struct{})
	rootCalled := make(chan struct{}, 1)
	rootMux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != "test.Root" {
			return false, nil
		}
		if err := strm.MsgRecv(&resource.ResourceClientInitRequest{}); err != nil {
			return true, err
		}
		switch methodID {
		case "CreateChild":
			owner, err := resource_server.MustGetResourceClientContext(strm.Context())
			if err != nil {
				return true, err
			}
			childID, err := owner.AddResource(srpc.NewMux(), func() {
				close(releaseStarted)
				<-allowRelease
			})
			if err != nil {
				return true, err
			}
			return true, strm.MsgSend(&resource.ResourceAttachAddAck{ResourceId: childID})
		case "AfterRelease":
			rootCalled <- struct{}{}
			return true, strm.MsgSend(&resource.ResourceClientInit{})
		default:
			return false, nil
		}
	}))
	server := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))))
	client, err := NewClient(t.Context(), service)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Release()

	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatalf("root client: %v", err)
	}
	child := new(resource.ResourceAttachAddAck)
	if err := rootClient.ExecCall(t.Context(), "test.Root", "CreateChild", &resource.ResourceClientInitRequest{}, child); err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	childRef := client.CreateResourceReference(child.GetResourceId())
	childRef.Release()
	select {
	case <-releaseStarted:
	case <-time.After(time.Second):
		t.Fatal("release callback did not start")
	}

	callDone := make(chan error, 1)
	go func() {
		callDone <- rootClient.ExecCall(t.Context(), "test.Root", "AfterRelease", &resource.ResourceClientInitRequest{}, &resource.ResourceClientInit{})
	}()
	select {
	case <-rootCalled:
		t.Fatal("ResourceRpc started before the prior release callback completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(allowRelease)
	select {
	case err := <-callDone:
		if err != nil {
			t.Fatalf("AfterRelease: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ResourceRpc did not start after the release callback completed")
	}
	select {
	case <-rootCalled:
	case <-time.After(time.Second):
		t.Fatal("root handler was not called")
	}
}

func TestAttachedResourceTreeCanPublishCallableChild(t *testing.T) {
	rootMux := srpc.NewMux()
	server := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))))
	client, err := NewClient(t.Context(), service)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Release()

	childReleased := make(chan struct{}, 1)
	rootID, err := client.AttachResourceTree(t.Context(), "test-root", srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != "test.Root" || methodID != "CreateChild" {
			return false, nil
		}
		if err := strm.MsgRecv(&resource.ResourceClientInitRequest{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		childID, err := owner.AddResource(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
			if serviceID != "test.Child" || methodID != "Ping" {
				return false, nil
			}
			if err := strm.MsgRecv(&resource.ResourceClientInitRequest{}); err != nil {
				return true, err
			}
			return true, strm.MsgSend(&resource.ResourceClientInit{})
		}), func() {
			childReleased <- struct{}{}
		})
		if err != nil {
			return true, err
		}
		return true, strm.MsgSend(&resource.ResourceAttachAddAck{ResourceId: childID})
	}))
	if err != nil {
		t.Fatalf("AttachResource: %v", err)
	}

	rootRef := client.CreateResourceReference(rootID)
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatalf("root client: %v", err)
	}
	child := new(resource.ResourceAttachAddAck)
	if err := rootClient.ExecCall(t.Context(), "test.Root", "CreateChild", &resource.ResourceClientInitRequest{}, child); err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	if child.GetResourceId() == 0 {
		t.Fatal("CreateChild returned empty child resource id")
	}

	childRef := client.CreateResourceReference(child.GetResourceId())
	defer childRef.Release()
	childClient, err := childRef.GetClient()
	if err != nil {
		t.Fatalf("child client: %v", err)
	}
	if err := childClient.ExecCall(t.Context(), "test.Child", "Ping", &resource.ResourceClientInitRequest{}, &resource.ResourceClientInit{}); err != nil {
		t.Fatalf("Ping child: %v", err)
	}

	childRef.Release()
	select {
	case <-childReleased:
	case <-time.After(time.Second):
		t.Fatal("attached child release callback was not called")
	}
}

func TestAttachRawInvokerAndResourceTreeShareSessionWithDistinctContracts(t *testing.T) {
	rootMux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != "test.Root" || methodID != "UseRaw" {
			return false, nil
		}
		req := new(resource.ResourceAttachAddAck)
		if err := strm.MsgRecv(req); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		if _, err := owner.GetResourceValue(req.GetResourceId()); err != resource.ErrResourceNotFound {
			if err == nil {
				return true, errors.New("raw attached invoker unexpectedly resolved as resource value")
			}
			return true, err
		}
		rawClient, err := owner.GetAttachedResource(req.GetResourceId())
		if err != nil {
			return true, err
		}
		if err := rawClient.ExecCall(strm.Context(), "test.Raw", "Ping", &resource.ResourceClientInitRequest{}, &resource.ResourceClientInit{}); err != nil {
			return true, err
		}
		return true, strm.MsgSend(&resource.ResourceClientInit{})
	}))
	server := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))))
	client, err := NewClient(t.Context(), service)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Release()

	rawCalled := make(chan struct{}, 1)
	rawID, err := client.AttachRawInvoker(t.Context(), "raw-callback", srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != "test.Raw" || methodID != "Ping" {
			return false, nil
		}
		if err := strm.MsgRecv(&resource.ResourceClientInitRequest{}); err != nil {
			return true, err
		}
		select {
		case rawCalled <- struct{}{}:
		default:
		}
		return true, strm.MsgSend(&resource.ResourceClientInit{})
	}))
	if err != nil {
		t.Fatalf("AttachRawInvoker: %v", err)
	}
	rawSession := client.attach.currentSession()
	if rawSession == nil {
		t.Fatal("expected attach session after raw attach")
	}

	childReleased := make(chan struct{}, 1)
	treeID, err := client.AttachResourceTree(t.Context(), "tree-root", srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != "test.Tree" || methodID != "CreateChild" {
			return false, nil
		}
		if err := strm.MsgRecv(&resource.ResourceClientInitRequest{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		childID, err := owner.AddResource(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
			if serviceID != "test.Child" || methodID != "Ping" {
				return false, nil
			}
			if err := strm.MsgRecv(&resource.ResourceClientInitRequest{}); err != nil {
				return true, err
			}
			return true, strm.MsgSend(&resource.ResourceClientInit{})
		}), func() {
			childReleased <- struct{}{}
		})
		if err != nil {
			return true, err
		}
		return true, strm.MsgSend(&resource.ResourceAttachAddAck{ResourceId: childID})
	}))
	if err != nil {
		t.Fatalf("AttachResourceTree: %v", err)
	}
	if got := client.attach.currentSession(); got != rawSession {
		t.Fatalf("attach session changed between raw and tree attaches: %p != %p", got, rawSession)
	}

	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatalf("root client: %v", err)
	}
	if err := rootClient.ExecCall(t.Context(), "test.Root", "UseRaw", &resource.ResourceAttachAddAck{ResourceId: rawID}, &resource.ResourceClientInit{}); err != nil {
		t.Fatalf("UseRaw: %v", err)
	}
	select {
	case <-rawCalled:
	case <-time.After(time.Second):
		t.Fatal("raw attached invoker was not called")
	}

	treeRef := client.CreateResourceReference(treeID)
	defer treeRef.Release()
	treeClient, err := treeRef.GetClient()
	if err != nil {
		t.Fatalf("tree client: %v", err)
	}
	child := new(resource.ResourceAttachAddAck)
	if err := treeClient.ExecCall(t.Context(), "test.Tree", "CreateChild", &resource.ResourceClientInitRequest{}, child); err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	if child.GetResourceId() == 0 {
		t.Fatal("CreateChild returned empty child resource id")
	}

	childRef := client.CreateResourceReference(child.GetResourceId())
	defer childRef.Release()
	childClient, err := childRef.GetClient()
	if err != nil {
		t.Fatalf("child client: %v", err)
	}
	if err := childClient.ExecCall(t.Context(), "test.Child", "Ping", &resource.ResourceClientInitRequest{}, &resource.ResourceClientInit{}); err != nil {
		t.Fatalf("Ping child: %v", err)
	}

	childRef.Release()
	select {
	case <-childReleased:
	case <-time.After(time.Second):
		t.Fatal("attached tree child release callback was not called")
	}
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout")
}

var (
	_ resource.SRPCResourceServiceClient                = (*mockResourceService)(nil)
	_ resource.SRPCResourceService_ResourceClientClient = (*mockResourceClientClient)(nil)
	_ resource.SRPCResourceService_ResourceAttachClient = (*mockResourceAttachClient)(nil)
	_ resource.SRPCResourceService_ResourceRpcClient    = resource.SRPCResourceService_ResourceRpcClient(nil)
	_ rpcstream.RpcStreamPacket
)

func TestResourceControlQueueRetiresWhenIdleStreamCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	failure := make(chan error, 1)
	stream := &mockResourceClientClient{ctx: ctx, events: make(chan *resource.ResourceClientResponse)}
	newResourceControlQueue(stream, func(err error) { failure <- err }, func() { close(done) })

	cancel()
	select {
	case err := <-failure:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queue failure = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle queue did not retire after its stream closed")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle queue did not finish after its stream closed")
	}
}
