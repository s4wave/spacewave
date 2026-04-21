package simulate

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/net/crypto"
	bp "github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/common/pconn"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// Peer holds state about an executing Peer
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
	var rels []func()
	rel := func() {
		for _, r := range rels {
			r()
		}
	}

	np := &Peer{
		graphPeer: gp,
		le:        le.WithField("sim-peer", gp.ID()),
		// WithField("sim-peer", gp.GetPeerID().String()).
		staticPeerMap: make(map[string]*dialer.DialerOpts),
	}

	var ctxCancel func()
	np.ctx, ctxCancel = context.WithCancel(ctx)
	rels = append(rels, ctxCancel)

	var err error
	np.testbed, err = testbed.NewTestbed(
		np.ctx,
		np.le, // np.le,
		testbed.TestbedOpts{PrivKey: gp.GetPeerPriv()},
	)
	if err != nil {
		rel()
		return nil, err
	}

	np.testbed.StaticResolver.AddFactory(inproc.NewFactory(np.testbed.Bus))
	for _, extraFactoryCtor := range gp.GetExtraFactories() {
		np.testbed.StaticResolver.AddFactory(extraFactoryCtor(np.testbed.Bus))
	}
	for _, extraFactoryCtor := range gp.GetExtraFactoryAdders() {
		extraFactoryCtor(np.testbed.Bus, np.testbed.StaticResolver)
	}

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

	// Don't call ApplyConfigSet yet until the simulator is fully started.
	np.rels = rels
	return np, nil
}

// finishSetup applies the peer config set once simulator startup is complete.
func (p *Peer) finishSetup() error {
	_, applyRef, err := p.testbed.Bus.AddDirective(
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

// GetInproc returns the in-proc transport.
func (p *Peer) GetInproc() *inproc.Inproc {
	return p.inproc
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
