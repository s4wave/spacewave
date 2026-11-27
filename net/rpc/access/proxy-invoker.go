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
// note: usually the caller does not need waitAck set to true.
func NewProxyInvoker(client SRPCAccessRpcServiceClient, req *LookupRpcServiceRequest, waitAck bool) *ProxyInvoker {
	return &ProxyInvoker{client: client, req: req, waitAck: waitAck}
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (r *ProxyInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Select the requested service without changing the proxy lookup request.
	req := r.req
	if serviceID != "" && serviceID != req.GetServiceId() {
		req = req.CloneVT()
		req.ServiceId = serviceID
	}

	// Encode the service lookup for the remote RPC stream.
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

	// Track completion of both directions of the proxied call.
	type response struct {
		// err is the result of reading and forwarding the remote response.
		err error
		// complete records a terminal packet from the remote method.
		complete bool
	}
	serverDone := make(chan response, 1)
	clientDone := make(chan error, 1)

	// Read messages from prw -> write to invoker stream.
	go func() {
		// Reuse a zero-copy message and collect the remote method's result.
		proxyMsg := srpc.NewRawMessage(nil, false)
		var result response

		// Handle the Packet here because srpc.Stream MsgSend will be
		// encoded and wrapped in a Body packet.
		handler := srpc.NewPacketDataHandler(func(pkt *srpc.Packet) error {
			switch body := pkt.GetBody().(type) {
			case *srpc.Packet_CallCancel:
				// unexpected from server -> client but handle anyway
				return errors.New("rpc canceled by the remote")
			case *srpc.Packet_CallData:
				// Forward the remote call data to the invoking stream.
				data, dataIsZero := body.CallData.GetData(), body.CallData.GetDataIsZero()
				complete, errStr := body.CallData.GetComplete(), body.CallData.GetError()
				if len(data) != 0 || dataIsZero {
					proxyMsg.SetData(data)
					if err := strm.MsgSend(proxyMsg); err != nil {
						return err
					}
				}

				// Record the outcome sent by the remote method before ending the read.
				result.complete = errStr != "" || complete
				if errStr != "" {
					return errors.New(errStr)
				}
				if complete {
					return io.EOF
				}
			}
			return nil
		})

		// Deliver remote packets until the service completes or the stream fails.
		if err := rpcstream.ReadToHandler(rpcStream, handler); err != io.EOF {
			result.err = err
		}
		serverDone <- result
	}()

	// Write messages from invoker stream -> rpc client.
	go func() {
		// Forward invoking stream messages through a reusable zero-copy message.
		readMsg := srpc.NewRawMessage(nil, false) // zero-copy mode
		for {
			// Complete the remote request when the invoking stream reaches EOF.
			err := strm.MsgRecv(readMsg)
			if err == io.EOF {
				// EOF = normal exit
				err = packetWriter.WritePacket(srpc.NewCallDataPacket(nil, false, true, nil))
				clientDone <- err
				return
			}

			// Send each received request body to the remote service.
			if err == nil {
				callData := readMsg.GetData()
				err = packetWriter.WritePacket(srpc.NewCallDataPacket(callData, len(callData) == 0, false, nil))
			}

			// Notify the remote service and caller when request forwarding fails.
			if err != nil {
				// attempt to write the error back to the client rpc
				_ = packetWriter.WritePacket(srpc.NewCallDataPacket(nil, false, true, err))
				clientDone <- err
				return
			}
		}
	}()

	// Return remote completion, or wait for it after request forwarding ends.
	select {
	case result := <-serverDone:
		return true, result.err
	case err := <-clientDone:
		// End the read when request forwarding fails. An already queued
		// terminal packet still carries the outcome of the RPC.
		if err != nil {
			_ = rpcStream.Close()
		}
		result := <-serverDone
		if err != nil && !result.complete {
			return true, err
		}
		return true, result.err
	}
}

// _ is a type assertion
var (
	_ srpc.Invoker                      = (*ProxyInvoker)(nil)
	_ bifrost_rpc.LookupRpcServiceValue = (*ProxyInvoker)(nil)
)
