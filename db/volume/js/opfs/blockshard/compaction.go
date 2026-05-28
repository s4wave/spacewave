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
	return buildCompactionPlanWithLimit(shard.ID(), shard.Manifest(), trigger, shard.maxSegmentDataBytes)
}

// ExecuteCompaction runs compaction for a plan. Caller must hold the publish lock.
func ExecuteCompaction(shard *Shard, plan *CompactionPlan) error {
	if err := shard.reloadManifestFromDisk(context.Background()); err != nil {
		return errors.Wrap(err, "reload manifest")
	}
	m := shard.Manifest()
	inputNames := make(map[string]bool, len(plan.InputSegs))
	for _, seg := range plan.InputSegs {
		inputNames[seg.Filename] = true
	}
	if err := verifyCompactionInputs(m, inputNames); err != nil {
		return err
	}

	iters := make([]*segment.EntryIterator, len(plan.InputSegs))
	for i, meta := range plan.InputSegs {
		f, err := shard.getSegmentFile(context.Background(), &meta)
		if err != nil {
			return errors.Errorf("open input segment %s: %v", meta.Filename, err)
		}
		size := int64(meta.Size)
		if size == 0 {
			size, err = f.Size()
			if err != nil {
				return errors.Errorf("get input segment %s size: %v", meta.Filename, err)
			}
		}
		if err := segment.VerifyChecksum(f, size); err != nil {
			return errors.Errorf("verify input segment %s: %v", meta.Filename, err)
		}
		lookup, err := loadLookupMeta(context.Background(), f, &meta)
		if err != nil {
			return errors.Errorf("parse input segment %s: %v", meta.Filename, err)
		}
		iters[i] = segment.NewEntryIterator(f, lookup)
	}

	writer := compactionOutputWriter{
		shard:        shard,
		outputLevel:  plan.OutputLevel,
		maxDataBytes: shard.maxSegmentDataBytes,
	}
	if err := mergeSegmentIterators(iters, func(entry segment.Entry) error {
		if entry.Tombstone && !manifestMayContainKeyOutsideInputs(m, inputNames, entry.Key) {
			return nil
		}
		if err := writer.Add(entry); err != nil {
			return errors.Wrap(err, "write compacted segment")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "merge segments")
	}
	if err := writer.Flush(); err != nil {
		return errors.Wrap(err, "write compacted segment")
	}
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
		return err
	}

	if err := shard.writeManifest(newManifest); err != nil {
		return errors.Wrap(err, "write compaction manifest")
	}
	for i := range outputs {
		shard.cacheLookup(outputs[i].Meta.Filename, outputs[i].Lookup)
	}
	return nil
}

type compactionOutputWriter struct {
	shard        *Shard
	outputLevel  uint8
	maxDataBytes int
	entries      []segment.Entry
	dataBytes    int
	outputs      []writtenSegment
}

func (w *compactionOutputWriter) Add(entry segment.Entry) error {
	entrySize := estimateSegmentEntryDataBytes(entry)
	if w.maxDataBytes > 0 && len(w.entries) != 0 && w.dataBytes+entrySize > w.maxDataBytes {
		if err := w.Flush(); err != nil {
			return err
		}
	}
	w.entries = append(w.entries, entry)
	w.dataBytes += entrySize
	return nil
}

func (w *compactionOutputWriter) Flush() error {
	if len(w.entries) == 0 {
		return nil
	}
	output, err := w.shard.writeSegment(context.Background(), w.entries, w.outputLevel)
	if err != nil {
		return err
	}
	w.outputs = append(w.outputs, output)
	for i := range w.entries {
		w.entries[i] = segment.Entry{}
	}
	w.entries = w.entries[:0]
	w.dataBytes = 0
	return nil
}

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
