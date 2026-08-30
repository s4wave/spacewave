//go:build !tinygo

package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

func sessionTransportFactories(b bus.Bus) []controller.Factory {
	return []controller.Factory{
		websocket.NewFactory(b),
		webrtc.NewFactory(b),
	}
}

func (t *SessionTransport) startWebRTCControllers(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
) (*transport_controller.Controller, func(), error) {
	if t.signalingURL == "" {
		return nil, nil, nil
	}

	ticket, err := acquireSignalTicket(ctx, t.signalingURL, t.sessionKey, t.peerID, t.signingEnvPfx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, err
	}

	wsURL, err := signalWebSocketURL(t.signalingURL, ticket)
	if err != nil {
		return nil, nil, err
	}

	le.Debug("connecting to signaling")

	sigCtrl := newWSSignalingCtrl(le, b, wsURL, t.sessionKey, "webrtc", t.peerID)
	if _, err := b.AddController(ctx, sigCtrl, nil); err != nil {
		return nil, nil, err
	}

	rtcCtrl, _, rtcRef, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx, b,
		resolver.NewLoadControllerWithConfig(&webrtc.Config{
			SignalingId: "webrtc",
			WebRtc: &webrtc.WebRtcConfig{
				IceServers: []*webrtc.IceServerConfig{
					{Urls: []string{"stun:stun.l.google.com:19302"}},
				},
			},
			AllPeers:                true,
			AllPeersLowerPeerOffers: true,
		}),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	le.Debug("signaling and webrtc controllers started")
	return rtcCtrl, rtcRef.Release, nil
}
