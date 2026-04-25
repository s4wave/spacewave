//go:build !js

package provider_spacewave

import (
	"context"
	"net/http"

	ws "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
)

// dialSessionWS dials the session WebSocket and returns the connection or a
// classified dial error.
func dialSessionWS(ctx context.Context, wsURL string) (*ws.Conn, error) {
	conn, resp, err := ws.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, classifySessionWSDialError(err, resp)
	}
	return conn, nil
}

// classifySessionWSDialError decodes a failed websocket handshake into a
// structured cloud error when the server returned one.
func classifySessionWSDialError(
	err error,
	resp *http.Response,
) error {
	if err == nil || resp == nil || resp.StatusCode == http.StatusSwitchingProtocols {
		return errors.Wrap(err, "dial session websocket")
	}
	body, bodyErr := readResponseBody(resp)
	if bodyErr != nil {
		return errors.Wrap(err, "dial session websocket")
	}
	cloudErr := parseCloudResponseError(resp, body)
	if cloudErr.Code == "" {
		return errors.Wrap(err, "dial session websocket")
	}
	return cloudErr
}
