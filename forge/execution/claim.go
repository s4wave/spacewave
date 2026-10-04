package forge_execution

import (
	"time"

	"github.com/pkg/errors"
)

// DefaultClaimLease is how long a claim stays live without renewal.
const DefaultClaimLease = 5 * time.Minute

// ClaimClockSkew is the allowance for peer clock difference and target drain.
// Claimants cancel their targets this long before the published lease expiry.
// Reclaim cannot overlap the previous target when the reclaimer's clock lead
// plus the target's cancellation drain time stays within this allowance.
const ClaimClockSkew = 5 * time.Second

// Validate checks that the claim identifies a claim id, fencing epoch, and lease.
func (c *Claim) Validate() error {
	if c.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if c.GetEpoch() == 0 {
		return errors.New("claim epoch cannot be zero")
	}
	if c.GetLeaseExpiresAt() == nil {
		return errors.New("claim lease_expires_at cannot be empty")
	}
	return nil
}

// LeaseExpired reports whether the claim lease has run out at now.
func (c *Claim) LeaseExpired(now time.Time) bool {
	return !now.Before(c.GetLeaseExpiresAt().AsTime())
}
