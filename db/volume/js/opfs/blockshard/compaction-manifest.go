package blockshard

import "github.com/pkg/errors"

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
	output SegmentMeta,
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
	for _, seg := range current.Segments {
		if inputNames[seg.Filename] {
			next.PendingDelete = append(next.PendingDelete, RetiredSegmentMeta{
				SegmentMeta:          cloneSegmentMeta(seg),
				RetireGeneration:     nextGen,
				DeleteAfterUnixMilli: nowUnixMilli + graceMilli,
			})
			continue
		}
		next.Segments = append(next.Segments, cloneSegmentMeta(seg))
	}
	next.Segments = append(next.Segments, cloneSegmentMeta(output))
	return next, nil
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
