//go:build !js

package storebench

import "time"

// HotKeyPrefix prefixes the World objects the load writes. Key sampling skips
// them, so the reads of a later run see the same store objects.
const HotKeyPrefix = "store-bench/"

// LoadConfig shapes the synthetic server load a run drives.
type LoadConfig struct {
	// Label names the run in the report.
	Label string
	// Watchers is the number of view watchers.
	Watchers int
	// ViewReads is the number of sampled store objects each view reads.
	ViewReads int
	// HotViewFraction is the share of views that also read a written object,
	// so the writes rerun them.
	HotViewFraction float64
	// ViewInterval is the least time between two evaluations of one view.
	ViewInterval time.Duration
	// SampleKeys is the number of store object keys sampled for the views and
	// the write reads.
	SampleKeys int
	// WriteRate is the World writes started per second.
	WriteRate float64
	// Writers is the number of writes that may run at once.
	Writers int
	// HotKeys is the number of objects the writes update.
	HotKeys int
	// BodyBytes is the size of the object body each write stores.
	BodyBytes int
	// WriteReads is the number of sampled store objects each write reads.
	WriteReads int
	// Warmup is the load run before measuring.
	Warmup time.Duration
	// Duration is the measured load.
	Duration time.Duration
	// Seed seeds the key sampling and the view key choice.
	Seed uint64
	// ProfilePath is where the CPU profile of the measured load is written.
	// Empty skips writing it; the report still attributes the CPU.
	ProfilePath string
	// AllocProfilePath is where the heap allocation profile is written at the
	// end of the window, beside one taken at its start (with a .start suffix)
	// for pprof -base. Empty skips both.
	AllocProfilePath string
	// StallDumpDir is where goroutine dumps of stalled writes are written.
	// Empty turns the dumps off.
	StallDumpDir string
	// StallThreshold is how long a write runs before it counts as stalled.
	StallThreshold time.Duration
	// StallStacks also writes every goroutine's stack at each stall. Each
	// dump stops the world, which lengthens the stall it records.
	StallStacks bool
	// GCMark runs collection passes on the account volume back to back
	// through the measured window, so every write overlaps a pass.
	GCMark bool
}
