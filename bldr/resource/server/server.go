package resource_server

import (
	"context"
	"strconv"
	"sync"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

// ResourceServer provides the Resources RPC API.
//
// The server and client track Resource handles via integer IDs per client.
// Each resource has a unique ID, but the server may send the same resource ID
// to a client multiple times (e.g., when creating multiple references).
// The client uses reference counting to track when all references are released.
type ResourceServer struct {
	// rootResourceMux is the invoker for root resources
	rootResourceMux srpc.Invoker

	// bcast guards below fields
	// note: bcast is only ever locked for very short periods of time.
	// long-lived operations are taken while unlocked.
	// signals changes to the client transmit queues.
	bcast broadcast.Broadcast
	// clientHandleIDCtr is a counter for new handle ids.
	// add 1 to it and use the added value for the next id.
	clientHandleIDCtr uint32
	// resourceIDCtr is a counter for resource IDs across all clients.
	// globally unique to avoid ID collisions between clients.
	resourceIDCtr uint32
	// clients contains the map of ongoing client sessions.
	clients map[uint32]*RemoteResourceClient
}

// NewResourceServer constructs a new ResourceServer.
func NewResourceServer(rootResourceMux srpc.Invoker) *ResourceServer {
	if rootResourceMux == nil {
		rootResourceMux = srpc.NewMux()
	}
	return &ResourceServer{
		rootResourceMux: rootResourceMux,
		clients:         make(map[uint32]*RemoteResourceClient, 1),
	}
}

// Register registers the server with the mux.
func (s *ResourceServer) Register(mux srpc.Mux) error {
	return resource.SRPCRegisterResourceService(mux, s)
}

// ResourceClient starts an instance of a client for the ResourceService,
// yielding a new client ID. The client can use that ID for future RPCs
// accessing the Resource tree. When the streaming RPC ends, all resources
// owned by the client will be released.
func (s *ResourceServer) ResourceClient(
	req *resource.ResourceClientRequest,
	strm resource.SRPCResourceService_ResourceClientStream,
) error {
	ctx := strm.Context()

	// Add the client to the client set.
	clientCtx, clientCancel := context.WithCancel(ctx)

	var waitCh <-chan struct{}
	var clientHandleID uint32
	var clientObj *RemoteResourceClient
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		s.clientHandleIDCtr++
		clientHandleID = s.clientHandleIDCtr
		clientObj = &RemoteResourceClient{
			server:    s,
			clientID:  clientHandleID,
			ctx:       clientCtx,
			resources: make(map[uint32]*trackedResource),
		}
		s.clients[clientHandleID] = clientObj
		waitCh = getWaitCh()
	})

	// Remove the client when returning.
	defer func() {
		clientCancel()
		clientObj.releaseAllAttachedResources()
		var releaseFns []func()
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			clientObj.released = true
			delete(s.clients, clientHandleID)

			// Release all resources owned by this client
			for _, resource := range clientObj.resources {
				if resource.releaseFn != nil {
					releaseFns = append(releaseFns, resource.releaseFn)
				}
			}
		})
		for _, releaseFn := range releaseFns {
			releaseFn()
		}
	}()

	// Add root resource to client's resources
	var rootResourceID uint32
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.resourceIDCtr++
		rootResourceID = s.resourceIDCtr
		clientObj.resources[rootResourceID] = &trackedResource{
			mux:           s.rootResourceMux,
			ownerClientID: clientHandleID,
			releaseFn:     nil, // Root resource is never released
		}
	})

	// Send the init message with the assigned root resource ID.
	if err := strm.Send(&resource.ResourceClientResponse{
		Body: &resource.ResourceClientResponse_Init{
			Init: &resource.ResourceClientInit{
				ClientHandleId: clientHandleID,
				RootResourceId: rootResourceID,
			},
		},
	}); err != nil {
		return err
	}

	// Process the client message queue asynchronously.
	var released bool
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}

		var txQueue []*resource.ResourceClientResponse
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			txQueue = clientObj.txQueue
			clientObj.txQueue = nil
			released = clientObj.released
			waitCh = getWaitCh()
		})

		if released {
			return resource.ErrClientReleased
		}

		for _, event := range txQueue {
			if err := strm.Send(event); err != nil {
				return err
			}
		}
	}
}

// ResourceRpc is a rpc request for an open resource handle.
// Exposes service(s) depending on the resource type.
// Component ID: resource_id from ResourceClient call.
func (s *ResourceServer) ResourceRpc(
	strm resource.SRPCResourceService_ResourceRpcStream,
) error {
	return rpcstream.HandleRpcStream(
		strm,
		func(ctx context.Context, componentID string, released func()) (srpc.Invoker, func(), error) {
			resourceIDU64, err := strconv.ParseUint(componentID, 10, 32)
			if err != nil {
				return nil, nil, err
			}
			resourceIDU32 := uint32(resourceIDU64)

			// Look up the resource in all clients.
			var mux srpc.Invoker
			var client ResourceClientContext
			s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				for _, c := range s.clients {
					if c.released {
						continue
					}

					res := c.resources[resourceIDU32]
					if res != nil {
						mux = res.mux
						client = c
						break
					}
					ar := c.attachedResources[resourceIDU32]
					if ar != nil {
						mux = srpc.NewClientInvoker(ar.srpcClient)
						break
					}
				}
			})

			if mux == nil {
				return nil, nil, resource.ErrResourceOrClientReleased
			}

			return &resourceServerClientInvoker{mux: mux, client: client}, nil, nil
		},
	)
}

