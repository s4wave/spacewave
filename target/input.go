package forge_target

import "sort"

// SortInputs sorts the inputs slice by name.
func SortInputs(inps []*Input) {
	sort.Slice(inps, func(i, j int) bool {
		return inps[i].GetName() < inps[j].GetName()
	})
}

// GetInputsNames returns the list of names for a set of inputs.
func GetInputsNames(inps []*Input) []string {
	out := make([]string, len(inps))
	for i, inp := range inps {
		out[i] = inp.GetName()
	}
	return out
}
