package packsweep

import "github.com/aperturerobotics/fastjson"

// CheckResult is what a check found.
type CheckResult struct {
	// Candidates is the candidate packs.
	Candidates int
	// CandidateKeys is the keys their indexes hold.
	CandidateKeys uint64
	// Reached is the blocks the roots reach.
	Reached uint64
	// InCandidates is the reached blocks a candidate holds.
	InCandidates uint64
	// Shared is those another pack also serves, which a removal keeps.
	Shared uint64
	// Sampled is the other reached blocks read by value.
	Sampled uint64
	// Accepted is the accepted missing blocks reached.
	Accepted uint64
	// Bad is the bad blocks and candidate packs; the check passes at zero.
	Bad uint64
	// BadReasons counts the bad by reason.
	BadReasons map[string]uint64
	// BadPacks counts, for each candidate, its bad keys and its own failure.
	BadPacks map[string]uint64
	// BadBlocks lists the first bad found, up to the check's MaxBad.
	BadBlocks []CheckBad
}

// JSON returns the result as a JSON object: the totals, then the bad by
// candidate and the listed bad.
func (r *CheckResult) JSON(a *fastjson.Arena) *fastjson.Value {
	// Describe each listed bad block or pack.
	bad := a.NewArray()
	for i, b := range r.BadBlocks {
		packs := a.NewArray()
		for j, id := range b.Packs {
			packs.SetArrayItem(j, a.NewString(id))
		}
		o := jsonObject(a,
			jsonField{"packs", packs},
			jsonField{"reason", a.NewString(b.Reason)},
		)
		if b.Ref != nil {
			o.Set("ref", a.NewString(b.Ref.MarshalString()))
		}
		bad.SetArrayItem(i, o)
	}

	// Join them with the totals.
	return jsonObject(a,
		jsonField{"candidates", a.NewNumberInt(r.Candidates)},
		jsonField{"candidate_keys", jsonUint(a, r.CandidateKeys)},
		jsonField{"reached", jsonUint(a, r.Reached)},
		jsonField{"in_candidates", jsonUint(a, r.InCandidates)},
		jsonField{"shared", jsonUint(a, r.Shared)},
		jsonField{"sampled", jsonUint(a, r.Sampled)},
		jsonField{"accepted", jsonUint(a, r.Accepted)},
		jsonField{"bad", jsonUint(a, r.Bad)},
		jsonField{"bad_reasons", JSONCounts(a, r.BadReasons)},
		jsonField{"bad_packs", JSONCounts(a, r.BadPacks)},
		jsonField{"bad_blocks", bad},
	)
}
