package forge_target

import (
	"slices"
	"strings"
)

// SortInputs sorts the inputs slice by name.
func SortInputs(inps []*Input) {
	slices.SortFunc(inps, func(a, b *Input) int {
		return strings.Compare(a.GetName(), b.GetName())
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
