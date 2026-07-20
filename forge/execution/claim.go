package forge_execution

import "github.com/pkg/errors"

// Validate checks that the claim identifies an owner and fencing epoch.
func (c *Claim) Validate() error {
	if c.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if c.GetEpoch() == 0 {
		return errors.New("claim epoch cannot be zero")
	}
	return nil
}
