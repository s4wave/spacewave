package provider

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// Validate checks the fixed account relationship and independently authorized peers.
func (t *AccountTransition) Validate() error {
	// Require the transition to carry its operation identity.
	if t.GetOperationId() == "" {
		return errors.New("account transition requires an operation identity")
	}
	for _, ref := range []*ProviderResourceRef{t.GetSource(), t.GetDestination()} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}

	// Require the source and destination accounts to differ.
	source, destination := t.GetSource(), t.GetDestination()
	if source.GetProviderId() == destination.GetProviderId() && source.GetProviderAccountId() == destination.GetProviderAccountId() {
		return errors.New("account transition requires different accounts")
	}

	// Validate and deduplicate each authorized peer list.
	for _, peers := range [][]string{t.GetDestinationPeerIds(), t.GetSessionPeerIds()} {
		if len(peers) == 0 {
			return errors.New("account transition requires authorized Session peers")
		}
		seen := make(map[string]bool, len(peers))
		for _, id := range peers {
			if _, _, err := peer.ParsePeerIDWithPubKey(id); err != nil {
				return err
			}
			if seen[id] {
				return errors.New("account transition repeats a Session peer")
			}
			seen[id] = true
		}
	}
	return nil
}
