package resource_client

import (
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// ResourceRef is a reference to a remote resource.
// Each reference must be explicitly released when no longer needed.
type ResourceRef interface {
	// GetResourceID returns the resource ID.
	GetResourceID() uint32
	// GetClient returns the SRPC client for this resource.
	// The client is created lazily on first access.
	GetClient() (srpc.Client, error)
	// Release releases this reference.
	// When the last reference to a resource is released, the server is notified.
	Release()
}

// Client manages one immutable remote ResourceClient generation.
// It serializes lifecycle controls and reference-counts each remote resource.
type Client struct {
	// ctx ends when the generation retires
	ctx context.Context
	// cancel retires the generation transport
	cancel context.CancelFunc
	// attachCtx scopes client-published resource operations.
	attachCtx    context.Context
	cancelAttach context.CancelFunc
	// service opens ResourceRpc and ResourceAttach streams
	service resource.SRPCResourceServiceClient
	// done closes after the ResourceClient response stream has ended.
	done chan struct{}

	clientHandleID uint32
	rootResourceID uint32

	resourceLifetime *resourceLifetime
	attach           *attachLifetime
	controls         *resourceControlQueue
}

// NewClient constructs a ResourceClient generation and waits for its init.
func NewClient(ctx context.Context, service resource.SRPCResourceServiceClient) (*Client, error) {
	// Open the immutable generation transport.
	clientCtx, clientCancel := context.WithCancel(ctx)
	stream, err := service.ResourceClient(clientCtx)
	if err != nil {
		clientCancel()
		return nil, pkgerrors.Wrap(err, "start resource client stream")
	}

	// Start the sole control writer and send Init as its first packet.
	var client *Client
	controls := newResourceControlQueue(
		stream,
		func(error) {
			if client != nil {
				client.retire()
			}
		},
		nil,
	)
	if !controls.enqueue(&resource.ResourceClientRequest{Body: &resource.ResourceClientRequest_Init{
		Init: &resource.ResourceClientInitRequest{},
	}}) {
		clientCancel()
		return nil, errors.New("resource client control queue closed before init")
	}
	select {
	case err := <-controls.firstSent:
		if err != nil {
			clientCancel()
			return nil, pkgerrors.Wrap(err, "send resource client init")
		}
	case <-clientCtx.Done():
		clientCancel()
		return nil, clientCtx.Err()
	}

	// Receive and validate the matching generation identity.
	resp, err := stream.Recv()
	if err != nil {
		clientCancel()
		return nil, pkgerrors.Wrap(err, "receive resource client init")
	}
	initMsg, ok := resp.Body.(*resource.ResourceClientResponse_Init)
	if !ok || initMsg.Init == nil {
		clientCancel()
		return nil, errors.New("unexpected non-init msg as first response to ResourceClient")
	}
	clientHandleID, rootResourceID := initMsg.Init.ClientHandleId, initMsg.Init.RootResourceId
	if clientHandleID == 0 || rootResourceID == 0 {
		clientCancel()
		return nil, errors.New("resource client init returned an empty handle or root ID")
	}
	attachCtx, cancelAttach := context.WithCancel(clientCtx)
	client = &Client{
		ctx:            clientCtx,
		cancel:         clientCancel,
		attachCtx:      attachCtx,
		cancelAttach:   cancelAttach,
		service:        service,
		clientHandleID: clientHandleID,
		rootResourceID: rootResourceID,
		controls:       controls,
		done:           make(chan struct{}),
	}

	// Construct the generation-owned resource and attachment lifetimes.
	client.resourceLifetime = newResourceLifetime(clientCtx, service, controls.enqueue)
	client.attach = newAttachLifetime(client)
	go client.execute(stream)
	return client, nil
}

func (c *Client) retire() {
	c.resourceLifetime.releaseAll()
	c.cancelAttach()
	c.cancel()
}

func (c *Client) execute(stream resource.SRPCResourceService_ResourceClientClient) {
	defer close(c.done)
	defer func() {
		c.controls.retire(errors.New("resource client stream closed"))
		c.retire()
		_ = stream.Close()
	}()
	for {
		msg, err := stream.Recv()
		if err != nil {
			return
		}
		switch body := msg.Body.(type) {
		case *resource.ResourceClientResponse_ResourceReleased:
			if body.ResourceReleased != nil {
				c.resourceLifetime.releaseFromServer(body.ResourceReleased.ResourceId)
			}
		case *resource.ResourceClientResponse_ControlAck:
			if body.ControlAck == nil {
				return
			}
			if err := c.resourceLifetime.acknowledgeControl(body.ControlAck.ControlId); err != nil {
				return
			}
		}
	}
}

// AccessRootResource returns a reference to this generation's root resource.
func (c *Client) AccessRootResource() ResourceRef {
	return c.CreateResourceReference(c.rootResourceID)
}

// CreateResourceReference acquires a local reference to resourceID.
func (c *Client) CreateResourceReference(resourceID uint32) ResourceRef {
	return c.resourceLifetime.createReference(resourceID)
}

// Release queues final releases and closes this ResourceClient generation.
func (c *Client) Release() {
	c.resourceLifetime.releaseAll()
	c.cancelAttach()
	c.controls.finish()
}

// Done closes after this ResourceClient generation has drained and retired.
func (c *Client) Done() <-chan struct{} { return c.done }

// attachSession manages the single ResourceAttach stream + yamux session.
// One session serves all attached resources.
type attachSession struct {
	owner      *attachLifetime
	ctx        context.Context
	cancel     context.CancelFunc
	strm       resource.SRPCResourceService_ResourceAttachClient
	mc         srpc.MuxedConn
	router     *resource.RoutedInvoker
	pending    *attachPendingAcks
	muxes      map[uint32]struct{}
	releaseFns map[uint32]func()
	released   bool
	sendCh     chan *attachSendRequest
	closeOnce  sync.Once
	mu         sync.Mutex
}

// attachResult carries the outcome of one attach request.
type attachResult struct {
	// resourceID is the server-assigned resource id on success
	resourceID uint32
	// err is the attach failure
	err error
}

// pendingAttach tracks one in-flight attach request.
type pendingAttach struct {
	// ch carries the eventual attach result
	ch chan attachResult
	// canceled indicates the caller returned before AddAck arrived
	canceled bool
	// resolved indicates AddAck arrived before the caller selected a result.
	resolved bool
	// result is the resolved AddAck result.
	result attachResult
}

// attachSendRequest is one serialized stream send.
type attachSendRequest struct {
	// req is the attach request to send
	req *resource.ResourceAttachRequest
	// errCh receives the send result
	errCh chan error
}

// AttachRawInvoker provides a leaf callback mux that server-side handlers can
// invoke. Raw invokers are not resource trees; handlers should use
// GetAttachedResource for this shape.
func (c *Client) AttachRawInvoker(
	ctx context.Context,
	label string,
	mux srpc.Invoker,
) (uint32, error) {
	return c.AttachResource(ctx, label, mux)
}

// AttachResourceTree provides a Resource SDK tree root that server-side
// handlers can wrap as a resource reference and use to call child resources.
func (c *Client) AttachResourceTree(
	ctx context.Context,
	label string,
	mux srpc.Invoker,
) (uint32, error) {
	return c.AttachResource(ctx, label, mux)
}

// AttachResource provides a mux that server-side handlers can invoke.
// The mux is served over a yamux session inside the ResourceAttach bidi
// stream. Returns the server-assigned resource ID. Multiple resources
// share one yamux session. The caller's mux serves RPCs coming from the
// server side, routed by rpcstream component_id = resourceID.
func (c *Client) AttachResource(
	ctx context.Context,
	label string,
	mux srpc.Invoker,
) (uint32, error) {
	resourceID, _, err := c.attachResource(ctx, label, mux)
	return resourceID, err
}

func (c *Client) attachResource(
	ctx context.Context,
	label string,
	mux srpc.Invoker,
) (uint32, *attachSession, error) {
	sess, err := c.attach.ensureSession()
	if err != nil {
		return 0, nil, err
	}

	attachID, ch := sess.pending.add()

	// Send Add.
	err = sess.send(&resource.ResourceAttachRequest{
		Body: &resource.ResourceAttachRequest_Add{
			Add: &resource.ResourceAttachAdd{
				AttachId: attachID,
				Label:    label,
			},
		},
	})
	if err != nil {
		sess.pending.remove(attachID)
		return 0, nil, err
	}

	// Wait for AddAck.
	select {
	case <-ctx.Done():
		if resourceID, detach := sess.pending.cancel(attachID); detach {
			_ = sess.sendDetach(resourceID)
		}
		return 0, nil, ctx.Err()
	case <-sess.ctx.Done():
		if resourceID, detach := sess.pending.cancel(attachID); detach {
			_ = sess.sendDetach(resourceID)
		}
		return 0, nil, sess.ctx.Err()
	case result := <-ch:
		sess.pending.complete(attachID)
		if result.err != nil {
			return 0, nil, result.err
		}
		if err := sess.setMux(result.resourceID, mux); err != nil {
			return 0, nil, err
		}
		return result.resourceID, sess, nil
	}
}

// DetachResource withdraws a previously attached resource.
func (c *Client) DetachResource(ctx context.Context, resourceID uint32) error {
	sess := c.attach.currentSession()
	if sess == nil {
		return errors.New("no attach session")
	}

	err := sess.sendDetach(resourceID)
	if err != nil {
		return err
	}

	sess.releaseAttachedResource(resourceID)
	return nil
}

type attachedResourceOwner struct {
	client *Client
}

func (o *attachedResourceOwner) Context() context.Context {
	return o.client.attachCtx
}

func (o *attachedResourceOwner) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return o.AddResourceValue(mux, nil, releaseFn)
}

