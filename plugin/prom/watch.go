package prom_plugin

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/world"
)

// watchWorld calls read in a read transaction on eng now and after each
// commit to the World, at most once per gap, until read or ctx fails. A
// commit that lands during read causes one more read.
func watchWorld(ctx context.Context, eng world.Engine, gap time.Duration, read func(context.Context, world.WorldState) error) error {
	for {
		// Read the World as of its current seqno.
		seqno, err := eng.GetSeqno(ctx)
		if err != nil {
			return err
		}
		if err := world.ExecTransaction(ctx, eng, false, read); err != nil {
			return err
		}

		// Wait for the next commit, then for the rest of the gap.
		next := time.Now().Add(gap)
		if _, err := eng.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
		if wait := time.Until(next); wait > 0 {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			//gostyle:timer deadline
			case <-time.After(wait):
			}
		}
	}
}
