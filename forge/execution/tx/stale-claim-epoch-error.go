package execution_tx

import "strconv"

// StaleClaimEpochError reports a write carrying a non-authoritative claim epoch.
type StaleClaimEpochError struct {
	ClaimEpoch   uint64
	CurrentEpoch uint64
}

// Error returns the stale claim epoch conflict.
func (e *StaleClaimEpochError) Error() string {
	return "stale execution claim epoch " + strconv.FormatUint(e.ClaimEpoch, 10) +
		"; current epoch is " + strconv.FormatUint(e.CurrentEpoch, 10)
}
