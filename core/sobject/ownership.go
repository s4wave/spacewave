package sobject

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// SOOwnershipState names where ownership of a SharedObject stands.
type SOOwnershipState int

const (
	// SOOwnershipSettled is ownership with no transfer pending: every signing
	// owner owns.
	SOOwnershipSettled SOOwnershipState = iota
	// SOOwnershipHandingOver is ownership after a transfer that carries its
	// owner's departure and before the successor commits that departure. The
	// departing owner still signs in the config, but follows, serves blocks
	// to the successor and never signs again.
	SOOwnershipHandingOver
)

// SOOwnership is who owns a SharedObject, derived from its config and the
// change that produced the config head. It is computed on read and never
// stored, so it cannot disagree with the config.
type SOOwnership struct {
	// state is where ownership stands.
	state SOOwnershipState
	// signing are the owners whose signatures the config accepts.
	signing []string
	// departing are the owners the head transfer's leave request names.
	departing []string
}

// DeriveSOOwnership derives ownership from config and the change head that
// produced it, which may be nil when history is not retained.
func DeriveSOOwnership(config *SharedObjectConfig, head *SOConfigChange) (*SOOwnership, error) {
	// Settled unless the head transfers ownership with a departure.
	own := &SOOwnership{signing: config.SigningOwners()}
	departing, err := SODepartingPeers(head)
	if err != nil || len(departing) == 0 {
		return own, err
	}

	// The departing owners hand over to the owners that remain.
	own.state = SOOwnershipHandingOver
	own.departing = departing
	return own, nil
}

// ReadSOOwnership derives ownership of config, reading the change that produced
// its head with readEntry. A head whose change is not retained, or a nil
// readEntry, reads as settled.
func ReadSOOwnership(
	ctx context.Context,
	config *SharedObjectConfig,
	readEntry func(context.Context, []byte) (*SOConfigChange, error),
) (*SOOwnership, error) {
	// Without a head or history there is no transfer to read.
	head := config.GetConfigChainHash()
	if len(head) == 0 || readEntry == nil {
		return DeriveSOOwnership(config, nil)
	}

	// Derive the state from the retained change that produced the head.
	entry, err := readEntry(ctx, head)
	if errors.Is(err, ErrConfigHistoryUnavailable) {
		return DeriveSOOwnership(config, nil)
	}
	if err != nil {
		return nil, err
	}
	return DeriveSOOwnership(config, entry)
}

// refuseDepartingSigners returns ErrHandingOver when entry is signed by an
// owner departing at the head entry builds on. Its successor commits the
// departure, and any other change it signs on the transfer head forks the
// config chain. The head is named by entry's previous hash, so its change is
// read without the state lock, which a history reader may take.
func (s *SOHost) refuseDepartingSigners(ctx context.Context, entry *SOConfigChange) error {
	// Read the departure the head carries, if any.
	head := entry.GetPreviousHash()
	if len(head) == 0 {
		return nil
	}
	prev, err := s.ReadConfigEntry(ctx, head)
	if errors.Is(err, ErrConfigHistoryUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}
	departing, err := SODepartingPeers(prev)
	if err != nil || len(departing) == 0 {
		return err
	}

	// Refuse a change any departing owner signs.
	for _, sig := range entry.GetSignatures() {
		_, signer, err := signerOf(sig)
		if err != nil {
			return err
		}
		if slices.Contains(departing, signer.String()) {
			return ErrHandingOver
		}
	}
	return nil
}

// State returns where ownership stands.
func (o *SOOwnership) State() SOOwnershipState {
	return o.state
}

// Settled reports whether no transfer is pending.
func (o *SOOwnership) Settled() bool {
	return o.state == SOOwnershipSettled
}

// Owns reports whether peerID owns: it signs as an owner and is not departing.
func (o *SOOwnership) Owns(peerID string) bool {
	return slices.Contains(o.signing, peerID) && !o.Departing(peerID)
}

// Departing reports whether peerID is an owner handing over.
func (o *SOOwnership) Departing(peerID string) bool {
	return slices.Contains(o.departing, peerID)
}

// Owners returns the owners that own, in participant order: the signing
// owners less the departing ones.
func (o *SOOwnership) Owners() []string {
	return slices.DeleteFunc(slices.Clone(o.signing), o.Departing)
}

// SigningOwners returns the owners whose signatures the config accepts,
// including departing ones, in participant order.
func (o *SOOwnership) SigningOwners() []string {
	return slices.Clone(o.signing)
}
