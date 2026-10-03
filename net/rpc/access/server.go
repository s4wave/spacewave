package bifrost_rpc_access

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// AccessRpcServiceServer is the server for AccessRpcService.
// If waitOne is set, waits for at least one value before returning.
type AccessRpcServiceServer struct {
	b       bus.Bus
	waitOne bool

	// serverIdCb is an optional callback to override the ServerID.
	serverIdCb func(remoteServerID string) (string, error)
}

// NewAccessRpcServiceServer builds a AccessRpcService server with a bus.
// If waitOne is set, waits for at least one value before returning.
// serverIdCb is an optional callback to override the ServerID.
func NewAccessRpcServiceServer(
	b bus.Bus,
	waitOne bool,
	serverIdCb func(remoteServerID string) (string, error),
) *AccessRpcServiceServer {
	return &AccessRpcServiceServer{b: b, waitOne: waitOne, serverIdCb: serverIdCb}
}

// LookupRpcService looks up the rpc service via the bus.
func (s *AccessRpcServiceServer) LookupRpcService(
	req *LookupRpcServiceRequest,
	strm SRPCAccessRpcService_LookupRpcServiceStream,
) error {
	// Track service availability and queued responses under the broadcast lock.
	var bcast broadcast.Broadcast
	var sendQueue []*LookupRpcServiceResponse
	var disposed bool
	var resErr error
	var resIdle bool

	// Resolve the requested server through the server ID override.
	serverID := req.GetServerId()
	if s.serverIdCb != nil {
		var err error
		serverID, err = s.serverIdCb(serverID)
		if err != nil {
			return err
		}
	}

	// Subscribe to service changes before attaching the lookup directive.
	var waitCh <-chan struct{}
	bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	// Attach the service lookup and queue availability changes until release.
	dir := bifrost_rpc.NewLookupRpcService(req.GetServiceId(), serverID)
	vals := make(map[uint32]struct{})
	di, ref, err := s.b.AddDirective(dir, bus.NewCallbackHandler(
		func(av directive.AttachedValue) {
			// Accept only RPC service invokers from the lookup directive.
			_, ok := av.GetValue().(bifrost_rpc.LookupRpcServiceValue)
			if !ok {
				return
			}

			// Notify the stream when its first service invoker arrives.
			bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				vals[av.GetValueID()] = struct{}{}
				if len(vals) == 1 {
					sendQueue = append(sendQueue, &LookupRpcServiceResponse{
						Exists: true,
					})
					broadcast()
				}
			})
		}, func(av directive.AttachedValue) {
			bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				_, exists := vals[av.GetValueID()]
				if !exists {
					return
				}
				delete(vals, av.GetValueID())
				if len(vals) == 0 {
					sendQueue = append(sendQueue, &LookupRpcServiceResponse{
						Removed: true,
					})
					broadcast()
				}
			})
		}, func() {
			bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				if !disposed {
					disposed = true
					broadcast()
				}
			})
		},
	))
	if err != nil {
		return err
	}
	defer ref.Release()

	// Queue resolver idle transitions and retain the first resolver error.
	defer di.AddIdleCallback(func(isIdle bool, resErrs []error) {
		bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// Retain the first lookup failure for the stream to report.
			if resErr == nil {
				for _, err := range resErrs {
					if err != nil {
						resErr = err
						broadcast()
						break
					}
				}
			}

			// Queue the lookup idle state only when it changes.
			if isIdle == resIdle {
				return
			}
			resIdle = isIdle
			sendQueue = append(sendQueue, &LookupRpcServiceResponse{
				Idle: isIdle,
			})
			broadcast()
		})
	})()

	// Send lookup changes until cancellation, resolver failure, or disposal.
	for {
		// Wait for the next lookup change or stream cancellation.
		select {
		case <-strm.Context().Done():
			return context.Canceled
		case <-waitCh:
		}

		// Drain queued responses with the current lookup state and next wakeup.
		var currSendQueue []*LookupRpcServiceResponse
		var currDisposed bool
		var currResErr error
		var currIdle bool
		bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			currSendQueue, currDisposed = sendQueue, disposed
			currResErr, currIdle = resErr, resIdle
			sendQueue = nil
		})

		// Report a failed lookup once its resolvers become idle.
		if currIdle && currResErr != nil && currResErr != context.Canceled {
			return currResErr
		}

		// Deliver each queued availability or idle response to the stream.
		for _, msg := range currSendQueue {
			if err := strm.Send(msg); err != nil {
				return err
			}
		}

		// End the stream when the lookup directive is disposed.
		if currDisposed {
			return errors.New("directive disposed")
		}
	}
}

// CallRpcService looks up the rpc service with the request & invokes the RPC.
func (s *AccessRpcServiceServer) CallRpcService(strm SRPCAccessRpcService_CallRpcServiceStream) error {
	return rpcstream.HandleRpcStream(strm, func(ctx context.Context, componentID string, released func()) (srpc.Invoker, func(), error) {
		// parse component id json
		req := &LookupRpcServiceRequest{}
		if err := req.UnmarshalComponentID(componentID); err != nil {
			return nil, nil, err
		}

		// Validate the requested service before resolving its server.
		if err := req.Validate(); err != nil {
			return nil, nil, err
		}

		// Resolve the requested server through the server ID override.
		serverID := req.GetServerId()
		if s.serverIdCb != nil {
			var err error
			serverID, err = s.serverIdCb(serverID)
			if err != nil {
				return nil, nil, err
			}
		}

		// lookup the rpc service invokers
		invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
			ctx,
			s.b,
			req.GetServiceId(),
			serverID,
			s.waitOne,
			released,
		)
		if err != nil || invokerRef == nil {
			return nil, nil, err
		}

		// Release a lookup that produced no callable service.
		if len(invokers) == 0 {
			invokerRef.Release()
			return nil, nil, nil
		}

		// return the invoker slice
		return srpc.InvokerSlice(invokers), invokerRef.Release, nil
	})
}

// _ is a type assertion
var _ SRPCAccessRpcServiceServer = (*AccessRpcServiceServer)(nil)
