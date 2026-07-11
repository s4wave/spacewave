//go:build !tinygo

package transport

import (
	"context"
	"strings"

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
		return nil, nil, err
	}

	wsURL := strings.Replace(t.signalingURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/signal/ws?tk=" + ticket

	le.Debug("connecting to signaling")

	sigClient, sigConn, sigCleanup, err := dialSignalingClient(ctx, le, wsURL, t.sessionKey)
	if err != nil {
		return nil, nil, err
	}
	sigCtrl := newWSSignalingCtrl(le, b, sigClient, sigConn, "webrtc", t.peerID)
	if _, err := b.AddController(ctx, sigCtrl, nil); err != nil {
		sigCleanup()
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
			AllPeers: true,
		}),
		nil,
	)
	if err != nil {
		sigCleanup()
		return nil, nil, err
	}

	le.Debug("signaling and webrtc controllers started")
	return rtcCtrl, func() {
		rtcRef.Release()
		sigCleanup()
	}, nil
}
