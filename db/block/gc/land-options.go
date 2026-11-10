package block_gc

import "time"

// LandOptions configures one Land.
type LandOptions struct {
	// Now is the time the journal record carries.
	Now time.Time
	// TrustReplaced lets an adopted Volume skip the listed check of a child
	// that the stored list of a block in a batch entry's
	// PutBatchEntry.Replaced names, as holdChildren describes. Off, every
	// child a born block names outside its batch takes the check. It costs
	// one child list read per distinct replaced block.
	TrustReplaced bool
}
