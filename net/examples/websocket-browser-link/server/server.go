package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/net/examples/websocket-browser-link/common"
	bifrost_http_listener "github.com/s4wave/spacewave/net/http/listener"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	wtpt "github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.New()
	le  = logrus.NewEntry(log)
)

func init() {
	log.SetLevel(logrus.DebugLevel)
}

func main() {
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	b, sr, privKey, err := common.BuildCommonBus(ctx)
	if err != nil {
		return err
	}
	sr.AddFactory(wtpt.NewFactory(b))
	sr.AddFactory(bifrost_http_listener.NewFactory(b))

	localPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	// accept & echo the pubsub channel
	channelID := "test-channel"
	channelSub, _, channelSubRef, err := pubsub.ExBuildChannelSubscription(ctx, b, false, channelID, privKey, nil)
	if err != nil {
		return err
	}
	defer channelSubRef.Release()
	le.Infof("built channel subscription for channel %s", channelID)

	relHandler := channelSub.AddHandler(func(m pubsub.Message) {
		from := m.GetFrom()
		// ignore unauthenticated and/or from ourselves
		if !m.GetAuthenticated() || from == localPeerID {
			return
		}
		le.Infof("got pubsub message from %s: %s", from.String(), string(m.GetData()))
	})
	defer relHandler()

	// start the websocket handler
	wsHttpPath := "/bifrost.ws"
	wsPeerPath := "/peer"
	ws, _, wsRef, err := loader.WaitExecControllerRunningTyped[*wtpt.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&wtpt.Config{
			HttpPath:     wsHttpPath,
			HttpPeerPath: wsPeerPath,
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer wsRef.Release()

	// get transport
	tpt, err := ws.GetTransport(ctx)
	if err != nil {
		return err
	}
	wsServer := tpt.(*wtpt.WebSocket)

	// get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// serve static files from ../browser/ relative to cwd
	browserDir := filepath.Join(cwd, "..", "browser")
	fileServer := http.FileServer(http.Dir(browserDir))

	// start the http server
	mux := http.NewServeMux()
	mux.Handle("GET "+wsHttpPath, wsServer)
	mux.Handle("GET "+wsPeerPath, wsServer)
	mux.Handle("GET /", fileServer)
	mux.Handle("GET /index.html", fileServer)
	mux.Handle("GET /wasm_exec.js", fileServer)
	mux.Handle("GET /test.wasm", fileServer)

	le.Info("listening on :8080")
	return http.ListenAndServe(":8080", mux) //nolint:gosec
}
