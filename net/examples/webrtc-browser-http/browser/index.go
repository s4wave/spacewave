//go:build js

package main

import (
	"context"
	"fmt"
	"io"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/core"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/s4wave/spacewave/net/protocol"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	"github.com/s4wave/spacewave/net/stream"
	srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

// Configuration - these will be updated with actual peer IDs
var (
	signalingServerID = "SIGNALING_SERVER_PEER_ID" // Replace with actual signaling server peer ID
	backendPeerID     = "BACKEND_PEER_ID"          // Replace with actual backend peer ID
	protocolID        = "webrtc-browser-http/v1"
)

var (
	log     *logrus.Logger
	le      *logrus.Entry
	b       bus.Bus
	privKey crypto.PrivKey
)

func init() {
	log = logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.Formatter = &logrus.TextFormatter{
		DisableColors: true,
	}
	le = logrus.NewEntry(log)
}

func main() {
	le.Info("WebRTC Browser HTTP Client starting...")

	// Read initial peer IDs from HTML input fields
	doc := js.Global().Get("document")
	signalingInput := doc.Call("getElementById", "signalingServerId")
	backendInput := doc.Call("getElementById", "backendPeerId")

	if !signalingInput.IsNull() && !signalingInput.IsUndefined() {
		val := signalingInput.Get("value").String()
		if val != "" {
			signalingServerID = val
			le.Infof("Loaded signaling server ID from HTML: %s", signalingServerID)
		}
	}

	if !backendInput.IsNull() && !backendInput.IsUndefined() {
		val := backendInput.Get("value").String()
		if val != "" {
			backendPeerID = val
			le.Infof("Loaded backend peer ID from HTML: %s", backendPeerID)
		}
	}

	// Make log available to JavaScript console
	js.Global().Set("goLog", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			le.Info(args[0].String())
		}
		return nil
	}))

	// Expose config setters
	js.Global().Set("setSignalingServer", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			signalingServerID = args[0].String()
			le.Infof("Signaling server set to: %s", signalingServerID)
		}
		return nil
	}))

	js.Global().Set("setBackendPeer", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			backendPeerID = args[0].String()
			le.Infof("Backend peer set to: %s", backendPeerID)
		}
		return nil
	}))

	// Expose connect and fetch functions
	js.Global().Set("connectWebRTC", js.FuncOf(connectWebRTC))
	js.Global().Set("fetchViaWebRTC", js.FuncOf(fetchViaWebRTC))

	le.Info("Browser client initialized. Ready to connect!")

	// Keep the Go program running
	<-make(chan struct{})
}

func getWSBaseURL() string {
	location := js.Global().Get("location")
	hostname := location.Get("hostname").String()

	wsProtocol := "ws"
	if location.Get("protocol").String() == "https:" {
		wsProtocol = "wss"
	}

	// Signaling server runs on port 2253
	return fmt.Sprintf("%s://%s:2253/bifrost-ws", wsProtocol, hostname)
}

