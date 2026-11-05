package kvtx

import (
	"bytes"
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// legacyGraphKinds are the record kinds of the reference graph of store
// formats 3 to 6 under the liveness prefix that format 7 does not read: every
// kind but the child list, which format 7 keeps. They are lower case and
// format 7's own kinds are upper case, so a record of one is never read as the
// other.
var legacyGraphKinds = []byte{'a', 'f', 'g', 'h', 'i', 'l', 'm', 'n', 'p', 'r', 's', 'w', 'y'}

// legacySweepKeys bounds the keys one legacy sweep transaction deletes.
const legacySweepKeys = 2048

// legacySweepPause is how long the legacy sweep waits after a transaction that
// let a waiting writer in, before it takes the store's writer again.
const legacySweepPause = 50 * time.Millisecond

// runLegacySweep deletes the records of an old reference graph left by the
// format 7 upgrade, a bounded batch per transaction, letting a waiting writer
// in between, and then clears the state's LegacyGraph. It deletes only the
// legacy kinds under the liveness prefix, never a child list, a block or
// another record. A crash leaves the rest for the
// next open.
func (v *Volume) runLegacySweep(ctx context.Context) {
	// Delete the legacy kinds under the liveness prefix, as background work.
	defer close(v.legacyDone)
	ctx = block.WithBackground(ctx)
	for _, kind := range legacyGraphKinds {
		prefix := append(bytes.Clone(livenessPrefix), kind)
		if err := v.sweepLegacyPrefix(ctx, prefix); err != nil {
			if ctx.Err() == nil && v.le != nil {
				v.le.WithError(err).Warn("deleting the legacy reference graph failed; the next open resumes")
			}
			return
		}
	}

	// Record that no legacy record remains.
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		state, err := p.rec.GetState(ctx)
		if err != nil || !state.GetLegacyGraph() {
			return err
		}
		state.LegacyGraph = false
		return p.rec.SetState(ctx, state)
	})
	if err != nil && ctx.Err() == nil && v.le != nil {
		v.le.WithError(err).Warn("recording the deleted legacy reference graph failed")
	}
}

// sweepLegacyPrefix deletes every key under prefix, legacySweepKeys per
// transaction.
func (v *Volume) sweepLegacyPrefix(ctx context.Context, prefix []byte) error {
	for {
		// Delete the next batch.
		var deleted int
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			// Delete through a view of the transaction that records its
			// writes, so the transaction commits them.
			vtx, err := p.store.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			deleted, err = deletePrefixBatch(ctx, vtx, prefix, legacySweepKeys)
			return err
		})
		if err != nil || deleted < legacySweepKeys {
			return err
		}

		// Let a waiting writer in before the next batch.
		if v.writersWaiting() {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-time.After(legacySweepPause):
			}
		}
	}
}

// deletePrefixBatch deletes up to limit keys under prefix in tx and returns
// how many it deleted.
func deletePrefixBatch(ctx context.Context, tx kvtx.Tx, prefix []byte, limit int) (int, error) {
	// Collect the next keys.
	it := tx.Iterate(ctx, prefix, true, false)
	var keys [][]byte
	for it.Next(); it.Valid() && len(keys) < limit; it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	err := it.Err()
	it.Close()
	if err != nil {
		return 0, err
	}

	// Delete them.
	for _, key := range keys {
		if err := tx.Delete(ctx, key); err != nil {
			return 0, err
		}
	}
	return len(keys), nil
}
