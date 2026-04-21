package signaling_rpc_server

import (
	"context"

	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
)

// Listen watches the list of ongoing sessions with the peer.
func (s *Server) Listen(req *signaling.ListenRequest, strm signaling.SRPCSignaling_ListenStream) error {
	ctx := strm.Context()
	pid, err := s.ident(ctx)
	if err != nil {
		return err
	}
	pidStr := pid.String()

	/*
		pubKey, err := pid.ExtractPublicKey()
		if err != nil {
			return err
		}
	*/

	// Register this peer
	s.mtx.Lock()
	tkr, existed := s.getPeer(pidStr)
	if existed {
		// userp any existing Listen call for this peer
		tkr.listenNonce++
		tkr.broadcast()
	}
	listenNonce := tkr.listenNonce
	s.mtx.Unlock()

	// Cleanup when we exit
	defer func() {
		s.mtx.Lock()
		currTkr := s.peers[pidStr]
		if currTkr == tkr && currTkr.listenNonce == listenNonce {
			currTkr.listenNonce++
			currTkr.listening = false
			currTkr.broadcast()
			s.maybeReleasePeer(pidStr)
		}
		s.mtx.Unlock()
	}()

	sentWant := make(map[string]struct{})
	for {
		s.mtx.Lock()
		if tkr.listenNonce != listenNonce {
			s.mtx.Unlock()
			return signaling.ErrUserpedListen
		}
		var txWant, txNotWant string
		for wantPeerID := range tkr.wantPeers {
			if _, ok := sentWant[wantPeerID]; !ok {
				txWant = wantPeerID
				break
			}
		}
		for sentPeerID := range sentWant {
			if _, ok := tkr.wantPeers[sentPeerID]; !ok {
				txNotWant = sentPeerID
				break
			}
		}
		waitCh := tkr.getWaitCh()
		s.mtx.Unlock()

		if txNotWant != "" {
			if err := strm.Send(&signaling.ListenResponse{
				Body: &signaling.ListenResponse_ClearPeer{ClearPeer: txNotWant},
			}); err != nil {
				return err
			}
			delete(sentWant, txNotWant)
		}

		if txWant != "" {
			if err := strm.Send(&signaling.ListenResponse{
				Body: &signaling.ListenResponse_SetPeer{SetPeer: txWant},
			}); err != nil {
				return err
			}
			sentWant[txWant] = struct{}{}
		}

		if txNotWant == "" && txWant == "" {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
			}
		}
	}
}
