package packsweep

import (
	"context"
	"encoding/binary"

	"github.com/s4wave/spacewave/db/kvtx"
)

// placeBatch is the placement rows one transaction writes.
const placeBatch = 8192

// compactPack is a new pack a compaction wrote, recorded before its upload.
type compactPack struct {
	// id is the pack's random id.
	id string
	// adopted is set once the backend adopted it and its keys are placed.
	adopted bool
	// victims are the packs its blocks came from.
	victims []string
}

// marshal encodes the row.
func (p *compactPack) marshal() []byte {
	// Write the state, then the victims.
	b := []byte{0}
	if p.adopted {
		b[0] = 1
	}
	b = binary.AppendUvarint(b, uint64(len(p.victims)))
	for _, v := range p.victims {
		b = appendString(b, v)
	}
	return b
}

// unmarshalCompactPack decodes a row.
func unmarshalCompactPack(id string, b []byte) (*compactPack, error) {
	// Read the state, then the victims.
	r := &rowReader{b: b, ok: true}
	p := &compactPack{id: id, adopted: r.byte() == 1}
	n := r.uvarint()
	for i := uint64(0); i < n && r.ok; i++ {
		p.victims = append(p.victims, r.string())
	}
	return p, r.finish("compacted pack row")
}

// readCompactPacks reads the new packs' rows, in id order.
func readCompactPacks(ctx context.Context, store kvtx.Store) ([]*compactPack, error) {
	// Open a read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Decode each row.
	var out []*compactPack
	err = tx.ScanPrefix(ctx, []byte(compactPackPrefix), func(key, value []byte) error {
		p, err := unmarshalCompactPack(string(key[len(compactPackPrefix):]), value)
		if err == nil {
			out = append(out, p)
		}
		return err
	})
	return out, err
}

// CompactPacks counts the packs the compactions wrote: "adopted" once the
// backend adopted them and their keys are placed, "pending" while a killed run
// left them for the next to finish or drop.
func CompactPacks(ctx context.Context, store kvtx.Store) (map[string]uint64, error) {
	// Read the rows.
	rows, err := readCompactPacks(ctx, store)
	if err != nil {
		return nil, err
	}

	// Count them by state.
	counts := make(map[string]uint64)
	for _, p := range rows {
		if p.adopted {
			counts["adopted"]++
		} else {
			counts["pending"]++
		}
	}
	return counts, nil
}

// writeCompactPack writes a new pack's row.
func writeCompactPack(ctx context.Context, store kvtx.Store, p *compactPack) error {
	// Open a write.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write the row.
	if err := tx.Set(ctx, []byte(compactPackPrefix+p.id), p.marshal()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// placePack records that the adopted pack p holds keys: each key's placement
// row names it, a batch per transaction, and the last marks it adopted. A run
// killed before the last finds the pack unadopted and places its keys again.
func placePack(ctx context.Context, store kvtx.Store, p *compactPack, keys [][]byte) error {
	for start := 0; start == 0 || start < len(keys); start += placeBatch {
		// Open the batch's transaction.
		end := min(start+placeBatch, len(keys))
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}

		// Name the pack on each key's row, and mark it adopted with the last.
		for _, key := range keys[start:end] {
			if err == nil {
				err = tx.Set(ctx, prefixed(placedPrefix, Tail(key)), []byte(p.id))
			}
		}
		if err == nil && end == len(keys) {
			p.adopted = true
			err = tx.Set(ctx, []byte(compactPackPrefix+p.id), p.marshal())
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		tx.Discard()
		if err != nil {
			return err
		}
	}
	return nil
}

// placement reports whether a key is placed in a pack a compaction adopted
// that the sweep has not held since: the backend's index then reads the key
// from that registered pack, so another pack holding it may go.
type placement struct {
	// tx is the scratch read.
	tx kvtx.TxOps
	// packs caches whether each named pack places its keys.
	packs map[string]bool
}

// newPlacement checks placements in tx.
func newPlacement(tx kvtx.TxOps) *placement {
	return &placement{tx: tx, packs: make(map[string]bool)}
}

// placed reports whether the key of tail is placed.
func (p *placement) placed(ctx context.Context, tail []byte) (bool, error) {
	// Read the key's row.
	id, found, err := p.tx.Get(ctx, prefixed(placedPrefix, tail))
	if err != nil || !found {
		return false, err
	}

	// Check the pack it names.
	if ok, seen := p.packs[string(id)]; seen {
		return ok, nil
	}
	ok, err := p.places(ctx, string(id))
	if err != nil {
		return false, err
	}
	p.packs[string(id)] = ok
	return ok, nil
}

// places reports whether pack id is adopted and holds no record, so the
// backend reads its keys.
func (p *placement) places(ctx context.Context, id string) (bool, error) {
	// Read its row.
	data, found, err := p.tx.Get(ctx, []byte(compactPackPrefix+id))
	if err != nil || !found {
		return false, err
	}
	row, err := unmarshalCompactPack(id, data)
	if err != nil || !row.adopted {
		return false, err
	}

	// A pack the sweep holds places nothing.
	held, err := p.tx.Exists(ctx, []byte(packRecordPrefix+id))
	return !held, err
}
