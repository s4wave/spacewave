package simulate

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	bp "github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/stream"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/common/pconn"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// Peer holds state about an executing Peer.
type Peer struct {
	// graphPeer is the underlying graph peer.
	graphPeer *graph.Peer
	// le is the root log entry
	le *logrus.Entry
	// ctx is the context for the peer
	ctx context.Context
	// rels are functions to call to release this peer
	rels []func()
	// testbed is the peer's testbed.
	testbed *testbed.Testbed
	// inproc is the in-process transport instance.
	inproc *inproc.Inproc
	// transportController is the transport controller
	transportController *transport_controller.Controller
	// staticPeerMap is the static peer map
	staticPeerMap map[string]*dialer.DialerOpts
}

// newPeer constructs a new Peer.
func newPeer(ctx context.Context, le *logrus.Entry, gp *graph.Peer, verbose bool) (*Peer, error) {
	// Collect cleanup callbacks before constructing the testbed.
	var rels []func()
	rel := func() {
		for _, r := range rels {
			r()
		}
	}

	// Build the peer testbed and register its static factories.
	np := &Peer{
		graphPeer:     gp,
		le:            le.WithField("sim-peer", gp.ID()),
		staticPeerMap: make(map[string]*dialer.DialerOpts),
	}

	// Create the peer lifecycle context and retain its cancellation callback.
	var ctxCancel func()
	np.ctx, ctxCancel = context.WithCancel(ctx) //nolint:gosec // cancel stored in rels and released by Close
	rels = append(rels, ctxCancel)

	// Start the peer testbed with the graph peer's private key.
	var err error
	np.testbed, err = testbed.NewTestbed(
		np.ctx,
		np.le,
		testbed.TestbedOpts{PrivKey: gp.GetPeerPriv()},
	)
	if err != nil {
		rel()
		return nil, err
	}

	// Register the in-process transport and graph-specific peer factories.
	np.testbed.StaticResolver.AddFactory(inproc.NewFactory(np.testbed.Bus))
	gp.RegisterFactories(np.testbed.Bus, np.testbed.StaticResolver)

	// Start the in-process transport controller with configured dialers.
	tpc, _, tp1Ref, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		np.ctx,
		np.testbed.Bus,
		resolver.NewLoadControllerWithConfig(&inproc.Config{
			TransportPeerId: np.GetPeerID().String(),
			PacketOpts: &pconn.Opts{
				Verbose: verbose,
				Quic: &transport_quic.Opts{
					Verbose: verbose,
				},
			},
			Dialers: np.staticPeerMap,
		}),
		nil,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, tp1Ref.Release)
	tp, err := tpc.GetTransport(np.ctx)
	if err != nil {
		rel()
		return nil, err
	}
	np.inproc = tp.(*inproc.Inproc)
	np.transportController = tpc

	// Start the config-set controller without applying peer config yet.
	_, _, tp2Ref, err := bus.ExecOneOff(
		np.ctx,
		np.testbed.Bus,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
		nil,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, tp2Ref.Release)

	// Retain release callbacks until the simulator finishes startup.
	// Don't call ApplyConfigSet yet until the simulator is fully started.
	np.rels = rels
	return np, nil
}

// finishSetup applies the peer config set once simulator startup is complete.
func (p *Peer) finishSetup(ctx context.Context) error {
	_, applyRef, err := p.testbed.Bus.AddDirective(
		ctx,
		configset.NewApplyConfigSet(p.graphPeer.GetConfigSet()),
		nil,
	)
	if err != nil {
		return err
	}

	p.rels = append(p.rels, applyRef.Release)
	return nil
}

// GetPeerID returns the Peer ID.
func (p *Peer) GetPeerID() bp.ID {
	return p.graphPeer.GetPeerID()
}

// GetLogger returns the logger for the peer.
func (p *Peer) GetLogger() *logrus.Entry {
	return p.le
}

// GetPeerPriv returns the Peer private key.
func (p *Peer) GetPeerPriv() crypto.PrivKey {
	return p.graphPeer.GetPeerPriv()
}

// GetTestbed returns the testbed.
func (p *Peer) GetTestbed() *testbed.Testbed {
	return p.testbed
}

// GetTransportController returns the transport controller.
func (p *Peer) GetTransportController() *transport_controller.Controller {
	return p.transportController
}

// Close cancels the Peer's subroutines.
func (p *Peer) Close() {
	rels := p.rels
	p.rels = nil
	for _, rel := range rels {
		rel()
	}
}

// connect connects both peers and installs their static dialers during simulator setup.
func (p *Peer) connect(ctx context.Context, other *Peer) {
	// Connect both packet endpoints under the simulator's context.
	other.inproc.ConnectToInproc(ctx, p.inproc)
	p.inproc.ConnectToInproc(ctx, other.inproc)

	// Publish both addresses before either peer's deferred configuration starts.
	p.staticPeerMap[other.GetPeerID().String()] = &dialer.DialerOpts{
		Address: other.inproc.LocalAddr().String(),
	}
	other.staticPeerMap[p.GetPeerID().String()] = &dialer.DialerOpts{
		Address: p.inproc.LocalAddr().String(),
	}
}

// TestConnectivity tests for basic connectivity between two peers.
// Uses an echo test: dials the Echo controller.
func (p *Peer) TestConnectivity(ctx context.Context, other *Peer) error {
	// Retain a directive establishing the link between the simulated peers.
	_, esRef, err := p.testbed.Bus.AddDirective(ctx, link.NewEstablishLinkWithPeer(
		p.GetPeerID(),
		other.GetPeerID(),
	), nil)
	if err != nil {
		return err
	}
	defer esRef.Release()

	// Open the echo stream over the simulated peer link.
	ms1, ms1Rel, err := link.OpenStreamWithPeerEx(
		ctx,
		p.testbed.Bus,
		stream_echo.DefaultProtocolID,
		p.GetPeerID(),
		other.GetPeerID(),
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		return err
	}
	defer ms1Rel()

	// Verify that the stream reports the expected remote peer.
	mns1rp := ms1.GetLink().GetRemotePeer().String()
	if px1p := other.GetPeerID().String(); px1p != mns1rp {
		return errors.Errorf(
			"stream on p0 remote peer id %s != expected %s",
			mns1rp,
			px1p,
		)
	}

	// Verify that the stream reports the expected local peer.
	mns1lp := ms1.GetLink().GetLocalPeer().String()
	if px0p := p.GetPeerID().String(); px0p != mns1lp {
		return errors.Errorf(
			"stream on p0 local peer id %s != expected %s",
			mns1lp,
			px0p,
		)
	}

	// Send the connectivity probe through the simulated peer stream.
	data := []byte("testing 1234")
	_, err = ms1.GetStream().Write(data)
	if err != nil {
		return err
	}

	// expect remote to echo back exactly len(data) bytes
	outData := make([]byte, len(data)*2)
	on, oe := ms1.GetStream().Read(outData)
	if oe != nil {
		return oe
	}

	// Verify the echoed probe has the expected byte count.
	outData = outData[:on]
	if on != len(data) {
		return errors.Errorf(
			"length incorrect received %v != %v data: %v",
			on,
			len(data),
			string(outData),
		)
	}
	return nil
}
