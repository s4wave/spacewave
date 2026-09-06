//go:build js

package blockshard

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// DefaultRetireGracePeriod is the minimum wall-clock delay before reclaiming
// retired segments.
const DefaultRetireGracePeriod = 250 * time.Millisecond

// PlanCompaction identifies a level whose segments exceed the trigger threshold.
// Reads manifest outside the publish lock (snapshot-based).
func PlanCompaction(shard *Shard, trigger int) *CompactionPlan {
	return buildCompactionPlanWithLimit(shard.ID(), shard.manifestSnapshot(), trigger, shard.maxSegmentDataBytes)
}

// ExecuteCompaction runs compaction for a plan. Caller must hold the publish lock.
func ExecuteCompaction(ctx context.Context, shard *Shard, plan *CompactionPlan) error {
	// Refresh the generation under the caller-owned publication lock.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := shard.reloadManifestFromDisk(ctx); err != nil {
		return errors.Wrap(err, "reload manifest")
	}
	m := shard.manifestSnapshot()
	inputNames := make(map[string]bool, len(plan.InputSegs))
	for _, seg := range plan.InputSegs {
		inputNames[seg.Filename] = true
	}
	if err := verifyCompactionInputs(m, inputNames); err != nil {
		return err
	}

	// Pin every immutable input until the merge finishes.
	iters := make([]*segment.EntryIterator, len(plan.InputSegs))
	leases := make([]*segmentCacheLease, 0, len(plan.InputSegs))
	defer func() {
		for _, lease := range leases {
			lease.Release()
		}
	}()
	for i, meta := range plan.InputSegs {
		if err := ctx.Err(); err != nil {
			return err
		}
		lease, err := shard.acquireSegment(ctx, &meta)
		if err != nil {
			return errors.Errorf("acquire input segment %s: %v", meta.Filename, err)
		}
		leases = append(leases, lease)
		size := int64(meta.Size)
		if size == 0 {
			size, err = lease.Size()
			if err != nil {
				return errors.Errorf("get input segment %s size: %v", meta.Filename, err)
			}
		}
		if err := segment.VerifyChecksum(lease, size); err != nil {
			return errors.Errorf("verify input segment %s: %v", meta.Filename, err)
		}
		iters[i] = segment.NewEntryIterator(lease, lease.lookup)
	}

	// Stream the merged entries into bounded output segments.
	writer := compactionOutputWriter{
		ctx:          ctx,
		shard:        shard,
		outputLevel:  plan.OutputLevel,
		maxDataBytes: shard.maxSegmentDataBytes,
	}
	cleanupOutputs := func(err error) error {
		err = opfs.WithQuotaEstimate(err, 0)
		if cleanupErr := shard.cleanupWrittenSegments(writer.Outputs()); cleanupErr != nil {
			return errors.Wrapf(err, "clean failed compaction segments: %v", cleanupErr)
		}
		return err
	}
	if err := mergeSegmentIterators(iters, func(entry segment.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Tombstone && !manifestMayContainKeyOutsideInputs(m, inputNames, entry.Key) {
			return nil
		}
		if err := writer.Add(entry); err != nil {
			return errors.Wrap(err, "write compacted segment")
		}
		return nil
	}); err != nil {
		return cleanupOutputs(errors.Wrap(err, "merge segments"))
	}
	if err := writer.Flush(); err != nil {
		return cleanupOutputs(errors.Wrap(err, "write compacted segment"))
	}

	// Collect output metadata for the next manifest generation.
	outputs := writer.Outputs()
	outMetas := make([]SegmentMeta, len(outputs))
	for i := range outputs {
		outMetas[i] = outputs[i].Meta
	}

	// Build new manifest: remove inputs and add a compacted output when any
	// live entries or retained tombstones remain.
	shard.mu.Lock()
	gen := shard.manifest.Generation + 1
	newManifest, err := buildCompactedManifest(
		shard.manifest,
		inputNames,
		outMetas,
		gen,
		uint64(shard.nowFn().UnixMilli()),
		uint64(DefaultRetireGracePeriod/time.Millisecond),
	)
	shard.mu.Unlock()
	if err != nil {
		return cleanupOutputs(err)
	}

	// Publish the replacement generation after every output is complete.
	if err := shard.writeManifest(ctx, newManifest); err != nil {
		return cleanupOutputs(errors.Wrap(err, "write compaction manifest"))
	}
	return nil
}

// compactionOutputWriter builds bounded immutable segments for one merge.
type compactionOutputWriter struct {
	// ctx bounds the current compaction operation.
	ctx context.Context
	// shard owns output file creation.
	shard *Shard
	// outputLevel is the destination compaction level.
	outputLevel uint8
	// maxDataBytes limits the data retained before a flush.
	maxDataBytes int
	// entries holds the current output segment entries.
	entries []segment.Entry
	// dataBytes tracks the encoded data size of entries.
	dataBytes int
	// outputs retains completed files for publication or cleanup.
	outputs []writtenSegment
}

// Add flushes a full output before accepting the next merged entry.
func (w *compactionOutputWriter) Add(entry segment.Entry) error {
	// Keep each output within the configured data limit.
	entrySize := estimateSegmentEntryDataBytes(entry)
	if w.maxDataBytes > 0 && len(w.entries) != 0 && w.dataBytes+entrySize > w.maxDataBytes {
		if err := w.Flush(); err != nil {
			return err
		}
	}

	// Retain the entry until its output segment is written.
	w.entries = append(w.entries, entry)
	w.dataBytes += entrySize
	return nil
}

// Flush writes the current output and releases buffered entry references.
func (w *compactionOutputWriter) Flush() error {
	// Skip empty output and honor cancellation before creating a file.
	if len(w.entries) == 0 {
		return nil
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}

	// Keep completed output metadata for publication or cleanup.
	output, err := w.shard.writeSegment(w.ctx, w.entries, w.outputLevel)
	if err != nil {
		return err
	}
	w.outputs = append(w.outputs, output)

	// Release values while reusing the bounded entry buffer.
	for i := range w.entries {
		w.entries[i] = segment.Entry{}
	}
	w.entries = w.entries[:0]
	w.dataBytes = 0
	return nil
}

// Outputs returns completed segments owned by the current compaction.
func (w *compactionOutputWriter) Outputs() []writtenSegment {
	return w.outputs
}

// DeleteOldSegments removes input segment files after compaction.
// Should be called after a grace period. Caller must hold publish lock.
func DeleteOldSegments(shard *Shard, filenames []string) {
	for _, name := range filenames {
		opfs.DeleteFile(shard.dir, name)
	}
}
