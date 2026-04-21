package bifrost_rpc_access

import (
	"errors"
	"io"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ProxyInvoker is an srpc.Invoker that invokes via the proxy client.
type ProxyInvoker struct {
	client  SRPCAccessRpcServiceClient
	req     *LookupRpcServiceRequest
	waitAck bool
}

// NewProxyInvoker constructs a new srpc.Invoker with a client and request.
//
// if waitAck is set, waits for ack from the remote before starting the proxied rpc.
// note: usually you do not need waitAck set to true.
func NewProxyInvoker(client SRPCAccessRpcServiceClient, req *LookupRpcServiceRequest, waitAck bool) *ProxyInvoker {
	return &ProxyInvoker{client: client, req: req, waitAck: waitAck}
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (r *ProxyInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	req := r.req
	if serviceID != "" && serviceID != req.GetServiceId() {
		req = req.CloneVT()
		req.ServiceId = serviceID
	}
	componentID, err := req.MarshalComponentID()
	if err != nil {
		return false, err
	}

	// Remote will lookup the service, then return either an error or ack.
	rpcStream, err := rpcstream.OpenRpcStream(strm.Context(), r.client.CallRpcService, componentID, r.waitAck)
	if err != nil {
		return false, err
	}
	defer rpcStream.Close()

	// each packet in rpcStream is now either an Ack or a Body packet.
	// each Body packet contains a *srpc.Packet from the remote service.

	// Start the RPC with the remote
	startPkt := srpc.NewCallStartPacket(serviceID, methodID, nil, false)
	packetWriter := rpcstream.NewRpcStreamWriter(rpcStream)
	if err := packetWriter.WritePacket(startPkt); err != nil {
		return false, err
	}

	errCh := make(chan error, 3)

	// Read messages from prw -> write to invoker stream.
	go func() {
		proxyMsg := srpc.NewRawMessage(nil, false) // zero-copy mode

		// We have to handle the Packet here because srpc.Stream MsgSend will be
		// encoded and wrapped in a Body packet.
		handler := srpc.NewPacketDataHandler(func(pkt *srpc.Packet) error {
			switch body := pkt.GetBody().(type) {
			case *srpc.Packet_CallCancel:
				// unexpected from server -> client but handle anyway
				return errors.New("rpc canceled by the remote")
			case *srpc.Packet_CallData:
				data, dataIsZero := body.CallData.GetData(), body.CallData.GetDataIsZero()
				complete, errStr := body.CallData.GetComplete(), body.CallData.GetError()
				if len(data) != 0 || dataIsZero {
					proxyMsg.SetData(data)
					if err := strm.MsgSend(proxyMsg); err != nil {
						return err
					}
				}
				if errStr != "" {
					return errors.New(errStr)
				}
				if complete {
					errCh <- nil
					return io.EOF
				}
			}
			return nil
		})
		errCh <- rpcstream.ReadToHandler(rpcStream, handler)
	}()

	// Write messages from invoker stream -> rpc client.
	go func() {
		readMsg := srpc.NewRawMessage(nil, false) // zero-copy mode
		for {
			err := strm.MsgRecv(readMsg)
			if err == io.EOF {
				// EOF = normal exit
				err = packetWriter.WritePacket(srpc.NewCallDataPacket(nil, false, true, nil))
				errCh <- err
				return
			}
			if err == nil {
				callData := readMsg.GetData()
				err = packetWriter.WritePacket(srpc.NewCallDataPacket(callData, len(callData) == 0, false, nil))
			}
			if err != nil {
				// attempt to write the error back to the client rpc
				_ = packetWriter.WritePacket(srpc.NewCallDataPacket(nil, false, true, err))
				errCh <- err
				return
			}
		}
	}()

	// Wait for an error
	resErr := <-errCh
	return true, resErr
}

// _ is a type assertion
var (
	_ srpc.Invoker                      = ((*ProxyInvoker)(nil))
	_ bifrost_rpc.LookupRpcServiceValue = ((*ProxyInvoker)(nil))
)
