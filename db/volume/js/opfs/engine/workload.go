package engine

import (
	"context"
	"runtime/trace"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// logBlock records a block operation when an execution trace is running,
// encoding the reference only then.
func logBlock(ctx context.Context, op workload.Op, id uint64, ref *block.BlockRef, size int64) {
	if !trace.IsEnabled() {
		return
	}
	key, _ := blockKey(ref) // an invalid reference fails the operation itself
	workload.Record{Op: op, ID: id, Size: size, Key: key}.Log(ctx)
}

// foundSize returns size for a found value and -1 for an absent one.
func foundSize(found bool, size int) int64 {
	if !found {
		return -1
	}
	return int64(size)
}

// presence returns 1 for a present key or block and 0 for an absent one.
func presence(found bool) int64 {
	if !found {
		return 0
	}
	return 1
}

// statSize returns the stat's length, or -1 for an absent block.
func statSize(stat *block.BlockStat) int64 {
	if stat == nil {
		return -1
	}
	return stat.Size
}
