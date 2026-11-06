package block_gc

import "time"

// Phase is a step of a collection pass, for the Volume's health.
type Phase int

const (
	// PhaseNet nets the journal into the counts and runs the cascades.
	PhaseNet Phase = iota
	// PhaseSweep frees the ripe Dead blocks.
	PhaseSweep
)

// CollectorOptions adjusts a Collector.
type CollectorOptions struct {
	// Frees turns freeing on. With it off, a pass releases lapsed leases and
	// nets the journal, and frees nothing. A Volume whose collector frees
	// adopts itself in the background when it opens unadopted, such as one
	// upgraded from an older format; with Frees off it stays unadopted, since
	// nothing reads exact counts until frees turn on.
	Frees bool
	// Grace is how old a journal record must be before a pass nets it. It
	// must exceed the time a reader takes to pin a head it saw and the lease
	// of a leased root, so a reader whose process stalled past its lease
	// still finds its blocks. Zero uses DefaultGrace.
	Grace time.Duration
	// Now returns the time records and leases are read on; nil is time.Now.
	Now func() time.Time
	// Budget bounds the time one Collect spends before it reports More; zero
	// uses DefaultBudget.
	Budget time.Duration
	// Phase reports each step of a pass, returning the func that ends it; nil
	// reports nothing.
	Phase func(Phase) func()
	// WritersWaiting reports that a foreground writer waits on the store, as
	// kvtx.WriterQueueStore does; nil reports none. The free loop stops at the
	// next block, and a paged cascade at the next page, once one waits.
	WritersWaiting func() bool
}

// sliceTarget is how long the collector's own work in one transaction aims
// to take, not counting the wait for the writer or the commit. The pace
// shrinks the next slice's budgets after one ran longer.
const sliceTarget = 2 * time.Millisecond

// paceMaxShift bounds how far the pace shrinks a slice: to 1/64 of its base
// budgets.
const paceMaxShift = 6

// DefaultGrace is the grace period of a Collector that sets none.
const DefaultGrace = 15 * time.Minute

// DefaultBudget is the time budget of a Collect that sets none.
const DefaultBudget = time.Second

// sliceRecords bounds the journal records one net transaction nets, and the
// cascade records it loads.
const sliceRecords = 256

// slicePairs bounds the journal entries one net transaction applies, and
// the fill ups, so one large commit's record splits across slices instead of
// making one long one.
const slicePairs = 4096

// sliceFrees bounds the Dead blocks one free transaction frees.
const sliceFrees = 128

// sliceReleases bounds the children one net transaction's cascades move. A
// block with more children moves them across transactions, through release
// pages, as continueRelease describes. A moved child costs about five
// operations when it reaches zero, its count, cascade and list reads and
// its zero record and queue key, so a slice of moves is about 15,400.
const sliceReleases = 3072

// releasePageChildren is how many children one release page holds, so a
// net transaction that resumes a paged cascade reads only the pages it
// moves. It is also the slice's moves while a writer waits: a cascade
// beside a waiting writer moves one page, about 5,100 operations, per
// transaction.
const releasePageChildren = 1024

// journalPagePairs bounds the entries of one journal record: a Land with
// more writes its record in pages, so a net slice decodes only the pages it
// works on.
const journalPagePairs = 2048

// sliceLeases bounds the lapsed leased roots one transaction releases.
const sliceLeases = 256
