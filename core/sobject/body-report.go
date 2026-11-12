package sobject

import (
	"slices"
)

// BodyReport is what a SharedObject's health shows of this device's replay.
type BodyReport struct {
	// RejectedEdits are this device's operations an earlier replay here
	// applied and the current order refuses.
	RejectedEdits []*SORejectedEdit
	// CheckpointMismatch names the latest installed checkpoint whose state
	// differs from the state the body reached after the operations it covers,
	// or is nil.
	CheckpointMismatch *SOCheckpointMismatch
	// Unordered counts the edits the appointed sequencer has not placed.
	Unordered uint32
	// Damage is the store refusal replay could not heal, or nil.
	Damage *SOStoreDamage
}

// equal reports whether r and other show the same health.
func (r *BodyReport) equal(other *BodyReport) bool {
	return other != nil &&
		r.Unordered == other.Unordered &&
		r.Damage.EqualVT(other.Damage) &&
		r.CheckpointMismatch.EqualVT(other.CheckpointMismatch) &&
		slices.EqualFunc(r.RejectedEdits, other.RejectedEdits, (*SORejectedEdit).EqualVT)
}
