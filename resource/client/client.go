package resource_client

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/aperturerobotics/bldr/resource"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
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

// Client manages connections to remote resources via RPC.
// Handles resource lifecycle, reference counting, and cleanup.
//
// Note: Server-side handlers may send the same resource ID to the client multiple times.
// Additionally, client code may create multiple references to the same resource ID.
// We use reference counting (via resourceRefSet) to track when all client-side
// references to a resource have been released before notifying the server.
type Client struct {
	// ctx is the context for the client (canceled when client is released)
	ctx context.Context
	// cancel cancels the context
	cancel context.CancelFunc
	// service is the resource service client
	service resource.SRPCResourceServiceClient
	// clientHandleID is the client handle ID from initialization
	clientHandleID uint32
	// rootResourceID is the root resource ID from initialization
	rootResourceID uint32
	// mtx guards below fields
	mtx sync.Mutex
	// resources tracks all references to each resource ID
	// map[resource_id]*resourceRefSet
	resources map[uint32]*resourceRefSet
	// srpcClients holds lazy-created SRPC clients per resource
	// map[resource_id]srpc.Client
	srpcClients map[uint32]srpc.Client
	// resourceContexts holds per-resource contexts for cancellation
	// map[resource_id]context.CancelFunc
	resourceContexts map[uint32]context.CancelFunc
	// attachSess is the single attach session (lazy, one per client).
	attachSess *attachSession
}

// resourceRefSet tracks all references to a single resource ID.
type resourceRefSet struct {
	// refs contains all active references
	refs map[*resourceRef]struct{}
	// released indicates if this resource was released by the server
	released bool
}

// NewClient constructs and initializes a new Client.
// Does not return until the init message is received from the server.
// The context is used for the persistent client connection.
func NewClient(ctx context.Context, service resource.SRPCResourceServiceClient) (*Client, error) {
	clientCtx, clientCancel := context.WithCancel(ctx)

	// Start ResourceClient stream
	stream, err := service.ResourceClient(clientCtx, &resource.ResourceClientRequest{})
	if err != nil {
		clientCancel()
		return nil, err
	}

	// Wait for init message
	resp, err := stream.Recv()
	if err != nil {
		clientCancel()
		return nil, err
	}

	// Handle error response
	if errMsg, ok := resp.Body.(*resource.ResourceClientResponse_ClientError); ok {
		clientCancel()
		return nil, errors.New(errMsg.ClientError)
	}

	// Handle successful init
	initMsg, ok := resp.Body.(*resource.ResourceClientResponse_Init)
	if !ok || initMsg.Init == nil {
		clientCancel()
		return nil, errors.New("unexpected non-init msg as first response to ResourceClient")
	}

	clientHandleID, rootResourceID := initMsg.Init.ClientHandleId, initMsg.Init.RootResourceId
	if clientHandleID == 0 {
		clientCancel()
		return nil, errors.New("unexpected empty client handle id in resource client init")
	}
	if rootResourceID == 0 {
		clientCancel()
		return nil, errors.New("unexpected empty root resource id in resource client init")
	}

	client := &Client{
		ctx:              clientCtx,
		cancel:           clientCancel,
		service:          service,
		clientHandleID:   clientHandleID,
		rootResourceID:   rootResourceID,
		resources:        make(map[uint32]*resourceRefSet),
		srpcClients:      make(map[uint32]srpc.Client),
		resourceContexts: make(map[uint32]context.CancelFunc),
	}

	// Start background goroutine to handle resource notifications
	go client.execute(stream)

	return client, nil
}

// execute is the goroutine managing the Client.
// Handles incoming ResourceClientResponse messages from the server.
func (c *Client) execute(stream resource.SRPCResourceService_ResourceClientClient) {
	defer func() {
		c.Release()
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
				c.handleServerResourceRelease(body.ResourceReleased.ResourceId)
			}
		case *resource.ResourceClientResponse_ClientError:
			// Server sent an error, close the client
			return
		}
	}
}

// handleServerResourceRelease handles server-initiated resource releases.
func (c *Client) handleServerResourceRelease(resourceID uint32) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	refSet, ok := c.resources[resourceID]
	if !ok {
		return
	}

	// Mark as released by server
	refSet.released = true

	// Mark all client-side refs as released
	for ref := range refSet.refs {
		ref.released = true
	}

	// Cancel resource context to close all streams
	if cancel, ok := c.resourceContexts[resourceID]; ok {
		cancel()
		delete(c.resourceContexts, resourceID)
	}

	// Clean up
	delete(c.resources, resourceID)
	delete(c.srpcClients, resourceID)
}

