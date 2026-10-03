package resource_sobject

import (
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

// WatchSharedObjectParticipation observes accepted authority without requiring body decryption.
func (r *SharedObjectResource) WatchSharedObjectParticipation(
	_ *s4wave_sobject.WatchSharedObjectParticipationRequest,
	stream s4wave_sobject.SRPCSharedObjectResourceService_WatchSharedObjectParticipationStream,
) error {
	// The mounted SharedObject supplies both the viewer and the retained native state.
	host, ok := r.sharedObject.(sobject.InviteHost)
	if !ok {
		return errors.New("shared object does not expose native participation")
	}
	ctx := stream.Context()
	state, release, err := host.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()

	// Root progress and transport health do not manufacture participation transitions.
	var previous *sobject.SharedObjectConfig
	return ccontainer.WatchChanges(ctx, nil, state, func(value *sobject.SOState) error {
		// Ignore SharedObject snapshots without a new accepted configuration.
		if value.GetConfig() == nil || value.GetConfig().EqualVT(previous) {
			return nil
		}

		// Describe participation using the viewer and accepted SharedObject configuration.
		config := value.GetConfig().CloneVT()
		participation := &s4wave_sobject.SharedObjectParticipation{
			ViewerPeerId: r.sharedObject.GetPeerID().String(),
			Config:       config,
		}

		// Attach retained configuration history when the SharedObject exposes it.
		if history, ok := r.sharedObject.(sobject.SharedObjectConfigHistoryAccessor); ok {
			base, changes, err := history.ReadSharedObjectConfigHistory(ctx, config)
			if err != nil && !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
				return err
			}
			participation.ConfigHistoryBase = base
			participation.ConfigHistoryChanges = changes
		}

		// Publish participation before remembering the configuration for this stream.
		if err := stream.Send(participation); err != nil {
			return err
		}
		previous = config
		return nil
	}, nil)
}