// resourceServerClientInvoker wraps an invoker to use a specific stream context.
type resourceServerClientInvoker struct {
	mux    srpc.Invoker
	client ResourceClientContext
}

func (c *resourceServerClientInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Add client context to the stream
	if c.client != nil {
		childCtx := WithResourceClientContext(strm.Context(), c.client)
		childStrm := srpc.NewStreamWithContext(strm, childCtx)
		return c.mux.InvokeMethod(serviceID, methodID, childStrm)
	}
	return c.mux.InvokeMethod(serviceID, methodID, strm)
}

// ResourceRefRelease releases a client's resource.
func (s *ResourceServer) ResourceRefRelease(
	ctx context.Context,
	req *resource.ResourceRefReleaseRequest,
) (*resource.ResourceRefReleaseResponse, error) {
	resourceID := req.GetResourceId()
	clientID := req.GetClientHandleId()
	if clientID == 0 {
		return nil, resource.ErrInvalidClientID
	}

	var found bool
	var isRootResource bool
	var attachedClient *RemoteResourceClient
	var releaseFn func()
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		client := s.clients[clientID]
		if client == nil || client.released {
			return
		}

		res := client.resources[resourceID]
		if res != nil {
			// Check if this is a root resource (has no releaseFn)
			isRootResource = res.releaseFn == nil

			// Don't actually delete root resources, just mark as found
			if !isRootResource {
				// The release RPC is the acknowledgment for dropping this
				// client reference. No ResourceClient queue event is produced.
				delete(client.resources, resourceID)
				releaseFn = res.releaseFn
			}
			found = true
			return
		}
		if ar := client.attachedResources[resourceID]; ar != nil {
			attachedClient = client
			found = true
		}
	})

	if attachedClient != nil {
		attachedClient.ReleaseResource(resourceID)
	}
	if releaseFn != nil {
		releaseFn()
	}

	if !found {
		return nil, resource.ErrResourceNotFound
	}

	return &resource.ResourceRefReleaseResponse{}, nil
}

// ResourceAttach allows a client to provide resources that server-side
// RPC handlers can invoke via getAttachedRef(id). One stream = one yamux
// session = N resources. Session-only Init/Ack, then Add/AddAck per resource.
func (s *ResourceServer) ResourceAttach(
	strm resource.SRPCResourceService_ResourceAttachStream,
) error {
	// Read Init packet.
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

	// Find owning client.
	var client *RemoteResourceClient
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		client = s.clients[clientHandleID]
	})
	if client == nil {
		_ = send(&resource.ResourceAttachResponse{
			Body: &resource.ResourceAttachResponse_Ack{
				Ack: &resource.ResourceAttachAck{Error: "client not found"},
			},
		})
		return resource.ErrResourceOrClientReleased
	}

	// Send session Ack.
	if err := send(&resource.ResourceAttachResponse{
		Body: &resource.ResourceAttachResponse_Ack{
			Ack: &resource.ResourceAttachAck{},
		},
	}); err != nil {
		return err
	}

	// Create attach context derived from the stream.
	ctx := strm.Context()
	attachCtx, attachCancel := context.WithCancel(ctx)
	defer attachCancel()

	// Track attached resources for cleanup.
	var attachedIDs []uint32
	defer func() {
		for _, id := range attachedIDs {
			client.removeAttachedResource(id, false)
		}
	}()

	// srpcClient is the shared SRPC client over the yamux session. Add control
	// packets can arrive while the yamux connection is still being built, so
	// the Client waits for the OpenStreamFunc to be bound before routing.
	var openStream srpc.OpenStreamFunc
	var openStreamMtx sync.Mutex
	openStreamReady := make(chan struct{})
	srpcClient := srpc.NewClient(func(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error) {
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

			// Allocate resource ID.
			var resourceID uint32
			s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				s.resourceIDCtr++
				resourceID = s.resourceIDCtr
			})

			// Create srpc.Client for this resource via routed SRPC over yamux.
			resClient := resource.NewRoutedClient(srpcClient, resourceID)

			// Derive a per-resource context so removing one resource does
			// not tear down the entire yamux session.
			_, resCancel := context.WithCancel(attachCtx)

			// Register on client.
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
				_ = send(&resource.ResourceAttachResponse{
					Body: &resource.ResourceAttachResponse_AddAck{
						AddAck: &resource.ResourceAttachAddAck{
							AttachId: attachID,
							Error:    addErr.Error(),
						},
					},
				})
				return
			}
			attachedIDs = append(attachedIDs, resourceID)

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
			// Remove from our cleanup list.
			for i, id := range attachedIDs {
				if id == resourceID {
					attachedIDs = append(attachedIDs[:i], attachedIDs[i+1:]...)
					break
				}
			}
			_ = send(&resource.ResourceAttachResponse{
				Body: &resource.ResourceAttachResponse_DetachAck{
					DetachAck: &resource.ResourceAttachDetachAck{
						ResourceId: resourceID,
					},
				},
			})
		}
	}

	// Build ReadWriteCloser adapter bridging mux_data and yamux.
	// The recv loop runs inside Read(), dispatching control messages via onControl.
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

	// SERVER side is yamux client (outbound=true): opens sub-streams to
	// invoke the client's muxes via routed SRPC.
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
