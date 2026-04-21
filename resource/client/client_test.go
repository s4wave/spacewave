package resource_client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/bldr/resource"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

type mockResourceService struct {
	mu             sync.Mutex
	attachCalls    int
	nextResourceID uint32
	onAttachSend   func(*mockResourceAttachClient, *resource.ResourceAttachRequest)
}

func (m *mockResourceService) SRPCClient() srpc.Client { return nil }

func (m *mockResourceService) ResourceClient(ctx context.Context, _ *resource.ResourceClientRequest) (resource.SRPCResourceService_ResourceClientClient, error) {
	return &mockResourceClientClient{ctx: ctx}, nil
}

func (m *mockResourceService) ResourceRpc(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
	return nil, errors.New("unused")
}

func (m *mockResourceService) ResourceRefRelease(ctx context.Context, in *resource.ResourceRefReleaseRequest) (*resource.ResourceRefReleaseResponse, error) {
	return &resource.ResourceRefReleaseResponse{}, nil
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
	initOnce sync.Once
}

func (m *mockResourceClientClient) Context() context.Context { return m.ctx }

func (m *mockResourceClientClient) CloseSend() error { return nil }

func (m *mockResourceClientClient) Close() error { return nil }

func (m *mockResourceClientClient) MsgSend(msg srpc.Message) error {
	return errors.New("unexpected send")
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

	<-m.ctx.Done()
	return nil, m.ctx.Err()
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
	ctx      context.Context
	recvCh   chan *resource.ResourceAttachResponse
	onSend   func(*mockResourceAttachClient, *resource.ResourceAttachRequest)
	service  *mockResourceService
	resource uint32
}

func (m *mockResourceAttachClient) Context() context.Context { return m.ctx }

func (m *mockResourceAttachClient) CloseSend() error { return nil }

func (m *mockResourceAttachClient) Close() error { return nil }

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

	c.mtx.Lock()
	sess := c.attachSess
	c.mtx.Unlock()
	if sess == nil {
		t.Fatalf("expected attach session")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	if len(sess.pending) != 0 {
		t.Fatalf("expected no pending attaches, got %d", len(sess.pending))
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
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return c.attachSess == nil
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

	c.mtx.Lock()
	sess := c.attachSess
	c.mtx.Unlock()
	if sess == nil {
		t.Fatalf("expected attach session")
	}
	if err := sess.mc.Close(); err != nil {
		t.Fatalf("close attach session: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		return c.attachSess == nil
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

var _ resource.SRPCResourceServiceClient = (*mockResourceService)(nil)
var _ resource.SRPCResourceService_ResourceClientClient = (*mockResourceClientClient)(nil)
var _ resource.SRPCResourceService_ResourceAttachClient = (*mockResourceAttachClient)(nil)
var _ resource.SRPCResourceService_ResourceRpcClient = (resource.SRPCResourceService_ResourceRpcClient)(nil)
var _ rpcstream.RpcStreamPacket
