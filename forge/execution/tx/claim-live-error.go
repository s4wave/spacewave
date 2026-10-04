package execution_tx

import (
	"strconv"
	"time"
)

// ClaimLiveError reports a reclaim of a claim whose lease has not expired.
type ClaimLiveError struct {
	// ClaimID identifies the live claimant.
	ClaimID string
	// Epoch is the live claim's fencing token.
	Epoch uint64
	// LeaseExpiresAt is the earliest time a peer may reclaim this claim.
	LeaseExpiresAt time.Time
}

// Error names the live claim and when its lease expires.
func (e *ClaimLiveError) Error() string {
	return "execution claim " + e.ClaimID + " at epoch " + strconv.FormatUint(e.Epoch, 10) +
		" is live until " + e.LeaseExpiresAt.Format(time.RFC3339Nano)
}
