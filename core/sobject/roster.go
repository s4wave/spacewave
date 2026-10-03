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
// already drops them.
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
	entry, err := BuildSOConfigChange(host.GetSharedObjectID(), current, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SET_ROSTER, signer, nil)
	if err != nil {
		return false, errors.Wrap(err, "build config change")
	}
	if err := host.ApplyConfigChange(ctx, entry, nil); err != nil {
		return false, err
	}
	return true, nil
}

// RestoreRoster returns each dropped device to the trimming roster once it
// has built on the local peer's latest operation, while the local peer is the
// checkpointer. A dropped writer acknowledges like a roster member, so a
// device that returns and only reads still answers the checkpointer. It
// watches the state and returns when ctx ends or a change fails.
func RestoreRoster(ctx context.Context, so SharedObject, host RosterHost) error {
	// Watch the state as the local peer.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	peerID := so.GetPeerID().String()

	// Restore the caught-up devices of each state.
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

		// Only the checkpointer restores, against its own operations.
		cfg, err := snap.GetConfig(ctx)
		if err != nil {
			return err
		}
		if cfg.Checkpointer() != peerID || len(cfg.GetRosterDroppedPeerIds()) == 0 {
			continue
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return err
		}

		// Keep dropping the devices that have not caught up.
		dropped := cfg.GetRosterDroppedPeerIds()
		keep := slices.DeleteFunc(slices.Clone(dropped), func(device string) bool {
			return set.BuiltOnLatest(device, peerID)
		})
		if len(keep) == len(dropped) {
			continue
		}
		if _, err := host.SetRosterDropped(ctx, keep); err != nil {
			return err
		}
	}
}
