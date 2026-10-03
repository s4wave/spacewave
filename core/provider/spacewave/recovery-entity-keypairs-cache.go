package provider_spacewave

import (
	"context"
	"slices"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// recoveryEntityKeypairs returns the current keypairs of entityIDs, the sorted
// readable entities of a shared object config.
//
// The Cloud lists keypairs for exactly the readable entities of its current
// config, so a cached listing answers while it names the same entities. A
// config change that adds or removes a readable entity fetches again. A keypair
// an entity adds without such a change is seen after the next one, or by a new
// SessionClient.
func (c *SessionClient) recoveryEntityKeypairs(
	ctx context.Context,
	soID string,
	entityIDs []string,
) (*api.ListSORecoveryEntityKeypairsResponse, error) {
	// Answer from the cached listing when it names the same entities.
	c.recoveryKeypairsMtx.Lock()
	cached := c.recoveryKeypairs[soID]
	c.recoveryKeypairsMtx.Unlock()
	if cached != nil && slices.Equal(listedEntityIDs(cached), entityIDs) {
		return cached, nil
	}

	// Fetch the current listing.
	resp, err := c.ListSORecoveryEntityKeypairs(ctx, soID)
	if err != nil {
		return nil, err
	}

	// Cache it unless an entity has no keypairs yet, which a later listing
	// may fill.
	if slices.ContainsFunc(resp.GetEntities(), func(entity *api.SORecoveryEntityKeypairs) bool {
		return len(entity.GetKeypairs()) == 0
	}) {
		return resp, nil
	}
	c.recoveryKeypairsMtx.Lock()
	if c.recoveryKeypairs == nil {
		c.recoveryKeypairs = make(map[string]*api.ListSORecoveryEntityKeypairsResponse)
	}
	c.recoveryKeypairs[soID] = resp
	c.recoveryKeypairsMtx.Unlock()
	return resp, nil
}

// listedEntityIDs returns the sorted entity IDs a keypair listing names.
func listedEntityIDs(resp *api.ListSORecoveryEntityKeypairsResponse) []string {
	ids := make([]string, 0, len(resp.GetEntities()))
	for _, entity := range resp.GetEntities() {
		if entity.GetEntityId() != "" {
			ids = append(ids, entity.GetEntityId())
		}
	}
	slices.Sort(ids)
	return ids
}
