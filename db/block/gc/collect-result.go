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
	// old generation is Open, when nothing is deleted.
	Parked uint64
	// Requeued counts the parked blocks the pass moved back onto the free
	// queue once the generation flipped.
	Requeued uint64
	// Unmarked counts the unverified lists the pass entered in the unmarked
	// run.
	Unmarked uint64
	// Popped counts the front entries the pass took off because the free
	// proof held for their blocks.
	Popped uint64
	// Seeded counts the unverified seeds the seeding found and pushed onto the
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
	// Kept counts the times a free found Dead blocks waiting on their ripe
	// sequence or the hold mark, and the blocks it kept because a record
	// past the cursor names them.
	Kept uint64
	// TailReading counts the free transactions that freed nothing because
	// their read of the journal past the cursor stopped at its budget short
	// of the journal's end; the next one reads on.
	TailReading uint64
	// TailSaturated counts the free transactions that found the journal past
	// the cursor naming more entries than CollectorOptions.TailEntries, so
	// they kept every block they reached.
	TailSaturated uint64
	// StallHoldMark counts the free and pop slices that found a ripe block
	// they could take and waited only because the cursor trails the hold
	// mark: a Land held a block the collector may have at zero, and nothing
	// frees or pops until netting passes that Land's record, about one Grace
	// later. A writer that holds such a block again within each Grace keeps
	// the cursor behind the mark on every pass, which stops every free.
	StallHoldMark uint64
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
	// More reports that the pass stopped with work left.
	More bool
	// Duration is the time the pass took.
	Duration time.Duration
}
