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
	return buildCompactionPlanWithLimit(shardID, m, trigger, 0)
}

func buildCompactionPlanWithLimit(shardID int, m *Manifest, trigger int, maxSegmentDataBytes int) *CompactionPlan {
	if m == nil {
		return nil
	}
	if trigger < 2 {
		trigger = DefaultL0Trigger
	}

	for level := uint8(0); level <= maxCompactionLevel; level++ {
		levelSegs := segmentsAtLevel(m, level)
		levelTrigger := compactionLevelTrigger(trigger, level)
		if len(levelSegs) < levelTrigger {
			continue
		}
		selectedSegs := selectCompactionLevelSegments(
			levelSegs,
			levelTrigger,
			maxCompactionInputBytes(maxSegmentDataBytes, levelTrigger),
		)
		if len(selectedSegs) < levelTrigger {
			continue
		}
		if level == maxCompactionLevel && !compactionCanReduceMaxLevel(selectedSegs, maxSegmentDataBytes) {
			continue
		}

		outputLevel := min(level+1, maxCompactionLevel)

		inputNames := make(map[string]bool, len(selectedSegs))
		minKey, maxKey := segmentRange(selectedSegs)
		for _, seg := range selectedSegs {
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

func selectCompactionLevelSegments(segs []SegmentMeta, minCount int, maxInputBytes int) []SegmentMeta {
	if maxInputBytes < 1 {
		return segs
	}
	out := make([]SegmentMeta, 0, min(len(segs), minCount))
	total := 0
	for _, seg := range segs {
		size := int(seg.Size)
		if size < 1 {
			size = 1
		}
		// The trigger batch is the unit of compaction progress. Encoded
		// SSTable overhead can make minCount bounded-data segments exceed the
		// input cap, so the cap only limits optional extra segments.
		if len(out) >= minCount && total+size > maxInputBytes {
			break
		}
		out = append(out, seg)
		total += size
	}
	if len(out) < minCount {
		return nil
	}
	return out
}

func maxCompactionInputBytes(maxSegmentDataBytes int, levelTrigger int) int {
	if maxSegmentDataBytes < 1 || levelTrigger < 1 {
		return 0
	}
	return maxSegmentDataBytes * (levelTrigger + 1)
}

func compactionCanReduceMaxLevel(segs []SegmentMeta, maxSegmentDataBytes int) bool {
	if maxSegmentDataBytes < 1 {
		return true
	}
	total := 0
	for _, seg := range segs {
		total += int(seg.Size)
	}
	return ceilDiv(total, maxSegmentDataBytes) < len(segs)
}

func ceilDiv(n, d int) int {
	if d < 1 || n < 1 {
		return 0
	}
	return (n + d - 1) / d
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
	outputs []SegmentMeta,
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

	insertedOutput := len(outputs) == 0
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
			next.Segments = appendClonedSegmentMetas(next.Segments, outputs)
			insertedOutput = true
		}
		next.Segments = append(next.Segments, cloneSegmentMeta(seg))
	}
	if !insertedOutput {
		next.Segments = appendClonedSegmentMetas(next.Segments, outputs)
	}
	return next, nil
}

func appendClonedSegmentMetas(dst []SegmentMeta, src []SegmentMeta) []SegmentMeta {
	for i := range src {
		dst = append(dst, cloneSegmentMeta(src[i]))
	}
	return dst
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
