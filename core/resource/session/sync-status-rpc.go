package resource_session

import (
	session_syncstatus "github.com/s4wave/spacewave/core/session/syncstatus"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchSyncStatus streams the session sync and network activity snapshot.
func (r *SessionResource) WatchSyncStatus(
	req *s4wave_session.WatchSyncStatusRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchSyncStatusStream,
) error {
	return session_syncstatus.Watch(strm.Context(), r.session, strm.Send)
}
