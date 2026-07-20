package execution_tx

import "strconv"

// ClaimHeldError reports that another controller owns the live execution claim.
type ClaimHeldError struct {
	ClaimID string
	Epoch   uint64
}

// Error returns the claim ownership conflict.
func (e *ClaimHeldError) Error() string {
	return "execution claim held by " + e.ClaimID + " at epoch " + strconv.FormatUint(e.Epoch, 10)
}
