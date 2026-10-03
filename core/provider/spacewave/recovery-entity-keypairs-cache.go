package provider_spacewave

import (
	"context"
	"slices"
	"sync"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// recoveryKeypairCache holds the last recovery entity keypair listing of each
// shared object, keyed by shared object ID.
//
// A ProviderAccount owns one and shares it with every SessionClient it
// configures, so the listings outlive a replaced client and serve every Session
// of the account.
type recoveryKeypairCache struct {
	// mtx guards listings.
	mtx sync.Mutex
	// listings maps a shared object ID to its last complete listing.
	listings map[string]*api.ListSORecoveryEntityKeypairsResponse
}

// get returns the cached listing of soID, or nil.
func (c *recoveryKeypairCache) get(soID string) *api.ListSORecoveryEntityKeypairsResponse {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.listings[soID]
}

// set caches the listing of soID.
func (c *recoveryKeypairCache) set(soID string, resp *api.ListSORecoveryEntityKeypairsResponse) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.listings == nil {
		c.listings = make(map[string]*api.ListSORecoveryEntityKeypairsResponse)
	}
	c.listings[soID] = resp
}

// recoveryEntityKeypairs returns the current keypairs of entityIDs, the sorted
// readable entities of a shared object config.
//
// The Cloud lists keypairs for exactly the readable entities of its current
// config, so a cached listing answers while it names the same entities. A
// config change that adds or removes a readable entity fetches again. A keypair
// an entity adds without such a change is seen after the next one, or after a
// restart. A client without an account cache always fetches.
func (c *SessionClient) recoveryEntityKeypairs(
	ctx context.Context,
	soID string,
	entityIDs []string,
) (*api.ListSORecoveryEntityKeypairsResponse, error) {
	// Answer from the cached listing when it names the same entities.
	cache := c.recoveryKeypairs
	if cache != nil {
		cached := cache.get(soID)
		if cached != nil && slices.Equal(listedEntityIDs(cached), entityIDs) {
			return cached, nil
		}
	}

	// Fetch the current listing.
	resp, err := c.ListSORecoveryEntityKeypairs(ctx, soID)
	if err != nil {
		return nil, err
	}

	// Cache it unless an entity has no keypairs yet, which a later listing
	// may fill.
	complete := !slices.ContainsFunc(resp.GetEntities(), func(entity *api.SORecoveryEntityKeypairs) bool {
		return len(entity.GetKeypairs()) == 0
	})
	if cache != nil && complete {
		cache.set(soID, resp)
	}
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
