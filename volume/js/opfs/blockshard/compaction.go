//go:build js

package blockshard

import (
	"bytes"
	"time"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

// DefaultL0Trigger is the L0 segment count threshold before compaction.
const DefaultL0Trigger = 4

// DefaultRetireGracePeriod is the minimum wall-clock delay before reclaiming
// retired segments.
const DefaultRetireGracePeriod = 250 * time.Millisecond

// CompactionPlan describes a set of segments to compact.
type CompactionPlan struct {
	ShardID   int
	InputSegs []SegmentMeta
	// Generation is the manifest generation at plan time.
	Generation uint64
}

// PlanCompaction identifies L0 segments exceeding the trigger threshold.
// Reads manifest outside the publish lock (snapshot-based).
func PlanCompaction(shard *Shard, trigger int) *CompactionPlan {
	if trigger < 2 {
		trigger = DefaultL0Trigger
	}

	m := shard.Manifest()
	var l0 []SegmentMeta
	for _, seg := range m.Segments {
		if seg.Level == 0 {
			l0 = append(l0, seg)
		}
	}

	if len(l0) < trigger {
		return nil
	}

	return &CompactionPlan{
		ShardID:    shard.ID(),
		InputSegs:  l0,
		Generation: m.Generation,
	}
}

// ExecuteCompaction runs compaction for a plan. Caller must hold the publish lock.
func ExecuteCompaction(shard *Shard, plan *CompactionPlan) error {
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

	// Build output SSTable.
	w := segment.NewWriter()
	w.SetBloomFPR(shard.bloomFPR)
	for i := range merged {
		if merged[i].Tombstone {
			w.AddTombstone(merged[i].Key)
		} else {
			w.Add(merged[i].Key, merged[i].Value)
		}
	}

	var outBuf bytes.Buffer
	written, err := w.Build(&outBuf)
	if err != nil {
		return errors.Wrap(err, "build compacted segment")
	}

	outData := outBuf.Bytes()

	// Derive metadata directly from the merged entries (no re-parse needed).
	outMeta := SegmentMeta{
		EntryCount: uint32(len(merged)),
		Size:       uint32(written),
		Level:      1,
		MinKey:     merged[0].Key,
		MaxKey:     merged[len(merged)-1].Key,
	}

	// Allocate sequence number and filename.
	shard.mu.Lock()
	shard.seqNum++
	seq := shard.seqNum
	gen := shard.manifest.Generation + 1
	shard.mu.Unlock()

	filename := "seg-" + zeroPad(seq, 6) + ".sst"
	outMeta.Filename = filename

	// Write output segment.
	if err := shard.writeFileData(filename, outData); err != nil {
		return errors.Wrap(err, "write compacted segment")
	}

	// Build new manifest: remove inputs, add L1 output.
	shard.mu.Lock()
	newManifest, err := buildCompactedManifest(
		shard.manifest,
		inputNames,
		outMeta,
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
	return nil
}

// DeleteOldSegments removes input segment files after compaction.
// Should be called after a grace period. Caller must hold publish lock.
func DeleteOldSegments(shard *Shard, filenames []string) {
	for _, name := range filenames {
		opfs.DeleteFile(shard.dir, name)
	}
}
