package common

import (
	"math"
	"os"
	"strconv"
	"time"
)

// FreelistPunchEnv names the environment variable that turns on the online
// freelist punch: "on", "1" or "true" turns it on for every store opened
// after. FreelistPunchReserveEnv, FreelistPunchMaxRangesEnv and
// FreelistPunchIntervalEnv and FreelistPunchFloorEnv set its tunables; Store.SetFreelistPunch changes
// them on an open store.
const (
	FreelistPunchEnv          = "SPACEWAVE_SQLITE_FREELIST_PUNCH"
	FreelistPunchReserveEnv   = "SPACEWAVE_SQLITE_FREELIST_PUNCH_RESERVE"
	FreelistPunchMaxRangesEnv = "SPACEWAVE_SQLITE_FREELIST_PUNCH_MAX_RANGES"
	FreelistPunchIntervalEnv  = "SPACEWAVE_SQLITE_FREELIST_PUNCH_INTERVAL"
	FreelistPunchFloorEnv     = "SPACEWAVE_SQLITE_FREELIST_PUNCH_FLOOR"
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
	// DefaultFreelistPunchFloor is the free disk space, less the holes
	// already in the file, below which no round punches.
	DefaultFreelistPunchFloor = 8 << 30
	// freelistPunchFloorReserves is the least floor in reserves: a Floor
	// under this many times Reserve is taken as that.
	freelistPunchFloorReserves = 4
)

// FreelistPunch configures the online freelist punch. On Linux, a round in
// the checkpointer checkpoints the WAL to empty, begins a read transaction,
// requires the WAL still empty, so that no checkpoint can write the file
// until the transaction ends, walks the file's freelist strictly and
// releases the blocks of up to MaxRanges ranges of free leaf pages past
// Reserve bytes of head trunk leaves with fallocate(PUNCH_HOLE|KEEP_SIZE),
// then fsyncs. A file with auto_vacuum is refused, and no round runs while
// the disk's free space less the file's holes is under Floor. Elsewhere it
// does nothing.
type FreelistPunch struct {
	// Enabled turns the punch on.
	Enabled bool
	// Reserve is the bytes of leaves kept allocated, taken whole trunks from
	// the head of the chain, which SQLite allocates from first.
	Reserve int64
	// MaxRanges bounds the ranges one round punches, and so the time its
	// read transaction holds checkpoints off after its walk.
	MaxRanges int
	// Interval is the least time between rounds.
	Interval time.Duration
	// Floor is the least free disk space, in bytes, the filesystem must have
	// beyond the holes already in the file for a round to punch: SQLite can
	// refill every hole as it reuses free pages, and a refill that finds the
	// disk full fails a checkpoint. Punching leaves free space less holes as
	// it was, so a disk that held Floor beyond the holes before a round
	// holds it after SQLite has refilled them all, unless other files grew.
	// A Floor under four times Reserve is taken as that.
	Floor int64
}

// freelistPunchFromEnv reads the punch configuration from the environment,
// with the defaults for what it does not set.
func freelistPunchFromEnv() FreelistPunch {
	// Start from the defaults with the punch off.
	c := FreelistPunch{
		Reserve:   DefaultFreelistPunchReserve,
		MaxRanges: DefaultFreelistPunchMaxRanges,
		Interval:  DefaultFreelistPunchInterval,
		Floor:     DefaultFreelistPunchFloor,
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
	if v, err := strconv.ParseInt(os.Getenv(FreelistPunchFloorEnv), 10, 64); err == nil && v >= 0 {
		c.Floor = v
	}
	return c.normalized()
}

// normalized returns c with a cap of at least one range and a floor of at
// least four reserves.
func (c FreelistPunch) normalized() FreelistPunch {
	c.MaxRanges = max(c.MaxRanges, 1)
	if c.Reserve <= math.MaxInt64/freelistPunchFloorReserves {
		c.Floor = max(c.Floor, freelistPunchFloorReserves*c.Reserve)
	} else {
		c.Floor = math.MaxInt64
	}
	return c
}

// SetFreelistPunch replaces the store's punch configuration; the next round
// uses it, from the start of the file. A MaxRanges below one is taken as
// one, and a Floor under four times Reserve as that.
func (s *Store[T]) SetFreelistPunch(c FreelistPunch) {
	// Store the setting with its cap and floor raised to their least.
	c = c.normalized()
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
