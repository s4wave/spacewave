package block_gc

import "time"

// Stall is why a collection pass deleted nothing it would have.
type Stall uint8

// The reasons a pass stalls.
const (
	// StallNone is a pass that did not stall.
	StallNone Stall = iota
	// StallNoFlip is a Volume Sweeping with no Flip in its state: nothing
	// says which blocks were Dead at the flip, so the pass sweeps nothing and
	// frees nothing. No flip of this format writes that state.
	StallNoFlip
	// StallHoldMark is a pass whose flip or free waited on the hold mark: a
	// Land past the cursor held a block at zero, and nothing frees or flips
	// until that record nets, at least Grace after it committed. Holds of
	// zero blocks arriving with no gap of Grace keep the Volume here; the
	// writers wait on nothing.
	StallHoldMark
)

// String names the stall.
func (s Stall) String() string {
	switch s {
	case StallNone:
		return "none"
	case StallNoFlip:
		return "sweeping with no flip cursor"
	case StallHoldMark:
		return "waiting on the hold mark"
	}
	return "unknown"
}

// CollectResult is one collection pass's accounting.
type CollectResult struct {
	// Netted counts the journal records the pass netted.
	Netted uint64
	// Counted counts the count records the pass wrote or deleted.
	Counted uint64
	// Queued counts the stored blocks the pass brought to zero holders.
	Queued uint64
	// Freed counts the blocks the pass freed.
	Freed uint64
	// Parked counts the Dead blocks the free held back because the Volume's
	// old generation is Open, when nothing is deleted.
	Parked uint64
	// Unmarked counts the unverified lists the pass entered in the unmarked
	// run.
	Unmarked uint64
	// Popped counts the front entries the pass took off because the free
	// proof held for their blocks.
	Popped uint64
	// Seeded counts the unverified seeds the flip found and pushed onto the
	// front, which hold the flip until the walk decodes them.
	Seeded uint64
	// Returned counts the children a re-hold returned to the settle's front
	// while the old generation is Open: blocks back from zero whose lists
	// the walk must verify before the flip.
	Returned uint64
	// Flipped reports that the pass moved the generation to Sweeping.
	Flipped bool
	// Unadopted reports that the flip returned the Volume to unadopted
	// because a list rewrite left the counts short, so an adoption recounts
	// the lists before the flip.
	Unadopted bool
	// Swept counts the unmarked blocks the pass deleted.
	Swept uint64
	// Settled reports that the pass deleted the run and settled the
	// generation.
	Settled bool
	// Stall is why the pass deleted nothing it would have, StallNone when
	// it did not stall.
	Stall Stall
	// Kept counts the times a free found Dead blocks waiting on their ripe
	// sequence or the hold mark.
	Kept uint64
	// Leaked counts the children a down cascade reached that the store
	// lacks, whose own children the cascade cannot reach.
	Leaked uint64
	// Stranded counts the down cascades the pass dropped because the store
	// lost their block with its child list: the block's children keep its
	// holder, garbage an audit counts as leaked.
	Stranded uint64
	// Underflows counts the count changes that would have gone below zero,
	// which the pass refused, keeping the block; any is an error in the
	// journal.
	Underflows uint64
	// Lapsed counts the leased roots the pass released.
	Lapsed uint64
	// HoldMark is the hold mark the pass read: one past the last journal
	// record of a Land that held a block the collector may have at zero. A
	// mark that moved since the last pass shows such a Land; the free and the
	// settle's pop wait while the cursor trails it.
	HoldMark uint64
	// Cursor is the cursor the pass read with the hold mark.
	Cursor uint64
	// More reports that the pass stopped with work left.
	More bool
	// Duration is the time the pass took.
	Duration time.Duration
}