func (o *attachedResourceOwner) AddResourceValue(mux srpc.Invoker, _ any, releaseFn func()) (uint32, error) {
	resourceID, sess, err := o.client.attachResource(o.client.attachCtx, "attached-child", mux)
	if err != nil {
		return 0, err
	}
	if releaseFn != nil {
		sess.setRelease(resourceID, releaseFn)
	}
	return resourceID, nil
}

func (o *attachedResourceOwner) ReleaseResource(resourceID uint32) bool {
	return o.client.DetachResource(o.client.attachCtx, resourceID) == nil
}

func (o *attachedResourceOwner) GetResourceValue(resourceID uint32) (any, error) {
	return nil, resource.ErrResourceNotFound
}

func (o *attachedResourceOwner) GetAttachedResource(id uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}

// openAttachSession opens a new attach session for the client.
func (c *Client) openAttachSession() (*attachSession, error) {
	// Open ResourceAttach bidi stream.
	strm, err := c.service.ResourceAttach(c.attachCtx)
	if err != nil {
		return nil, err
	}

	// Send session-only Init.
	err = strm.Send(&resource.ResourceAttachRequest{
		Body: &resource.ResourceAttachRequest_Init{
			Init: &resource.ResourceAttachInit{
				ClientHandleId: c.clientHandleID,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Read session Ack.
	ackPkt, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	ack := ackPkt.GetAck()
	if ack == nil {
		return nil, errors.New("expected ack packet")
	}
	if ack.GetError() != "" {
		return nil, errors.New(ack.GetError())
	}

	owner := &attachedResourceOwner{client: c}
	router := resource.NewRoutedInvokerWithContext(func(ctx context.Context, _ uint32) context.Context {
		return resource_server.WithResourceClientContext(ctx, owner)
	})
	sess := newAttachSession(c.attachCtx, c.attach, strm, router)
	if err := sess.start(); err != nil {
		return nil, err
	}

	return sess, nil
}

func newAttachSession(
	ctx context.Context,
	owner *attachLifetime,
	strm resource.SRPCResourceService_ResourceAttachClient,
	router *resource.RoutedInvoker,
) *attachSession {
	sessCtx, cancel := context.WithCancel(ctx)
	return &attachSession{
		owner:      owner,
		ctx:        sessCtx,
		cancel:     cancel,
		strm:       strm,
		router:     router,
		pending:    newAttachPendingAcks(),
		muxes:      make(map[uint32]struct{}),
		releaseFns: make(map[uint32]func()),
		sendCh:     make(chan *attachSendRequest),
	}
}

func (s *attachSession) start() error {
	go s.executeSendLoop()

	mc, err := srpc.NewMuxedConnWithRwc(s.ctx, s.newRWC(), false, nil)
	if err != nil {
		s.close()
		return err
	}
	s.mc = mc

	go s.executeAccept()
	return nil
}

func (s *attachSession) newRWC() *resource.AttachMuxDataRwc {
	return resource.NewAttachMuxDataRwc(
		func(data []byte) error {
			return s.send(&resource.ResourceAttachRequest{
				Body: &resource.ResourceAttachRequest_MuxData{MuxData: data},
			})
		},
		func() ([]byte, error) {
			pkt, recvErr := s.strm.Recv()
			if recvErr != nil {
				return nil, recvErr
			}
			switch body := pkt.GetBody().(type) {
			case *resource.ResourceAttachResponse_AddAck:
				resourceID, detach := s.pending.resolve(body.AddAck)
				if detach {
					_ = s.sendDetach(resourceID)
				}
				return nil, nil
			case *resource.ResourceAttachResponse_DetachAck:
				s.releaseAttachedResource(body.DetachAck.GetResourceId())
				return nil, nil
			case *resource.ResourceAttachResponse_MuxData:
				return body.MuxData, nil
			}
			return nil, nil
		},
	)
}

func (s *attachSession) executeAccept() {
	_ = srpc.NewServer(s.router).AcceptMuxedConn(s.ctx, s.mc)
	s.close()
}

func (c *Client) setAttachedRelease(resourceID uint32, releaseFn func()) {
	c.attach.setRelease(resourceID, releaseFn)
}

func (s *attachSession) setMux(resourceID uint32, mux srpc.Invoker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.released {
		return context.Canceled
	}
	if s.muxes == nil {
		s.muxes = make(map[uint32]struct{})
	}
	s.router.SetMux(resourceID, mux)
	s.muxes[resourceID] = struct{}{}
	return nil
}

func (s *attachSession) setRelease(resourceID uint32, releaseFn func()) {
	if releaseFn == nil {
		return
	}
	s.mu.Lock()
	if !s.released {
		s.releaseFns[resourceID] = releaseFn
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	releaseFn()
}

func (s *attachSession) releaseAttachedResource(resourceID uint32) {
	s.mu.Lock()
	releaseFn := s.releaseFns[resourceID]
	_, hasMux := s.muxes[resourceID]
	delete(s.releaseFns, resourceID)
	delete(s.muxes, resourceID)
	s.mu.Unlock()

	if hasMux {
		s.router.RemoveMux(resourceID)
	}
	if releaseFn != nil {
		releaseFn()
	}
}

func (s *attachSession) releaseAllAttachedResources() {
	muxIDs, releaseFns := s.drainAttachedResources()
	s.releaseDrainedAttachedResources(muxIDs, releaseFns)
}

func (s *attachSession) drainAttachedResources() ([]uint32, []func()) {
	s.mu.Lock()
	if s.released {
		s.mu.Unlock()
		return nil, nil
	}
	s.released = true
	muxIDs := make([]uint32, 0, len(s.muxes))
	for id := range s.muxes {
		muxIDs = append(muxIDs, id)
		delete(s.muxes, id)
	}
	releaseFns := make([]func(), 0, len(s.releaseFns))
	for id, releaseFn := range s.releaseFns {
		delete(s.releaseFns, id)
		if releaseFn != nil {
			releaseFns = append(releaseFns, releaseFn)
		}
	}
	s.mu.Unlock()

	return muxIDs, releaseFns
}

func (s *attachSession) releaseDrainedAttachedResources(muxIDs []uint32, releaseFns []func()) {
	for _, id := range muxIDs {
		s.router.RemoveMux(id)
	}
	for _, releaseFn := range releaseFns {
		releaseFn()
	}
}

func (s *attachSession) close() {
	s.closeOnce.Do(func() {
		muxIDs, releaseFns := s.drainAttachedResources()
		s.cancel()
		if s.mc != nil {
			_ = s.mc.Close()
		}
		if s.strm != nil {
			if closeSend, ok := s.strm.(interface{ CloseSend() error }); ok {
				_ = closeSend.CloseSend()
			}
			_ = s.strm.Close()
		}
		if s.owner != nil {
			s.owner.clearSession(s)
		}
		s.pending.failAll(context.Canceled)
		s.releaseDrainedAttachedResources(muxIDs, releaseFns)
	})
}

func (s *attachSession) sendDetach(resourceID uint32) error {
	return s.send(&resource.ResourceAttachRequest{
		Body: &resource.ResourceAttachRequest_Detach{
			Detach: &resource.ResourceAttachDetach{
				ResourceId: resourceID,
			},
		},
	})
}

// send queues a serialized stream send on the attach session.
func (s *attachSession) send(req *resource.ResourceAttachRequest) error {
	errCh := make(chan error, 1)
	sendReq := &attachSendRequest{
		req:   req,
		errCh: errCh,
	}

	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sendCh <- sendReq:
	}

	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case err := <-errCh:
		return err
	}
}

// executeSendLoop serializes writes onto the attach stream.
func (s *attachSession) executeSendLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case sendReq := <-s.sendCh:
			sendReq.errCh <- s.strm.Send(sendReq.req)
		}
	}
}
