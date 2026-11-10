package block_gc

import "time"

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
	// child lists are unverified and Land did not give birth to them.
	Parked uint64
	// Requeued counts the parked blocks the pass moved back onto the free
	// queue once the lists were verified.
	Requeued uint64
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
	// More reports that the pass stopped with work left.
	More bool
	// Duration is the time the pass took.
	Duration time.Duration
}
