package signaling_rpc_server

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	"github.com/sirupsen/logrus"
)

// Server implements the signaling service server.
//
// This server expects to be used with Bifrost authenticated streams.
// Each incoming stream context should have a MountedStreamContext.
// To override this value use NewServerWithIdentity.
type Server struct {
	// le is the logger
	le *logrus.Entry
	// ident determines the peer for a stream context
	ident func(ctx context.Context) (peer.ID, error)
	// mtx guards below fields
	mtx sync.Mutex
	// peers contains the list of listening peers
	// keyed by peer id string
	peers map[string]*serverPeerTracker
	// sessions is the map of ongoing session RPCs to ensure no duplicates
	// close the channel to signal that the session was userped
	sessions map[sessionKey]*sessionTracker
}

// NewServer constructs a new server.
func NewServer(le *logrus.Entry) *Server {
	return NewServerWithIdentify(le, nil)
}

// NewServerWithIdentify constructs a new server that identifies the remote peer
// corresponding with the stream context via a custom callback function.
func NewServerWithIdentify(le *logrus.Entry, ident func(ctx context.Context) (peer.ID, error)) *Server {
	if ident == nil {
		ident = func(ctx context.Context) (peer.ID, error) {
			ms := link.GetMountedStreamContext(ctx)
			var pid peer.ID
			if ms != nil {
				pid = ms.GetPeerID()
			}
			if pid == "" {
				return "", errors.New("no mounted stream context")
			}
			return ms.GetPeerID(), nil
		}
	}
	return &Server{
		le:       le,
		ident:    ident,
		peers:    make(map[string]*serverPeerTracker),
		sessions: make(map[sessionKey]*sessionTracker),
	}
}

// getPeer gets or creates a peer tracker for a peer id.
func (s *Server) getPeer(pidStr string) (*serverPeerTracker, bool) {
	tkr, exists := s.peers[pidStr]
	if !exists {
		tkr = &serverPeerTracker{
			wantPeers: make(map[string]struct{}),
		}
		s.peers[pidStr] = tkr
	}
	return tkr, exists
}

// maybeReleasePeer releases the peer tracker if it has no references.
// returns if it was found & released
func (s *Server) maybeReleasePeer(pidStr string) bool {
	tkr := s.peers[pidStr]
	if tkr == nil || tkr.listening || len(tkr.wantPeers) != 0 {
		return false
	}
	delete(s.peers, pidStr)
	tkr.broadcast()
	return true
}

// _ is a type assertion
var _ signaling.SRPCSignalingServer = ((*Server)(nil))
