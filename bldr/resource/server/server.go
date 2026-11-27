// Package resource_server implements the server end of the Resource SDK: it
// tracks each client generation and the resource tree it holds.
//
// A handler creates a child resource with ConstructChildResource, or AddResource
// on the call's ResourceClientContext. The child is pending until the client
// adopts it. A child still pending when its handler returns unpublished is
// released.
// ConstructChildResource gives the child a context that outlives the call and
// lasts until the child is released or the client disconnects, and runs the
// child's release function while that context is still live on an explicit
// release.
package resource_server

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/tidwall/btree"
)

// ResourceServer serves one root resource and its client-owned descendants.
type ResourceServer struct {
	// rootResourceMux serves the retained root for every generation.
	rootResourceMux srpc.Invoker
	// filter wraps the invoker of every served resource, or is nil.
	filter InvokerFilter
	// originID is the origin controller of directives added from resource calls.
	originID string

	// bcast guards generation admission, ID allocation, routing and the handle count.
	bcast broadcast.Broadcast
	// clientHandleIDCtr allocates immutable generation IDs under bcast.
	clientHandleIDCtr uint32
	// resourceIDCtr allocates resource IDs across generations under bcast.
	resourceIDCtr uint32
	// clients retains live generations under bcast.
	clients map[uint32]*RemoteResourceClient
	// routes indexes a resource ID to its generation; lookup rechecks that generation.
	routes map[uint32]*RemoteResourceClient
	// trackedCount counts handles admitted by live generations.
	trackedCount int
}

// InvokerFilter wraps the invoker of a served resource. It sees every call a
// client makes to the root and to each resource added beneath it.
type InvokerFilter func(srpc.Invoker) srpc.Invoker

// NewResourceServer constructs a ResourceServer for rootResourceMux.
// originID names the controller for calls without an existing origin; empty leaves it unset.
func NewResourceServer(rootResourceMux srpc.Invoker, originID string) *ResourceServer {
	return NewFilteredResourceServer(rootResourceMux, nil, originID)
}

// NewFilteredResourceServer constructs a ResourceServer that routes every
// resource call through filter. A nil filter serves calls directly.
// originID names the controller for calls without an existing origin; empty leaves it unset.
func NewFilteredResourceServer(rootResourceMux srpc.Invoker, filter InvokerFilter, originID string) *ResourceServer {
	// Supply an empty service mux when the caller has no root methods.
	if rootResourceMux == nil {
		rootResourceMux = srpc.NewMux()
	}

	return &ResourceServer{
		rootResourceMux: rootResourceMux,
		filter:          filter,
		originID:        originID,
		clients:         make(map[uint32]*RemoteResourceClient),
		routes:          make(map[uint32]*RemoteResourceClient),
	}
}

// withOrigin returns ctx recording the server's origin controller.
// A ctx that already records an origin controller is returned unchanged.
func (s *ResourceServer) withOrigin(ctx context.Context) context.Context {
	if s.originID == "" || directive.OriginFromContext(ctx).ControllerID != "" {
		return ctx
	}
	return directive.WithOriginController(ctx, s.originID)
}

// Register registers the Resource service with mux.
func (s *ResourceServer) Register(mux srpc.Mux) error {
	return resource.SRPCRegisterResourceService(mux, s)
}

// ResourceClient starts a new immutable client generation. The first packet
// must be Init. Every later packet is one FIFO Adopt or Release control.
func (s *ResourceServer) ResourceClient(strm resource.SRPCResourceService_ResourceClientStream) error {
	// Require Init before allocating generation state.
	ctx := strm.Context()
	first, err := strm.Recv()
	if err != nil {
		return err
	}
	if _, ok := first.GetBody().(*resource.ResourceClientRequest_Init); !ok || first.GetInit() == nil {
		return errors.New("expected ResourceClient init packet")
	}

	// Register the immutable generation and its outbound wait channel.
	clientCtx, clientCancel := context.WithCancel(s.withOrigin(ctx))
	var client *RemoteResourceClient
	var waitCh <-chan struct{}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Publish a fully initialized generation with its retained root.
		s.clientHandleIDCtr++
		s.resourceIDCtr++
		client = &RemoteResourceClient{
			server: s, clientID: s.clientHandleIDCtr, ctx: clientCtx,
			rootResourceID:    s.resourceIDCtr,
			resources:         make(map[uint32]*trackedResource),
			children:          make(map[uint32]*btree.Map[uint32, struct{}]),
			attachedResources: make(map[uint32]*attachedResource),
		}
		client.resources[client.rootResourceID] = &trackedResource{mux: s.rootResourceMux, ownerClientID: client.clientID}
		s.clients[client.clientID] = client
		s.routes[client.rootResourceID] = client
		s.trackedCount++
		broadcast()
	})
	defer func() {
		clientCancel()
		s.releaseClientGeneration(client)
	}()

	// Observe outbound notifications on this generation alone.
	client.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) { waitCh = getWaitCh() })
	clientHandleID, rootID := client.clientID, client.rootResourceID
	if err := strm.Send(&resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_Init{
		Init: &resource.ResourceClientInit{
			ClientHandleId: clientHandleID,
			RootResourceId: rootID,
		},
	}}); err != nil {
		return err
	}

	// Receive controls independently so outbound release events cannot starve.
	controlCh := make(chan *resource.ResourceClientRequest)
	recvErr := make(chan error, 1)
	recvCtx, recvCancel := context.WithCancel(ctx)
	defer recvCancel()
	go func() {
		for {
			req, err := strm.Recv()
			if err != nil {
				select {
				case recvErr <- err:
				case <-recvCtx.Done():
				}
				return
			}
			select {
			case controlCh <- req:
			case <-recvCtx.Done():
				return
			}
		}
	}()

	// Process controls and server-originated release notifications in order.
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-recvErr:
			return err
		case req := <-controlCh:
			controlID, err := client.applyControl(req)
			if err != nil {
				return err
			}
			client.queueControlAck(controlID)
		case <-waitCh:
		}

		// Snapshot outbound notifications with the next matching wait channel.
		var txQueue []*resource.ResourceClientResponse
		client.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			txQueue, client.txQueue = client.txQueue, nil
			waitCh = getWaitCh()
		})

		// Transmit notifications outside the lifecycle lock.
		for _, event := range txQueue {
			if err := strm.Send(event); err != nil {
				return err
			}
		}
	}
}

