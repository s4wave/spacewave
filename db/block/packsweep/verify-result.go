package packsweep

import "github.com/aperturerobotics/fastjson"

// VerifyResult is what a verify read.
type VerifyResult struct {
	// Roots counts each root's blocks, in walk order.
	Roots []*VerifyRootResult
	// Blocks is the blocks read and checked.
	Blocks uint64
	// Bytes is their bytes.
	Bytes uint64
	// Bad is the blocks missing, unreadable, corrupt or without their refs.
	Bad uint64
	// BadReasons counts the bad blocks by packcopy hole reason.
	BadReasons map[string]uint64
	// BadBlocks lists the first bad blocks found, up to the verify's MaxBad.
	BadBlocks []VerifyBad
}

// JSON returns the result as a JSON object: the totals, each root, then the
// listed bad blocks.
func (r *VerifyResult) JSON(a *fastjson.Arena) *fastjson.Value {
	// Describe each root.
	roots := a.NewArray()
	for i, rr := range r.Roots {
		roots.SetArrayItem(i, jsonObject(a,
			jsonField{"name", a.NewString(rr.Name)},
			jsonField{"ref", a.NewString(rr.Ref.MarshalString())},
			jsonField{"blocks", jsonUint(a, rr.Blocks)},
			jsonField{"bytes", jsonUint(a, rr.Bytes)},
			jsonField{"bad", jsonUint(a, rr.Bad)},
		))
	}

	// Describe each listed bad block.
	bad := a.NewArray()
	for i, b := range r.BadBlocks {
		bad.SetArrayItem(i, jsonObject(a,
			jsonField{"root", a.NewString(b.Root)},
			jsonField{"ref", a.NewString(b.Ref.MarshalString())},
			jsonField{"reason", a.NewString(b.Reason)},
		))
	}

	// Join them with the totals.
	return jsonObject(a,
		jsonField{"blocks", jsonUint(a, r.Blocks)},
		jsonField{"bytes", jsonUint(a, r.Bytes)},
		jsonField{"bad", jsonUint(a, r.Bad)},
		jsonField{"bad_reasons", JSONCounts(a, r.BadReasons)},
		jsonField{"roots", roots},
		jsonField{"bad_blocks", bad},
	)
}
