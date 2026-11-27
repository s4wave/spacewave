//go:build !js

package restore

import (
	"context"
	stderrors "errors"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/block/packcopy"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// volumeRefillOwner labels leased fill roots when no Space bucket requested the fill.
const volumeRefillOwner = "volume-refill"

// refillOutcome names what happened to one input ref.
type refillOutcome uint8

const (
	// refillAlready means the target already held the block's bytes.
	refillAlready refillOutcome = iota
	// refillMissing means the source lacked the block's bytes.
	refillMissing
	// refillMismatch means the source bytes did not hash to the ref.
	refillMismatch
	// refillRestored means the fill made the block available.
	refillRestored
	// refillParked means a parked block was restored under a leased root.
	refillParked
	// refillDropped means the records did not accept the copied block.
	refillDropped
	// refillOutcomeCount sizes the outcome counters.
	refillOutcomeCount
)

// RefillResult counts the outcomes of the input refs and the bytes restored.
type RefillResult struct {
	// Counts holds already stored, missing, mismatched, restored, parked and dropped ref counts, in that order.
	Counts [refillOutcomeCount]uint64
	// Bytes is the total size of the restored blocks.
	Bytes uint64
}

// Refill copies verified source blocks through the stopped target's fill path.
// dryRun classifies without writing. lacks receives refs that could not be filled.
// The result includes completed work if an error occurs.
func Refill(ctx context.Context, target *volume_s4db.Volume, source packcopy.Source, input io.Reader, dryRun bool, lacks io.Writer) (RefillResult, error) {
	r := volumeRefill{target: target, source: source, dryRun: dryRun, lacks: lacks}
	err := r.run(ctx, input)
	return r.result, err
}

// volumeRefill copies verified source blocks through a stopped target's fill path.
type volumeRefill struct {
	// target is the stopped volume receiving fills.
	target *volume_s4db.Volume
	// source reads unverified block bytes from a stopped copy or a directory.
	source packcopy.Source
	// dryRun previews the results using the target's current records.
	dryRun bool
	// lacks receives the unfilled refs in input order.
	lacks io.Writer
	// result accumulates the counts of the processed refs.
	result RefillResult
}

// run reads at most one group of refs ahead of the target's existence check.
func (r *volumeRefill) run(ctx context.Context, input io.Reader) error {
	return StreamRefs(ctx, input, volume_kvtx.MaxFillBatch, func(refs []*block.BlockRef) error {
		return r.refillBatch(ctx, refs)
	})
}

// refillBatch probes the target once, then verifies and fills the missing bytes.
func (r *volumeRefill) refillBatch(ctx context.Context, refs []*block.BlockRef) error {
	// Check for bytes in the physical store, including those of parked blocks.
	present, err := r.target.HasBlockBytesBatch(ctx, refs)
	if err != nil {
		return errors.Wrap(err, "check target blocks")
	}

	// Retain the outcomes for ordered reporting, and buffer only one fill at a time.
	outcomes := make([]refillOutcome, len(refs))
	entries := make([]*block.PutBatchEntry, 0, len(refs))
	indexes := make([]int, 0, len(refs))
	var size int
	for i, ref := range refs {
		// Leave already stored blocks alone, even if their source bytes are corrupt.
		if present[i] {
			continue
		}

		// Read and verify the physical block without opening the source as a Volume.
		stored, err := r.source.GetStoredBlock(ctx, ref)
		if err != nil {
			return errors.Wrapf(err, "read source block %s", ref.MarshalString())
		}
		if stored == nil {
			outcomes[i] = refillMissing
			continue
		}
		data := stored.Data
		if err := ref.VerifyData(data, false); err != nil {
			outcomes[i] = refillMismatch
			continue
		}

		// Bound the transaction like the serve queue, allowing one larger block alone.
		if len(entries) != 0 && size+len(data) > volume_kvtx.MaxFillBatchBytes {
			if err := r.fill(ctx, entries, indexes, outcomes); err != nil {
				return err
			}
			clear(entries)
			entries, indexes, size = entries[:0], indexes[:0], 0
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, CacheFill: true})
		indexes = append(indexes, i)
		size += len(data)
	}

	// Finish the fills before counting and listing outcomes in input order.
	if len(entries) != 0 {
		if err := r.fill(ctx, entries, indexes, outcomes); err != nil {
			return err
		}
	}
	for i, outcome := range outcomes {
		r.result.Counts[outcome]++
		if why := outcome.reason(); why != "" {
			if _, err := io.WriteString(r.lacks, refs[i].MarshalString()+"\t"+why+"\n"); err != nil {
				return errors.Wrap(err, "write lacks output")
			}
		}
	}
	return nil
}

// fill classifies a batch and, in a writing run, applies the existing fill path.
func (r *volumeRefill) fill(ctx context.Context, entries []*block.PutBatchEntry, indexes []int, outcomes []refillOutcome) error {
	// Translate the Volume's fill fates into the command's report rows.
	fates, err := r.target.FillFates(ctx, entries)
	if err != nil {
		return errors.Wrap(err, "classify target fills")
	}
	for i, fate := range fates {
		switch fate {
		case block_gc.FillStoreBytes:
			outcomes[indexes[i]] = refillRestored
		case block_gc.FillLand:
			outcomes[indexes[i]] = refillParked
		case block_gc.FillDropHole, block_gc.FillDropBlock:
			outcomes[indexes[i]] = refillDropped
		}
	}

	// Let the write determine which entries stored bytes.
	if !r.dryRun {
		if err := r.writeFills(ctx, entries, indexes, outcomes); err != nil {
			return err
		}
	}

	// Count bytes restored with or without a hold.
	for i, entry := range entries {
		switch outcomes[indexes[i]] {
		case refillRestored, refillParked:
			r.result.Bytes += uint64(len(entry.Data))
		}
	}
	return nil
}

// writeFills applies a batch and reconciles its drop and existence results with the classification.
func (r *volumeRefill) writeFills(ctx context.Context, entries []*block.PutBatchEntry, indexes []int, outcomes []refillOutcome) error {
	// Every entry's bytes hashed to its ref here. A directory source is only
	// read. An s4db source has one read-only open, and only the engine's
	// reader-slot bookkeeping writes there.
	// The command supplies no child refs, adds no lists or holders itself, and
	// neither deletes nor rewrites records. The fill path writes only the bytes
	// of listed blocks. The fill path can also Land a parked block with its
	// kept list and a leased root.
	existed, err := r.target.PrepareOwnedBlockBatch(ctx, volumeRefillOwner, entries)
	var dropped []int
	if err != nil {
		drops, ok := stderrors.AsType[*block.FillsDroppedError](err)
		if !ok {
			return errors.Wrap(err, "refill target blocks")
		}
		dropped = drops.At
	}

	// The path returns drops and pre-existing bytes. Successful new fills keep
	// the hold classification from the snapshot of this stopped Volume.
	for i := range entries {
		switch {
		case len(dropped) != 0 && dropped[0] == i:
			outcomes[indexes[i]] = refillDropped
			dropped = dropped[1:]
		case existed[i]:
			outcomes[indexes[i]] = refillAlready
		}
	}
	return nil
}

// reason returns the explanation for an unfilled ref, or empty for a stored block.
func (o refillOutcome) reason() string {
	switch o {
	case refillMissing:
		return MissingReason
	case refillMismatch:
		return MismatchReason
	case refillDropped:
		return "records do not accept a copied fill"
	default:
		return ""
	}
}
