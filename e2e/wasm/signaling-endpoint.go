//go:build !js

package wasm

import (
	"context"
	"net/http"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/peer"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_frame "github.com/s4wave/spacewave/net/signaling/rpc/frame"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/sirupsen/logrus"
)

// signalingPeerKey identifies the peer on a fixture's accepted RPC connection.
type signalingPeerKey struct{}

// registerE2ESignaling serves the real signaling protocol on the loopback fixture.
// Tickets carry test peer identities; this endpoint is not a cloud auth server.
func registerE2ESignaling(ctx context.Context, mux *http.ServeMux) error {
	// Share rendezvous state across all browser connections.
	signaling := signaling_server.NewServerWithIdentify(logrus.NewEntry(logrus.New()), func(ctx context.Context) (peer.ID, error) {
		return ctx.Value(signalingPeerKey{}).(peer.ID), nil
	})
	rpcMux := srpc.NewMux()
	if err := signaling_rpc.SRPCRegisterSignaling(rpcMux, signaling); err != nil {
		return err
	}

	// Issue identities only for syntactically valid session peers.
	mux.HandleFunc("/api/signal/ticket", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Peer-ID, X-Timestamp, X-Sw-Hash, X-Signature")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		pid, err := peer.IDB58Decode(r.Header.Get("X-Peer-ID"))
		if err != nil || pid == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, err := (&api.SignalTicketResponse{Token: pid.String()}).MarshalVT()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := w.Write(data); err != nil {
			return
		}
	})

	// Bind every RPC stream to the ticket identity until the fixture shuts down.
	mux.HandleFunc("/api/signal/ws", func(w http.ResponseWriter, r *http.Request) {
		pid, err := peer.IDB58Decode(r.URL.Query().Get("tk"))
		if err != nil || pid == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		streamCtx, cancel := context.WithCancel(context.WithValue(ctx, signalingPeerKey{}, pid))
		defer cancel()
		// Socket closure and fixture cancellation terminate the read pump.
		frames := signaling_rpc_frame.NewConn(streamCtx, conn)
		_ = frames.ReadPump(signaling_rpc_frame.NewServerAccept(streamCtx, rpcMux))
	})
	return nil
}
