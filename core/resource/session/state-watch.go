package resource_session

import s4wave_session "github.com/s4wave/spacewave/sdk/session"

// WatchStateAtoms streams the known session state atom store ids on change.
func (r *SessionResource) WatchStateAtoms(
	_ *s4wave_session.WatchSessionStateAtomsRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchStateAtomsStream,
) error {
	return r.session.WatchStateAtomStoreIDs(
		strm.Context(),
		func(storeIDs []string) error {
			return strm.Send(&s4wave_session.WatchSessionStateAtomsResponse{
				StoreIds:   storeIDs,
				StoreCount: uint32(len(storeIDs)),
			})
		},
	)
}