// AccessRootResource gets a reference to the root resource.
// The client must already be initialized (via NewClient).
func (c *Client) AccessRootResource() ResourceRef {
	return c.CreateResourceReference(c.rootResourceID)
}

// CreateResourceReference creates a reference to a specific resource by ID.
// The resource should already exist on the server.
// Multiple references to the same resource ID are tracked via reference counting.
func (c *Client) CreateResourceReference(resourceID uint32) ResourceRef {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Get or create resource ref set
	refSet, ok := c.resources[resourceID]
	if !ok {
		refSet = &resourceRefSet{
			refs: make(map[*resourceRef]struct{}),
		}
		c.resources[resourceID] = refSet
	}

	// Create new reference
	ref := &resourceRef{
		client:     c,
		resourceID: resourceID,
	}

	// Track this reference
	refSet.refs[ref] = struct{}{}

	return ref
}

// Release releases the client and all resources.
// All sub-resources will be automatically released as well.
func (c *Client) Release() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Mark all refs as released
	for _, refSet := range c.resources {
		refSet.released = true
		for ref := range refSet.refs {
			ref.released = true
		}
	}

	// Cancel all resource contexts
	for _, cancel := range c.resourceContexts {
		cancel()
	}

	// Clean up
	clear(c.resources)
	clear(c.srpcClients)
	clear(c.resourceContexts)

	// Cancel context
	c.cancel()
}

// attachSession manages the single ResourceAttach stream + yamux session.
// One session serves all attached resources.
type attachSession struct {
	strm        resource.SRPCResourceService_ResourceAttachClient
	mc          srpc.MuxedConn
	router      *resource.RoutedInvoker
	attachIDCtr uint32
	pending     map[uint32]chan uint32 // attachID -> result chan
	mu          sync.Mutex
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
	c.mtx.Lock()
	sess := c.attachSess
	c.mtx.Unlock()

	if sess == nil {
		var err error
		sess, err = c.ensureAttachSession(ctx)
		if err != nil {
			return 0, err
		}
	}

	// Allocate attach correlation ID.
	sess.mu.Lock()
	sess.attachIDCtr++
	attachID := sess.attachIDCtr
	ch := make(chan uint32, 1)
	sess.pending[attachID] = ch
	sess.mu.Unlock()

	// Send Add.
	err := sess.strm.Send(&resource.ResourceAttachRequest{
		Body: &resource.ResourceAttachRequest_Add{
			Add: &resource.ResourceAttachAdd{
				AttachId: attachID,
				Label:    label,
			},
		},
	})
	if err != nil {
		sess.mu.Lock()
		delete(sess.pending, attachID)
		sess.mu.Unlock()
		return 0, err
	}

	// Wait for AddAck.
	select {
	case <-ctx.Done():
		sess.mu.Lock()
		delete(sess.pending, attachID)
		sess.mu.Unlock()
		return 0, ctx.Err()
	case resourceID := <-ch:
		sess.router.SetMux(resourceID, mux)
		return resourceID, nil
	}
}

// DetachResource withdraws a previously attached resource.
func (c *Client) DetachResource(ctx context.Context, resourceID uint32) error {
	c.mtx.Lock()
	sess := c.attachSess
	c.mtx.Unlock()

	if sess == nil {
		return errors.New("no attach session")
	}

	err := sess.strm.Send(&resource.ResourceAttachRequest{
		Body: &resource.ResourceAttachRequest_Detach{
			Detach: &resource.ResourceAttachDetach{
				ResourceId: resourceID,
			},
		},
	})
	if err != nil {
		return err
	}

	sess.router.RemoveMux(resourceID)
	return nil
}

