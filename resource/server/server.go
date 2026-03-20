package resource_server

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/bldr/resource"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
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
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			clientObj.released = true
			delete(s.clients, clientHandleID)

			// Release all resources owned by this client
			for _, resource := range clientObj.resources {
				if resource.releaseFn != nil {
					go resource.releaseFn()
				}
			}
		})
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

			// Look up the resource in all clients
			var mux srpc.Invoker
			var client *RemoteResourceClient
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
	client *RemoteResourceClient
}

func (c *resourceServerClientInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Add client context to the stream
	childCtx := WithResourceClientContext(strm.Context(), c.client)
	childStrm := srpc.NewStreamWithContext(strm, childCtx)
	return c.mux.InvokeMethod(serviceID, methodID, childStrm)
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
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		client := s.clients[clientID]
		if client == nil || client.released {
			return
		}

		res := client.resources[resourceID]
		if res == nil {
			return
		}

		// Check if this is a root resource (has no releaseFn)
		isRootResource = res.releaseFn == nil

		// Don't actually delete root resources, just mark as found
		if !isRootResource {
			delete(client.resources, resourceID)
			broadcast()

			// Call release callback if provided
			if res.releaseFn != nil {
				go res.releaseFn()
			}
		}
		found = true
	})

	if !found {
		return nil, resource.ErrResourceNotFound
	}

	return &resource.ResourceRefReleaseResponse{}, nil
}

// _ is a type assertion
var _ resource.SRPCResourceServiceServer
