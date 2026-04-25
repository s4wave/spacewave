package resource_root

import (
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// WatchListenerStatus streams the current desktop resource listener
// status: effective socket path, whether the listener is currently
// bound, and the count of connected resource clients. The UI uses
// this to render a live status chip on the session-local command-line
// setup page.
func (s *CoreRootServer) WatchListenerStatus(
	_ *s4wave_root.WatchListenerStatusRequest,
	strm s4wave_root.SRPCRootResourceService_WatchListenerStatusStream,
) error {
	broker := resource_listener.GetProcessStatusBroker()
	ctx := strm.Context()
	var prev s4wave_root.WatchListenerStatusResponse
	first := true
	for {
		snapshot, waitCh := broker.Snapshot()
		current := s4wave_root.WatchListenerStatusResponse{
			SocketPath:       snapshot.SocketPath,
			Listening:        snapshot.Listening,
			ConnectedClients: snapshot.ConnectedClients,
		}
		if first || !listenerStatusEqual(prev, current) {
			first = false
			prev = current
			if err := strm.Send(&current); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// listenerStatusEqual compares two listener status snapshots.
func listenerStatusEqual(a, b s4wave_root.WatchListenerStatusResponse) bool {
	return a.GetSocketPath() == b.GetSocketPath() &&
		a.GetListening() == b.GetListening() &&
		a.GetConnectedClients() == b.GetConnectedClients()
}
