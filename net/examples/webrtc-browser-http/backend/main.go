package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/net/core"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	stream_forwarding "github.com/s4wave/spacewave/net/stream/forwarding"
	srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

var (
	signalingServerID string = "12D3KooWNyn6cNNxHnLc5Nw8b7XkVaAWKB9vbfe921LuysEoY1Cz" // Signaling server peer ID
	signalingAddr     string = "ws://127.0.0.1:2253/bifrost-ws"
	targetAddr        string = "http://127.0.0.1:8080"
	protocolID        string = "webrtc-browser-http/v1"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	app := cli.NewApp()
	app.Name = "webrtc-backend"
	app.Usage = "Backend server that forwards HTTP requests from WebRTC to local service"
	app.HideHelpCommand = true

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "signaling-server",
			Usage:       "peer ID of signaling server",
			EnvVars:     []string{"SIGNALING_SERVER"},
			Value:       signalingServerID,
			Destination: &signalingServerID,
		},
		&cli.StringFlag{
			Name:        "signaling-addr",
			Usage:       "WebSocket address of signaling server",
			EnvVars:     []string{"SIGNALING_ADDR"},
			Value:       signalingAddr,
			Destination: &signalingAddr,
		},
		&cli.StringFlag{
			Name:        "target",
			Usage:       "target HTTP service address",
			EnvVars:     []string{"TARGET_ADDR"},
			Value:       targetAddr,
			Destination: &targetAddr,
		},
		&cli.StringFlag{
			Name:        "protocol-id",
			Usage:       "protocol ID for the forwarding service",
			EnvVars:     []string{"PROTOCOL_ID"},
			Value:       protocolID,
			Destination: &protocolID,
		},
	}

	app.Action = func(c *cli.Context) error {
		return run(ctx, le)
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, le *logrus.Entry) error {
	// Create the Controller Bus
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}

	// Add factories
	sr.AddFactory(websocket.NewFactory(b))
	sr.AddFactory(webrtc.NewFactory(b))
	sr.AddFactory(signaling_rpc_client.NewFactory(b))
	sr.AddFactory(stream_forwarding.NewFactory(b))

	// Load or create the peer private key
	privKey, err := keyfile.OpenOrWritePrivKey(le, "../../priv/backend-node.pem")
	if err != nil {
		return err
	}

	// Load the peer from the private key
	localPeer, err := peer.NewPeer(privKey)
	if err != nil {
		return err
	}

	localPeerID := localPeer.GetPeerID()
	localPeerIDStr := localPeerID.String()
	le.Infof("backend node starting with peer id: %v", localPeerIDStr)

	// Load the peer controller
	peerCtrl := peer_controller.NewController(le, localPeer)
	relPeerCtrl, err := b.AddController(ctx, peerCtrl, nil)
	if err != nil {
		return err
	}
	defer relPeerCtrl()

	// Check if target is accessible
	le.Infof("checking target service at %s", targetAddr)
	resp, err := http.Get(targetAddr) //nolint:gosec
	if err != nil {
		le.Warnf("target service not accessible: %v", err)
		le.Warnf("make sure to start the HTTP service (e.g., python3 -m http.server 8080)")
	} else {
		resp.Body.Close()
		le.Infof("target service is accessible")
	}

	// Connect to signaling server via WebSocket
	le.Infof("connecting to signaling server %s at %s", signalingServerID, signalingAddr)
	_, _, wsRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&websocket.Config{
			TransportPeerId: localPeerIDStr,
			Dialers: map[string]*dialer.DialerOpts{
				signalingServerID: {
					Address: signalingAddr,
				},
			},
		}),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start websocket transport: %w", err)
	}
	defer wsRef.Release()

	// Start signaling client
	_, _, signalingRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&signaling_rpc_client.Config{
			SignalingId: "webrtc",
			ProtocolId:  "webrtc/signaling",
			Client: &srpc_client.Config{
				ServerPeerIds: []string{signalingServerID},
			},
		}),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start signaling client: %w", err)
	}
	defer signalingRef.Release()

	// Start WebRTC transport
	_, _, webrtcRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&webrtc.Config{
			SignalingId: "webrtc",
			WebRtc: &webrtc.WebRtcConfig{
				IceServers: []*webrtc.IceServerConfig{
					{
						Urls: []string{
							"stun:stun.l.google.com:19302",
							"stun:stun.stunprotocol.org:3478",
						},
					},
				},
			},
			AllPeers:   true,
			BlockPeers: []string{signalingServerID}, // Don't try to connect to signaling server via WebRTC
			Verbose:    true,
		}),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start webrtc transport: %w", err)
	}
	defer webrtcRef.Release()

	// Start stream forwarding service
	le.Infof("starting stream forwarding service for protocol %s -> %s", protocolID, targetAddr)
	_, _, forwardRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&stream_forwarding.Config{
			ProtocolId:      protocolID,
			TargetMultiaddr: "/ip4/127.0.0.1/tcp/8080",
		}),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start stream forwarding: %w", err)
	}
	defer forwardRef.Release()

	le.Infof("backend node is ready - peers can connect via WebRTC")
	le.Infof("peer ID: %s", localPeerIDStr)

	// Wait for context to be done
	<-ctx.Done()
	return nil
}