// ResourceRpc routes one ResourceRpc stream to its generation-owned resource.
func (s *ResourceServer) ResourceRpc(strm resource.SRPCResourceService_ResourceRpcStream) error {
	return resource.HandleResourceRpc(strm, func(ctx context.Context, resourceID uint32) (srpc.Invoker, error) {
		// Reject the zero resource ID before resolving the generation.
		if resourceID == 0 {
			return nil, resource.ErrInvalidResourceID
		}

		// Look up the generation before checking its retirement fence.
		var mux srpc.Invoker
		var client *RemoteResourceClient
		var owner *RemoteResourceClient
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { owner = s.routes[resourceID] })
		if owner != nil {
			owner.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				if owner.released {
					return
				}
				if res := owner.resources[resourceID]; res != nil {
					mux, client = res.mux, owner
					return
				}
				if ar := owner.attachedResources[resourceID]; ar != nil {
					mux = srpc.NewClientInvoker(ar.srpcClient)
				}
			})
		}

		// Fail when no live generation owns the resource.
		if mux == nil {
			return nil, resource.ErrResourceOrClientReleased
		}
		if client != nil && s.filter != nil {
			mux = s.filter(mux)
		}
		return &resourceServerClientInvoker{mux: mux, client: client, parentResourceID: resourceID}, nil
	})
}

