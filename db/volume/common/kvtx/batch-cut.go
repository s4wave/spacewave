package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// backgroundBatchHold is how long a background staged batch holds the store's
// writer turn before it commits what it wrote and queues for the turn again.
// A foreground writer that arrives during a background batch waits about
// this long, plus one commit, instead of the whole batch. Zero puts each
// batch in one transaction.
var backgroundBatchHold = 50 * time.Millisecond

// batchCutEntries is how many entries the first cut of a batch writes. Each
// later cut doubles while a transaction holds the turn under half of
// backgroundBatchHold and halves when it holds it longer, down to one entry.
const batchCutEntries = 128

// putStagedCut stores entries under stage, as prepareStaged does, in
// consecutive in-order cuts of one transaction each, sized so each holds the
// writer turn about backgroundBatchHold. It returns at the first refusal; the
// cuts before it stay committed, as smaller batches would.
//
// A staged batch cut into several transactions is equivalent to one, because
// the stage holds every block it stored until the swap names it. Each cut
// roots the blocks it stores under the stage in the same transaction and
// records the roots before the next cut, so a later cut whose parent names
// them releases their roots as a later batch would. A parent cut apart from
// a child it names that comes after it leaves the child held under its own
// stage root as well, which keeps data, not loses it, until the stage
// releases. A crash between cuts leaves a stage that is resumed or discarded
// whole: its roots lapse with its lease, as for any stage of a process that
// died.
func (v *Volume) putStagedCut(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error) {
	existed := make([]bool, 0, len(entries))
	n := batchCutEntries
	for remaining := entries; len(remaining) != 0; {
		cut := remaining[:min(n, len(remaining))]
		start := time.Now()
		got, err := v.prepareStaged(ctx, stage, cut)
		if err != nil {
			return nil, err
		}
		existed = append(existed, got...)
		remaining = remaining[len(cut):]

		// Size the next cut from how long this one held the turn.
		switch held := time.Since(start); {
		case held < backgroundBatchHold/2:
			n *= 2
		case held > backgroundBatchHold && n > 1:
			n /= 2
		}
	}
	return existed, nil
}
