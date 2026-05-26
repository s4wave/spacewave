//go:build js

package blockshard

import (
	"bytes"
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

	// Read input segments into memory.
	readers := make([]*segment.Reader, len(plan.InputSegs))
	for i, meta := range plan.InputSegs {
		data := readFileBytes(shard.dir, meta.Filename)
		if data == nil {
			return errors.Errorf("read input segment %s: not found", meta.Filename)
		}
		rd, err := segment.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return errors.Errorf("parse input segment %s: %v", meta.Filename, err)
		}
		readers[i] = rd
	}

	// K-way merge.
	merged, err := MergeSegments(readers)
	if err != nil {
		return errors.Wrap(err, "merge segments")
	}
	if len(merged) == 0 {
		return nil
	}
	merged = pruneCompactedTombstones(merged, m, inputNames)

	// Build output SSTables.
	var outputs []writtenSegment
	if len(merged) != 0 {
		groups := splitSegmentEntries(merged, shard.maxSegmentDataBytes)
		outputs = make([]writtenSegment, 0, len(groups))
		for i := range groups {
			output, err := shard.writeSegment(context.Background(), groups[i], plan.OutputLevel)
			if err != nil {
				return errors.Wrap(err, "write compacted segment")
			}
			outputs = append(outputs, output)
		}
	}
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

// DeleteOldSegments removes input segment files after compaction.
// Should be called after a grace period. Caller must hold publish lock.
func DeleteOldSegments(shard *Shard, filenames []string) {
	for _, name := range filenames {
		opfs.DeleteFile(shard.dir, name)
	}
}
