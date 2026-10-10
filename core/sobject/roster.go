package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// RosterHost is an optional interface on SharedObject implementations whose
// local peer can sign changes to the trimming roster as an owner.
type RosterHost interface {
	// SetRosterDropped drops exactly the writers in dropped from the trimming
	// roster. It reports false when the roster already drops them.
	SetRosterDropped(ctx context.Context, dropped []string) (bool, error)
}

// SetSORoster signs, as the owner of signer, one configuration change that
// drops exactly the writers in dropped from the trimming roster. Peers that
// cannot write are ignored. It reports false without a change when the roster
// already drops them. Under group control signer, a voter, agrees to the
// change and it returns ErrAwaitingGroup.
func SetSORoster(ctx context.Context, host *SOHost, dropped []string, signer crypto.PrivKey) (bool, error) {
	// Read the current config.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return false, errors.Wrap(err, "get current SO state")
	}
	current := state.GetConfig()

	// Keep the writers in peer ID order.
	writers := current.Writers()
	next := slices.DeleteFunc(slices.Clone(dropped), func(peerID string) bool {
		return !slices.Contains(writers, peerID)
	})
	slices.Sort(next)
	next = slices.Compact(next)
	if slices.Equal(next, current.GetRosterDroppedPeerIds()) {
		return false, nil
	}

	// Sign and apply the change.
	nextCfg := current.CloneVT()
	nextCfg.RosterDroppedPeerIds = next
	if err := ChangeSOConfig(ctx, host, state, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SET_ROSTER, signer, nil, nil); err != nil {
		return false, err
	}
	return true, nil
}

// MaintainRoster keeps the trimming roster current while the local peer is the
// checkpointer. It returns each dropped device to the roster once it has built
// on the local peer's latest operation. A dropped writer acknowledges like a
// roster member, so a device that returns and only reads still answers the
// checkpointer. Once the operations above the checkpoint reach RosterDropBytes
// it drops the members that have not built on the local peer's latest
// operation, so a device that stays offline cannot hold the state past what a
// peer can sync. It watches the state and returns when ctx ends or a change
// fails.
func MaintainRoster(ctx context.Context, so SharedObject, host RosterHost) error {
	// Watch the state as the local peer.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	peerID := so.GetPeerID().String()

	// Adjust the roster of each state.
	var snap SharedObjectStateSnapshot
	for {
		// Wait for the next state.
		snap, err = ctr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}
		if snap == nil {
			continue
		}

		// Only the checkpointer restores, against its own operations. A group
		// changes its roster only by decision.
		cfg, err := snap.GetConfig(ctx)
		if err != nil {
			return err
		}
		if cfg.IsGroupControl() || cfg.Checkpointer() != peerID {
			continue
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return err
		}

		next := nextRosterDropped(cfg, set, peerID)
		if slices.Equal(next, cfg.GetRosterDroppedPeerIds()) {
			continue
		}
		if _, err := host.SetRosterDropped(ctx, next); err != nil {
			return err
		}
	}
}

// nextRosterDropped returns the writers the checkpointer should drop from the
// trimming roster, in peer ID order: the dropped writers that have not built
// on its latest operation, plus, once the operations reach RosterDropBytes,
// the roster members that have not.
func nextRosterDropped(cfg *SharedObjectConfig, set *SOOperationSet, checkpointer string) []string {
	// Keep the dropped writers that have not caught up.
	next := slices.DeleteFunc(slices.Clone(cfg.GetRosterDroppedPeerIds()), func(device string) bool {
		return set.BuiltOnLatest(device, checkpointer)
	})

	// Drop the lagging members of an operation set that outgrew the budget.
	if set.Size() >= RosterDropBytes {
		for _, member := range cfg.TrimRoster() {
			if member != checkpointer && !set.BuiltOnLatest(member, checkpointer) {
				next = append(next, member)
			}
		}
		slices.Sort(next)
	}
	return next
}