func connectWebRTC(this js.Value, args []js.Value) interface{} {
	go func() {
		le.Info("Initializing Bifrost bus...")
		ctx := context.Background()

		// Create bus and resolver
		var err error
		localB, sr, err := core.NewCoreBus(ctx, le)
		if err != nil {
			le.WithError(err).Error("failed to build bus")
			return
		}
		b = localB // Set the package-level bus variable

		// Add factories
		sr.AddFactory(websocket.NewFactory(b))
		sr.AddFactory(webrtc.NewFactory(b))
		sr.AddFactory(signaling_rpc_client.NewFactory(b))
		sr.AddFactory(peer_controller.NewFactory(b))

		// Generate a local peer
		localPeer, err := peer.NewPeer(nil)
		if err != nil {
			le.WithError(err).Error("failed to create local peer")
			return
		}

		privKey, err = localPeer.GetPrivKey(ctx)
		if err != nil {
			le.WithError(err).Error("failed to get private key")
			return
		}

		localPeerID := localPeer.GetPeerID()
		peerPrivKeyPem, err := keypem.MarshalPrivKeyPem(privKey)
		if err != nil {
			le.WithError(err).Error("failed to marshal private key")
			return
		}

		le.Infof("Local peer ID: %s", localPeerID.String())

		// Load the peer controller
		_, _, err = b.AddDirective(
			resolver.NewLoadControllerWithConfig(&peer_controller.Config{
				PrivKey: string(peerPrivKeyPem),
			}),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Debug("Peer controller resolved")
			}, nil, nil),
		)
		if err != nil {
			le.WithError(err).Error("failed to add peer controller")
			return
		}

		// Connect to signaling server via WebSocket
		wsBaseURL := getWSBaseURL()
		le.Infof("Connecting to signaling server at %s", wsBaseURL)

		_, wsRef, err := b.AddDirective(
			resolver.NewLoadControllerWithConfig(&websocket.Config{
				Dialers: map[string]*dialer.DialerOpts{
					signalingServerID: {
						Address: wsBaseURL,
					},
				},
			}),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Info("WebSocket transport established")
			}, nil, nil),
		)
		if err != nil {
			le.WithError(err).Error("failed to add websocket directive")
			return
		}
		defer wsRef.Release()

		// Start signaling client
		le.Info("Starting signaling client...")
		_, signalingRef, err := b.AddDirective(
			resolver.NewLoadControllerWithConfig(&signaling_rpc_client.Config{
				SignalingId: "webrtc",
				ProtocolId:  "webrtc/signaling",
				Client: &srpc_client.Config{
					ServerPeerIds: []string{signalingServerID},
				},
			}),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Info("Signaling client established")
			}, nil, nil),
		)
		if err != nil {
			le.WithError(err).Error("failed to start signaling client")
			return
		}
		defer signalingRef.Release()

		// Start WebRTC transport
		le.Info("Starting WebRTC transport...")
		_, webrtcRef, err := b.AddDirective(
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
				BlockPeers: []string{signalingServerID},
				Verbose:    true,
			}),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Info("WebRTC transport established")
			}, nil, nil),
		)
		if err != nil {
			le.WithError(err).Error("failed to start webrtc transport")
			return
		}
		defer webrtcRef.Release()

		// Establish link with backend peer
		le.Infof("Establishing link with backend peer %s...", backendPeerID)
		remotePeerID, err := peer.IDB58Decode(backendPeerID)
		if err != nil {
			le.WithError(err).Error("failed to decode backend peer ID")
			return
		}

		_, linkRef, err := b.AddDirective(
			link.NewEstablishLinkWithPeer("", remotePeerID),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Info("Link established with backend peer!")
			}, nil, nil),
		)
		if err != nil {
			le.WithError(err).Error("failed to establish link")
			return
		}
		defer linkRef.Release()

		le.Info("WebRTC connection ready! You can now use fetchViaWebRTC().")

		// Keep connection alive
		<-ctx.Done()
	}()

	return nil
}

func fetchViaWebRTC(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		le.Error("fetchViaWebRTC requires a URL path argument")
		return nil
	}

	urlPath := args[0].String()
	le.Infof("Fetching via WebRTC: %s", urlPath)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if b == nil {
			le.Error("Bus not initialized. Call connectWebRTC() first.")
			return
		}

		if privKey == nil {
			le.Error("Private key not initialized.")
			return
		}

		localPeerID, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			le.WithError(err).Error("failed to get local peer ID")
			return
		}

		remotePeerID, err := peer.IDB58Decode(backendPeerID)
		if err != nil {
			le.WithError(err).Error("failed to decode backend peer ID")
			return
		}

		// Open stream to backend
		le.Infof("Opening stream to %s with protocol %s", backendPeerID, protocolID)
		strm, rel, err := link.OpenStreamWithPeerEx(
			ctx,
			b,
			protocol.ID(protocolID),
			localPeerID,
			remotePeerID,
			0, // no specific transport ID
			stream.OpenOpts{},
		)
		if err != nil {
			le.WithError(err).Error("failed to open stream")
			return
		}
		defer rel()
		defer strm.GetStream().Close()

		le.Info("Stream opened successfully")

		// Construct HTTP request
		httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n", urlPath)

		// Write request
		_, err = strm.GetStream().Write([]byte(httpRequest))
		if err != nil {
			le.WithError(err).Error("failed to write request")
			return
		}
		le.Info("HTTP request sent")

		// Read response
		response, err := io.ReadAll(strm.GetStream())
		if err != nil {
			le.WithError(err).Error("failed to read response")
			return
		}

		le.Infof("Received response (%d bytes)", len(response))
		le.Infof("Response:\n%s", string(response))
	}()

	return nil
}
