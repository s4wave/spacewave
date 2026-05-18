package blockshard

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// DefaultL0Trigger is the L0 segment count threshold before compaction.
const DefaultL0Trigger = 4

const maxCompactionLevel = 2

// CompactionPlan describes a set of segments to compact.
type CompactionPlan struct {
	ShardID int
	// InputSegs are ordered by manifest age, oldest first.
	InputSegs []SegmentMeta
	// Generation is the manifest generation at plan time.
	Generation uint64
	// OutputLevel is the level assigned to the compacted segment.
	OutputLevel uint8
}

func buildCompactionPlan(shardID int, m *Manifest, trigger int) *CompactionPlan {
	if m == nil {
		return nil
	}
	if trigger < 2 {
		trigger = DefaultL0Trigger
	}

	for level := uint8(0); level <= maxCompactionLevel; level++ {
		levelSegs := segmentsAtLevel(m, level)
		if len(levelSegs) < compactionLevelTrigger(trigger, level) {
			continue
		}

		outputLevel := min(level+1, maxCompactionLevel)

		inputNames := make(map[string]bool, len(levelSegs))
		minKey, maxKey := segmentRange(levelSegs)
		for _, seg := range levelSegs {
			inputNames[seg.Filename] = true
		}
		if outputLevel != level {
			for _, seg := range m.Segments {
				if seg.Level == outputLevel && keyRangesOverlap(minKey, maxKey, seg.MinKey, seg.MaxKey) {
					inputNames[seg.Filename] = true
				}
			}
		}

		inputs := make([]SegmentMeta, 0, len(inputNames))
		for _, seg := range m.Segments {
			if inputNames[seg.Filename] {
				inputs = append(inputs, cloneSegmentMeta(seg))
			}
		}
		return &CompactionPlan{
			ShardID:     shardID,
			InputSegs:   inputs,
			Generation:  m.Generation,
			OutputLevel: outputLevel,
		}
	}

	return nil
}

func segmentsAtLevel(m *Manifest, level uint8) []SegmentMeta {
	var out []SegmentMeta
	for _, seg := range m.Segments {
		if seg.Level == level {
			out = append(out, cloneSegmentMeta(seg))
		}
	}
	return out
}

func compactionLevelTrigger(base int, level uint8) int {
	if base < 2 {
		base = DefaultL0Trigger
	}
	out := base
	for range level {
		out *= 2
	}
	return out
}

func segmentRange(segs []SegmentMeta) ([]byte, []byte) {
	if len(segs) == 0 {
		return nil, nil
	}
	minKey := segs[0].MinKey
	maxKey := segs[0].MaxKey
	for i := 1; i < len(segs); i++ {
		if bytes.Compare(segs[i].MinKey, minKey) < 0 {
			minKey = segs[i].MinKey
		}
		if bytes.Compare(segs[i].MaxKey, maxKey) > 0 {
			maxKey = segs[i].MaxKey
		}
	}
	return minKey, maxKey
}

func keyRangesOverlap(aMin, aMax, bMin, bMax []byte) bool {
	if len(aMin) == 0 || len(aMax) == 0 || len(bMin) == 0 || len(bMax) == 0 {
		return true
	}
	return bytes.Compare(aMin, bMax) <= 0 && bytes.Compare(aMax, bMin) >= 0
}

func verifyCompactionInputs(m *Manifest, inputNames map[string]bool) error {
	for name := range inputNames {
		found := false
		for _, seg := range m.Segments {
			if seg.Filename == name {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("input segment %s no longer in manifest", name)
		}
	}
	return nil
}

func buildCompactedManifest(
	current *Manifest,
	inputNames map[string]bool,
	output *SegmentMeta,
	nextGen uint64,
	nowUnixMilli uint64,
	graceMilli uint64,
) (*Manifest, error) {
	if err := verifyCompactionInputs(current, inputNames); err != nil {
		return nil, err
	}

	next := current.Clone()
	next.Generation = nextGen
	next.Segments = next.Segments[:0]
	lastInputIdx := -1
	for i, seg := range current.Segments {
		if inputNames[seg.Filename] {
			lastInputIdx = i
		}
	}

	insertedOutput := output == nil
	for i, seg := range current.Segments {
		if inputNames[seg.Filename] {
			next.PendingDelete = append(next.PendingDelete, RetiredSegmentMeta{
				SegmentMeta:          cloneSegmentMeta(seg),
				RetireGeneration:     nextGen,
				DeleteAfterUnixMilli: nowUnixMilli + graceMilli,
			})
			continue
		}
		if !insertedOutput && i > lastInputIdx {
			next.Segments = append(next.Segments, cloneSegmentMeta(*output))
			insertedOutput = true
		}
		next.Segments = append(next.Segments, cloneSegmentMeta(seg))
	}
	if !insertedOutput {
		next.Segments = append(next.Segments, cloneSegmentMeta(*output))
	}
	return next, nil
}

func pruneCompactedTombstones(
	merged []segment.Entry,
	current *Manifest,
	inputNames map[string]bool,
) []segment.Entry {
	out := merged[:0]
	for _, entry := range merged {
		if entry.Tombstone && !manifestMayContainKeyOutsideInputs(current, inputNames, entry.Key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func manifestMayContainKeyOutsideInputs(
	current *Manifest,
	inputNames map[string]bool,
	key []byte,
) bool {
	for _, seg := range current.Segments {
		if inputNames[seg.Filename] {
			continue
		}
		if bytes.Compare(key, seg.MinKey) >= 0 && bytes.Compare(key, seg.MaxKey) <= 0 {
			return true
		}
	}
	return false
}

func selectReclaimablePending(
	current *Manifest,
	nowUnixMilli uint64,
) ([]RetiredSegmentMeta, []RetiredSegmentMeta) {
	keep := make([]RetiredSegmentMeta, 0, len(current.PendingDelete))
	reclaim := make([]RetiredSegmentMeta, 0, len(current.PendingDelete))
	for _, seg := range current.PendingDelete {
		if current.Generation >= seg.RetireGeneration+2 && nowUnixMilli >= seg.DeleteAfterUnixMilli {
			reclaim = append(reclaim, cloneRetiredSegmentMeta(seg))
			continue
		}
		keep = append(keep, cloneRetiredSegmentMeta(seg))
	}
	return keep, reclaim
}

func buildReclaimManifest(current *Manifest, keep []RetiredSegmentMeta) *Manifest {
	next := current.Clone()
	next.Generation = current.Generation + 1
	next.PendingDelete = keep
	return next
}
