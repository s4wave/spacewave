package packsweep

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// DefaultReceiptMinSample is the value sample a hide needs its receipt's
// check to have taken when the trash sets none: one reached block in a
// thousand, which reads a few thousand values in a store of millions.
const DefaultReceiptMinSample = 0.001

// DefaultReceiptMaxAge is how old a receipt a hide accepts when the trash
// sets no bound: the 24 hour quarantine between the check and the trash, with
// two days to spare for a recheck that waits.
const DefaultReceiptMaxAge = 72 * time.Hour

// ReceiptPolicy is what a hide asks of the cycle's receipt.
type ReceiptPolicy struct {
	// MinSample is the least value sample the receipt's check took; zero
	// uses DefaultReceiptMinSample.
	MinSample float64
	// MaxAge is the oldest receipt accepted; zero uses
	// DefaultReceiptMaxAge.
	MaxAge time.Duration
	// Now reads the clock the age is counted on; nil uses time.Now.
	Now func() time.Time
}

// ErrNoReceipt wraps every refusal of a hide for want of a receipt that
// covers its packs.
var ErrNoReceipt = errors.New("no passing check covers the packs to hide")

// checkReceipt refuses unless the cycle's receipt covers every pack of ids:
// written by a check of this cycle's judgment that sampled at least the
// policy's rate, walked fresh roots, is no older than the policy's bound,
// and proved each pack. The trash acts on a subset of the packs the check
// proved, since the recheck only lifts; removal is monotone, so a proof that
// removing a set loses nothing holds for every subset of it.
func checkReceipt(ctx context.Context, store kvtx.Store, policy ReceiptPolicy, ids []string) error {
	// Read the head and the receipt in one read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	head, err := readHead(ctx, tx)
	if err != nil {
		return err
	}
	rec, found, err := readReceipt(ctx, tx)
	if err != nil {
		return err
	}

	// Refuse a receipt that is missing, of another judgment, or too weak.
	if err := policy.refuse(head, rec, found); err != nil {
		return errors.Wrap(ErrNoReceipt, err.Error())
	}

	// Refuse a pack the check did not prove.
	for _, id := range ids {
		ok, err := tx.Exists(ctx, []byte(receiptPackPrefix+id))
		if err != nil {
			return err
		}
		if !ok {
			return errors.Wrapf(ErrNoReceipt, "pack %s is not in the receipt of %d packs", id, rec.Packs)
		}
	}
	return nil
}

// refuse returns why the receipt cannot cover a hide of the cycle head, nil
// when it can.
func (p ReceiptPolicy) refuse(head *Head, rec *Receipt, found bool) error {
	// Apply the defaults.
	minSample, maxAge, now := p.MinSample, p.MaxAge, p.Now
	if minSample <= 0 {
		minSample = DefaultReceiptMinSample
	}
	if maxAge <= 0 {
		maxAge = DefaultReceiptMaxAge
	}
	if now == nil {
		now = time.Now
	}

	// Check the receipt against the head and the bounds.
	if !found {
		return errors.New("no check passed in this cycle")
	}
	switch age := now().Sub(rec.CheckedAt); {
	case rec.Cycle != head.Cycle || !rec.JudgedAt.Equal(head.JudgedAt):
		return errors.Errorf("the receipt is of cycle %d judged at %s, not the cycle's judgment", rec.Cycle, rec.JudgedAt.Format(time.RFC3339))
	case rec.CycleRootsOnly:
		return errors.New("the check walked the cycle's roots alone")
	case rec.Sample < minSample:
		return errors.Errorf("the check sampled %g of the blocks, under %g", rec.Sample, minSample)
	case age > maxAge || age < 0:
		return errors.Errorf("the check ran %s ago, outside %s", age.Round(time.Second), maxAge)
	}
	return nil
}
