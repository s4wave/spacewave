//go:build js

package provider_spacewave

import (
	"context"

	ws "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
)

// dialSessionWS dials the session WebSocket and returns the connection or a
// wrapped dial error. On the js/wasm build the websocket library does not
// surface an http.Response for handshake failures, so structured cloud error
// classification is not available.
func dialSessionWS(ctx context.Context, wsURL string) (*ws.Conn, error) {
	conn, _, err := ws.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "dial session websocket")
	}
	return conn, nil
}
