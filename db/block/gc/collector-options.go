package block_gc

import "time"

// Phase is a step of a collection pass, for the Volume's health.
type Phase int

const (
	// PhaseNet nets the journal into the counts and the free queue.
	PhaseNet Phase = iota
	// PhaseSweep frees the queued blocks.
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
}

// DefaultGrace is the grace period of a Collector that sets none.
const DefaultGrace = 15 * time.Minute

// DefaultBudget is the time budget of a Collect that sets none.
const DefaultBudget = time.Second

// sliceRecords bounds the journal records one net transaction nets.
const sliceRecords = 256

// slicePairs bounds the pairs one net transaction applies, so one large
// commit's record splits across slices instead of making one long one.
const slicePairs = 4096

// sliceFrees bounds the queued blocks one free transaction examines.
const sliceFrees = 128

// sliceReleases bounds the children one free transaction releases. A block
// with more children releases them across transactions, as continueRelease
// describes.
const sliceReleases = 4096

// releasePageChildren is how many children one release page holds, so a
// free transaction that resumes a release reads only the pages it releases.
const releasePageChildren = 2048

// journalPagePairs bounds the pairs of one journal record: a Land with more
// writes its record in pages, so a net or index slice decodes only the pages
// it works on.
const journalPagePairs = 2048

// sliceLeases bounds the lapsed leased roots one transaction releases.
const sliceLeases = 256
