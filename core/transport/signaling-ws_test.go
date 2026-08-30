package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	ws "github.com/aperturerobotics/go-websocket"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	"github.com/sirupsen/logrus"
)

func TestWSSignalPeerResolverWaitsForConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ctrl := &wsSignalingCtrl{ready: make(chan struct{})}
	resolver := &wsSignalPeerResolver{
		c:   ctrl,
		dir: signaling.NewSignalPeer("webrtc", peer.ID("local"), peer.ID("remote")),
	}
	if err := resolver.Resolve(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve error = %v, want context cancellation while waiting for signaling", err)
	}
}

func TestWSSignalingControllerReconnectsAfterDialFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	ctrl := &wsSignalingCtrl{
		le:         logrus.NewEntry(logrus.New()),
		ready:      make(chan struct{}),
		retryDelay: time.Nanosecond,
		dial: func(context.Context, *logrus.Entry, string, crypto.PrivKey) (*signaling_rpc_client.Client, *ws.Conn, func(), error) {
			calls++
			if calls == 2 {
				cancel()
			}
			return nil, nil, nil, errors.New("dial failed")
		},
	}
	if err := ctrl.Execute(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v, want context cancellation", err)
	}
	if calls != 2 {
		t.Fatalf("dial attempts = %d, want 2", calls)
	}
}
