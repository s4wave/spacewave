package signaling_rpc_server

import (
	"context"

	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
)

// Listen watches the list of ongoing sessions with the peer.
func (s *Server) Listen(req *signaling.ListenRequest, strm signaling.SRPCSignaling_ListenStream) error {
	// Identify the authenticated signaling peer that will receive session requests.
	ctx := strm.Context()
	pid, err := s.ident(ctx)
	if err != nil {
		return err
	}
	pidStr := pid.String()

	// Register the peer and supersede any previous listen stream.
	s.mtx.Lock()
	tkr, existed := s.getPeer(pidStr)
	listenNonce := tkr.beginListen(existed)
	s.mtx.Unlock()

	// Clear the registration when this listen stream exits.
	defer func() {
		s.mtx.Lock()
		currTkr := s.peers[pidStr]
		if currTkr == tkr && currTkr.endListen(listenNonce) {
			s.maybeReleasePeer(pidStr)
		}
		s.mtx.Unlock()
	}()

	// Reconcile desired peer notifications with the last sent state.
	sentWant := make(map[string]struct{})
	for {
		s.mtx.Lock()
		txWant, txNotWant, waitCh, err := tkr.nextListenChanges(listenNonce, sentWant)
		if err != nil {
			s.mtx.Unlock()
			return err
		}
		s.mtx.Unlock()

		// Notify the peer about removed session requests.
		if txNotWant != "" {
			if err := strm.Send(&signaling.ListenResponse{
				Body: &signaling.ListenResponse_ClearPeer{ClearPeer: txNotWant},
			}); err != nil {
				return err
			}
			delete(sentWant, txNotWant)
		}

		// Notify the peer about new session requests.
		if txWant != "" {
			if err := strm.Send(&signaling.ListenResponse{
				Body: &signaling.ListenResponse_SetPeer{SetPeer: txWant},
			}); err != nil {
				return err
			}
			sentWant[txWant] = struct{}{}
		}

		// Wait for a state change or stream cancellation.
		if txNotWant == "" && txWant == "" {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
			}
		}
	}
}
