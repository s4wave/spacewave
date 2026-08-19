package resource_server

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/sirupsen/logrus"
)

// ResourceServer serves one root resource and its client-owned descendants.
type ResourceServer struct {
	rootResourceMux srpc.Invoker
	le              *logrus.Entry

	pendingWarningAge     time.Duration
	now                   func() time.Time
	pendingWarningHandler func(pendingResourceWarning)

	// bcast guards the client and resource lifecycle state below
	bcast             broadcast.Broadcast
	clientHandleIDCtr uint32
	resourceIDCtr     uint32
	clients           map[uint32]*RemoteResourceClient
}

// NewResourceServer constructs a ResourceServer for rootResourceMux.
func NewResourceServer(rootResourceMux srpc.Invoker) *ResourceServer {
	if rootResourceMux == nil {
		rootResourceMux = srpc.NewMux()
	}
	return &ResourceServer{
		rootResourceMux:   rootResourceMux,
		le:                logrus.NewEntry(logrus.New()),
		pendingWarningAge: 10 * time.Second,
		now:               time.Now,
		clients:           make(map[uint32]*RemoteResourceClient),
	}
}

// Register registers the Resource service with mux.
func (s *ResourceServer) Register(mux srpc.Mux) error {
	return resource.SRPCRegisterResourceService(mux, s)
}

// ResourceClient starts a new immutable client generation. The first packet
// must be Init; every later packet is one FIFO Adopt or Release control.
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
	clientCtx, clientCancel := context.WithCancel(ctx)
	var client *RemoteResourceClient
	var clientHandleID uint32
	var waitCh <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		s.clientHandleIDCtr++
		clientHandleID = s.clientHandleIDCtr
		client = &RemoteResourceClient{
			server:            s,
			clientID:          clientHandleID,
			ctx:               clientCtx,
			resources:         make(map[uint32]*trackedResource),
			children:          make(map[uint32]map[uint32]struct{}),
			tombstones:        make(map[uint32]struct{}),
			attachedResources: make(map[uint32]*attachedResource),
		}
		s.clients[clientHandleID] = client
		waitCh = getWaitCh()
	})
	defer func() {
		clientCancel()
		s.releaseClientGeneration(client)
	}()

	// Retain the root for the generation and publish its identity.
	var rootID uint32
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.resourceIDCtr++
		rootID = s.resourceIDCtr
		client.rootResourceID = rootID
		client.resources[rootID] = &trackedResource{
			mux:           s.rootResourceMux,
			ownerClientID: clientHandleID,
			createdAt:     s.now(),
		}
		broadcast()
	})
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
	go s.scanPendingResources(clientCtx, client)

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
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
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

func (s *ResourceServer) releaseClientGeneration(client *RemoteResourceClient) {
	// Stop attached resource transports before clearing generation state.
	client.releaseAllAttachedResources()

	// Remove the retained root tree and any detached resources child-first.
	var releaseFns []func()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		client.released = true
		delete(s.clients, client.clientID)
		if client.rootResourceID != 0 {
			client.releaseAllChildrenLocked(client.rootResourceID, &releaseFns)
		}
		remaining := make([]uint32, 0, len(client.resources))
		for id := range client.resources {
			remaining = append(remaining, id)
		}
		slices.Sort(remaining)
		for _, id := range remaining {
			client.releaseAllChildrenLocked(id, &releaseFns)
		}
		clear(client.resources)
		clear(client.children)
		clear(client.tombstones)
		broadcast()
	})

	// Run resource callbacks after lifecycle state is no longer visible.
	for _, releaseFn := range releaseFns {
		if releaseFn != nil {
			releaseFn()
		}
	}
}

// ResourceRpc routes one ResourceRpc stream to its generation-owned resource.
func (s *ResourceServer) ResourceRpc(strm resource.SRPCResourceService_ResourceRpcStream) error {
	return rpcstream.HandleRpcStream(strm, func(ctx context.Context, componentID string, _ func()) (srpc.Invoker, func(), error) {
		resourceIDU64, err := strconv.ParseUint(componentID, 10, 32)
		if err != nil {
			return nil, nil, resource.ErrInvalidComponentIDFormat
		}
		resourceID := uint32(resourceIDU64)
		var mux srpc.Invoker
		var client *RemoteResourceClient
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, candidate := range s.clients {
				if candidate.released {
					continue
				}
				if res := candidate.resources[resourceID]; res != nil {
					mux, client = res.mux, candidate
					break
				}
				if ar := candidate.attachedResources[resourceID]; ar != nil {
					mux = srpc.NewClientInvoker(ar.srpcClient)
					break
				}
			}
		})
		if mux == nil {
			return nil, nil, resource.ErrResourceOrClientReleased
		}
		return &resourceServerClientInvoker{mux: mux, client: client, parentResourceID: resourceID}, nil, nil
	})
}

type resourceServerClientInvoker struct {
	mux              srpc.Invoker
	client           *RemoteResourceClient
	parentResourceID uint32
}

func (c *resourceServerClientInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if c.client == nil {
		return c.mux.InvokeMethod(serviceID, methodID, strm)
	}
	resourceCtx := newResourceRPCContext(c.client, c.parentResourceID, serviceID, methodID)
	childCtx := WithResourceClientContext(strm.Context(), resourceCtx)
	return c.mux.InvokeMethod(serviceID, methodID, srpc.NewStreamWithContext(strm, childCtx))
}

// _ is a type assertion.
var _ resource.SRPCResourceServiceServer = (*ResourceServer)(nil)

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

	// Track attached resources for cleanup. onControl mutates attachedIDs on
	// the mux rx-pump goroutine while this cleanup runs on the RPC goroutine
	// after attachCtx is canceled, so the mutex is required for the sync edge.
	var attachedMtx sync.Mutex
	var attachedIDs []uint32
	defer func() {
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

			// Derive a per-resource context so removing one resource does
			// not tear down the entire yamux session.
			resCtx, resCancel := context.WithCancel(attachCtx)

			// Create srpc.Client for this resource via routed SRPC over yamux.
			resClient := resource.NewRoutedClientWithDone(srpcClient, resourceID, resCtx.Done())

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
			// Remove from our cleanup list.
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
