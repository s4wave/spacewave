package block_gc

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ErrNotHole is returned when a write-off names a block with no hole
// record: one the Volume stores or lists, or one no audit found lost.
var ErrNotHole = errors.New("the block has no recorded hole")

// WriteOffResult reports what WriteOffHoles did.
type WriteOffResult struct {
	// WrittenOff counts the holes now written off, those an earlier call
	// wrote off among them.
	WrittenOff uint64
	// Ended counts the named holes whose record left while the call ran,
	// by a put, a list rebuild or an adoption that found no holder, which
	// need no write-off.
	Ended uint64
}

// WriteOffHoles writes off the recorded hole of each block in refs: a block
// the store lost with its child list, which a live list still names and
// which the operator accepts no read will resolve. A written-off hole no
// longer holds the flip, as Records.writeOffHole describes with what the
// write-off gives up. It changes no count and no list, and a later put of
// the block from its content still ends the hole and holds its children.
//
// It writes nothing when a ref has no hole record, and returns ErrNotHole
// naming the first such ref, so a mistyped list writes off nothing. Only
// an audit records a hole, so every ref named here is one an audit found
// absent under a live root. Any other error returns a zero result and may
// leave the first slices written off; a second call writes off the rest.
//
// Cost: one read snapshot of a point read per ref, then one transaction
// per sliceRecords refs, each of three point reads and three writes per
// ref. An operator runs it, on a list a person read.
func WriteOffHoles(ctx context.Context, store Store, refs []*block.BlockRef) (WriteOffResult, error) {
	// Require a hole record for every ref before writing any.
	err := store.View(ctx, func(rec *Records) error {
		for _, ref := range refs {
			hole, err := rec.tx.Exists(ctx, rec.key(kindHole, BlockKey(ref)))
			if err != nil {
				return err
			}
			if !hole {
				return errors.Wrap(ErrNotHole, ref.MarshalString())
			}
		}
		return nil
	})
	if err != nil {
		return WriteOffResult{}, err
	}

	// Write them off in slices. The position carries the counts, so a
	// transaction the store runs again counts once.
	type position struct {
		next int
		res  WriteOffResult
	}
	var done position
	_, err = eachSlice(ctx, store.Update, position{}, nil, func(rec *Records, _ block.StoreOps, from position) (position, bool, error) {
		end := min(from.next+sliceRecords, len(refs))
		for _, ref := range refs[from.next:end] {
			hole, err := rec.writeOffHole(ctx, BlockKey(ref))
			if err != nil {
				return from, false, err
			}
			if hole {
				from.res.WrittenOff++
			} else {
				from.res.Ended++
			}
		}
		from.next, done = end, position{next: end, res: from.res}
		return from, end != len(refs), nil
	})
	if err != nil {
		return WriteOffResult{}, err
	}
	return done.res, nil
}

// ScanHoles calls fn with the block of each recorded hole and whether an
// operator wrote it off, in key order, until fn returns an error. An
// operator's command lists the holes with it, to read which blocks a
// write-off may name and which holes still hold the flip.
//
// Cost: one read snapshot holding a prefix scan of the hole records with one
// value read each. It keeps nothing, so the caller streams the holes out.
func ScanHoles(ctx context.Context, store Store, fn func(ref *block.BlockRef, writtenOff bool) error) error {
	return store.View(ctx, func(rec *Records) error {
		return rec.scan(ctx, rec.key(kindHole), nil, func(key []byte, value func() ([]byte, error)) (bool, error) {
			// Parse the hole's block and read whether it is written off.
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return false, err
			}
			v, err := value()
			if err != nil {
				return false, err
			}
			return true, fn(ref, bytes.Equal(v, holeWrittenOff))
		})
	})
}