// ensureAttachSession opens the ResourceAttach stream if not already open.
func (c *Client) ensureAttachSession(ctx context.Context) (*attachSession, error) {
	c.mtx.Lock()
	if c.attachSess != nil {
		sess := c.attachSess
		c.mtx.Unlock()
		return sess, nil
	}
	c.mtx.Unlock()

	// Open ResourceAttach bidi stream.
	strm, err := c.service.ResourceAttach(c.ctx)
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

	router := resource.NewRoutedInvoker()
	sess := &attachSession{
		strm:    strm,
		router:  router,
		pending: make(map[uint32]chan uint32),
	}

	// Build ReadWriteCloser adapter bridging mux_data and yamux.
	// The recv loop dispatches control messages (AddAck, DetachAck) inline.
	rwc := resource.NewAttachMuxDataRwc(
		func(data []byte) error {
			return strm.Send(&resource.ResourceAttachRequest{
				Body: &resource.ResourceAttachRequest_MuxData{MuxData: data},
			})
		},
		func() ([]byte, error) {
			pkt, recvErr := strm.Recv()
			if recvErr != nil {
				return nil, recvErr
			}
			switch body := pkt.GetBody().(type) {
			case *resource.ResourceAttachResponse_AddAck:
				addAck := body.AddAck
				if addAck.GetError() != "" {
					return nil, nil
				}
				sess.mu.Lock()
				ch := sess.pending[addAck.GetAttachId()]
				delete(sess.pending, addAck.GetAttachId())
				sess.mu.Unlock()
				if ch != nil {
					ch <- addAck.GetResourceId()
				}
				return nil, nil
			case *resource.ResourceAttachResponse_DetachAck:
				return nil, nil
			case *resource.ResourceAttachResponse_MuxData:
				return body.MuxData, nil
			}
			return nil, nil
		},
	)

	// CLIENT side is yamux server (outbound=false): accepts sub-streams
	// from the server to serve RPCs via routed SRPC.
	mc, err := srpc.NewMuxedConnWithRwc(ctx, rwc, false, nil)
	if err != nil {
		return nil, err
	}
	sess.mc = mc

	// Accept incoming yamux sub-streams, dispatch via routed invoker.
	srv := srpc.NewServer(router)
	go srv.AcceptMuxedConn(ctx, mc)

	c.mtx.Lock()
	c.attachSess = sess
	c.mtx.Unlock()

	return sess, nil
}

// releaseResourceRefLocked is called when a client-side reference is released.
// Only notifies the server when the last reference to a resource ID is released.
// Must be called with mtx held.
func (c *Client) releaseResourceRefLocked(ref *resourceRef) {
	resourceID := ref.resourceID

	refSet, ok := c.resources[resourceID]
	if !ok {
		return
	}

	// Remove this specific ref
	delete(refSet.refs, ref)

	// If no more client-side references, clean up completely
	if len(refSet.refs) == 0 {
		// Cancel resource context to close all streams
		if cancel, ok := c.resourceContexts[resourceID]; ok {
			cancel()
			delete(c.resourceContexts, resourceID)
		}

		delete(c.resources, resourceID)
		delete(c.srpcClients, resourceID)

		// Notify server (best-effort, ignore errors)
		// Use client context - when it's canceled, the ResourceClient stream ends anyway
		go func() {
			_, _ = c.service.ResourceRefRelease(c.ctx, &resource.ResourceRefReleaseRequest{
				ClientHandleId: c.clientHandleID,
				ResourceId:     resourceID,
			})
		}()
	}
}

// getOrCreateSRPCClientLocked gets or creates an SRPC client for a resource.
// Must be called with mtx held.
func (c *Client) getOrCreateSRPCClientLocked(resourceID uint32) (srpc.Client, error) {
	// Check if client already exists
	if client, ok := c.srpcClients[resourceID]; ok {
		return client, nil
	}

	// Check if resource exists
	if _, ok := c.resources[resourceID]; !ok {
		return nil, resource.ErrResourceNotFound
	}

	// Create per-resource context derived from client context
	// This allows canceling all streams for this resource when it's released
	resourceCtx, resourceCancel := context.WithCancel(c.ctx)
	c.resourceContexts[resourceID] = resourceCancel

	// Create new SRPC client using rpcstream pattern
	// The service.ResourceRpc function returns SRPCResourceService_ResourceRpcClient which implements rpcstream.RpcStream
	resourceIDStr := strconv.FormatUint(uint64(resourceID), 10)

	// Wrap the rpcCaller to use the per-resource context
	wrappedCaller := func(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
		return c.service.ResourceRpc(resourceCtx)
	}

	client := rpcstream.NewRpcStreamClient(
		wrappedCaller, // RpcStreamCaller
		resourceIDStr, // componentID
		true,          // waitAck
	)

	// Cache the client
	c.srpcClients[resourceID] = client

	return client, nil
}

// resourceRef implements ResourceRef.
type resourceRef struct {
	client     *Client
	resourceID uint32
	released   bool // protected by client.mtx
}

func (r *resourceRef) GetResourceID() uint32 {
	return r.resourceID
}

// GetClient returns the srpc.Client or an error if the resource or client was released.
func (r *resourceRef) GetClient() (srpc.Client, error) {
	r.client.mtx.Lock()
	defer r.client.mtx.Unlock()

	if r.released {
		return nil, resource.ErrResourceOrClientReleased
	}

	return r.client.getOrCreateSRPCClientLocked(r.resourceID)
}

// Release releases the resource ref.
func (r *resourceRef) Release() {
	r.client.mtx.Lock()
	defer r.client.mtx.Unlock()

	if r.released {
		return
	}
	r.released = true

	r.client.releaseResourceRefLocked(r)
}
