package block_gc

import "time"

// Phase is a step of a collection pass, for the Volume's health.
type Phase int

const (
	// PhaseNet nets the up and down logs into the counts.
	PhaseNet Phase = iota
	// PhaseSweep frees the ripe blocks at zero.
	PhaseSweep
)

// CollectorOptions adjusts a Collector.
type CollectorOptions struct {
	// Frees turns freeing on. With it off, a pass releases lapsed leases
	// and nets the logs, and frees nothing. A Volume whose collector frees
	// adopts itself in the background when it opens unadopted, such as one
	// upgraded from an older format. With Frees off it stays unadopted,
	// since nothing reads exact counts until frees turn on.
	Frees bool
	// Grace is how old a down page must be before a pass nets it. It must
	// exceed the time a reader takes to pin a head it saw and the lease of a
	// leased root, so a reader whose process stalled past its lease still
	// finds its blocks. Zero uses DefaultGrace.
	Grace time.Duration
	// Now returns the time records and leases are read on. Nil is time.Now.
	Now func() time.Time
	// Budget bounds the time one Collect spends before it reports More.
	// Zero uses DefaultBudget.
	Budget time.Duration
	// Phase reports each step of a pass, returning the func that ends it.
	// Nil reports nothing.
	Phase func(Phase) func()
	// WritersWaiting reports that a foreground writer waits on the store,
	// as kvtx.WriterQueueStore does. Nil reports none. The free loop stops
	// at the next block, and a paged release at the next page, once one
	// waits.
	WritersWaiting func() bool
	// ParkKeepsHolds selects the park that keeps a block's holds while the
	// old generation is Open: its children keep the parked block as a
	// holder until the sweep gives their downs, so the free stops at the
	// first parked level until the flip. Off, the default, a park gives the
	// children their downs as a free does and the free cascades, as
	// Collector's invariant 10 describes. Each path reads which park a block
	// has, so the option may change between passes.
	ParkKeepsHolds bool
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

// sliceRecords bounds the down pages one net transaction restamps, and the
// roots and pages one settle slice reads.
const sliceRecords = 256

// slicePairs bounds the log keys one net transaction applies, in whole
// pages, so a backlog splits across slices instead of making one long one.
const slicePairs = 4096

// sliceFrees bounds the blocks one free transaction frees.
const sliceFrees = 128

// sliceReleases bounds the children one transaction's frees move down. A
// freed block with more children moves them across transactions, through
// release pages, as continueRelease describes. A moved child costs about
// three operations, its count read and, at zero, its list read and its
// count and queue writes, so a slice of moves is about 9,200.
const sliceReleases = 3072

// releasePageChildren is how many children one release page holds, so a
// net transaction that resumes a paged release reads only the pages it
// moves. It is also the slice's moves while a writer waits.
const releasePageChildren = 1024

// logPageKeys bounds the block keys of one up or down page: a Land with
// more writes more pages, so a net slice decodes only the pages it works
// on.
const logPageKeys = 2048

// sliceLeases bounds the lapsed leases one transaction reads, and the roots
// of them it releases.
const sliceLeases = 256