// ResourceAttach allows a client to provide resources that server-side
// RPC handlers can invoke via getAttachedRef(id). One stream = one yamux
// session = N resources. Session-only Init/Ack, then Add/AddAck per resource.
func (s *ResourceServer) ResourceAttach(
	strm resource.SRPCResourceService_ResourceAttachStream,
) error {
	// Read the Init packet.
	initPkt, err := strm.Recv()
	if err != nil {
		return err
	}
	init := initPkt.GetInit()
	if init == nil {
		return errors.New("expected init packet")
	}
	clientHandleID := init.GetClientHandleId()
	var sendMtx sync.Mutex
	send := func(resp *resource.ResourceAttachResponse) error {
		sendMtx.Lock()
		defer sendMtx.Unlock()
		return strm.Send(resp)
	}

	// Find the owning client.
	var client *RemoteResourceClient
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		client = s.clients[clientHandleID]
	})
	if client == nil {
		_ = send(&resource.ResourceAttachResponse{
			Body: &resource.ResourceAttachResponse_Ack{
				Ack: &resource.ResourceAttachAck{Failure: resource.FailureFromError(resource.ErrClientReleased)},
			},
		})
		return resource.ErrResourceOrClientReleased
	}

	// Send the session Ack.
	if err := send(&resource.ResourceAttachResponse{
		Body: &resource.ResourceAttachResponse_Ack{
			Ack: &resource.ResourceAttachAck{},
		},
	}); err != nil {
		return err
	}

	// Create an attach context derived from the stream.
	ctx := strm.Context()
	attachCtx, attachCancel := context.WithCancel(ctx)
	defer attachCancel()

	// Track attached resources for cleanup. onControl mutates attachedIDs on
	// the mux rx-pump goroutine while this cleanup runs on the RPC goroutine
	// after attachCtx is canceled, so the mutex is required for the sync edge.
	var attachedMtx sync.Mutex
	var attachedIDs []uint32

	// Remove every still-attached resource when the attach session ends.
	defer func() {
		// Snapshot the attached IDs and clear the cleanup list under the mutex.
		attachedMtx.Lock()
		ids := attachedIDs
		attachedIDs = nil
		attachedMtx.Unlock()
		for _, id := range ids {
			client.removeAttachedResource(id, false)
		}
	}()

	// srpcClient is the shared SRPC client over the yamux session. Add control
	// packets can arrive while the yamux connection is still being built, so
	// the Client waits for the OpenStreamFunc to be bound before routing.
	var openStream srpc.OpenStreamFunc
	var openStreamMtx sync.Mutex
	openStreamReady := make(chan struct{})

	// Wait for the yamux connection inside the SRPC client's packet writer.
	srpcClient := srpc.NewClient(func(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error) {
		// Wait until the yamux connection is bound or the contexts end.
		select {
		case <-openStreamReady:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-attachCtx.Done():
			return nil, attachCtx.Err()
		}
		openStreamMtx.Lock()
		fn := openStream
		openStreamMtx.Unlock()
		return fn(ctx, msgHandler, closeHandler)
	})

	// onControl handles Add and Detach messages inline from the recv loop.
	onControl := func(req *resource.ResourceAttachRequest) {
		switch body := req.GetBody().(type) {
		case *resource.ResourceAttachRequest_Add:
			add := body.Add
			attachID := add.GetAttachId()
			label := add.GetLabel()

			// Allocate a resource ID.
			var resourceID uint32
			s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				s.resourceIDCtr++
				resourceID = s.resourceIDCtr
			})

			// Derive a per-resource context so removing one resource does
			// not tear down the entire yamux session.
			resCtx, resCancel := context.WithCancel(attachCtx)

			// Create an srpc.Client for this resource via
			// routed SRPC over yamux.
			resClient := resource.NewRoutedClientWithDone(srpcClient, resourceID, resCtx.Done())

			// Register the resource on the client.
			addErr := client.AddAttachedResource(resourceID, label, resCancel, resClient, func() {
				_ = send(&resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_DetachAck{
						DetachAck: &resource.ResourceAttachDetachAck{
							ResourceId: resourceID,
						},
					},
				})
			})
			if addErr != nil {
				resCancel()
				_ = send(&resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId: attachID,
							Failure:  resource.FailureFromError(addErr),
						},
					},
				})
				return
			}
			attachedMtx.Lock()
			attachedIDs = append(attachedIDs, resourceID)
			attachedMtx.Unlock()

			_ = send(&resource.ResourceAttachResponse{
				Body: &resource.ResourceAttachResponse_AddAck{
					AddAck: &resource.ResourceAttachAddAck{
						AttachId:   attachID,
						ResourceId: resourceID,
					},
				},
			})

		case *resource.ResourceAttachRequest_Detach:
			resourceID := body.Detach.GetResourceId()
			client.removeAttachedResource(resourceID, false)
			// Remove the resource ID from the cleanup list.
			attachedMtx.Lock()
			for i, id := range attachedIDs {
				if id == resourceID {
					attachedIDs = append(attachedIDs[:i], attachedIDs[i+1:]...)
					break
				}
			}
			attachedMtx.Unlock()
			_ = send(&resource.ResourceAttachResponse{
				Body: &resource.ResourceAttachResponse_DetachAck{
					DetachAck: &resource.ResourceAttachDetachAck{
						ResourceId: resourceID,
					},
				},
			})
		}
	}

	// Build a ReadWriteCloser adapter bridging mux_data and yamux.
	// The recv loop runs inside Read(), dispatching control messages via
	// onControl.
	rwc := resource.NewAttachMuxDataRwc(
		func(data []byte) error {
			return send(&resource.ResourceAttachResponse{
				Body: &resource.ResourceAttachResponse_MuxData{MuxData: data},
			})
		},
		func() ([]byte, error) {
			pkt, recvErr := strm.Recv()
			if recvErr != nil {
				return nil, recvErr
			}
			switch pkt.GetBody().(type) {
			case *resource.ResourceAttachRequest_Add, *resource.ResourceAttachRequest_Detach:
				onControl(pkt)
				return nil, nil
			case *resource.ResourceAttachRequest_MuxData:
				return pkt.GetMuxData(), nil
			}
			return nil, nil
		},
	)

	// The SERVER side is the yamux client (outbound=true): opens sub-streams
	// to invoke the client's muxes via routed SRPC.
	mc, mcErr := srpc.NewMuxedConnWithRwc(attachCtx, rwc, true, nil)
	if mcErr != nil {
		return mcErr
	}
	openStreamMtx.Lock()
	openStream = srpc.NewOpenStreamWithMuxedConn(mc)
	openStreamMtx.Unlock()
	close(openStreamReady)

	// Block until the attach context is canceled (stream closes or client disconnects).
	<-attachCtx.Done()
	return nil
}

// _ is a type assertion
var _ resource.SRPCResourceServiceServer = (*ResourceServer)(nil)
