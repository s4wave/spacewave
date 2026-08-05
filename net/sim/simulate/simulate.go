package simulate

import (
	"context"
	"sync"

	bpeer "github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/sirupsen/logrus"
)

// Simulator manages state for all simulated machines.
type Simulator struct {
	// ctx is the context
	ctx context.Context
	// ctxCancel cancels the context
	ctxCancel context.CancelFunc
	// verbose enables verbose mode
	verbose bool
	// le is the logger
	le *logrus.Entry
	// mtx guards below fields
	mtx sync.Mutex
	// peers contains running peer nodes
	peers map[string]*Peer
}

// NewSimulator constructs a new sim.
func NewSimulator(
	ctx context.Context,
	le *logrus.Entry,
	grp *graph.Graph,
	opts ...SimulatorOption,
) (*Simulator, error) {
	// Initialize simulator state and apply construction options.
	s := &Simulator{
		le:    le,
		peers: make(map[string]*Peer),
	}

	// Create the simulator cancellation context before starting peers.
	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	for _, opt := range opts {
		if opt != nil {
			if err := opt(s); err != nil {
				return nil, err
			}
		}
	}

	// Instantiate graph peers and prepare their in-process links.
	allNodes := grp.AllNodes()
	le.Debugf("processing %d nodes in graph", len(allNodes))
	var allPeers []*Peer
	for _, node := range allNodes {
		peer, isPeer := node.(*graph.Peer)
		if !isPeer {
			continue
		}

		peerID := peer.GetPeerID()
		peerIDStr := peerID.String()
		if _, epOk := s.peers[peerIDStr]; epOk {
			continue
		}

		pushedPeer, err := s.pushPeer(peer)
		if err != nil {
			s.ctxCancel()
			return nil, err
		}
		allPeers = append(allPeers, pushedPeer)

		// Discover peers linked through the graph.
		linkedPeers := peer.GetLinkedPeers(grp)
		le.Debugf("peer %s has %d linked peers", peerIDStr, len(linkedPeers))

		// Connect each discovered peer in both directions.
		for _, lpeer := range linkedPeers {
			lpeerPeerIDStr := lpeer.GetPeerID().String()
			op, ok := s.peers[lpeerPeerIDStr]
			if !ok {
				continue
			}

			le.Debugf("added in-memory link from %s to %s", lpeerPeerIDStr, peerIDStr)
			op.inproc.ConnectToInproc(ctx, pushedPeer.inproc)
			pushedPeer.inproc.ConnectToInproc(s.ctx, op.inproc)

			pushedPeer.staticPeerMap[lpeerPeerIDStr] = &dialer.DialerOpts{
				Address: op.inproc.LocalAddr().String(),
			}
			op.staticPeerMap[peerIDStr] = &dialer.DialerOpts{
				Address: pushedPeer.inproc.LocalAddr().String(),
			}
		}
	}

	// Apply each peer's deferred configuration after all links exist.
	for _, addedPeer := range allPeers {
		if err := addedPeer.finishSetup(); err != nil {
			s.ctxCancel()
			return nil, err
		}
	}

	return s, nil
}

// pushPeer creates and starts a peer in the simulator.
// Called only during NewSimulator construction before the Simulator is shared,
// so it does not take mtx.
func (s *Simulator) pushPeer(peer *graph.Peer) (*Peer, error) {
	p, err := newPeer(s.ctx, s.le, peer, s.verbose)
	if err != nil {
		return nil, err
	}
	peerIDStr := p.GetPeerID().String()
	s.peers[peerIDStr] = p
	return p, nil
}

// GetPeerByID returns a peer by ID.
func (s *Simulator) GetPeerByID(id bpeer.ID) *Peer {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.peers[id.String()]
}

// Close closes the simulator.
func (s *Simulator) Close() {
	s.ctxCancel()
}
