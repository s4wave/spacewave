package world_block

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/util/diskfree"
)

// ErrRegenFloor is returned by a build that stopped because free disk fell
// below its floor. The old tree stays live; the stage's blocks and the sort's
// runs are released as the build returns.
var ErrRegenFloor = errors.New("free disk below the regeneration's floor")

// regenFloorCheckBytes is how much the build writes between measurements of
// free disk. The store writes several times what the build yields, since a
// SQLite Volume logs each page it writes before it checkpoints it, so the
// step stays small: a measurement is one statfs, and the disk passes the
// floor by at most the step's amplified size before the build stops.
const regenFloorCheckBytes = 1 << 20

// regenFloor stops a build that would leave less than bytes free on the file
// system of any of its directories. It counts what the build writes and
// measures free disk at the start and after each regenFloorCheckBytes, so
// whatever else fills the disk meanwhile, such as the store's write-ahead
// log or the users' own writes, counts against the floor too. Each catch-up
// round checks it once before it starts. A nil floor never stops the build.
// It is used from one goroutine.
type regenFloor struct {
	// dirs are directories on each file system the build fills.
	dirs []string
	// bytes is the least free disk the build leaves on each.
	bytes uint64
	// unchecked counts the bytes written since the last measurement.
	unchecked int64
}

// newRegenFloor returns the floor of cfg, or nil when it sets none.
func newRegenFloor(cfg RegenConfig) *regenFloor {
	if cfg.FloorBytes == 0 {
		return nil
	}
	f := &regenFloor{bytes: cfg.FloorBytes}
	for _, dir := range []string{cfg.FloorDir, cfg.SpillDir} {
		if dir != "" {
			f.dirs = append(f.dirs, dir)
		}
	}
	return f
}

// wrote counts n bytes the build wrote, measuring free disk when the count
// since the last measurement reaches regenFloorCheckBytes.
func (f *regenFloor) wrote(n int64) error {
	if f == nil {
		return nil
	}
	f.unchecked += n
	if f.unchecked < regenFloorCheckBytes {
		return nil
	}
	return f.check()
}

// check measures free disk on each directory's file system and returns
// ErrRegenFloor when one is below the floor.
func (f *regenFloor) check() error {
	if f == nil {
		return nil
	}
	f.unchecked = 0
	for _, dir := range f.dirs {
		free, err := diskfree.Bytes(dir)
		if err != nil {
			return errors.Wrapf(err, "free disk of %s", dir)
		}
		if free < f.bytes {
			return errors.Wrapf(ErrRegenFloor, "%d bytes free in %s, floor %d", free, dir, f.bytes)
		}
	}
	return nil
}
