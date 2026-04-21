package testbed

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	srr "github.com/aperturerobotics/controllerbus/controller/resolver/static"
	core "github.com/s4wave/spacewave/net/core/test"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	"github.com/sirupsen/logrus"
)

// Testbed is a constructed testbed.
type Testbed struct {
	// Context is the root context.
	Context context.Context
	// Logger is the logger
	Logger *logrus.Entry
	// StaticResolver is the static resolver.
	StaticResolver *srr.Resolver
	// Bus is the controller bus
	Bus bus.Bus
	// PrivKey is the private key.
	PrivKey crypto.PrivKey
	// PeerID is the peer id for private key.
	PeerID peer.ID
	// Release releases the testbed.
	Release func()
}

// TestbedOpts are extra options to construct the testbed.
type TestbedOpts struct {
	// PrivKey overrides the private key.
	PrivKey crypto.PrivKey
	// NoPeer disables generating + starting the peer and filling PrivKey.
	NoPeer bool
	// NoEcho disables starting the echo listener.
	NoEcho bool
}

// NewTestbed constructs a new core bus with a attached kvtx in-memory volume,
// logger, and other core controllers required for a test to function.
func NewTestbed(ctx context.Context, le *logrus.Entry, opts TestbedOpts) (*Testbed, error) {
	var rels []func()
	t := &Testbed{
		Context: ctx,
		Logger:  le,
		PrivKey: opts.PrivKey,
		Release: func() {
			for _, rel := range rels {
				rel()
			}
		},
	}

	b, sr, err := core.NewTestingBus(ctx, le)
	if err != nil {
		return nil, err
	}
	t.StaticResolver = sr
	t.Bus = b
	sr.AddFactory(stream_echo.NewFactory(t.Bus))

	if !opts.NoPeer && t.PrivKey == nil {
		npeer, err := peer.NewPeer(nil)
		if err != nil {
			return nil, err
		}
		t.PrivKey, err = npeer.GetPrivKey(ctx)
		if err != nil {
			return nil, err
		}
	}
	if t.PrivKey != nil {
		pid, err := peer.IDFromPrivateKey(t.PrivKey)
		if err != nil {
			return nil, err
		}
		t.PeerID = pid
	}

	if !opts.NoPeer {
		// start peer controller
		peerConfig, err := peer_controller.NewConfigWithPrivKey(t.PrivKey)
		if err != nil {
			return nil, err
		}
		_, _, peerRef, err := bus.ExecOneOff(
			ctx,
			t.Bus,
			resolver.NewLoadControllerWithConfig(peerConfig),
			nil,
			nil,
		)
		if err != nil {
			t.Release()
			return nil, err
		}
		rels = append(rels, peerRef.Release)
	}

	if !opts.NoEcho {
		_, _, echoRef, err := bus.ExecOneOff(
			ctx,
			t.Bus,
			resolver.NewLoadControllerWithConfig(&stream_echo.Config{}),
			nil,
			nil,
		)
		if err != nil {
			t.Release()
			return nil, err
		}
		rels = append(rels, echoRef.Release)
	}

	return t, nil
}
