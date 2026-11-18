package packsweep

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
	"slices"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// receiptVersion is the encoding of the receipt row.
const receiptVersion = 1

// Receipt is the record a check writes into the cycle's scratch store when it
// passes: the cycle and judgment it proved, the digests of the packs it proved
// removable and the roots it walked, its value sample, and when it ran. Each
// pack it proved has a row of its own, which the trash reads for every pack it
// hides. Only Check writes one, and a new cycle drops it with the cycle's
// other rows.
type Receipt struct {
	// Cycle is the cycle the check read.
	Cycle int
	// JudgedAt is the time of the judgment the check read.
	JudgedAt time.Time
	// CheckedAt is when the check finished, on the command's clock.
	CheckedAt time.Time
	// Packs is the count of packs proved.
	Packs uint64
	// PacksDigest is the SHA-256 of the proved packs' sorted ids, each
	// followed by a newline, in hex.
	PacksDigest string
	// Roots is the count of roots walked.
	Roots uint64
	// RootsDigest is the SHA-256 of the walked roots' sorted b58 refs, each
	// followed by a newline, in hex.
	RootsDigest string
	// Sample is the fraction of the other reached blocks the check read by
	// value.
	Sample float64
	// CycleRootsOnly is set when the check walked the cycle's roots alone,
	// with no fresh root read when it ran.
	CycleRootsOnly bool
}

// ReadReceipt reads the cycle's receipt; found is false when no check passed
// since the cycle began.
func ReadReceipt(ctx context.Context, store kvtx.Store) (rec *Receipt, found bool, err error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()
	return readReceipt(ctx, tx)
}

// readReceipt reads the receipt row in tx.
func readReceipt(ctx context.Context, tx kvtx.TxOps) (*Receipt, bool, error) {
	// Read the row.
	data, found, err := tx.Get(ctx, []byte(receiptKey))
	if err != nil || !found {
		return nil, false, err
	}

	// Check its version.
	if len(data) == 0 || data[0] != receiptVersion {
		return nil, false, errors.New("unknown receipt version")
	}

	// Decode its fields.
	r := &rowReader{b: data[1:], ok: true}
	rec := &Receipt{Cycle: int(r.uvarint())} //nolint:gosec // written from an int.
	rec.JudgedAt, rec.CheckedAt = r.time(), r.time()
	rec.Packs, rec.PacksDigest = r.uvarint(), r.string()
	rec.Roots, rec.RootsDigest = r.uvarint(), r.string()
	rec.Sample = math.Float64frombits(r.uvarint())
	rec.CycleRootsOnly = r.byte() != 0
	return rec, true, r.finish("receipt")
}

// marshal encodes the receipt row.
func (r *Receipt) marshal() []byte {
	// Append each field in order, the flag last.
	b := binary.AppendUvarint([]byte{receiptVersion}, uint64(r.Cycle)) //nolint:gosec // a cycle is never negative.
	b = appendTime(appendTime(b, r.JudgedAt), r.CheckedAt)
	b = appendString(binary.AppendUvarint(b, r.Packs), r.PacksDigest)
	b = appendString(binary.AppendUvarint(b, r.Roots), r.RootsDigest)
	b = binary.AppendUvarint(b, math.Float64bits(r.Sample))
	return append(b, headFlag(r.CycleRootsOnly, 1))
}

// digestLines returns the count and the hex SHA-256 of the values, sorted,
// each followed by a newline.
func digestLines(values []string) (uint64, string) {
	d := newLineDigest()
	for _, v := range slices.Sorted(slices.Values(values)) {
		d.add(v)
	}
	return d.sum()
}

// lineDigest hashes values given in sorted order, each followed by a
// newline, and counts them.
type lineDigest struct {
	h hash.Hash
	n uint64
}

// newLineDigest returns an empty digest.
func newLineDigest() *lineDigest {
	return &lineDigest{h: sha256.New()}
}

// add hashes the next value.
func (d *lineDigest) add(v string) {
	_, _ = d.h.Write([]byte(v + "\n"))
	d.n++
}

// sum returns the count and the hex digest.
func (d *lineDigest) sum() (uint64, string) {
	return d.n, hex.EncodeToString(d.h.Sum(nil))
}

// JSON describes the receipt.
func (r *Receipt) JSON(a *fastjson.Arena) *fastjson.Value {
	only := a.NewFalse()
	if r.CycleRootsOnly {
		only = a.NewTrue()
	}
	return jsonObject(a,
		jsonField{"cycle", a.NewNumberInt(r.Cycle)},
		jsonField{"judged_at", a.NewString(r.JudgedAt.UTC().Format(time.RFC3339Nano))},
		jsonField{"checked_at", a.NewString(r.CheckedAt.UTC().Format(time.RFC3339Nano))},
		jsonField{"packs", jsonUint(a, r.Packs)},
		jsonField{"packs_digest", a.NewString(r.PacksDigest)},
		jsonField{"roots", jsonUint(a, r.Roots)},
		jsonField{"roots_digest", a.NewString(r.RootsDigest)},
		jsonField{"sample", a.NewNumberFloat64(r.Sample)},
		jsonField{"cycle_roots_only", only},
	)
}

// rootRefs returns the b58 refs of the roots that name a block.
func rootRefs(roots []block.NamedRoot) []string {
	var refs []string
	for _, root := range roots {
		if !root.Ref.GetEmpty() {
			refs = append(refs, root.Ref.MarshalString())
		}
	}
	return refs
}

// writeReceipt replaces the cycle's receipt with rec, for the packs ids. It
// drops the old receipt's row first and writes the new one last, so a write
// killed midway leaves no receipt rather than one over the wrong packs. It
// refuses when the cycle or judgment moved on since the check read them.
func writeReceipt(ctx context.Context, store kvtx.Store, rec *Receipt, ids []string) error {
	// Drop the old receipt, then its pack rows.
	if err := deleteKey(ctx, store, []byte(receiptKey)); err != nil {
		return err
	}
	if err := deletePrefix(ctx, store, []byte(receiptPackPrefix)); err != nil {
		return err
	}

	// Write the pack rows a batch at a time.
	for batch := range slices.Chunk(ids, deleteBatch) {
		if err := setKeys(ctx, store, receiptPackPrefix, batch); err != nil {
			return err
		}
	}

	// Write the receipt in the transaction that confirms the head it proved.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	head, err := readHead(ctx, tx)
	if err != nil {
		return err
	}
	if head.Cycle != rec.Cycle || !head.JudgedAt.Equal(rec.JudgedAt) {
		return errors.Errorf("the cycle moved on to %d judged at %s during the check", head.Cycle, head.JudgedAt.Format(time.RFC3339))
	}
	if err := tx.Set(ctx, []byte(receiptKey), rec.marshal()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// deleteKey deletes one key in its own transaction.
func deleteKey(ctx context.Context, store kvtx.Store, key []byte) error {
	// Delete and commit.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// setKeys sets prefix followed by each id to an empty value in one
// transaction.
func setKeys(ctx context.Context, store kvtx.Store, prefix string, ids []string) error {
	// Open a write, set each key, and commit.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, id := range ids {
		if err := tx.Set(ctx, []byte(prefix+id), nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
