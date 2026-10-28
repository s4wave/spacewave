package dexaccess

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/peer"
)

// WatchHolders keeps the holders of exchange at the readable participants of
// the shared object on host, other than this device's local identities, until
// ctx ends or the watch fails. It leaves the holders unknown when it returns.
func WatchHolders(ctx context.Context, host *sobject.SOHost, exchange *dex_solicit.Controller, local ...peer.ID) error {
	// Watch the accepted state, leaving the holders unknown on return.
	states, release, err := host.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()
	defer exchange.SetHolders(nil)

	// Set the holders from each new state.
	var state *sobject.SOState
	for {
		state, err = states.WaitValueChange(ctx, state, nil)
		if err != nil {
			return err
		}
		exchange.SetHolders(Holders(state, local...))
	}
}

// Holders returns the participants of state whose role can read, other than
// local. It returns nil, for unknown holders, when state is nil.
func Holders(state *sobject.SOState, local ...peer.ID) []peer.ID {
	// Leave the holders unknown without a state.
	if state == nil {
		return nil
	}

	// Collect each reader that is not this device.
	holders := []peer.ID{}
	for _, participant := range state.GetConfig().GetParticipants() {
		if !sobject.CanReadState(participant.GetRole()) {
			continue
		}
		id, err := participant.ParsePeerID()
		if err != nil || slices.Contains(local, id) {
			continue
		}
		holders = append(holders, id)
	}
	return holders
}
