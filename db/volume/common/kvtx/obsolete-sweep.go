package kvtx

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// legacyGraphKinds are the record kinds of the reference graph of store
// formats 3 to 6 under the liveness prefix that format 7 does not read: every
// kind but the child list, which format 7 keeps. Format 7's own kinds are
// upper case, and the lower case kinds a later format adds avoid these, so a
// record of one is never read as the other.
var legacyGraphKinds = []byte{'a', 'f', 'g', 'h', 'i', 'l', 'm', 'n', 'p', 'r', 's', 'w', 'y'}

// obsoleteSweepKeys bounds the keys one obsolete sweep transaction deletes.
// A transaction of 256 deletes holds the writer about a millisecond on s4db,
// at about 3 us a key, so a commit that arrives during one waits at most
// that long. The background turn lets every waiting commit in before the
// next transaction.
const obsoleteSweepKeys = 256

// runObsoleteSweep deletes the records of each kind the state records as
// obsolete, left by a format upgrade, a bounded batch per transaction,
// letting a waiting writer in between, and drops each kind from the state
// once none of its records remain. It deletes only those kinds under the
// liveness prefix: no format the Volume runs reads those kinds. It never
// deletes a child list, a block or another record. A crash leaves the rest for
// the next open.
func (v *Volume) runObsoleteSweep(ctx context.Context, kinds []byte) {
	// Delete each obsolete kind under the liveness prefix, as background
	// work, then drop it from the state.
	defer close(v.obsoleteDone)
	ctx = block.WithBackground(ctx)
	for _, kind := range kinds {
		prefix := append(bytes.Clone(livenessPrefix), kind)
		err := v.sweepObsoletePrefix(ctx, prefix)
		if err == nil {
			err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
				// Drop the kind from the state's obsolete kinds.
				state, err := p.rec.GetState(ctx)
				if err != nil {
					return err
				}
				i := bytes.IndexByte(state.GetObsoleteKinds(), kind)
				if i < 0 {
					return nil
				}
				state.ObsoleteKinds = slices.Delete(bytes.Clone(state.GetObsoleteKinds()), i, i+1)
				return p.rec.SetState(ctx, state)
			})
		}
		if err != nil {
			if ctx.Err() == nil && v.le != nil {
				v.le.WithError(err).Warn("deleting obsolete liveness records failed; the next open resumes")
			}
			return
		}
	}
}

// sweepObsoletePrefix deletes every key under prefix, obsoleteSweepKeys per
// transaction.
func (v *Volume) sweepObsoletePrefix(ctx context.Context, prefix []byte) error {
	var after []byte
	for {
		// Delete the next batch, from the last key the batch before reached.
		var deleted int
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			// Delete through a view of the transaction that records its
			// writes, so the transaction commits them.
			vtx, err := p.store.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			deleted, after, err = deletePrefixBatch(ctx, vtx, prefix, after, obsoleteSweepKeys)
			return err
		})
		if err != nil || deleted < obsoleteSweepKeys {
			return err
		}
		if err := context.Cause(ctx); err != nil {
			return err
		}
	}
}

// deletePrefixBatch deletes up to limit keys under prefix after the key
// after, nil for the first batch, in tx, and returns how many keys it deleted
// and the last key.
func deletePrefixBatch(ctx context.Context, tx kvtx.Tx, prefix, after []byte, limit int) (int, []byte, error) {
	// Collect the next keys.
	it := tx.Iterate(ctx, prefix, true, false)
	if err := seekPastKey(it, after); err != nil {
		it.Close()
		return 0, nil, err
	}
	var keys [][]byte
	for ; it.Valid() && len(keys) < limit; it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	err := it.Err()
	it.Close()
	if err != nil || len(keys) == 0 {
		return 0, nil, err
	}

	// Delete them.
	for _, key := range keys {
		if err := tx.Delete(ctx, key); err != nil {
			return 0, nil, err
		}
	}
	return len(keys), keys[len(keys)-1], nil
}
