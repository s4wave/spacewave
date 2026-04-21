//go:build js

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/examples/websocket-browser-link/common"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	wtpt "github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

func getHTTPBaseURL() string {
	location := js.Global().Get("location")
	proto := location.Get("protocol").String()
	hostname := location.Get("hostname").String()
	port := location.Get("port").String()

	if port != "" {
		return fmt.Sprintf("%s//%s:%s", proto, hostname, port)
	}
	return fmt.Sprintf("%s//%s", proto, hostname)
}

func getWSBaseURL() string {
	location := js.Global().Get("location")
	hostname := location.Get("hostname").String()
	port := location.Get("port").String()

	wsProtocol := "ws"
	if location.Get("protocol").String() == "https:" {
		wsProtocol = "wss"
	}

	if port != "" {
		return fmt.Sprintf("%s://%s:%s/bifrost.ws", wsProtocol, hostname, port)
	}
	return fmt.Sprintf("%s://%s/bifrost.ws", wsProtocol, hostname)
}

func main() {
	ctx := context.Background()
	le := common.GetLogEntry()
	if err := run(ctx, le); err != nil {
		le.WithError(err).Fatal("error running demo")
	}
}

// run runs the demo.
func run(ctx context.Context, le *logrus.Entry) error {
	b, sr, privKey, err := common.BuildCommonBus(ctx)
	if err != nil {
		return err
	}
	sr.AddFactory(wtpt.NewFactory(b))

	// get the peer id from an http endpoint
	peerIDURL := getHTTPBaseURL() + "/peer"
	le.
		WithField("url", peerIDURL).
		Debug("getting peer id")
	resp, err := http.Get(peerIDURL)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	peerIDStr := string(body)
	remotePeerID, err := peer.IDB58Decode(peerIDStr)
	if err != nil {
		return err
	}
	peerIDStr = remotePeerID.String()

	wsBaseURL := getWSBaseURL()
	le.
		WithField("url", wsBaseURL).
		WithField("peer", peerIDStr).
		Debug("contacting websocket peer")

	// NOTE: deterministic due to the use of a prng to generate the key
	_, wsRef, err := b.AddDirective(
		resolver.NewLoadControllerWithConfig(&wtpt.Config{
			Dialers: map[string]*dialer.DialerOpts{
				peerIDStr: {
					Address: wsBaseURL,
				},
			},
		}),
		bus.NewCallbackHandler(func(val directive.AttachedValue) {
			le.Infof("websocket transport resolved: %#v", val.GetValue())
		}, nil, nil),
	)
	defer wsRef.Release()

	_, dialRef, err := b.AddDirective(
		link.NewEstablishLinkWithPeer("", remotePeerID),
		nil,
	)
	if err != nil {
		return err
	}
	defer dialRef.Release()

	// open a pubsub channel
	channelID := "test-channel"
	channelSub, _, channelSubRef, err := pubsub.ExBuildChannelSubscription(ctx, b, false, channelID, privKey, nil)
	if err != nil {
		return err
	}
	defer channelSubRef.Release()
	le.Infof("built channel subscription for channel %s", channelID)

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-time.After(time.Second):
		}

		msg := fmt.Sprintf("Hello from browser: %s", time.Now().String())
		if err := channelSub.Publish([]byte(msg)); err != nil {
			le.WithError(err).Warn("unable to publish pubsub message")
		}
	}
}
