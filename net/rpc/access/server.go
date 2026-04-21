package bifrost_rpc_access

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	srpc "github.com/aperturerobotics/starpc/srpc"
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
	var bcast broadcast.Broadcast
	var sendQueue []*LookupRpcServiceResponse
	var disposed bool
	var resErr error
	var resIdle bool

	serverID := req.GetServerId()
	if s.serverIdCb != nil {
		var err error
		serverID, err = s.serverIdCb(serverID)
		if err != nil {
			return err
		}
	}

	var waitCh <-chan struct{}
	bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	dir := bifrost_rpc.NewLookupRpcService(req.GetServiceId(), serverID)
	vals := make(map[uint32]struct{})
	di, ref, err := s.b.AddDirective(dir, bus.NewCallbackHandler(
		func(av directive.AttachedValue) {
			_, ok := av.GetValue().(bifrost_rpc.LookupRpcServiceValue)
			if !ok {
				return
			}
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

	defer di.AddIdleCallback(func(isIdle bool, resErrs []error) {
		bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if resErr == nil {
				for _, err := range resErrs {
					if err != nil {
						resErr = err
						broadcast()
						break
					}
				}
			}
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

	for {
		select {
		case <-strm.Context().Done():
			return context.Canceled
		case <-waitCh:
		}

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
		if currIdle && currResErr != nil && currResErr != context.Canceled {
			return currResErr
		}
		for _, msg := range currSendQueue {
			if err := strm.Send(msg); err != nil {
				return err
			}
		}
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
		if err := req.Validate(); err != nil {
			return nil, nil, err
		}
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
		if len(invokers) == 0 {
			invokerRef.Release()
			return nil, nil, nil
		}
		// return the invoker slice
		return srpc.InvokerSlice(invokers), invokerRef.Release, nil
	})
}

// _ is a type assertion
var _ SRPCAccessRpcServiceServer = ((*AccessRpcServiceServer)(nil))
