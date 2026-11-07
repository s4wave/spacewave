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
	return changeSORoster(ctx, host, signer, func([]string) []string {
		return dropped
	})
}

// DropFromSORoster signs, as the owner of signer, one configuration change
// that adds the writers in peers to those the trimming roster drops. It keeps
// a writer that never writes an operation, such as a Session whose storage
// identity writes for it, from holding the stable point at zero. Peers that
// cannot write are ignored. It reports false without a change when the roster
// already drops them, and a concurrent local change of the config does not
// fail it. Under group control it returns ErrAwaitingGroup.
func DropFromSORoster(ctx context.Context, host *SOHost, peers []string, signer crypto.PrivKey) (bool, error) {
	return retryConfigChange(ctx, host, func() (bool, error) {
		return changeSORoster(ctx, host, signer, func(dropped []string) []string {
			return slices.Concat(dropped, peers)
		})
	})
}

// changeSORoster makes one attempt to drop the writers that next returns,
// given those the config the host holds now drops.
func changeSORoster(ctx context.Context, host *SOHost, signer crypto.PrivKey, next func(dropped []string) []string) (bool, error) {
	// Read the current config.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return false, errors.Wrap(err, "get current SO state")
	}
	current := state.GetConfig()

	// Keep the writers in peer ID order.
	writers := make(map[string]bool)
	for _, peerID := range current.Writers() {
		writers[peerID] = true
	}
	dropped := slices.DeleteFunc(slices.Clone(next(current.GetRosterDroppedPeerIds())), func(peerID string) bool {
		return !writers[peerID]
	})
	slices.Sort(dropped)
	dropped = slices.Compact(dropped)
	if slices.Equal(dropped, current.GetRosterDroppedPeerIds()) {
		return false, nil
	}

	// Sign and apply the change.
	nextCfg := current.CloneVT()
	nextCfg.RosterDroppedPeerIds = dropped
	if err := ChangeSOConfig(ctx, host, state, nextCfg, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SET_ROSTER, signer, nil, nil); err != nil {
		return false, err
	}
	return true, nil
}

// MaintainRoster keeps the trimming roster current while the local peer is the
// checkpointer, or the owner that takes over from a stalled one. It returns
// each dropped device to the roster once it has built on the local peer's
// latest operation. A dropped writer acknowledges like a roster member, so a
// device that returns and only reads still answers the checkpointer. Once the
// operations above the checkpoint reach RosterDropBytes it drops the members
// that have not built on the local peer's latest operation, so a device that
// stays offline cannot hold the state past what a peer can sync. That includes
// an offline checkpointer: the first owner whose predecessors on the roster
// have all fallen behind it drops them and becomes the checkpointer. It
// watches the state and returns when ctx ends or a change fails.
func MaintainRoster(ctx context.Context, so SharedObject, host RosterHost) error {
	return watchStates(ctx, so, func(snap SharedObjectStateSnapshot, cfg *SharedObjectConfig, peerID string) error {
		// A group changes its roster only by decision.
		if cfg.IsGroupControl() || !isOwnerPeer(cfg, peerID) {
			return nil
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return err
		}
		next := nextRosterDropped(cfg, set, peerID)
		if slices.Equal(next, cfg.GetRosterDroppedPeerIds()) {
			return nil
		}
		_, err = host.SetRosterDropped(ctx, next)
		return err
	})
}

// nextRosterDropped returns the writers the owner self should drop from the
// trimming roster, in peer ID order. Only the checkpointer, or a successor
// whose owners ahead on the roster all lag behind it once the operations reach
// RosterDropBytes, changes the roster: it keeps the dropped writers that have
// not built on its latest operation, plus, past the budget, the roster members
// that have not. Any other owner returns the dropped writers unchanged.
func nextRosterDropped(cfg *SharedObjectConfig, set *SOOperationSet, self string) []string {
	// Act only as the checkpointer, or past the budget as its successor.
	dropped := cfg.GetRosterDroppedPeerIds()
	overBudget := set.Size() >= RosterDropBytes
	for _, member := range cfg.TrimRoster() {
		if member == self {
			break
		}
		if !isOwnerPeer(cfg, member) {
			continue
		}
		if !overBudget || set.BuiltOnLatest(member, self) {
			return dropped
		}
	}

	// Keep the dropped writers that have not caught up. Until self writes above
	// the checkpoint there is nothing to catch up to, and returning a device
	// whose last operations predate it would restore a peer that never came
	// back.
	nonce, _ := set.AuthorHead(self)
	wrote := nonce != 0 && !set.Covers(self, nonce)
	next := slices.DeleteFunc(slices.Clone(dropped), func(device string) bool {
		return wrote && set.BuiltOnLatest(device, self)
	})

	// Drop the lagging members of an operation set that outgrew the budget.
	if overBudget {
		for _, member := range cfg.TrimRoster() {
			if member != self && !set.BuiltOnLatest(member, self) {
				next = append(next, member)
			}
		}
		slices.Sort(next)
	}
	return next
}
