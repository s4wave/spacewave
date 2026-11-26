package block_gc

import "github.com/s4wave/spacewave/db/block"

// ListRebuildResult is what a list rebuild verified and wrote.
type ListRebuildResult struct {
	// Verified counts the lists checked against their block's decoded refs.
	Verified uint64
	// Rewritten counts those written because they were missing or lacked
	// children the decoded refs name.
	Rewritten uint64
	// Dropped counts the blocks whose decoded refs lack a child their stored
	// list names. A stored list can only miss children, so every edge it holds
	// is real, and such a report means a wrong constructor or a decoder bug:
	// the rebuild keeps the stored list and leaves the block on the front,
	// or, for a marked list a reverify compared, marked, which MarkedRefused
	// also counts.
	Dropped uint64
	// DroppedSample names up to sampleSize of them.
	DroppedSample []*block.BlockRef
	// Front counts the blocks on the settle's front when Finish returned:
	// unverified blocks a holder reaches that no Verify wrote. Any keeps the
	// old generation Open, so the collector frees nothing it holds.
	Front uint64
	// FrontSample names up to sampleSize of them.
	FrontSample []*block.BlockRef

	// MarkedRead counts the marked lists a reverify compared with their
	// block's decoded refs.
	MarkedRead uint64
	// MarkedMatched counts those that name every child the content names.
	MarkedMatched uint64
	// MarkedLacking counts those that lack a child the content names, which
	// the sweep would trust and so free a child the block still names.
	MarkedLacking uint64
	// MarkedLackingSample names up to sampleSize of them, each with the
	// first child it lacks.
	MarkedLackingSample []MarkedListSample
	// MarkedRefused counts the lacking marked lists that also name a child
	// the content lacks, so the merge refuses them and leaves them lacking.
	MarkedRefused uint64
	// MarkedUndecodable counts the marked lists of blocks the walker reached
	// and could not decode, which nothing compared.
	MarkedUndecodable uint64
	// MarkedUndecodableSample names up to sampleSize of them.
	MarkedUndecodableSample []MarkedListSample
}

// samples returns an empty result carrying res's samples, which a batch
// extends.
func (res *ListRebuildResult) samples() ListRebuildResult {
	return ListRebuildResult{
		DroppedSample:           res.DroppedSample,
		MarkedLackingSample:     res.MarkedLackingSample,
		MarkedUndecodableSample: res.MarkedUndecodableSample,
	}
}

// add adds a batch's counts and takes its samples, which extend res's.
func (res *ListRebuildResult) add(b ListRebuildResult) {
	// Add what the batch wrote.
	res.Verified += b.Verified
	res.Rewritten += b.Rewritten
	res.Dropped += b.Dropped
	res.DroppedSample = b.DroppedSample

	// Add what its reverify compared.
	res.MarkedRead += b.MarkedRead
	res.MarkedMatched += b.MarkedMatched
	res.MarkedLacking += b.MarkedLacking
	res.MarkedLackingSample = b.MarkedLackingSample
	res.MarkedRefused += b.MarkedRefused
	res.MarkedUndecodable += b.MarkedUndecodable
	res.MarkedUndecodableSample = b.MarkedUndecodableSample
}

// compareMarked counts p's marked list value against p's decoded refs and
// reports whether the list lacks a child they name.
func (res *ListRebuildResult) compareMarked(p pendingList, value []byte) (bool, error) {
	// Count an undecodable block's marked list.
	if p.undecodable {
		res.MarkedUndecodable++
		if len(res.MarkedUndecodableSample) < sampleSize {
			res.MarkedUndecodableSample = append(res.MarkedUndecodableSample, MarkedListSample{Ref: p.entry.Ref, Type: p.typ})
		}
		return false, nil
	}

	// Find the first child the content names that the list lacks.
	res.MarkedRead++
	old, encoded := listKeys(value), EncodeChildren(p.entry.Refs)
	union, err := unionChildren(old, encoded)
	if err != nil {
		return false, err
	}
	gained := appendGained(nil, old, union)
	if len(gained) == 0 {
		res.MarkedMatched++
		return false, nil
	}

	// Count and sample the lacking list, and a refusal of it.
	res.MarkedLacking++
	if !containsChildren(encoded, old) {
		res.MarkedRefused++
	}
	if len(res.MarkedLackingSample) < sampleSize {
		missing, _, err := ParseBlockKey(gained[0])
		if err != nil {
			return false, err
		}
		res.MarkedLackingSample = append(res.MarkedLackingSample, MarkedListSample{Ref: p.entry.Ref, Type: p.typ, Missing: missing})
	}
	return true, nil
}
