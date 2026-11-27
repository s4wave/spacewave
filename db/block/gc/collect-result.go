package block_gc

import "time"

// CollectResult is one collection pass's accounting.
type CollectResult struct {
	// Netted counts the up and down pages the pass netted.
	Netted uint64
	// Counted counts the count records the pass wrote or deleted.
	Counted uint64
	// Queued counts the stored blocks the pass brought to zero holders.
	Queued uint64
	// Freed counts the blocks the pass freed.
	Freed uint64
	// Parked counts the blocks at zero the free held back because the Volume's
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
	// Unreferenced counts the popped entries of unborn blocks that the
	// reference scan found no root or holding list for. Popped includes
	// them.
	Unreferenced uint64
	// RefScanned counts the roots and child lists the reference scan read.
	RefScanned uint64
	// Seeded counts the unverified seeds the seeding found and pushed onto the
	// front, which hold the flip until the walk decodes them.
	Seeded uint64
	// FrontScanned counts the front entries the pass read.
	FrontScanned uint64
	// Front is the entries left on the front by the scan of the whole front
	// the pass finished, valid when FrontDone is set.
	Front uint64
	// FrontDone reports that the pass finished a scan of the whole front.
	FrontDone bool
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
	// Kept counts the times a free found blocks at zero waiting on their
	// ripe sequence.
	Kept uint64
	// Leaked counts the downs that reached a block without a child list,
	// which floors at its implied holder, so its own children keep it.
	Leaked uint64
	// Underflows counts the count changes that would have gone below zero,
	// which the pass refused, keeping the block. Any such change is an
	// error in the logs.
	Underflows uint64
	// Lapsed counts the leased roots the pass released.
	Lapsed uint64
	// Slices counts the pass's write transactions, each committed when it
	// changed anything.
	Slices uint64
	// StoreTime is the time the pass's write transactions spent in the
	// store outside the collector's own work: waiting for the writer and
	// committing.
	StoreTime time.Duration
	// ScanTime is the time the free spent reading the queue for ripe
	// blocks, in read snapshots that hold no writer.
	ScanTime time.Duration
	// PaceShift is the pace's share at the pass's end, as a power of two: a
	// slice takes its base budgets shifted right by it.
	PaceShift uint
	// More reports that the pass stopped with work left.
	More bool
	// Duration is the time the pass took.
	Duration time.Duration
}
