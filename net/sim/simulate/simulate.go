package simulate

import (
	"context"
	"sync"

	bpeer "github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/sirupsen/logrus"
)

// Simulator manages state for all simulated machines.
type Simulator struct {
	// ctx is the context.
	ctx context.Context
	// ctxCancel cancels the context.
	ctxCancel context.CancelFunc
	// verbose enables verbose mode.
	verbose bool
	// le is the logger.
	le *logrus.Entry
	// mtx guards the peers field.
	mtx sync.Mutex
	// peers contains running peer nodes.
	peers map[bpeer.ID]*Peer
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
		peers: make(map[bpeer.ID]*Peer),
	}

	// Create the simulator cancellation context before starting peers.
	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	// Apply the simulator construction options before creating graph peers.
	for _, opt := range opts {
		if opt != nil {
			if err := opt(s); err != nil {
				s.ctxCancel()
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
		if _, epOk := s.peers[peerID]; epOk {
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
		le.Debugf("peer %s has %d linked peers", peerID, len(linkedPeers))

		// Connect each discovered peer in both directions.
		for _, lpeer := range linkedPeers {
			linkedPeerID := lpeer.GetPeerID()
			op, ok := s.peers[linkedPeerID]
			if !ok {
				continue
			}

			le.Debugf("added in-memory link from %s to %s", linkedPeerID, peerID)
			pushedPeer.connect(s.ctx, op)
		}
	}

	// Apply each peer's deferred configuration after all links exist.
	for _, addedPeer := range allPeers {
		if err := addedPeer.finishSetup(ctx); err != nil {
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
	// Start the graph peer using the simulator lifecycle context.
	p, err := newPeer(s.ctx, s.le, peer, s.verbose)
	if err != nil {
		return nil, err
	}

	// Register the running graph peer under its peer ID.
	peerID := p.GetPeerID()
	s.peers[peerID] = p
	return p, nil
}

// GetPeerByID returns a peer by ID.
func (s *Simulator) GetPeerByID(id bpeer.ID) *Peer {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.peers[id]
}

// Close closes the simulator.
func (s *Simulator) Close() {
	s.ctxCancel()
}
