package execution_tx

import (
	"strconv"
	"time"
)

// ClaimLiveError reports a reclaim of a claim whose lease has not expired.
type ClaimLiveError struct {
	ClaimID        string
	Epoch          uint64
	LeaseExpiresAt time.Time
}

// Error names the live claim and when its lease expires.
func (e *ClaimLiveError) Error() string {
	return "execution claim " + e.ClaimID + " at epoch " + strconv.FormatUint(e.Epoch, 10) +
		" is live until " + e.LeaseExpiresAt.Format(time.RFC3339Nano)
}
