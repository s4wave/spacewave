package common

import (
	"os"
	"strconv"
	"time"
)

// FreelistPunchEnv names the environment variable that turns on the online
// freelist punch: "on", "1" or "true" turns it on for every store opened
// after. FreelistPunchReserveEnv, FreelistPunchMaxRangesEnv and
// FreelistPunchIntervalEnv set its tunables; Store.SetFreelistPunch changes
// them on an open store.
const (
	FreelistPunchEnv          = "SPACEWAVE_SQLITE_FREELIST_PUNCH"
	FreelistPunchReserveEnv   = "SPACEWAVE_SQLITE_FREELIST_PUNCH_RESERVE"
	FreelistPunchMaxRangesEnv = "SPACEWAVE_SQLITE_FREELIST_PUNCH_MAX_RANGES"
	FreelistPunchIntervalEnv  = "SPACEWAVE_SQLITE_FREELIST_PUNCH_INTERVAL"
)

// Defaults of FreelistPunch.
const (
	// DefaultFreelistPunchReserve is the bytes of head trunk leaves the punch
	// keeps allocated, as world-punch's -reserve does.
	DefaultFreelistPunchReserve = 1 << 30
	// DefaultFreelistPunchMaxRanges bounds the ranges one round punches.
	DefaultFreelistPunchMaxRanges = 4096
	// DefaultFreelistPunchInterval is the least time between rounds.
	DefaultFreelistPunchInterval = 10 * time.Minute
)

// FreelistPunch configures the online freelist punch. On Linux, a round in
// the checkpointer takes the writer turn, checkpoints the WAL to empty,
// takes SQLite's write lock, walks the file's freelist strictly and releases
// the blocks of up to MaxRanges ranges of free leaf pages past Reserve bytes
// of head trunk leaves with fallocate(PUNCH_HOLE|KEEP_SIZE), then fsyncs. A
// file with auto_vacuum is refused. Elsewhere it does nothing.
type FreelistPunch struct {
	// Enabled turns the punch on.
	Enabled bool
	// Reserve is the bytes of leaves kept allocated, taken whole trunks from
	// the head of the chain, which SQLite allocates from first.
	Reserve int64
	// MaxRanges bounds the ranges one round punches, and so the time it
	// holds the writer turn after its walk.
	MaxRanges int
	// Interval is the least time between rounds.
	Interval time.Duration
}

// freelistPunchFromEnv reads the punch configuration from the environment,
// with the defaults for what it does not set.
func freelistPunchFromEnv() FreelistPunch {
	// Start from the defaults with the punch off.
	c := FreelistPunch{
		Reserve:   DefaultFreelistPunchReserve,
		MaxRanges: DefaultFreelistPunchMaxRanges,
		Interval:  DefaultFreelistPunchInterval,
	}

	// Turn it on and apply the tunables the environment sets.
	switch os.Getenv(FreelistPunchEnv) {
	case "on", "1", "true":
		c.Enabled = true
	}
	if v, err := strconv.ParseInt(os.Getenv(FreelistPunchReserveEnv), 10, 64); err == nil && v >= 0 {
		c.Reserve = v
	}
	if v, err := strconv.Atoi(os.Getenv(FreelistPunchMaxRangesEnv)); err == nil && v > 0 {
		c.MaxRanges = v
	}
	if v, err := time.ParseDuration(os.Getenv(FreelistPunchIntervalEnv)); err == nil && v >= 0 {
		c.Interval = v
	}
	return c
}

// SetFreelistPunch replaces the store's punch configuration; the next round
// uses it, from the start of the file. A MaxRanges below one is taken as one.
func (s *Store[T]) SetFreelistPunch(c FreelistPunch) {
	// Store the setting with a cap of at least one range.
	c.MaxRanges = max(c.MaxRanges, 1)
	s.punchCfg.Store(&c)

	// Restart the cursor, since a new reserve moves where the plan begins;
	// this waits out a round in progress.
	s.punchMtx.Lock()
	s.punchCursor = 0
	s.punchMtx.Unlock()
}

// FreelistPunchConfig returns the store's punch configuration.
func (s *Store[T]) FreelistPunchConfig() FreelistPunch {
	return *s.punchCfg.Load()
}
