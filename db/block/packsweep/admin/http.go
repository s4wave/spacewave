//go:build !tinygo

package block_packsweep_admin

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// NewHTTPHandler serves s over starpc on a WebSocket at path, or on every path
// when path is empty. The backend mounts it behind its own admin
// authorization: the handler checks no credential.
func NewHTTPHandler(s *Server, path string) (http.Handler, error) {
	mux := srpc.NewMux()
	if err := SRPCRegisterPackSweepAdmin(mux, s); err != nil {
		return nil, err
	}
	return srpc.NewHTTPServer(mux, path, nil)
}

// DialHTTP connects to a handler NewHTTPHandler serves at url, a ws or wss
// URL, sending header with the upgrade, such as the backend's admin
// credential. Closing the returned function closes the connection.
func DialHTTP(ctx context.Context, url string, header http.Header) (*Client, func(), error) {
	// Open the WebSocket.
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, nil, errors.Wrap(err, "dial the pack sweep admin service")
	}

	// Run starpc over it.
	muxed, err := srpc.NewWebSocketConn(context.WithoutCancel(ctx), conn, false, nil)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return nil, nil, err
	}
	closeConn := func() {
		_ = muxed.Close()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	return NewClient(NewSRPCPackSweepAdminClient(srpc.NewClientWithMuxedConn(muxed))), closeConn, nil
}
